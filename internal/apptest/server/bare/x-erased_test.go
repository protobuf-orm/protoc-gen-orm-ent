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
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
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

// TestPickIsAlive pins that a **reference** does not reach an erased row.
//
// Narrowing a read is not enough on its own, because a reference is composed
// into another entity's: a unique index that names an edge asks `NotePick` for
// a predicate and puts it inside `HasNoteWith`, and what the read narrows there
// is the child. So the erased row is reachable by naming it as somebody's
// parent -- through whatever scope the child is behind, since a scope narrows
// the child's path and not the parent's liveness. `NoteId`, which is how an Add
// resolves an edge, has the same shape.
func TestPickIsAlive(t *testing.T) {
	t.Run("a reference by a name stops matching once the row is gone", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		byAlias := &pb.NoteRef{}
		byAlias.SetAlias(pb.NoteRefByAlias_builder{Alias: z.Ptr("a")}.Build())

		p, err := bare.NotePick(byAlias)
		x.NoError(err)

		n, err := c.Server.Db.Note.Query().Where(p).Count(ctx)
		x.NoError(err)
		x.Equal(1, n, "while it is there")

		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		n, err = c.Server.Db.Note.Query().Where(p).Count(ctx)
		x.NoError(err)
		x.Equal(0, n, "and the predicate itself is what stops matching, not the read that used it")
	}))

	t.Run("and the key is the same answer", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		p, err := bare.NotePick(v.Ref())
		x.NoError(err)

		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		n, err := c.Server.Db.Note.Query().Where(p).Count(ctx)
		x.NoError(err)
		x.Equal(0, n)
	}))
}

// mustId reads an identifier the way the ent client wants one.
func mustId(x *require.Assertions, v []byte) uuid.UUID {
	k, err := uuid.FromBytes(v)
	x.NoError(err)
	return k
}

// TestPromotedUnique is the other spelling of the same thing. `slug` says
// `unique` on the field rather than as an index, and a field's uniqueness is
// ordinarily a constraint over every row -- which for an entity that erases
// softly would hold the value of an erased row for ever, while a declared
// index of the same entity gives it up. One of the two behaving differently is
// worse than either, so the field one is written as a partial index too.
func TestPromotedUnique(t *testing.T) {
	slug := func(ctx context.Context, x *require.Assertions, c *Client, alias, slug string) (*pb.Note, error) {
		return c.Note().Add(ctx, pb.NoteAddRequest_builder{
			Alias: z.Ptr(alias),
			Body:  z.Ptr(alias + "/" + slug),
			Slug:  z.Ptr(slug),
		}.Build())
	}

	t.Run("a unique field gives its value up too", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := slug(ctx, x, c, "a", "s")
		x.NoError(err)

		_, err = slug(ctx, x, c, "b", "s")
		x.Equal(codes.AlreadyExists, status.Code(err), "while it is there")

		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		u, err := slug(ctx, x, c, "c", "s")
		x.NoError(err, "and once it is gone")
		x.NotEqual(v.GetId(), u.GetId())
	}))

	// And the API did not change shape for it: the Ref of a unique field is
	// the bare scalar it always was, not the wrapper message a declared index
	// produces. Which props are keys is `graph`'s to say and it reads the
	// field's own `unique`; only the SQL moved.
	t.Run("and is still named by a bare value", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := slug(ctx, x, c, "a", "s")
		x.NoError(err)

		ref := &pb.NoteRef{}
		ref.SetSlug("s")

		u, err := c.Note().Get(ctx, pb.NoteGetRequest_builder{Ref: ref}.Build())
		x.NoError(err)
		x.Equal(v.GetId(), u.GetId())
	}))
}

// TestEraseSaysWhetherItErased is what once-only is built on.
//
// Erasing what is not there **succeeds**, and has to: a caller cancelling
// something that may already be gone should not have to tell a race from a
// mistake. So the RPC cannot say "this call did it" by failing -- and it used
// to answer `Empty`, which meant it could not say it at all.
//
// Anything single-use is spent by erasing it. Without this answer every
// concurrent presenter of one handle is told exactly what the winner is told:
// the server did the right thing at the row and then said nothing about it.
func TestEraseSaysWhetherItErased(t *testing.T) {
	t.Run("the one that did, and the one that did not", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		res, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)
		x.True(res.GetErased(), "the call that erased the row said it had not")

		res, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err, "erasing what is gone is not a failure")
		x.False(res.GetErased(), "a second erase claimed the row")
	}))

	t.Run("and a row that was never there", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		ref := &pb.NoteRef{}
		ref.SetAlias(pb.NoteRefByAlias_builder{Alias: z.Ptr("never-existed")}.Build())

		res, err := c.Note().Erase(ctx, ref)
		x.NoError(err)
		x.False(res.GetErased())
	}))

	// With no recorder the early read is skipped entirely, so the answer comes
	// from the statement rather than from a lookup before it. Both paths have
	// to agree, because which one runs is a deployment's choice and not a
	// caller's.
	t.Run("with no recorder installed, which is the other path", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v := note_(ctx, x, c, "a")

		res, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)
		x.True(res.GetErased())

		res, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)
		x.False(res.GetErased())
	}))
}

// TestErasingWhatIsAlreadyErasedRecordsNothing is the invariant `x-erase.go`
// states about itself.
//
// Its own words: *the predicate carries that narrowing too, so erasing what is
// already erased matches nothing, records nothing and succeeds -- which is what
// erasing what was never there has always done.*
//
// It did not carry it. With a recorder installed -- and only then, which is why
// this was invisible in every test that did not use one -- the predicate was
// **replaced** with `IDEQ(v)` rather than narrowed by it, throwing away both
// the liveness narrowing `Pick` had put on and the scope `narrow` had added.
//
// So a second erase matched, moved the timestamp again, and recorded a second
// Change saying it had erased the row. Two entries in a trail for one thing
// that happened once -- and `n > 0` stopped being the "did **this** call do it"
// signal the code around it reads it as, which is the property anything wanting
// once-only semantics has to build on.
func TestErasingWhatIsAlreadyErasedRecordsNothing(t *testing.T) {
	t.Run("a second erase is a no-op that succeeds", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v := note_(ctx, x, c, "a")

		r.mu.Lock()
		r.Changes = nil
		r.mu.Unlock()

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)
		x.Equal(1, r.Len(), "the erase that did it recorded nothing")

		// Succeeds, because erasing what is out of reach has always succeeded
		// and `keys.Undelegate`-shaped callers depend on it.
		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		x.Equal(1, r.Len(), "a second erase recorded a Change saying it erased the row")
	}))

	t.Run("and the row keeps the moment it was actually erased", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		v := note_(ctx, x, c, "a")

		_, err := c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		was, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.NotNil(was.DateErased)

		_, err = c.Note().Erase(ctx, v.Ref())
		x.NoError(err)

		// A second erase that matched would move it, and a row would then
		// answer with a time nothing happened at.
		now, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
		x.NoError(err)
		x.Equal(was.DateErased, now.DateErased,
			"a second erase moved the moment the row was erased at")
	}))
}
