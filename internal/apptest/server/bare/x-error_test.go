package bare_test

import (
	"context"
	"testing"

	uuid "github.com/google/uuid"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newID() []byte {
	v := uuid.New()
	return v[:]
}

func TestGetErrors(t *testing.T) {
	t.Run("by id, not found returns NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Get(ctx, pb.UserGetById(newID()))
		x.Equal(codes.NotFound, status.Code(err))
	}))
	t.Run("by unique index, not found returns NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = c.User().Get(ctx, pb.UserGetByAlias("ghost", tenant.Ref()))
		x.Equal(codes.NotFound, status.Code(err))
	}))
	t.Run("key not set returns InvalidArgument", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		req := pb.UserGetRequest_builder{Ref: pb.UserRef_builder{}.Build()}.Build()
		_, err := c.User().Get(ctx, req)
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))
	t.Run("malformed id returns InvalidArgument", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Get(ctx, pb.UserGetById([]byte{0x01, 0x02, 0x03}))
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))
}

func TestAddErrors(t *testing.T) {
	t.Run("duplicate key returns AlreadyExists", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		id := newID()
		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Id: id, Alias: z.Ptr("a"), Tenant: tenant.Ref()}.Build())
		x.NoError(err)
		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Id: id, Alias: z.Ptr("b"), Tenant: tenant.Ref()}.Build())
		x.Equal(codes.AlreadyExists, status.Code(err))
	}))
	t.Run("duplicate unique index returns AlreadyExists", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Alias: z.Ptr("dup"), Tenant: tenant.Ref()}.Build())
		x.NoError(err)
		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Alias: z.Ptr("dup"), Tenant: tenant.Ref()}.Build())
		x.Equal(codes.AlreadyExists, status.Code(err))
	}))
	t.Run("non-existent edge returns NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: pb.TenantById(newID())}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))
	t.Run("missing required edge returns InvalidArgument", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Add(ctx, pb.UserAddRequest_builder{}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))
	t.Run("malformed id returns InvalidArgument", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{Id: []byte{0x01, 0x02, 0x03}, Tenant: tenant.Ref()}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
	}))
}
