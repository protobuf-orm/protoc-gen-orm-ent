package bare_test

import (
	"context"
	"testing"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entuuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uuid"
)

// asked is what a [bare.Minter] was told, kept so that a test can say what the
// server asked rather than only what it did with the answer.
type asked struct {
	Entity string
	Given  uuid.UUID
	Ok     bool
}

// M is T for a test served with a minter.
func M(
	f func(rec *[]asked) bare.Minter,
	run func(ctx context.Context, x *require.Assertions, c *Client, rec *[]asked),
) func(t *testing.T) {
	return func(t *testing.T) {
		rec := &[]asked{}

		s := NewServerWith(t, bare.WithMinter(f(rec)))
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		run(t.Context(), require.New(t), c, rec)
	}
}

// watching is a minter that answers the way no minter at all would, and writes
// down what it was asked.
func watching(rec *[]asked) bare.Minter {
	return bare.MinterFunc(func(_ context.Context, entity string, given uuid.UUID, ok bool) (uuid.UUID, error) {
		*rec = append(*rec, asked{Entity: entity, Given: given, Ok: ok})
		if ok {
			return given, nil
		}

		return uuid.New(), nil
	})
}

func TestMinter(t *testing.T) {
	t.Run("is asked which entity, and what the request named", M(watching,
		func(ctx context.Context, x *require.Assertions, c *Client, rec *[]asked) {
			_, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.NoError(err)

			x.Len(*rec, 1)
			x.Equal("apptest.Tenant", (*rec)[0].Entity)
			x.False((*rec)[0].Ok, "the request named no key")
			x.Equal(uuid.Nil(), (*rec)[0].Given)
		}))

	t.Run("is handed the key the request named", M(watching,
		func(ctx context.Context, x *require.Assertions, c *Client, rec *[]asked) {
			k := uuid.New()

			v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{Id: k[:]}.Build())
			x.NoError(err)
			x.Equal(k[:], v.GetId())

			x.Len(*rec, 1)
			x.True((*rec)[0].Ok)
			x.Equal(k, (*rec)[0].Given)
		}))

	t.Run("is asked once per entity a call writes", M(watching,
		func(ctx context.Context, x *require.Assertions, c *Client, rec *[]asked) {
			tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.NoError(err)

			_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
			x.NoError(err)

			x.Len(*rec, 2)
			x.Equal("apptest.Tenant", (*rec)[0].Entity)
			x.Equal("apptest.User", (*rec)[1].Entity)
		}))

	t.Run("what it answers is what the row is stored under", func(t *testing.T) {
		want := uuid.New()

		M(func(*[]asked) bare.Minter {
			return bare.MinterFunc(func(context.Context, string, uuid.UUID, bool) (uuid.UUID, error) {
				return want, nil
			})
		}, func(ctx context.Context, x *require.Assertions, c *Client, _ *[]asked) {
			v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.NoError(err)
			x.Equal(want[:], v.GetId())

			// And it is the row's key and not only what came back.
			u, err := c.Tenant().Get(ctx, pb.TenantGetRequest_builder{
				Ref: pb.TenantRef_builder{Id: want[:]}.Build(),
			}.Build())
			x.NoError(err)
			x.Equal(want[:], u.GetId())
		})(t)
	})

	t.Run("a refusal is the caller's answer", func(t *testing.T) {
		M(func(*[]asked) bare.Minter {
			return bare.MinterFunc(func(context.Context, string, uuid.UUID, bool) (uuid.UUID, error) {
				return uuid.Nil(), status.Error(codes.InvalidArgument, "not that one")
			})
		}, func(ctx context.Context, x *require.Assertions, c *Client, _ *[]asked) {
			_, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.Error(err)
			x.Equal(codes.InvalidArgument, status.Code(err))
			x.Contains(err.Error(), "not that one")
		})(t)
	})

	// The bytes are checked before a minter is asked, so a minter only ever
	// sees something that is already a key. Which refusal a caller gets is the
	// difference between "that is not sixteen bytes" and whatever a minter has
	// to say about a key that is.
	t.Run("is not asked about something that is not a key", M(watching,
		func(ctx context.Context, x *require.Assertions, c *Client, rec *[]asked) {
			_, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{Id: []byte{1, 2, 3}}.Build())
			x.Error(err)
			x.Equal(codes.InvalidArgument, status.Code(err))

			x.Empty(*rec)
		}))
}

// TestNoMinter is what these servers did before there was anywhere to say
// otherwise, and what they still do when nobody says anything.
func TestNoMinter(t *testing.T) {
	t.Run("makes one up when the request named none", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		k, err := entuuid.FromBytes(v.GetId())
		x.NoError(err)
		x.NotEqual(uuid.Nil(), k)
	}))

	t.Run("keeps the one the request named", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		k := uuid.New()

		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{Id: k[:]}.Build())
		x.NoError(err)
		x.Equal(k[:], v.GetId())
	}))
}
