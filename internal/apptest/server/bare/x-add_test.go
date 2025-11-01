package bare_test

import (
	context "context"
	"testing"

	"github.com/google/uuid"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	t.Run("with no key, where key is optional", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		_, err = uuid.FromBytes(v.GetId())
		x.NoError(err)
	}))
	t.Run("with edge", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		_, err = c.User().Add(ctx, pb.UserAddRequest_builder{
			Tenant: tenant.Ref(),
		}.Build())
		x.NoError(err)
	}))
}
