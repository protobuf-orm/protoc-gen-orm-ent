package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/messagefield"
)

// TestMessageField is a message stored in a column, and it is the test that was
// missing.
//
// `MessageField` has had message fields since it was written, and every one of
// them was a `google.protobuf.Struct` -- the one message type that never
// broke. The well-known types are generated with the **open** API, so their
// fields are exported and `encoding/json` can see them; `field.Json` marshalled
// them correctly and always had.
//
// Anything generated the way this repository generates -- `Api_OPAQUE` -- has
// no exported fields at all:
//
//	type Held struct {
//		state            protoimpl.MessageState `protogen:"opaque.v1"`
//		xxx_hidden_What  string
//	}
//
// So `field.Json` stored `{}` for it. The insert reported success, the row
// compared equal to empty, and nothing failed at any layer. It is now a string
// carrying the canonical protobuf JSON, through `entpb.ValueScanner`.
//
// The reason to assert on the **row** and not only on the answer is that the
// answer was right the whole time: an Add echoes what it was given, so the
// value looked stored right up until somebody read it back.
func TestMessageField(t *testing.T) {
	t.Run("a message survives being written and read", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id:   z.Ptr("a"),
			Held: pb.Held_builder{What: z.Ptr("a thing"), HowMany: z.Ptr(int32(3))}.Build(),
		}.Build())
		x.NoError(err)
		x.Equal("a thing", v.GetHeld().GetWhat())

		// The row, which is the assertion that would have failed before.
		row, err := c.Server.Db.MessageField.Get(ctx, "a")
		x.NoError(err)
		x.Equal("a thing", row.Held.GetWhat())
		x.EqualValues(3, row.Held.GetHowMany())

		// And read back through the server, which is a second decode.
		got, err := c.MessageField().Get(ctx, pb.MessageFieldGetRequest_builder{Ref: pb.MessageFieldRef_builder{Id: z.Ptr("a")}.Build()}.Build())
		x.NoError(err)
		x.Equal("a thing", got.GetHeld().GetWhat())
		x.EqualValues(3, got.GetHeld().GetHowMany())
	}))

	t.Run("a message that was not given is absent rather than empty", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{Id: z.Ptr("b")}.Build())
		x.NoError(err)

		// NULL and not `{}`. A column that cannot tell "no value" from "a value
		// with nothing in it" cannot answer whether the field was ever set,
		// and that is a question every optional message field raises.
		n, err := c.Server.Db.MessageField.Query().
			Where(messagefield.Id("b"), messagefield.NullableHeldIsNil()).
			Count(ctx)
		x.NoError(err)
		x.Equal(1, n, "an unset message field is not NULL")
	}))

	t.Run("a message is replaced whole", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id:   z.Ptr("c"),
			Held: pb.Held_builder{What: z.Ptr("before"), HowMany: z.Ptr(int32(1))}.Build(),
		}.Build())
		x.NoError(err)

		u, err := c.MessageField().Patch(ctx, pb.MessageFieldPatchRequest_builder{
			Ref:  v.Ref(),
			Held: pb.Held_builder{What: z.Ptr("after")}.Build(),
		}.Build())
		x.NoError(err)

		x.Equal("after", u.GetHeld().GetWhat())
		// Zero rather than 1: the whole message was assigned, so a field the
		// new one did not carry is gone. That is what "a value of the row"
		// means and it is worth being able to point at.
		x.EqualValues(0, u.GetHeld().GetHowMany())

		row, err := c.Server.Db.MessageField.Get(ctx, "c")
		x.NoError(err)
		x.Equal("after", row.Held.GetWhat())
	}))
}

// TestMessageCollections is the same bug one level down, and the reason the
// case above passing was not enough.
//
// A message *field* is given a ValueScanner by the schema generator, which is
// what fixed it. A list of the same message, or a map whose values are, is a
// `field.Json` with no scanner at all -- so it is marshalled by
// `encoding/json`, which is exactly what could not see an opaque message in
// the first place. Every element went in as `{}`.
//
// Nothing in this repository held a collection of a message it generates: the
// repeated field was a `google.protobuf.Struct` and so were the map values,
// and those are the open API. The coverage was, again, of the case that
// worked.
func TestMessageCollections(t *testing.T) {
	t.Run("a list of messages survives being written and read", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id: z.Ptr("list"),
			HeldList: []*pb.Held{
				pb.Held_builder{What: z.Ptr("first"), HowMany: z.Ptr(int32(1))}.Build(),
				pb.Held_builder{What: z.Ptr("second"), HowMany: z.Ptr(int32(2))}.Build(),
			},
		}.Build())
		x.NoError(err)

		row, err := c.Server.Db.MessageField.Get(ctx, "list")
		x.NoError(err)
		x.Len(row.HeldList, 2)
		x.Equal("first", row.HeldList[0].GetWhat())
		x.EqualValues(1, row.HeldList[0].GetHowMany())
		x.Equal("second", row.HeldList[1].GetWhat())
		x.EqualValues(2, row.HeldList[1].GetHowMany())

		got, err := c.MessageField().Get(ctx, pb.MessageFieldGetRequest_builder{Ref: pb.MessageFieldRef_builder{Id: z.Ptr("list")}.Build()}.Build())
		x.NoError(err)
		x.Len(got.GetHeldList(), 2)
		x.Equal("first", got.GetHeldList()[0].GetWhat())
		x.Equal("second", got.GetHeldList()[1].GetWhat())
	}))

	t.Run("a map of messages survives being written and read", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MapField().Add(ctx, pb.MapFieldAddRequest_builder{
			Id: z.Ptr("map"),
			ImplicitHeld: map[string]*pb.Held{
				"a": pb.Held_builder{What: z.Ptr("apple"), HowMany: z.Ptr(int32(7))}.Build(),
			},
		}.Build())
		x.NoError(err)

		row, err := c.Server.Db.MapField.Get(ctx, "map")
		x.NoError(err)
		x.Len(row.ImplicitHeld, 1)
		x.Equal("apple", row.ImplicitHeld["a"].GetWhat())
		x.EqualValues(7, row.ImplicitHeld["a"].GetHowMany())

		got, err := c.MapField().Get(ctx, pb.MapFieldGetRequest_builder{Ref: pb.MapFieldRef_builder{Id: z.Ptr("map")}.Build()}.Build())
		x.NoError(err)
		x.Equal("apple", got.GetImplicitHeld()["a"].GetWhat())
	}))
}
