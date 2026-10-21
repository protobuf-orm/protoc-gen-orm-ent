package bare_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// counted is a server on a database that counts the statements it is sent, by
// the words they begin with.
type counted struct {
	*Server
	inserts atomic.Int64
	selects atomic.Int64
}

func newCounted(t *testing.T, opts ...bare.Option) *counted {
	t.Helper()

	driver, err := sql.Open("sqlite3", ":memory:?cache=shared&_fk=1")
	require.NoError(t, err)
	driver.DB().SetMaxOpenConns(1)

	c := &counted{}
	on := dialect.DebugWithContext(driver, func(_ context.Context, vs ...any) {
		q := fmt.Sprint(vs...)
		switch {
		case strings.Contains(q, "query=INSERT INTO `user`"):
			c.inserts.Add(1)
		case strings.Contains(q, "query=SELECT"):
			c.selects.Add(1)
		}
	})
	db := ent.NewClient(ent.Driver(on))
	require.NoError(t, db.Schema.Create(t.Context()))

	s, err := bare.NewServer(db, opts...)
	require.NoError(t, err)
	c.Server = &Server{t: t, Db: ent.NewClient(ent.Driver(driver)), Driver: driver, Server: s}
	t.Cleanup(func() { c.Close() })
	return c
}

func users(tenant *pb.Tenant, aliases ...string) *pb.UserAddManyRequest {
	items := make([]*pb.UserAddRequest, len(aliases))
	for i, alias := range aliases {
		items[i] = pb.UserAddRequest_builder{
			Tenant: tenant.Ref(),
			Alias:  z.Ptr(alias),
			Name:   z.Ptr(strings.ToUpper(alias)),
		}.Build()
	}
	return pb.UserAddManyRequest_builder{Items: items}.Build()
}

// TestAddMany: every item is added as an Add of it would be, in the order of
// the items, and all of them or none.
func TestAddMany(t *testing.T) {
	t.Run("adds every item, and answers in their order", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		res, err := c.User().AddMany(ctx, users(tenant, "a", "b", "c"))
		x.NoError(err)
		x.Len(res.GetItems(), 3)
		for i, alias := range []string{"a", "b", "c"} {
			v := res.GetItems()[i]
			x.Equal(alias, v.GetAlias())
			x.Equal(strings.ToUpper(alias), v.GetName())
			x.Equal(tenant.GetId(), v.GetTenant().GetId())

			got, err := c.User().Get(ctx, v.Ref().Pick())
			x.NoError(err)
			x.Equal(v.GetId(), got.GetId())
			x.Equal(v.GetName(), got.GetName())
		}
	}))

	t.Run("in one statement for all of them", func(t *testing.T) {
		x := require.New(t)
		s := newCounted(t)
		ctx := t.Context()

		tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		s.inserts.Store(0)

		_, err = s.User().AddMany(ctx, users(tenant, "a", "b", "c", "d"))
		x.NoError(err)
		x.Equal(int64(1), s.inserts.Load())
	})

	t.Run("and in a few, when there are more rows than one statement writes", func(t *testing.T) {
		x := require.New(t)
		s := newCounted(t)
		ctx := t.Context()

		tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		s.inserts.Store(0)

		aliases := make([]string, 501)
		for i := range aliases {
			aliases[i] = fmt.Sprintf("u%03d", i)
		}
		res, err := s.User().AddMany(ctx, users(tenant, aliases...))
		x.NoError(err)
		x.Len(res.GetItems(), 501)
		x.Equal(int64(2), s.inserts.Load(), "five hundred to a statement")
		x.Equal(aliases[500], res.GetItems()[500].GetAlias())
		x.Equal(501, s.Db.User.Query().CountX(ctx))
	})

	t.Run("more than its max is refused, and nothing is written", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		items := make([]*pb.SeatAddRequest, bare.SeatAddManyMax+1)
		for i := range items {
			items[i] = pb.SeatAddRequest_builder{Org: tenant.Ref()}.Build()
		}
		_, err = c.Seat().AddMany(ctx, pb.SeatAddManyRequest_builder{Items: items}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.Zero(c.Server.Db.Seat.Query().CountX(ctx))
	}))

	t.Run("an item that cannot be a row is refused naming it, and nothing is written", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		req := users(tenant, "a", "b", "c")
		req.GetItems()[2].SetId([]byte{1, 2, 3})
		_, err = c.User().AddMany(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.True(strings.HasPrefix(status.Convert(err).Message(), "items[2]: "), status.Convert(err).Message())
		x.Equal([]string{"items[2]"}, violations(err))
		x.Zero(c.Server.Db.User.Query().CountX(ctx))
	}))

	t.Run("and one whose reference names nothing, in the reference's words", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		holder, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Alias: z.Ptr("h")}.Build())
		x.NoError(err)

		// By alias, which is a read: the item is the one that named it.
		_, err = c.Seat().AddMany(ctx, pb.SeatAddManyRequest_builder{Items: []*pb.SeatAddRequest{
			pb.SeatAddRequest_builder{Org: tenant.Ref(), Holder: holder.Ref()}.Build(),
			pb.SeatAddRequest_builder{Org: tenant.Ref(), Holder: pb.UserRef_builder{
				Alias: pb.UserRefByAlias_builder{Alias: z.Ptr("nobody"), Tenant: tenant.Ref()}.Build(),
			}.Build()}.Build(),
		}}.Build())
		x.Equal(codes.NotFound, status.Code(err))
		x.Equal("items[1]: User not found", status.Convert(err).Message())
		x.Zero(c.Server.Db.Seat.Query().CountX(ctx))
	}))

	t.Run("one by an Id nothing has is the database's to refuse, for the call", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		gone, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = c.Tenant().Erase(ctx, gone.Ref())
		x.NoError(err)

		// An Id is not read before the INSERT, which is where the reference
		// meets the row it names -- or the key that says there is none.
		req := users(tenant, "a", "b")
		req.GetItems()[1].SetTenant(gone.Ref())
		_, err = c.User().AddMany(ctx, req)
		x.Equal(codes.NotFound, status.Code(err))
		x.Zero(c.Server.Db.User.Query().CountX(ctx))
	}))

	t.Run("a refusal by the database is the whole call's", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		// The alias is unique in a tenant, and two items share one.
		_, err = c.User().AddMany(ctx, users(tenant, "a", "b", "a"))
		x.Equal(codes.AlreadyExists, status.Code(err))
		x.Zero(c.Server.Db.User.Query().CountX(ctx))
	}))

	t.Run("no items is nothing written", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		res, err := c.User().AddMany(ctx, pb.UserAddManyRequest_builder{}.Build())
		x.NoError(err)
		x.Empty(res.GetItems())
		x.Zero(r.Len())
	}))
}

