package bare_test

import (
	"context"
	"testing"
	"time"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A unique index with a timestamp among its fields finds its row by all of
// them. The lookup used to return at the timestamp: what came before it was
// dropped and what came after was never read, so a reference by (desk,
// starts_at) matched every row at that time, and `Get` answered whichever the
// database returned first.
func TestRefByAnIndexWithATimestamp(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		at := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

		// Two shifts at one time, at two desks: the time alone names both.
		a, err := c.Shift().Add(ctx, pb.ShiftAddRequest_builder{Desk: "a", StartsAt: timestamppb.New(at)}.Build())
		x.NoError(err)
		b, err := c.Shift().Add(ctx, pb.ShiftAddRequest_builder{Desk: "b", StartsAt: timestamppb.New(at)}.Build())
		x.NoError(err)

		for _, want := range []*pb.Shift{a, b} {
			got, err := c.Shift().Get(ctx, pb.ShiftGetRequest_builder{
				Ref: pb.ShiftBySlot(want.GetDesk(), timestamppb.New(at)),
			}.Build())
			x.NoError(err)
			x.Equal(want.GetId(), got.GetId(), "desk %s", want.GetDesk())
		}

		// A desk with nothing at that time is nothing, not the first row at it.
		_, err = c.Shift().Get(ctx, pb.ShiftGetRequest_builder{Ref: pb.ShiftBySlot("c", timestamppb.New(at))}.Build())
		x.Equal(codes.NotFound, status.Code(err))

		// And the time still counts: the same desk at another time is another
		// shift, or none.
		_, err = c.Shift().Get(ctx, pb.ShiftGetRequest_builder{
			Ref: pb.ShiftBySlot("a", timestamppb.New(at.Add(time.Minute))),
		}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	})(t)
}
