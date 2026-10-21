package bare_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/lesomnus/z"
	entgo "github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A write inside a transaction somebody else began joins it, and cannot take
// back what it wrote: there are no savepoints, and ending the transaction is
// not the write's to do. So when it fails partway -- after it has begun to
// write -- the transaction is the one that is told. Its commit is refused, and
// nothing of the write holds; and whatever rows were written before the
// failure were recorded as they were written, so the transaction never holds
// a row with nothing said of it.
//
// On SQLite, which is where these run: a statement refused there is undone on
// its own and the transaction goes on, so the rest of a failed write would
// have stayed. PostgreSQL takes the whole transaction with a refused
// statement, but not with a failure that is not a statement's -- a recorder's,
// a read after the write.

// joined is a server with a recorder, and the same server rebound onto a
// transaction begun as a stack's is: what a write through it joins.
func joined(t *testing.T) (*Server, pb.Server, interface{ Commit() error }, *recorder) {
	t.Helper()

	r := &recorder{}
	s := NewServerWith(t, bare.WithRecorder(r))
	t.Cleanup(func() { s.Close() })

	drv, tx, err := dialect.BeginTx(t.Context(), s.Driver)
	require.NoError(t, err)
	t.Cleanup(func() { tx.Rollback() })

	sv, err := enttx.Rebind(pb.Server(s.Server), drv)
	require.NoError(t, err)

	return s, sv, tx, r
}

// added is how many changes the recorder was told of, made by `by`.
func added(r *recorder, by string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, c := range r.Changes {
		if c.By == by {
			n++
		}
	}
	return n
}

func TestAWriteThatFailsPartwayInAJoinedTransaction(t *testing.T) {
	t.Run("an AddMany whose second statement is refused records the first's rows, and its commit is refused", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		s, sv, tx, r := joined(t)

		tenant, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = sv.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Alias: z.Ptr("u500")}.Build())
		x.NoError(err)

		// Five hundred to a statement, and the last of 501 is an alias the
		// tenant has already: the second statement is refused, the first held.
		aliases := make([]string, 501)
		for i := range aliases {
			aliases[i] = fmt.Sprintf("u%03d", i)
		}
		_, err = sv.User().AddMany(ctx, users(tenant, aliases...))
		x.Equal(codes.AlreadyExists, status.Code(err))
		x.Equal(500, added(r, pb.UserService_AddMany_FullMethodName),
			"the rows of the statement that held were not recorded before the next was sent")

		x.ErrorIs(tx.Commit(), entgo.ErrPartial)
		x.Zero(s.Db.User.Query().CountX(ctx), "a write that failed partway held")
	})

	t.Run("an Add whose record is refused refuses the commit", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		s, sv, tx, r := joined(t)

		tenant, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		r.Do = func(_ context.Context, _ bare.Server, c bare.Change) error {
			if c.By == pb.UserService_Add_FullMethodName {
				return errors.New("refused")
			}
			return nil
		}

		_, err = sv.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Alias: z.Ptr("a")}.Build())
		x.Equal(codes.Internal, status.Code(err))

		x.ErrorIs(tx.Commit(), entgo.ErrPartial)
		x.Zero(s.Db.Tenant.Query().CountX(ctx), "a transaction a write failed partway in was committed")
	})

	t.Run("and so does an Add the database refuses, as PostgreSQL would", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		s, sv, tx, _ := joined(t)

		tenant, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = sv.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Alias: z.Ptr("a")}.Build())
		x.NoError(err)

		_, err = sv.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Alias: z.Ptr("a")}.Build())
		x.Equal(codes.AlreadyExists, status.Code(err))

		x.ErrorIs(tx.Commit(), entgo.ErrPartial)
		x.Zero(s.Db.Tenant.Query().CountX(ctx))
	})

	t.Run("an Apply whose record is refused refuses the commit", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		r := &recorder{}
		s := NewServerWith(t, bare.WithRecorder(r))
		defer s.Close()
		u := seedAt(ctx, x, s)

		drv, tx, err := dialect.BeginTx(ctx, s.Driver)
		x.NoError(err)
		defer tx.Rollback()
		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		r.Do = func(_ context.Context, _ bare.Server, c bare.Change) error {
			if c.By == pb.UserService_Apply_FullMethodName {
				return errors.New("refused")
			}
			return nil
		}
		_, err = sv.User().Apply(ctx, setName(x, u, "Ada"))
		x.Equal(codes.Internal, status.Code(err))

		x.ErrorIs(tx.Commit(), entgo.ErrPartial)
		x.Equal("", nameAt(ctx, x, s, u), "the name an apply failed partway on was written")
	})

	t.Run("an Erase whose record is refused refuses the commit", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		r := &recorder{}
		s := NewServerWith(t, bare.WithRecorder(r))
		defer s.Close()
		u := seedAt(ctx, x, s)

		drv, tx, err := dialect.BeginTx(ctx, s.Driver)
		x.NoError(err)
		defer tx.Rollback()
		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		r.Do = func(_ context.Context, _ bare.Server, c bare.Change) error {
			if c.By == pb.UserService_Erase_FullMethodName {
				return errors.New("refused")
			}
			return nil
		}
		_, err = sv.User().Erase(ctx, u.Ref())
		x.Equal(codes.Internal, status.Code(err))

		x.ErrorIs(tx.Commit(), entgo.ErrPartial)
		x.Equal(1, s.Db.User.Query().CountX(ctx), "the user an erase failed partway on is gone")
	})

	t.Run("a write refused before it wrote refuses nothing", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		s, sv, tx, _ := joined(t)

		tenant, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		// An item that cannot be a row is refused before anything is sent.
		req := users(tenant, "a", "b")
		req.GetItems()[1].SetId([]byte{1, 2, 3})
		_, err = sv.User().AddMany(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))

		x.NoError(tx.Commit())
		x.Equal(1, s.Db.Tenant.Query().CountX(ctx))
	})

	t.Run("and a write that held refuses nothing", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()
		s, sv, tx, r := joined(t)

		tenant, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = sv.User().AddMany(ctx, users(tenant, "a", "b", "c"))
		x.NoError(err)
		x.Equal(3, added(r, pb.UserService_AddMany_FullMethodName))

		x.NoError(tx.Commit())
		x.Equal(3, s.Db.User.Query().CountX(ctx))
	})
}