// TestRecordAddMany: a recorder is told about every row, as an Add of each
// would have told it, and an image is read by the INSERT.
func TestRecordAddMany(t *testing.T) {
	t.Run("a change for every row, with the row as it was stored", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		r.Images = bare.SnapshotOptions{Value: true}
		tenant := seedTenant(ctx, x, c, r)

		res, err := c.User().AddMany(ctx, users(tenant, "a", "b", "c"))
		x.NoError(err)
		x.Equal(3, r.Len())
		for i, ch := range r.Changes {
			x.Equal(pb.UserService_AddMany_FullMethodName, ch.By)
			x.Equal(pb.UserService_AddMany_FullMethodName, ch.Method)

			got, err := c.User().Get(ctx, res.GetItems()[i].Ref().Pick())
			x.NoError(err)
			x.True(proto.Equal(got, ch.Value), "change %d: %v, and the row is %v", i, ch.Value, got)
		}
	}))

	t.Run("and the images read by the INSERT, not by a query of their own", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{Images: bare.SnapshotOptions{Value: true}}
		s := newCounted(t, bare.WithRecorder(r))
		ctx := t.Context()

		tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		s.selects.Store(0)

		_, err = s.User().AddMany(ctx, users(tenant, "a", "b", "c"))
		x.NoError(err)
		x.Zero(s.selects.Load())
		x.Equal(4, r.Len(), "the tenant and three users")
	})

	t.Run("a refusal by the recorder takes every row back", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		r.Do = func(ctx context.Context, s bare.Server, ch bare.Change) error {
			if r.Len() == 2 {
				return errors.New("refused")
			}
			return nil
		}

		_, err := c.User().AddMany(ctx, users(tenant, "a", "b", "c"))
		x.Equal(codes.Internal, status.Code(err))
		x.Zero(c.Server.Db.User.Query().CountX(ctx))
	}))
}

