package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A unique index over edges named otherwise than what they point at finds its
// row by those edges. The lookup used to name each edge by its target's type
// -- `GetUser`, `HasUserWith` for an edge called `holder` -- and the bare
// server it generated did not compile.
func TestRefByAnIndexOverRenamedEdges(t *testing.T) {
	t.Run("two edges", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		org, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		// Users are unique by alias within a tenant.
		holder, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("holder")}.Build())
		x.NoError(err)
		other, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("other")}.Build())
		x.NoError(err)

		seat, err := c.Seat().Add(ctx, pb.SeatAddRequest_builder{Holder: holder.Ref(), Org: org.Ref()}.Build())
		x.NoError(err)

		got, err := c.Seat().Get(ctx, pb.SeatGetRequest_builder{
			Ref: pb.SeatRef_builder{Place: pb.SeatRefByPlace_builder{Holder: holder.Ref(), Org: org.Ref()}.Build()}.Build(),
		}.Build())
		x.NoError(err)
		x.Equal(seat.GetId(), got.GetId())

		// Both edges are in the predicate, not only the first.
		_, err = c.Seat().Get(ctx, pb.SeatGetRequest_builder{
			Ref: pb.SeatRef_builder{Place: pb.SeatRefByPlace_builder{Holder: other.Ref(), Org: org.Ref()}.Build()}.Build(),
		}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))
	t.Run("one nullable edge", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		org, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)
		holder, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("holder")}.Build())
		x.NoError(err)
		deputy, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: org.Ref(), Alias: z.Ptr("deputy")}.Build())
		x.NoError(err)

		seat, err := c.Seat().Add(ctx, pb.SeatAddRequest_builder{Holder: holder.Ref(), Org: org.Ref(), Deputy: deputy.Ref()}.Build())
		x.NoError(err)

		got, err := c.Seat().Get(ctx, pb.SeatGetRequest_builder{
			Ref: pb.SeatRef_builder{Deputy: pb.SeatRefByDeputy_builder{Deputy: deputy.Ref()}.Build()}.Build(),
		}.Build())
		x.NoError(err)
		x.Equal(seat.GetId(), got.GetId())
	}))
}
