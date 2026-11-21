package bare_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"uuid"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type admitFunc func(context.Context, bare.Change) error

func (f admitFunc) Admit(ctx context.Context, c bare.Change) error { return f(ctx, c) }

// The same assertions run on PostgreSQL in CI. Each test owns a schema, so a
// denied write cannot pass merely because another test removed its row later.
func admissionServer(t *testing.T, opts ...bare.Option) *Server {
	t.Helper()
	dsn := os.Getenv(Postgres)
	if dsn == "" {
		s := NewServerWith(t, opts...)
		t.Cleanup(func() { s.Close() })
		return s
	}
	drv, err := entsql.Open(dialect.Postgres, dsn)
	require.NoError(t, err)
	drv.DB().SetMaxOpenConns(1)
	name := fmt.Sprintf("admission_%x", uuid.New())
	_, err = drv.DB().ExecContext(t.Context(), "CREATE SCHEMA "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := drv.DB().ExecContext(context.Background(), "DROP SCHEMA "+name+" CASCADE")
		require.NoError(t, err)
		drv.Close()
	})
	_, err = drv.DB().ExecContext(t.Context(), "SET search_path TO "+name)
	require.NoError(t, err)
	db := ent.NewClient(ent.Driver(drv))
	require.NoError(t, db.Schema.Create(t.Context()))
	sv, err := bare.NewServer(db, opts...)
	require.NoError(t, err)
	return &Server{t: t, Db: ent.NewClient(ent.Driver(drv)), Driver: drv, Server: sv}
}

func TestAdmissionImagesAndIsolation(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	var changes []bare.Change
	r := &recorder{}
	s := admissionServer(t, bare.WithAdmission(admitFunc(func(_ context.Context, c bare.Change) error {
		changes = append(changes, c)
		// Neither the caller nor the recorder may inherit a policy's mutations.
		if v, ok := c.Request.(*pb.TenantAddRequest); ok {
			v.SetName("tampered request")
		}
		if v, ok := c.Value.(*pb.Tenant); ok {
			v.SetName("tampered value")
		}
		return nil
	})), bare.WithRecorder(r))
	req := pb.TenantAddRequest_builder{Name: z.Ptr("original")}.Build()
	v, err := s.Tenant().Add(ctx, req)
	x.NoError(err)
	x.Equal("original", req.GetName())
	x.Equal("original", v.GetName())
	x.Equal("original", r.Changes[0].Value.(*pb.Tenant).GetName())
	x.Equal("original", r.Changes[0].Request.(*pb.TenantAddRequest).GetName())
	x.Nil(changes[0].Previous)
	x.Equal("apptest.Tenant", changes[0].Entity)
	x.Equal(pb.TenantService_Add_FullMethodName, changes[0].By)
	reqPatch := pb.TenantPatchRequest_builder{Ref: v.Ref(), Name: z.Ptr("changed")}.Build()
	_, err = s.Tenant().Patch(ctx, reqPatch)
	x.NoError(err)
	x.True(proto.Equal(reqPatch, changes[1].Request))
	x.Equal("original", changes[1].Previous.(*pb.Tenant).GetName())
	x.Equal(pb.TenantService_Patch_FullMethodName, changes[1].Method)
	x.Equal("changed", r.Changes[1].Value.(*pb.Tenant).GetName())
	x.NotNil(changes[1].Patch)
	_, err = s.Tenant().Erase(ctx, v.Ref())
	x.NoError(err)
	x.Equal("changed", changes[2].Previous.(*pb.Tenant).GetName())
	x.Nil(changes[2].Value)
	_, err = s.Tenant().Erase(ctx, v.Ref())
	x.NoError(err)
	x.Len(changes, 3)
}

func TestAdmissionRefusesEveryWrite(t *testing.T) {
	for _, kind := range []string{"Add", "AddMany", "Patch", "Apply", "Erase"} {
		t.Run(kind, func(t *testing.T) {
			x := require.New(t)
			ctx := t.Context()
			denied := false
			calls := 0
			s := admissionServer(t, bare.WithAdmission(admitFunc(func(_ context.Context, c bare.Change) error {
				if denied {
					calls++
					return status.Error(codes.PermissionDenied, "approved rows are immutable")
				}
				return nil
			})))
			u := seedAt(ctx, x, s)
			denied = true
			switch kind {
			case "Add":
				_, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: u.GetTenant().Ref(), Alias: z.Ptr("new")}.Build())
				x.Equal(codes.PermissionDenied, status.Code(err))
			case "AddMany":
				_, err := s.User().AddMany(ctx, users(u.GetTenant(), "a", "b"))
				x.Equal(codes.PermissionDenied, status.Code(err))
			case "Patch":
				_, err := s.User().Patch(ctx, pb.UserPatchRequest_builder{Ref: u.Ref(), DateUpdated: u.GetDateUpdated(), Name: z.Ptr("changed")}.Build())
				x.Equal(codes.PermissionDenied, status.Code(err))
			case "Apply":
				_, err := s.User().Apply(ctx, setName(x, u, "changed"))
				x.Equal(codes.PermissionDenied, status.Code(err))
			case "Erase":
				_, err := s.User().Erase(ctx, u.Ref())
				x.Equal(codes.PermissionDenied, status.Code(err))
			}
			x.Equal(1, calls)
			x.Equal(1, s.Db.User.Query().CountX(ctx))
			x.Equal("", nameAt(ctx, x, s, u))
		})
	}
}