// TestAddRule: a rule a layer puts on the RPCs that add is asked about an Add,
// and about every item of an AddMany, and what it answers with is what is
// written.
func TestAddRule(t *testing.T) {
	asked := func(seen *[]string, refuse string) bare.UserAddRule {
		return func(_ context.Context, req *pb.UserAddRequest) (*pb.UserAddRequest, error) {
			*seen = append(*seen, req.GetAlias())
			if req.GetAlias() == refuse {
				return nil, status.Error(codes.InvalidArgument, "alias: not that one")
			}
			// A copy, changed: the request belongs to whoever called.
			r := proto.CloneOf(req)
			r.SetName("ruled " + req.GetName())
			return r, nil
		}
	}

	t.Run("an Add is asked about, and writes what it answers", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		var seen []string
		s := bare.UserWithAddRule(c.Server.User(), asked(&seen, "no"))
		req := users(tenant, "a").GetItems()[0]
		v, err := s.Add(ctx, req)
		x.NoError(err)
		x.Equal([]string{"a"}, seen)
		x.Equal("ruled A", v.GetName())
		x.Equal("A", req.GetName(), "the caller's request was written to")

		_, err = s.Add(ctx, users(tenant, "no").GetItems()[0])
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))

	t.Run("and so is every item of an AddMany, in order", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		var seen []string
		s := bare.UserWithAddRule(c.Server.User(), asked(&seen, "no"))
		req := users(tenant, "a", "b", "c")
		res, err := s.AddMany(ctx, req)
		x.NoError(err)
		x.Equal([]string{"a", "b", "c"}, seen)
		for i, v := range res.GetItems() {
			x.Equal("ruled "+strings.ToUpper(seen[i]), v.GetName())
		}
		x.Equal("A", req.GetItems()[0].GetName(), "the caller's request was written to")
	}))

	t.Run("refused for the first item it refuses, naming the item, and nothing is written", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		var seen []string
		s := bare.UserWithAddRule(c.Server.User(), asked(&seen, "no"))
		_, err = s.AddMany(ctx, users(tenant, "a", "no", "c"))
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.Equal([]string{"a", "no"}, seen, "asked about an item after the refusal")
		x.Equal([]string{"items[1]"}, violations(err))
		x.Zero(c.Server.Db.User.Query().CountX(ctx))
	}))

	// What the call asks besides its items is the server behind's to read: an
	// app's own fields on the request are what this app's schema has none of,
	// so what it is held to here is what it does not know.
	t.Run("and what the call asks besides its items reaches the server behind", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		behind := &keepsAddMany{UserServiceServer: c.Server.User()}
		var seen []string
		s := bare.UserWithAddRule(behind, asked(&seen, "no"))
		req := users(tenant, "a", "b")
		besides := protowire.AppendBytes(protowire.AppendTag(nil, 100, protowire.BytesType), []byte("dry run"))
		req.ProtoReflect().SetUnknown(besides)

		_, err = s.AddMany(ctx, req)
		x.NoError(err)
		x.Equal([]byte(besides), []byte(behind.got.ProtoReflect().GetUnknown()), "what the call asked besides its items was dropped")
		x.Len(behind.got.GetItems(), 2)
		x.Equal("ruled A", behind.got.GetItems()[0].GetName(), "the items the rule made were not handed on")
	}))

	t.Run("and a call over the max before the rule is asked at all", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		n := 0
		s := bare.SeatWithAddRule(c.Server.Seat(), func(_ context.Context, req *pb.SeatAddRequest) (*pb.SeatAddRequest, error) {
			n++
			return req, nil
		})
		items := make([]*pb.SeatAddRequest, bare.SeatAddManyMax+1)
		for i := range items {
			items[i] = pb.SeatAddRequest_builder{Org: tenant.Ref()}.Build()
		}
		_, err = s.AddMany(ctx, pb.SeatAddManyRequest_builder{Items: items}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.Zero(n)
	}))
}

// TestItemError: a refusal about one item says which, in the message and in
// the field path a program reads, and keeps the code it had.
func TestItemError(t *testing.T) {
	x := require.New(t)

	x.NoError(bare.ItemError(3, nil))

	err := bare.ItemError(2, status.Error(codes.NotFound, "Robot not found"))
	x.Equal(codes.NotFound, status.Code(err))
	x.Equal("items[2]: Robot not found", status.Convert(err).Message())
	x.Empty(violations(err), "a NotFound is not the request's field")

	err = bare.ItemError(2, status.Error(codes.InvalidArgument, "id: not sixteen bytes"))
	x.Equal(codes.InvalidArgument, status.Code(err))
	x.Equal([]string{"items[2]"}, violations(err))

	st, serr := status.New(codes.InvalidArgument, "bad").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{
			{Field: "alias", Description: "taken"},
			{Field: "[0].x", Description: "no"},
			{Field: "", Description: "whole"},
		},
	})
	x.NoError(serr)
	err = bare.ItemError(4, st.Err())
	x.Equal([]string{"items[4].alias", "items[4][0].x", "items[4]"}, violations(err))

	err = bare.ItemError(1, errors.New("boom"))
	x.Equal(codes.Unknown, status.Code(err))
	x.Equal("items[1]: boom", status.Convert(err).Message())
}

