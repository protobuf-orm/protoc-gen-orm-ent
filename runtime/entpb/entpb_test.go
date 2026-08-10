package entpb_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entpb"
)

// A well-known message, because this module has no generated ones of its own.
// What it is does not matter: the scanner asks only that it is a proto.Message.
var vs = entpb.ValueScanner[*timestamppb.Timestamp]{}

func TestValue(t *testing.T) {
	t.Run("a message is its canonical JSON", func(t *testing.T) {
		x := require.New(t)

		v, err := vs.Value(timestamppb.New(zero))
		x.NoError(err)
		x.Equal(`"2001-02-03T04:05:06Z"`, v)
	})

	// Nil rather than "{}", so that "no value" and "a value with nothing in it"
	// are different rows. A column that cannot tell those apart cannot answer
	// whether the field was ever set.
	t.Run("a message that is not there is NULL", func(t *testing.T) {
		x := require.New(t)

		v, err := vs.Value(nil)
		x.NoError(err)
		x.Nil(v)
	})
}

func TestFromValue(t *testing.T) {
	t.Run("JSON is a message again", func(t *testing.T) {
		x := require.New(t)

		v, err := vs.FromValue(&sql.NullString{String: `"2001-02-03T04:05:06Z"`, Valid: true})
		x.NoError(err)
		x.True(zero.Equal(v.AsTime()))
	})

	t.Run("NULL is nothing rather than an empty message", func(t *testing.T) {
		x := require.New(t)

		v, err := vs.FromValue(&sql.NullString{})
		x.NoError(err)
		x.Nil(v)
	})

	t.Run("what is not JSON is an error rather than a zero value", func(t *testing.T) {
		x := require.New(t)

		_, err := vs.FromValue(&sql.NullString{String: "not json", Valid: true})
		x.Error(err)
	})

	// The scanner ent hands back is the one ScanValue answered with, so
	// anything else is a bug in the wiring rather than in the data -- and it
	// should say so instead of returning a zero value that reads as an empty
	// column.
	t.Run("something else entirely is refused", func(t *testing.T) {
		x := require.New(t)

		_, err := vs.FromValue("a bare string")
		x.ErrorContains(err, "expected *sql.NullString")
	})
}

// A round trip, which is the only property any of this is for.
func TestRoundTrip(t *testing.T) {
	x := require.New(t)

	want := timestamppb.New(zero)

	v, err := vs.Value(want)
	x.NoError(err)

	got, err := vs.FromValue(&sql.NullString{String: v.(string), Valid: true})
	x.NoError(err)
	x.True(want.AsTime().Equal(got.AsTime()))
}

var zero = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