func TestAdmissionBatchAndJoinedTransaction(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			x := require.New(t)
			ctx := t.Context()
			calls := 0
			s := admissionServer(t, bare.WithAdmission(admitFunc(func(_ context.Context, c bare.Change) error {
				if c.By == pb.UserService_AddMany_FullMethodName {
					calls++
					x.IsType(&pb.UserAddManyRequest{}, c.Request)
					x.Nil(c.Previous)
					if refuse && calls == 2 {
						return status.Error(codes.Unavailable, "policy unavailable")
					}
				}
				return nil
			})))
			tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.NoError(err)
			drv, tx, err := dialect.BeginTx(ctx, s.Driver)
			x.NoError(err)
			defer tx.Rollback()
			sv, err := enttx.Rebind(pb.Server(s.Server), drv)
			x.NoError(err)
			_, err = sv.User().AddMany(ctx, users(tenant, "a", "b", "c"))
			if refuse {
				x.Equal(codes.Unavailable, status.Code(err))
				x.Error(tx.Commit())
				x.Zero(s.Db.User.Query().CountX(ctx))
			} else {
				x.NoError(err)
				x.NoError(tx.Commit())
				x.Equal(3, calls)
				x.Equal(3, s.Db.User.Query().CountX(ctx))
			}
		})
	}
}

func TestAdmissionEntBypass(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	calls := 0
	s := admissionServer(t, bare.WithAdmission(admitFunc(func(context.Context, bare.Change) error { calls++; return nil }), "apptest.Tenant"))
	db := s.Server.(bare.Server).Db
	_, err := db.Tenant.Create().SetId(uuid.New()).Save(ctx)
	x.Equal(codes.FailedPrecondition, status.Code(err))
	x.Zero(calls)
	drv, tx, err := dialect.BeginTx(ctx, s.Driver)
	x.NoError(err)
	defer tx.Rollback()
	sv, err := s.Server.(bare.Server).WithDriver(drv)
	x.NoError(err)
	guarded := sv.(bare.Server).Db
	_, err = guarded.Tenant.Create().SetId(uuid.New()).Save(ctx)
	x.NoError(err)
	// An admitted write of the same entity cannot cover the extra direct write.
	_, err = sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	x.Equal(codes.FailedPrecondition, status.Code(tx.Commit()))
	x.Zero(s.Db.Tenant.Query().CountX(ctx))
	x.Equal(1, calls)
	// The explicit server-only bypass works without relaxing normal writes.
	_, err = db.Tenant.Create().SetId(uuid.New()).Save(bare.TrustAdmission(ctx))
	x.NoError(err)
	x.Equal(1, s.Db.Tenant.Query().CountX(ctx))
}

func TestAdmissionOriginAndTrust(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	var changes []bare.Change
	r := &recorder{}
	s := admissionServer(t, bare.WithAdmission(admitFunc(func(_ context.Context, c bare.Change) error { changes = append(changes, c); return nil })), bare.WithRecorder(r))
	ask := wrapperspb.String("approve")
	outer := bare.Origin(ctx, "/app.JobService/Approve", ask)
	ask.Value = "changed"
	_, err := s.Tenant().Add(outer, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	x.Equal("/app.JobService/Approve", changes[0].Method)
	x.Equal("approve", changes[0].Request.(*wrapperspb.StringValue).GetValue())
	_, err = s.Tenant().Add(bare.TrustAdmission(ctx), pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	x.Len(changes, 1)
	x.Len(r.Changes, 2)
}

func TestAdmissionInvalidOption(t *testing.T) {
	s := NewServer(t)
	defer s.Close()
	allow := admitFunc(func(context.Context, bare.Change) error { return nil })
	_, err := bare.NewServer(s.Db, bare.WithAdmission(allow, "apptest.Missing"))
	require.ErrorContains(t, err, "unknown entity")
	require.Panics(t, func() { bare.NewTenantServiceServer(s.Db, bare.WithAdmission(nil)) })
}

// A completion hook registered after the ledger check must not add an
// unchecked write after that check has already answered yes.
func TestAdmissionCommitHookCannotWriteAfterValidation(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	s := admissionServer(t, bare.WithAdmission(admitFunc(func(context.Context, bare.Change) error { return nil })))
	drv, tx, err := dialect.BeginTx(ctx, s.Driver)
	x.NoError(err)
	defer tx.Rollback()
	sv, err := s.Server.(bare.Server).WithDriver(drv)
	x.NoError(err)
	_, err = sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	k, ok := dialect.KeepOf(drv)
	x.True(ok)
	k.BeforeCommit(func(ctx context.Context) error {
		_, err := sv.(bare.Server).Db.Tenant.Create().SetId(uuid.New()).Save(ctx)
		x.Equal(codes.FailedPrecondition, status.Code(err))
		return nil // Ignoring the refusal still cannot commit the transaction.
	})
	x.Equal(codes.FailedPrecondition, status.Code(tx.Commit()))
	x.Zero(s.Db.Tenant.Query().CountX(ctx))
}