// violations is the fields of the google.rpc.BadRequest `err` carries.
func violations(err error) []string {
	var fs []string
	for _, d := range status.Convert(err).Details() {
		if b, ok := d.(*errdetails.BadRequest); ok {
			for _, v := range b.GetFieldViolations() {
				fs = append(fs, v.GetField())
			}
		}
	}
	return fs
}

// TestAddManyWritesWhatAddWould: an item of an AddMany is the row an Add of it
// would be -- the defaults the option says, a message as its own value, a map
// and a list as JSON -- since both are written by the one `create`.
func TestAddManyWritesWhatAddWould(t *testing.T) {
	// same answers whether two rows say the same, but for what tells them
	// apart: the key, and the times the clock stamped.
	same := func(x *require.Assertions, a, b proto.Message) {
		clear := func(m proto.Message) proto.Message {
			m = proto.Clone(m)
			r := m.ProtoReflect()
			r.Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
				if f.Name() == "id" || (f.Message() != nil && f.Message().FullName() == "google.protobuf.Timestamp") {
					r.Clear(f)
				}
				return true
			})
			return m
		}
		x.True(proto.Equal(clear(a), clear(b)), "an Add wrote %v, and an AddMany %v", a, b)
	}
	all := func(s *pb.ValueFieldSelect) { s.SetAll(true) }

	t.Run("a row of values and defaults", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		item := pb.ValueFieldAddRequest_builder{
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
		}.Build()

		one := proto.CloneOf(item)
		one.SetId("one")
		_, err := c.ValueField().Add(ctx, one)
		x.NoError(err)

		many := proto.CloneOf(item)
		many.SetId("many")
		_, err = c.ValueField().AddMany(ctx, pb.ValueFieldAddManyRequest_builder{Items: []*pb.ValueFieldAddRequest{many}}.Build())
		x.NoError(err)

		a, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("one").WithSelect(all))
		x.NoError(err)
		b, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("many").WithSelect(all))
		x.NoError(err)
		same(x, a, b)
	}))

	t.Run("a row of a message", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		held := pb.Held_builder{What: z.Ptr("held"), HowMany: z.Ptr(int32(3))}.Build()

		_, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{Id: z.Ptr("one"), Held: held}.Build())
		x.NoError(err)
		_, err = c.MessageField().AddMany(ctx, pb.MessageFieldAddManyRequest_builder{Items: []*pb.MessageFieldAddRequest{
			pb.MessageFieldAddRequest_builder{Id: z.Ptr("many"), Held: held}.Build(),
		}}.Build())
		x.NoError(err)

		a, err := c.Server.Db.MessageField.Get(ctx, "one")
		x.NoError(err)
		b, err := c.Server.Db.MessageField.Get(ctx, "many")
		x.NoError(err)
		same(x, a.Proto(), b.Proto())
		x.Equal("held", b.Held.GetWhat())
	}))
}

// On PostgreSQL the images of an AddMany are the rows at the database's own
// precision, read by its INSERT and matched to their items by key: a clock
// with nanoseconds says which, since the column keeps them rounded to the
// microsecond.
func TestPostgresAddManyImages(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	db := snapshotDB(t, true)
	r := &recorder{Images: bare.SnapshotOptions{Value: true}}
	at := func() time.Time { return time.Date(2026, 10, 18, 1, 2, 3, 123456789, time.UTC) }
	s, err := bare.NewServer(db, bare.WithRecorder(r), bare.WithClock(at))
	x.NoError(err)

	tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	t.Cleanup(func() {
		db.User.Delete().ExecX(context.Background())
		_, _ = s.Tenant().Erase(context.Background(), tenant.Ref())
	})
	r.Changes = nil

	res, err := s.User().AddMany(ctx, users(tenant, "pa", "pb", "pc"))
	x.NoError(err)
	x.Len(r.Changes, 3)
	for i, ch := range r.Changes {
		got, err := s.User().Get(ctx, res.GetItems()[i].Ref().Pick())
		x.NoError(err)
		x.True(proto.Equal(got, ch.Value), "change %d: %v, and the row is %v", i, ch.Value, got)
		x.Equal(at().Round(time.Microsecond), ch.Value.(*pb.User).GetDateCreated().AsTime(), "rounded to the column, as PostgreSQL does")
	}
}

// keepsAddMany is the server behind a rule, keeping what an AddMany handed it.
type keepsAddMany struct {
	pb.UserServiceServer
	got *pb.UserAddManyRequest
}

func (s *keepsAddMany) AddMany(ctx context.Context, req *pb.UserAddManyRequest) (*pb.UserAddManyResponse, error) {
	s.got = req
	return s.UserServiceServer.AddMany(ctx, req)
}
