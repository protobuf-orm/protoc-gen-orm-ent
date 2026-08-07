package bare_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/note"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// note adds one, failing the test if it cannot.
func note_(ctx context.Context, x *require.Assertions, c *Client, alias string, body ...string) *pb.Note {
	// The body index says `includes_erased`, so a body is taken once and stays
	// taken. A test that reuses an alias has to vary it.
	b := alias + " body"
	if len(body) > 0 {
		b = body[0]
	}

	v, err := c.Note().Add(ctx, pb.NoteAddRequest_builder{
		Alias: z.Ptr(alias),
		Body:  z.Ptr(b),
	}.Build())
	x.NoError(err)

	return v
}

func TestEraseSoftly(t *testing.T) {
	t.Run("the row stays and says it is gone", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		// Gone, as far as anything here is concerned.
		_, err = c.Note().Get(ctx, pb.NoteGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.NotFound, status.Code(err))

		// And still there, which is the point: whatever referred to it -- an
		// audit trail, a foreign key -- still finds something.
		u, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.NotNil(u.DateErased)
	}))

	t.Run("what is gone is gone to every read", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")
		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		// Get by the key, and by the alias, which is a different query.
		_, err = c.Note().Get(ctx, pb.NoteGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.NotFound, status.Code(err))

		byAlias := &pb.NoteRef{}
		byAlias.SetAlias(pb.NoteRefByAlias_builder{Alias: z.Ptr("a")}.Build())
		_, err = c.Note().Get(ctx, pb.NoteGetRequest_builder{Ref: byAlias}.Build())
		x.Equal(codes.NotFound, status.Code(err))

		// Patch and Apply run one path and it is narrowed the same way.
		_, err = c.Note().Patch(ctx, pb.NotePatchRequest_builder{
			Ref:  v.Ref(),
			Body: z.Ptr("nope"),
			// Declining the lock, so that what comes back is about the row
			// being gone rather than about the version being unsaid.
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.Equal(codes.NotFound, status.Code(err))

		doc, err := patch.New("apptest.Note", patch.Target(patch.Name("body")).Assign(patch.Str("nope")))
		x.NoError(err)

		_, err = c.Note().Apply(ctx, pb.NoteApplyRequest_builder{Ref: v.Ref(), Patch: doc}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))

	t.Run("erasing it again erases nothing and succeeds", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		before, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)

		// Out of reach is not there, and erasing what is not there succeeds.
		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		// And the stamp is the one the first erase wrote, not a fresh one: the
		// second call matched nothing at all.
		after, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.True(before.DateErased.Equal(*after.DateErased))
	}))

	// An erase is a write, so the token moves with it. A row brought back by
	// hand would otherwise return holding a version that was current before it
	// left, and a client that had read it then would find its test still
	// passing against a row that had been gone in between.
	t.Run("the version moves with it", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		u, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.True(u.DateUpdated.After(v.GetDateUpdated().AsTime()))
	}))
}

func TestErasedNames(t *testing.T) {
	// The whole reason a unique index of a soft-erasing entity is partial.
	t.Run("an erased row gives up its name", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Add(ctx, pb.NoteAddRequest_builder{
			Alias: z.Ptr("a"),
			Body:  z.Ptr("other"),
		}.Build())
		x.Equal(codes.AlreadyExists, status.Code(err), "while it is there, the name is taken")

		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		u, err := c.Note().Add(ctx, pb.NoteAddRequest_builder{
			Alias: z.Ptr("a"),
			Body:  z.Ptr("other"),
		}.Build())
		x.NoError(err, "and once it is gone, it is free")
		x.NotEqual(v.GetId(), u.GetId())
	}))

	// And the index that said `includes_erased` keeps its name taken.
	t.Run("an index that says so keeps the name", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		_, err = c.Note().Add(ctx, pb.NoteAddRequest_builder{
			Alias: z.Ptr("b"),
			Body:  z.Ptr("a body"),
		}.Build())
		x.Equal(codes.AlreadyExists, status.Code(err))
	}))

	t.Run("two erased rows may share a name", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		for i := range 3 {
			v := note_(ctx, x, c, "a", fmt.Sprintf("body-%d", i))

			// Each of them takes `a`, gives it up, and the next one takes it
			// again. A partial index that was not partial would refuse the
			// second of these.
			_, err := c.Note().Erase(ctx, v.Ref())
			x.NoError(err, "round %d", i)
		}

		n, err := c.Server.Db.Note.Query().Where(note.AliasEQ("a")).Count(ctx)
		x.NoError(err)
		x.Equal(3, n, "three rows, all erased, all called a")
	}))
}

// TestAddIsAlive pins that nothing can create a row that is already gone.
func TestAddIsAlive(t *testing.T) {
	t.Run("an add has nothing to say about it", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		// There is no field to set: protoc-gen-orm-service leaves it out of an
		// AddRequest, so this is a compile-time fact as much as a runtime one.
		// What is checked here is the other half -- that the column comes up
		// null rather than at some zero time that reads as erased.
		v := note_(ctx, x, c, "a")

		u, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.Nil(u.DateErased)

		_, err = c.Note().Get(ctx, pb.NoteGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err, "and so it is there to be read")
	}))
}

// mustId reads an identifier the way the ent client wants one.
func mustId(x *require.Assertions, v []byte) uuid.UUID {
	k, err := uuid.FromBytes(v)
	x.NoError(err)
	return k
}
