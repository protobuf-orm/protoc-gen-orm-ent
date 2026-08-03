package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// seed adds a tenant and a user with labels, and returns the user.
func seed(ctx context.Context, x *require.Assertions, c *Client, labels map[string]string) *pb.User {
	tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)

	u, err := c.User().Add(ctx, pb.UserAddRequest_builder{
		Tenant: tenant.Ref(),
		Labels: labels,
	}.Build())
	x.NoError(err)

	return u
}

func get(ctx context.Context, x *require.Assertions, c *Client, u *pb.User) *pb.User {
	v, err := c.User().Get(ctx, u.Ref().Pick().WithSelect(func(s *pb.UserSelect) {
		s.SetAll(true)
	}))
	x.NoError(err)
	return v
}

func doc(x *require.Assertions, ops ...patch.Op) *pb.UserApplyRequest {
	p, err := patch.New("apptest.User", ops[0], ops[1:]...)
	x.NoError(err)
	return pb.UserApplyRequest_builder{Patch: p}.Build()
}

func TestApply(t *testing.T) {
	// The headline: one entry of a map changes and the others are untouched,
	// without ever reading the map. Two clients adding different labels cannot
	// lose each other's work the way a read-modify-write does.
	t.Run("assign one map entry leaves the others", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod", "team": "infra"})

		req := doc(x, patch.Target(patch.MapStr("tier")).In(patch.Name("labels")).Assign(patch.Str("gold")))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)

		x.Equal(map[string]string{
			"env": "prod", "team": "infra", "tier": "gold",
		}, get(ctx, x, c, u).GetLabels())
	}))

	t.Run("overwrite one map entry", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod", "team": "infra"})

		req := doc(x, patch.Target(patch.MapStr("env")).In(patch.Name("labels")).Assign(patch.Str("staging")))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)

		x.Equal(map[string]string{
			"env": "staging", "team": "infra",
		}, get(ctx, x, c, u).GetLabels())
	}))

	t.Run("remove one map entry", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod", "team": "infra"})

		req := doc(x, patch.Target(patch.MapStr("env")).In(patch.Name("labels")).Remove())
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)

		x.Equal(map[string]string{"team": "infra"}, get(ctx, x, c, u).GetLabels())
	}))

	// A miss the statement cannot report becomes a predicate, so the document
	// is abandoned rather than silently doing nothing.
	t.Run("removing an absent entry abandons the document", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod"})

		req := doc(x,
			patch.Target(patch.MapStr("nope")).In(patch.Name("labels")).Remove(),
			patch.Target(patch.Name("name")).Assign(patch.Str("should not land")),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.FailedPrecondition, status.Code(err))

		got := get(ctx, x, c, u)
		x.Equal(map[string]string{"env": "prod"}, got.GetLabels())
		x.Empty(got.GetName(), "the rest of the document must not have applied")
	}))

	t.Run("removing an absent entry under SKIP is tolerated", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod"})

		req := doc(x,
			patch.Target(patch.MapStr("nope")).In(patch.Name("labels")).Skip().Remove(),
			patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	t.Run("assign a scalar", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("Ada")))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	// A test is a WHERE predicate, so the update is a compare-and-swap.
	t.Run("a test that holds lets the write through", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		req := doc(x,
			patch.Target(patch.Name("name")).Test(patch.Str("")),
			patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	t.Run("a test that does not hold writes nothing", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		req := doc(x,
			patch.Target(patch.Name("name")).Test(patch.Str("somebody else")),
			patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.FailedPrecondition, status.Code(err))
		x.Empty(get(ctx, x, c, u).GetName())
	}))

	t.Run("a test on a map entry", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod"})

		req := doc(x,
			patch.Target(patch.MapStr("env")).In(patch.Name("labels")).Test(patch.Str("prod")),
			patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	// The (c) addressing: through the edge, at the target's key.
	t.Run("repoint an edge by the target key", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		other, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		req := doc(x, patch.Target(patch.Name("id")).In(patch.Name("tenant")).
			Assign(patch.Bytes(other.GetId())))
		req.SetRef(u.Ref())

		_, err = c.User().Apply(ctx, req)
		x.NoError(err)

		// Get with a select mask covers props, not edges; the plain form is
		// what returns an edge's key.
		got, err := c.User().Get(ctx, u.Ref().Pick())
		x.NoError(err)
		x.Equal(other.GetId(), got.GetTenant().GetId())
	}))

	t.Run("a missing row is NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("Ada")))
		req.SetRef(pb.UserById(make([]byte, 16)))

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.NotFound, status.Code(err))
	}))

	// A refusal must not look like a bad document: the producer has nothing to
	// fix, so it is Unimplemented rather than InvalidArgument.
	t.Run("what a row cannot express is Unimplemented", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		req := doc(x, patch.Target(patch.Name("name")).Move(patch.Here(patch.Name("alias"))))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.Unimplemented, status.Code(err))
	}))

	t.Run("a document for another message is InvalidArgument", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		p, err := patch.New("apptest.Tenant", patch.Target(patch.Name("name")).Assign(patch.Str("x")))
		x.NoError(err)

		req := pb.UserApplyRequest_builder{Ref: u.Ref(), Patch: p}.Build()
		_, err = c.User().Apply(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))

	// The version field is the server's to stamp; a test on it is the lock.
	t.Run("a successful apply stamps the version", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		before := get(ctx, x, c, u).GetDateUpdated().AsTime()

		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("Ada")))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)

		x.True(get(ctx, x, c, u).GetDateUpdated().AsTime().After(before))
	}))

	// Always. A document used to be able to bring its own version, which made
	// the stamp skippable and the token the client's to choose -- the same hole
	// `_force` carried, reachable here with no flag at all.
	t.Run("a document that writes the version is refused", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		before := get(ctx, x, c, u).GetDateUpdated().AsTime()

		req := doc(x,
			patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
			patch.Target(patch.Name("date_updated")).Assign(
				patch.Msg(patch.F(patch.Name("seconds"), patch.Int64(1577934245)))),
		)
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))

		// The whole document was abandoned, not just the version entry.
		got := get(ctx, x, c, u)
		x.Equal("", got.GetName())
		x.Equal(before, got.GetDateUpdated().AsTime())
	}))

	t.Run("a document that clears the version is refused", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		req := doc(x, patch.Target(patch.Name("date_updated")).Remove())
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))

	// A document of nothing but tests asserts something; it is not a write.
	// Stamping it would move the token every other holder is waiting on, for a
	// request that changed no data.
	t.Run("an assert-only document leaves the version alone", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		before := get(ctx, x, c, u).GetDateUpdated().AsTime()

		req := doc(x, patch.Target(patch.Name("name")).Test(patch.Str("")))
		req.SetRef(u.Ref())

		_, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal(before, get(ctx, x, c, u).GetDateUpdated().AsTime())
	}))

	// The document may rewrite the very column the ref selects on. The row is
	// found again by its key, which no document can move, so a write that
	// committed is not reported to its own author as NotFound.
	t.Run("renaming through the ref that names it still returns the row", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Alias: z.Ptr("old"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		byAlias := get(ctx, x, c, u).Ref()

		req := doc(x, patch.Target(patch.Name("alias")).Assign(patch.Str("new")))
		req.SetRef(byAlias)

		got, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("new", got.GetAlias())
	}))

	// Apply requires the document. Patch is the one that may have nothing to
	// say, and both run the same path underneath, so the difference has to be
	// stated at the RPC.
	t.Run("a request with no patch is refused", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Apply(ctx, pb.UserApplyRequest_builder{Ref: u.Ref()}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.Contains(status.Convert(err).Message(), "no patch")
	}))
}
