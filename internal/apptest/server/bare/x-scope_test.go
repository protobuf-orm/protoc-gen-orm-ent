package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/predicate"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/user"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// hidden is the alias of the User these tests are not allowed to see.
const hidden = "hidden"

// userScope is a [bare.Scope] with something to say about Users and nothing
// about anything else, which is what embedding [bare.Unscoped] is for: the
// other entities go on seeing every row, and so will one added afterwards.
type userScope struct {
	bare.Unscoped

	p func(ctx context.Context) (predicate.User, error)
}

func (s userScope) UserScope(ctx context.Context) (predicate.User, error) {
	return s.p(ctx)
}

// S is T for a test served behind a scope. `p` is asked on every query the User
// server builds; nothing else is scoped, so a test can still arrange whatever
// state it likes through the other services.
func S(
	p func(ctx context.Context) (predicate.User, error),
	run func(ctx context.Context, x *require.Assertions, c *Client, s *Server),
) func(t *testing.T) {
	return func(t *testing.T) {
		s := NewServerWith(t, bare.WithScope(userScope{p: p}))
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		run(t.Context(), require.New(t), c, s)
	}
}

// notHidden narrows to everything but the one User named [hidden].
func notHidden(context.Context) (predicate.User, error) {
	return user.AliasNEQ(hidden), nil
}

// unscoped is a User server built from the same database and told nothing, so
// a test can look at a row the scope keeps from it. It doubles as the check
// that a server built by hand with no options sees everything.
func unscoped(s *Server) pb.UserServiceServer {
	return bare.NewUserServiceServer(s.Db)
}

// sow puts two Users in, one of which the scope hides, and answers with both.
// They are written through the same scoped server the test reads through,
// which is the point: what is written is not narrowed and what is read is.
func sow(ctx context.Context, x *require.Assertions, c *Client) (seen *pb.User, unseen *pb.User) {
	t, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)

	add := func(alias string) *pb.User {
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{
			Tenant: t.Ref(),
			Alias:  z.Ptr(alias),
		}.Build())
		x.NoError(err)

		return v
	}

	return add("seen"), add(hidden)
}

func TestScopeGet(t *testing.T) {
	t.Run("answers with what is in scope", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		v, _ := sow(ctx, x, c)

		u, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Equal(v.GetId(), u.GetId())
	}))

	t.Run("a row out of scope is not there", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		_, v := sow(ctx, x, c)

		// NotFound and not PermissionDenied. That the row exists is itself
		// something a caller who may not read it should not be told, and this
		// falls out of the query rather than out of a second rule.
		_, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))

	t.Run("named by something other than the key, still not there", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		_, v := sow(ctx, x, c)

		ref := &pb.UserRef{}
		ref.SetAlias(pb.UserRefByAlias_builder{
			Alias:  z.Ptr(v.GetAlias()),
			Tenant: v.GetTenant().Ref(),
		}.Build())

		_, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: ref}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))
}

func TestScopeApply(t *testing.T) {
	t.Run("writes what is in scope", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		v, _ := sow(ctx, x, c)

		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("Anna")))
		req.SetRef(v.Ref())

		u, err := c.User().Apply(ctx, req)
		x.NoError(err)
		x.Equal("Anna", u.GetName())
	}))

	t.Run("a row out of scope is not there to write to", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, s *Server) {
		_, v := sow(ctx, x, c)

		req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str("Anna")))
		req.SetRef(v.Ref())

		_, err := c.User().Apply(ctx, req)
		// Not FailedPrecondition: nothing was asserted, the row was simply not
		// matched, and the answer has to be the one Get gives for it.
		x.Equal(codes.NotFound, status.Code(err))

		// And it really was not written.
		u, err := unscoped(s).Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Empty(u.GetName())
	}))
}

func TestScopeErase(t *testing.T) {
	t.Run("erases what is in scope", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, s *Server) {
		v, _ := sow(ctx, x, c)

		_, err := c.User().Erase(ctx, v.Ref())
		x.NoError(err)

		_, err = unscoped(s).Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))

	t.Run("erasing what is out of scope erases nothing", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, s *Server) {
		_, v := sow(ctx, x, c)

		// Erasing what is not there succeeds, and out of scope is not there.
		// The row surviving is the whole of what is under test.
		_, err := c.User().Erase(ctx, v.Ref())
		x.NoError(err)

		u, err := unscoped(s).Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Equal(v.GetId(), u.GetId())
	}))
}

func TestScopeRefusal(t *testing.T) {
	refuse := func(context.Context) (predicate.User, error) {
		return nil, status.Error(codes.PermissionDenied, "not today")
	}

	t.Run("is the caller's answer", S(refuse, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		// Adding still works, since Add builds a row rather than finding one
		// and so never asks. That is the boundary, and here it is also what
		// leaves a row for the read below to be refused about.
		v, _ := sow(ctx, x, c)

		// A hook may refuse rather than narrow, and what it answers with
		// reaches the caller as it is: it is the only thing here that knows
		// why.
		_, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.PermissionDenied, status.Code(err))
	}))
}

func TestScopeSaysNothing(t *testing.T) {
	nothing := func(context.Context) (predicate.User, error) {
		return nil, nil
	}

	t.Run("narrows nothing", S(nothing, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		_, v := sow(ctx, x, c)

		// A hook that answers with no predicate is not one that answers with
		// the empty set. It is how a call with nothing to decide by -- a
		// deployment writing to itself before anybody exists -- gets to see
		// everything.
		u, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Equal(v.GetId(), u.GetId())
	}))
}

// TestUnscoped pins what the embeddable default is worth on its own: a scope
// that says nothing about anything is a server that sees every row, so an app
// that narrows one entity has said nothing about the others.
func TestUnscoped(t *testing.T) {
	t.Run("sees every row", func(t *testing.T) {
		x := require.New(t)

		s := NewServerWith(t, bare.WithScope(bare.Unscoped{}))
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		ctx := t.Context()
		_, v := sow(ctx, x, c)

		u, err := c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.NoError(err)
		x.Equal(v.GetId(), u.GetId())
	})
}

// TestScopeLeavesAddAlone pins the boundary, which is not obvious: what a
// caller may create is not a predicate, and the servers in front are where that
// is said.
func TestScopeLeavesAddAlone(t *testing.T) {
	t.Run("adds what the scope then hides", S(notHidden, func(ctx context.Context, x *require.Assertions, c *Client, _ *Server) {
		tn, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{
			Tenant: tn.Ref(),
			Alias:  z.Ptr(hidden),
		}.Build())
		x.NoError(err)

		// Written, and then out of reach of the very caller that wrote it.
		_, err = c.User().Get(ctx, pb.UserGetRequest_builder{Ref: v.Ref()}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))
}
