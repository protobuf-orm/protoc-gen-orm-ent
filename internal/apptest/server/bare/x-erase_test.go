package bare_test

import (
	"context"
	"testing"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErase(t *testing.T) {
	t.Run("erase then get returns NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		_, err = c.User().Erase(ctx, v.Ref())
		x.NoError(err)

		_, err = c.User().Get(ctx, v.Pick())
		x.Equal(codes.NotFound, status.Code(err))
	}))
	t.Run("erase returns a non-nil Empty response", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		resp, err := c.User().Erase(ctx, v.Ref())
		x.NoError(err)
		x.NotNil(resp)
	}))
	// NOTE: Erase does not check the affected-row count, so erasing a missing
	// entity currently succeeds (idempotent) rather than returning NotFound.
	// This test pins the current behavior; change it if the service contract
	// should make Erase report NotFound.
	t.Run("erase of a non-existent entity is idempotent (success)", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Erase(ctx, pb.UserById(newId()))
		x.NoError(err)
	}))
}
