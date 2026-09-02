package bare_test

import (
	"context"
	"sync"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"uuid"
)

// recorder keeps what it was told, and does whatever the test set it to do
// while it is being told.
type recorder struct {
	mu sync.Mutex

	Changes []bare.Change

	// Do runs inside the write's transaction, with the server the recorder was
	// handed. It is where a test writes something of its own, or refuses.
	Do func(ctx context.Context, s bare.Server, c bare.Change) error
}

func (r *recorder) Record(ctx context.Context, s bare.Server, c bare.Change) error {
	r.mu.Lock()
	r.Changes = append(r.Changes, c)
	r.mu.Unlock()

	if r.Do == nil {
		return nil
	}

	return r.Do(ctx, s, c)
}

func (r *recorder) Only(x *require.Assertions) bare.Change {
	r.mu.Lock()
	defer r.mu.Unlock()

	x.Len(r.Changes, 1)
	return r.Changes[0]
}

func (r *recorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.Changes)
}

// R is T for a test that watches what the servers report.
func R(run func(ctx context.Context, x *require.Assertions, c *Client, r *recorder)) func(t *testing.T) {
	return func(t *testing.T) {
		r := &recorder{}

		s := NewServerWith(t, bare.WithRecorder(r))
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		run(t.Context(), require.New(t), c, r)
	}
}

// seedTenant is a Tenant to hang the Users of these tests on. The recorder sees
// it too, so a test that is about a User clears what it was told first.
func seedTenant(ctx context.Context, x *require.Assertions, c *Client, r *recorder) *pb.Tenant {
	v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)

	r.mu.Lock()
	r.Changes = nil
	r.mu.Unlock()

	return v
}

func TestRecordAdd(t *testing.T) {
	t.Run("says what was added, and its key", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		u := r.Only(x)
		x.Equal(pb.TenantService_Add_FullMethodName, u.Method)
		x.Nil(u.Patch)

		// The key of the row that was written, not the one the request asked
		// for -- this request asked for none.
		k, ok := u.Key.(uuid.UUID)
		x.True(ok, "key is the entity's own Go type")
		x.Equal(v.GetId(), k[:])
	}))
	t.Run("a key the schema spells as a string arrives as one", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		_, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{Id: z.Ptr("kk")}.Build())
		x.NoError(err)

		// `any` and not a uuid: what a recorder is handed is the key of the
		// entity that was written, and this one is a string.
		u := r.Only(x)
		x.Equal(pb.MessageFieldService_Add_FullMethodName, u.Method)
		x.Equal("kk", u.Key)
	}))
}

func TestRecordPatchAndApply(t *testing.T) {
	t.Run("a patch request arrives as the document it became", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v := seedTenant(ctx, x, c, r)

		_, err := c.Tenant().Patch(ctx, pb.TenantPatchRequest_builder{
			Ref:  v.Ref(),
			Name: z.Ptr("Acme"),
		}.Build())
		x.NoError(err)

		u := r.Only(x)

		// Patch is the RPC that carries no document of its own, so this is the
		// thing a layer in front of this server could not have worked out: the
		// request became a document, and the document is what was written. The
		// operation says it was an apply and the method says which door it came
		// in through.
		x.Equal(pb.TenantService_Patch_FullMethodName, u.Method)

		// The document the request became says what the request said.
		es := u.Patch.GetDelta().GetEntries()
		x.Len(es, 1)
		x.Equal("Acme", es[0].GetAssign().GetValue().GetS())

		k, ok := u.Key.(uuid.UUID)
		x.True(ok)
		x.Equal(v.GetId(), k[:])
	}))
	t.Run("a patch that asks for nothing is not a change", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v := seedTenant(ctx, x, c, r)

		_, err := c.Tenant().Patch(ctx, pb.TenantPatchRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Zero(r.Len())
	}))
	t.Run("an apply request arrives as the document it carried", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		r.Changes = nil
		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("John")))
		req.SetRef(v.Ref())
		_, err = c.User().Apply(ctx, req)
		x.NoError(err)

		u := r.Only(x)
		x.Equal(pb.UserService_Apply_FullMethodName, u.Method)

		// The document itself, and not merely that there was one: what a
		// recorder keeps is the thing the caller sent.
		x.True(proto.Equal(req.GetPatch(), u.Patch))

		k, ok := u.Key.(uuid.UUID)
		x.True(ok)
		x.Equal(v.GetId(), k[:])
	}))
	t.Run("a document that only asserts is not a change", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		r.Changes = nil
		req := doc(x, patch.Target(patch.Name("name")).Test(patch.Str("")))
		req.SetRef(v.Ref())
		_, err = c.User().Apply(ctx, req)
		x.NoError(err)

		// The row was read and found to be as the document said. Nothing was
		// written, so there is nothing to report.
		x.Zero(r.Len())
	}))
	t.Run("a test that does not hold reports nothing", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		r.Changes = nil
		req := doc(x,
			patch.Target(patch.Name("name")).Test(patch.Str("somebody else")),
			patch.Target(patch.Name("name")).Assign(patch.Str("John")),
		)
		req.SetRef(v.Ref())
		_, err = c.User().Apply(ctx, req)
		x.Equal(codes.FailedPrecondition, status.Code(err))
		x.Zero(r.Len())
	}))
}

