package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
)

// TestADefaultIsWhatTheOptionSays is `(orm.field).default` being read.
//
// It was not: the server wrote the zero value into an `Add` that left a field
// out whatever the option said, so `default: "7"` gave 0 and nothing said so.
// And a request that says the zero value is not one that left the field out.
func TestADefaultIsWhatTheOptionSays(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
		}.Build())
		x.NoError(err)

		got, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("v1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.EqualValues(7, got.GetImplicitI32Seven())
		x.True(got.GetImplicitBoolOn())
		x.Equal("nobody", got.GetImplicitStringNobody())
		x.Equal(pb.Level_LEVEL_HARD, got.GetImplicitLevelHard(), "an enum's default is named by its value")
		x.InDelta(0.5, got.GetImplicitF64Half(), 1e-9)

		_, err = c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v2"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitI32Seven:       z.Ptr(int32(0)),
			ImplicitBoolOn:         z.Ptr(false),
			ImplicitStringNobody:   z.Ptr(""),
		}.Build())
		x.NoError(err)

		got, err = c.ValueField().Get(ctx, pb.ValueFieldGetById("v2").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Zero(got.GetImplicitI32Seven(), "a request that said zero was given the default")
		x.False(got.GetImplicitBoolOn())
		x.Empty(got.GetImplicitStringNobody())
	})(t)
}

// TestADefaultedColumnComesToATableThatHasRows is the column's half of a
// default: a row that predates the column holds what the option says.
//
// It held NULL. A default makes a field optional, optional is nullable in ent,
// and the column was added to the table with nothing to put in the rows already
// there -- which reads back as the zero value in Go, and is nothing like it in
// SQL, where a comparison with the empty string, or with anything else, passes
// NULL by. Where the field had no default the column was NOT NULL with none,
// and the same `ADD COLUMN` was refused outright on a table with rows.
func TestADefaultedColumnComesToATableThatHasRows(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()
	s := NewServer(t)
	defer s.Close()
	c := NewClient(t, s)
	defer c.Close()

	_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
		Id:                     z.Ptr("v1"),
		ImplicitBytes:          []byte{0x01},
		ImplicitImmutableBytes: []byte{0x01},
	}.Build())
	x.NoError(err)

	// The columns gone again, which is the release before they came.
	db := s.Driver.(*sql.Driver).DB()
	columns := []string{
		"implicit_i32_seven",
		"implicit_bool_on",
		"implicit_string_nobody",
		"implicit_level_hard",
		"implicit_f64_half",
		"implicit_string_with_default",
	}
	for _, c := range columns {
		_, err := db.ExecContext(ctx, `ALTER TABLE "valuefield" DROP COLUMN "`+c+`"`)
		x.NoError(err, c)
	}

	x.NoError(s.Db.Schema.Create(ctx), "a column with a default was refused on a table with rows")

	// In SQL, where NULL is not the zero value.
	var (
		seven  int32
		on     bool
		nobody string
		hard   int32
		half   float64
		empty  *string
	)
	x.NoError(db.QueryRowContext(ctx, `SELECT "implicit_i32_seven", "implicit_bool_on", "implicit_string_nobody", "implicit_level_hard", "implicit_f64_half", "implicit_string_with_default" FROM "valuefield" WHERE "id" = 'v1'`).
		Scan(&seven, &on, &nobody, &hard, &half, &empty))
	x.EqualValues(7, seven)
	x.True(on)
	x.Equal("nobody", nobody)
	x.EqualValues(pb.Level_LEVEL_HARD, hard)
	x.InDelta(0.5, half, 1e-9)
	x.NotNil(empty, "a row that predates the column holds NULL where the default is the empty string")
	x.Empty(*empty)
}
