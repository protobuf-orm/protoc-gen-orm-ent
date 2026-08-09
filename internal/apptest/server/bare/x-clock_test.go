package bare_test

import (
	"context"
	"testing"
	"time"

	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// at is a time nobody's wall clock will answer with, so a stamp carrying it is
// one this test put there.
var at = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// C is T for a test served with a clock.
func C(now bare.Clock, run func(ctx context.Context, x *require.Assertions, c *Client)) func(t *testing.T) {
	return func(t *testing.T) {
		s := NewServerWith(t, bare.WithClock(now))
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		run(context.TODO(), require.New(t), c)
	}
}

// TestClock is what the hook is for: a test that can say what time it is.
//
// Every stamp a generated server writes used to read the wall clock, so the one
// thing a test could assert about a time was that it was near now -- and the
// interesting questions are not near now. They are "did the version move",
// "does an erase stamp both fields", "is what came back the row that was
// written". Each of those is a comparison, and a comparison needs a value the
// test chose.
//
// It is **not** about clock skew. What a deployment's clocks say to each other
// is the deployment's business.
func TestClock(t *testing.T) {
	t.Run("an added row is stamped with what the clock said", C(
		func() time.Time { return at },
		func(ctx context.Context, x *require.Assertions, c *Client) {
			v, err := c.Note().Add(ctx, pb.NoteAddRequest_builder{
				Alias: z.Ptr("a"),
				Body:  z.Ptr("a body"),
			}.Build())
			x.NoError(err)

			x.True(at.Equal(v.GetDateCreated().AsTime()), "date_created is %s", v.GetDateCreated().AsTime())
			x.True(at.Equal(v.GetDateUpdated().AsTime()), "date_updated is %s", v.GetDateUpdated().AsTime())
		}))

	// The version is the sharp one. It is refused to a patch document, so a
	// test has no other way to say what it should become -- and "did this write
	// move the version" is the question the whole compare-and-swap rests on.
	t.Run("a write moves the version to what the clock says", func(t *testing.T) {
		var now time.Time

		C(func() time.Time { return now },
			func(ctx context.Context, x *require.Assertions, c *Client) {
				now = at
				v, err := c.Note().Add(ctx, pb.NoteAddRequest_builder{
					Alias: z.Ptr("a"),
					Body:  z.Ptr("a body"),
				}.Build())
				x.NoError(err)
				x.True(at.Equal(v.GetDateUpdated().AsTime()))

				later := at.Add(time.Hour)
				now = later

				// The version has to be given, which is the whole of what a
				// version is -- and it is `at`, because the clock said so.
				u, err := c.Note().Patch(ctx, pb.NotePatchRequest_builder{
					Ref:         v.Ref(),
					Body:        z.Ptr("another body"),
					DateUpdated: timestamppb.New(at),
				}.Build())
				x.NoError(err)

				x.True(later.Equal(u.GetDateUpdated().AsTime()), "the version did not move to the clock")
				x.True(at.Equal(u.GetDateCreated().AsTime()), "date_created is immutable and moved")
			})(t)
	})

	// An erase stamps two fields, and they are written by two statements. That
	// they agree is not something a wall clock can be asked to demonstrate.
	t.Run("an erase stamps what it marks and the version together", C(
		func() time.Time { return at },
		func(ctx context.Context, x *require.Assertions, c *Client) {
			v := note_(ctx, x, c, "a")

			_, err := c.Note().Erase(ctx, v.Ref())
			x.NoError(err)

			row, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
			x.NoError(err)

			x.NotNil(row.DateErased)
			x.True(at.Equal(*row.DateErased), "date_erased is %s", row.DateErased)
			x.True(at.Equal(row.DateUpdated), "date_updated is %s", row.DateUpdated)
		}))

	// Saying nothing is the wall clock, which is what every server did before
	// there was anywhere to say otherwise.
	t.Run("no clock is the wall clock", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		before := time.Now().UTC().Add(-time.Second)

		v := note_(ctx, x, c, "a")

		got := v.GetDateCreated().AsTime()
		x.True(got.After(before), "%s is not near now", got)
		x.True(got.Before(time.Now().UTC().Add(time.Second)), "%s is not near now", got)
	}))

	// A clock that answers in another zone is stored as UTC, because a stamp is
	// compared and ordered rather than shown -- and a time carrying a zone
	// compares equal to itself written another way, which is true and is not
	// what anybody reading the comparison expects.
	t.Run("what a clock answers is stored as UTC", C(
		func() time.Time { return at.In(time.FixedZone("KST", 9*60*60)) },
		func(ctx context.Context, x *require.Assertions, c *Client) {
			v := note_(ctx, x, c, "a")

			row, err := c.Server.Db.Note.Get(ctx, mustId(x, v.GetId()))
			x.NoError(err)

			x.Equal(time.UTC, row.DateCreated.Location())
			x.True(at.Equal(row.DateCreated))
		}))
}