func TestRecordErase(t *testing.T) {
	t.Run("says which row it was, however the request named it", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{
			Tenant: tenant.Ref(),
			Alias:  z.Ptr("john"),
		}.Build())
		x.NoError(err)

		r.Changes = nil
		// Named by alias, which is not what the trail is read back with.
		_, err = c.User().Erase(ctx, pb.UserByAlias("john", tenant.Ref()))
		x.NoError(err)

		u := r.Only(x)
		x.Equal(pb.UserService_Erase_FullMethodName, u.Method)

		k, ok := u.Key.(uuid.UUID)
		x.True(ok)
		x.Equal(v.GetId(), k[:])
	}))
	t.Run("erasing what is not there still succeeds, and reports nothing", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		_, err := c.User().Erase(ctx, pb.UserById(newId()))
		x.NoError(err)
		x.Zero(r.Len())
	}))
}

// Add and Erase open a transaction of their own once there is a recorder, which
// they did not before. A caller who has already opened one -- to make this
// write and something else a single write -- must not have theirs committed or
// thrown away underneath them.
//
// These go through the servers rather than a gRPC client, because what is being
// asked about is which driver the servers run on.
func TestRecordInAnOpenTransaction(t *testing.T) {
	ctx := context.Background()

	// The connection is single and shared, so nothing may be read from outside
	// the transaction while it is open: it would wait for a connection the
	// transaction is holding. Every check below reads inside, or after the end.

	begin := func(t *testing.T, r *recorder) (*Server, pb.Server, interface {
		Commit() error
		Rollback() error
	}) {
		t.Helper()
		x := require.New(t)

		s := NewServerWith(t, bare.WithRecorder(r))
		t.Cleanup(func() { s.Close() })

		drv, tx, err := enttx.Begin(ctx, s.Driver)
		x.NoError(err)

		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		return s, sv, tx
	}

	t.Run("an add joins the caller's transaction, recorder and all", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}
		r.Do = func(ctx context.Context, s bare.Server, u bare.Change) error {
			_, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{Name: z.Ptr("shadow")}.Build())
			return err
		}

		s, sv, tx := begin(t, r)

		_, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{Name: z.Ptr("real")}.Build())
		x.NoError(err)
		x.Equal(1, r.Len())

		// Both rows are inside the caller's transaction, so letting it go takes
		// both -- which Add could not have said if it had committed the
		// transaction it was handed.
		x.NoError(tx.Rollback())

		n, err := s.Db.Tenant.Query().Count(ctx)
		x.NoError(err)
		x.Zero(n)
	})
	t.Run("an erase joins it too", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}

		s, sv, tx := begin(t, r)

		v, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		r.Changes = nil
		_, err = sv.Tenant().Erase(ctx, v.Ref())
		x.NoError(err)
		x.Equal(pb.TenantService_Erase_FullMethodName, r.Only(x).Method)

		x.NoError(tx.Commit())

		n, err := s.Db.Tenant.Query().Count(ctx)
		x.NoError(err)
		x.Zero(n, "the add and the erase were both inside, and both stuck")
	})
	t.Run("a refusal inside a caller's transaction is theirs to act on", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}
		r.Do = func(ctx context.Context, s bare.Server, u bare.Change) error {
			return status.Error(codes.ResourceExhausted, "no")
		}

		s, sv, tx := begin(t, r)

		_, err := sv.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.Equal(codes.Internal, status.Code(err))

		// The weaker half of the contract, said out loud: a server that joined
		// a transaction cannot undo it. The row is still there, unrecorded, and
		// it is whoever began the transaction who decides that this is not
		// good enough. A caller that carried on and committed would keep it.
		x.NoError(tx.Rollback())

		n, err := s.Db.Tenant.Query().Count(ctx)
		x.NoError(err)
		x.Zero(n, "and rolling back is what makes the refusal stick")
	})
}

