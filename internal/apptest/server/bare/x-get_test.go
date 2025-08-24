package bare_test

import (
	context "context"
	"testing"

	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Run("empty select returns edge IDs", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Pick()}.Build())
		x.NoError(err)

		w, err := c.User().Get(ctx, v.PickUp())
		x.NoError(err)
		x.NotEmpty(w.GetTenant().GetId())
		x.Equal(tenant.GetId(), w.GetTenant().GetId())
	}))
	t.Run("select field of edge", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{
			Alias: z.Ptr("foo"),
			Name:  z.Ptr("bar"),
		}.Build())
		x.NoError(err)

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Pick()}.Build())
		x.NoError(err)

		w, err := c.User().Get(ctx, v.PickUp())
		x.NoError(err)
		x.Empty(w.GetTenant().GetAlias())

		w, err = c.User().Get(ctx, v.PickUp().WithSelect(func(s *pb.UserSelect) {
			s.SetTenant(pb.TenantSelect_builder{
				Alias: z.Ptr(true),
			}.Build())
		}))
		x.NoError(err)
		x.NotEmpty(w.GetTenant().GetAlias())
		x.Empty(w.GetTenant().GetName())
		x.Equal(tenant.GetAlias(), w.GetTenant().GetAlias())
	}))
}
