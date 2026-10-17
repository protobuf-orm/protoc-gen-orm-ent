package bare_test

import (
	context "context"
	"sync/atomic"
	"testing"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Run("empty select returns edge Ids", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		w, err := c.User().Get(ctx, v.Pick())
		x.NoError(err)
		x.NotEmpty(w.GetTenant().GetId())
		x.Equal(tenant.GetId(), w.GetTenant().GetId())
	}))
	t.Run("empty select answers an edge with its key and nothing else", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		x.True(tenant.HasDateCreated())

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		// A reference, and not the tenant read with its key alone: that
		// answered every other field of it at its zero value, and a tenant
		// created in the year 1.
		w, err := c.User().Get(ctx, v.Pick())
		x.NoError(err)
		x.Equal(tenant.GetId(), w.GetTenant().GetId())
		x.False(w.GetTenant().HasDateCreated())
	}))
	t.Run("select field of edge", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		tenant, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{
			Alias: z.Ptr("foo"),
			Name:  z.Ptr("bar"),
		}.Build())
		x.NoError(err)

		v, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
		x.NoError(err)

		w, err := c.User().Get(ctx, v.Pick())
		x.NoError(err)
		x.Empty(w.GetTenant().GetAlias())

		w, err = c.User().Get(ctx, v.Pick().WithSelect(func(s *pb.UserSelect) {
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

// A Get that selects nothing is one query, however many edges the row has: the
// row holds each neighbour's key, which is all such a Get answers for one. It
// was a query for the row and one more for each edge.
func TestAGetThatSelectsNothingIsOneQuery(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()

	driver, err := sql.Open("sqlite3", "file:get-one-query?mode=memory&cache=shared&_fk=1")
	x.NoError(err)
	defer driver.Close()
	driver.DB().SetMaxOpenConns(1)

	var n atomic.Int32
	db := ent.NewClient(ent.Driver(dialect.DebugWithContext(driver, func(context.Context, ...any) { n.Add(1) })))
	x.NoError(db.Schema.Create(ctx))
	s, err := bare.NewServer(db)
	x.NoError(err)

	org, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	holder, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("holder")}.Build())
	x.NoError(err)
	deputy, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("deputy")}.Build())
	x.NoError(err)
	seat, err := s.Seat().Add(ctx, pb.SeatAddRequest_builder{Holder: holder.Ref(), Org: org.Ref(), Deputy: deputy.Ref()}.Build())
	x.NoError(err)

	n.Store(0)
	got, err := s.Seat().Get(ctx, seat.Pick())
	x.NoError(err)
	x.EqualValues(1, n.Load(), "three edges, and one query")
	x.Equal(holder.GetId(), got.GetHolder().GetId())
	x.Equal(org.GetId(), got.GetOrg().GetId())
	x.Equal(deputy.GetId(), got.GetDeputy().GetId())
}