// A service server built by hand carries what it was told to carry. Everything
// else here reaches the servers through the store, which builds them itself, so
// without this the constructor could stop passing the recorder on and the only
// symptom would be a deployment that quietly records nothing.
func TestRecordThroughTheConstructor(t *testing.T) {
	t.Run("told where to report, it reports", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}

		s := NewServer(t)
		defer s.Close()

		v := bare.NewTenantServiceServer(s.Db, bare.WithRecorder(r))
		_, err := v.Add(context.Background(), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		x.Equal(pb.TenantService_Add_FullMethodName, r.Only(x).Method)
	})
	t.Run("told nothing, it reports nowhere", func(t *testing.T) {
		x := require.New(t)

		s := NewServer(t)
		defer s.Close()

		v := bare.NewTenantServiceServer(s.Db)
		_, err := v.Add(context.Background(), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
	})
}

func TestRecordIsPartOfTheWrite(t *testing.T) {
	t.Run("what the recorder writes is committed with the write", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		// One Tenant of its own for every Tenant that is added, which is a
		// silly thing to want and the plainest way to see whether the recorder
		// is on the same transaction.
		r.Do = func(ctx context.Context, s bare.Server, u bare.Change) error {
			_, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{Name: z.Ptr("shadow")}.Build())
			return err
		}

		_, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{Name: z.Ptr("real")}.Build())
		x.NoError(err)

		// Read from outside, which is only possible once it is committed.
		n, err := c.Server.Db.Tenant.Query().Count(ctx)
		x.NoError(err)
		x.Equal(2, n)

		// And the recorder's own write was not reported: a trail that recorded
		// itself would not stop.
		x.Equal(1, r.Len())
	}))
	t.Run("a recorder that refuses takes the write down with it", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		r.Do = func(ctx context.Context, s bare.Server, u bare.Change) error {
			return status.Error(codes.ResourceExhausted, "no")
		}

		_, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())

		// Internal, and not what the recorder said. A recorder writes through
		// a server that answers in the same words this one does, so a conflict
		// it ran into would otherwise reach the caller as a conflict about the
		// row the caller was writing -- and a caller that reads that code, as
		// one looking for something already there does, would act on it.
		x.Equal(codes.Internal, status.Code(err))

		n, err := c.Server.Db.Tenant.Query().Count(ctx)
		x.NoError(err)
		x.Zero(n, "the row was written and then taken back")
	}))
	t.Run("a refusal takes an erase back too", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		r.Do = func(ctx context.Context, s bare.Server, u bare.Change) error {
			return status.Error(codes.ResourceExhausted, "no")
		}

		_, err = c.Tenant().Erase(ctx, v.Ref())
		x.Equal(codes.Internal, status.Code(err))

		_, err = c.Tenant().Get(ctx, v.Pick())
		x.NoError(err, "the row is still there")
	}))
}

// TestRecorders is several recorders at once, said as one value.
//
// It is a value rather than the option being given twice, and that is the whole
// of what changed here: neither answer to "you gave it twice, what did you
// mean" is safe to pick. Replacing loses the one that was going to write the
// trail; adding invents an order nobody chose. So the option refuses, and this
// is where the order and what a refusal costs are both written down.
func TestRecorders(t *testing.T) {
	build := func(t *testing.T, rs ...bare.Recorder) (*Server, *Client) {
		opts := []bare.Option{bare.WithRecorder(bare.Recorders(rs))}

		s := NewServerWith(t, opts...)
		t.Cleanup(func() { s.Close() })

		c := NewClient(t, s)
		t.Cleanup(func() { c.Close() })

		return s, c
	}

	t.Run("both are told", func(t *testing.T) {
		x := require.New(t)
		a, b := &recorder{}, &recorder{}
		_, c := build(t, a, b)

		v, err := c.Tenant().Add(t.Context(), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		for _, r := range []*recorder{a, b} {
			u := r.Only(x)
			x.Equal(pb.TenantService_Add_FullMethodName, u.Method)
			k, ok := u.Key.(uuid.UUID)
			x.True(ok)
			x.Equal(v.GetId(), k[:])
		}
	})

	t.Run("in the order they were given", func(t *testing.T) {
		x := require.New(t)

		var order []string
		say := func(name string) *recorder {
			r := &recorder{}
			r.Do = func(context.Context, bare.Server, bare.Change) error {
				order = append(order, name)
				return nil
			}
			return r
		}
		_, c := build(t, say("first"), say("second"))

		_, err := c.Tenant().Add(t.Context(), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		x.Equal([]string{"first", "second"}, order)
	})

	t.Run("one that refuses stops the rest, and the write", func(t *testing.T) {
		x := require.New(t)

		no := &recorder{}
		no.Do = func(context.Context, bare.Server, bare.Change) error {
			return status.Error(codes.ResourceExhausted, "no")
		}
		after := &recorder{}
		s, c := build(t, no, after)

		_, err := c.Tenant().Add(t.Context(), pb.TenantAddRequest_builder{}.Build())
		x.Equal(codes.Internal, status.Code(err))

		// Every recorder is required, so the write is undone for any of them.
		// A recorder that should not be able to refuse a write says so by not
		// refusing; there is no option here that means it.
		x.Zero(after.Len(), "the ones behind it were not told")

		n, err := s.Db.Tenant.Query().Count(t.Context())
		x.NoError(err)
		x.Zero(n, "the row was written and then taken back")
	})
}

// fakeStream is the little of a server transport stream that [grpc.Method]
// reads, so that a test can hand a server the context gRPC would have.
type fakeStream struct {
	grpc.ServerTransportStream

	method string
}

func (s fakeStream) Method() string { return s.method }

// asIfCalled answers with `ctx` looking the way it looks inside the handler
// gRPC dispatched for `method`.
func asIfCalled(ctx context.Context, method string) context.Context {
	return grpc.NewContextWithServerTransportStream(ctx, fakeStream{method: method})
}

// TestRecordMethod is about the difference between the two names a write has:
// what somebody asked for, and what this server did about it.
//
// They are the same for a request that reached this server as itself, which is
// every test above. They come apart for the request an app actually writes --
// an RPC of its own that ends in one of these -- and that is the case a trail
// is read for.
func TestRecordMethod(t *testing.T) {
	const asked = "/apptest.UserService/Rename"

	t.Run("the request says what was asked for", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}

		s := NewServer(t)
		defer s.Close()

		v := bare.NewTenantServiceServer(s.Db, bare.WithRecorder(r))
		_, err := v.Add(asIfCalled(t.Context(), asked), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		u := r.Only(x)
		x.Equal(asked, u.Method, "what gRPC dispatched, whatever leg of it this was")
		x.Equal(pb.TenantService_Add_FullMethodName, u.By, "and what this server did")
	})

	t.Run("a write nobody called is its own", func(t *testing.T) {
		x := require.New(t)
		r := &recorder{}

		s := NewServer(t)
		defer s.Close()

		// The deployment writing to itself before it serves anything. There is
		// no dispatched RPC to name, and the honest answer is what happened.
		v := bare.NewTenantServiceServer(s.Db, bare.WithRecorder(r))
		_, err := v.Add(t.Context(), pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		u := r.Only(x)
		x.Equal(pb.TenantService_Add_FullMethodName, u.Method)
		x.Equal(u.By, u.Method)
	})
}
