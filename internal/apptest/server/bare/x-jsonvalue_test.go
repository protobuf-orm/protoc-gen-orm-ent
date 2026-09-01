package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// A `test` on one entry of a JSON column extracts the entry and compares it, so
// the document's value has to be spelled the way the row spells it -- which is
// whatever encoding/json wrote, since that is what ent's Add binds and what
// this backend's own partial writes bind. Two kinds used to be spelled a second
// way in the comparison and could therefore never match: bytes, stored as
// base64 and compared as the raw bytes, and a message, stored as protojson
// compacted and HTML-escaped by encoding/json and compared as protojson left
// it.
//
// The cost of that is not a failed test. A test that does not hold abandons the
// whole document, so every other entry in it silently did not happen, and the
// caller is told only that something did not hold.
//
// These go through the RPC because that is the only place the two encoders meet
// a real column: one wrote the row, the other has to name what is in it.

// jsonStruct is the message these use, with a byte the two encoders disagree
// about: protojson leaves `<` alone and encoding/json escapes it.
func jsonStruct(x *require.Assertions) *structpb.Struct {
	v, err := structpb.NewStruct(map[string]any{"foo": "a<b"})
	x.NoError(err)
	return v
}

// jsonStructValue is the same message as a document literal.
func jsonStructValue(s string) patch.Value {
	return patch.Msg(patch.F(patch.Name("fields"), patch.Map(patch.E(
		patch.MapStr("foo"),
		patch.Msg(patch.F(patch.Name("string_value"), patch.Str(s))),
	))))
}

func applyValueField(ctx context.Context, c *Client, id string, ops ...patch.Op) error {
	p, err := patch.New("apptest.ValueField", ops[0], ops[1:]...)
	if err != nil {
		return err
	}
	_, err = c.ValueField().Apply(ctx, pb.ValueFieldApplyRequest_builder{
		Ref: pb.ValueFieldById(id), Patch: p,
	}.Build())
	return err
}

func applyMapField(ctx context.Context, c *Client, id string, ops ...patch.Op) error {
	p, err := patch.New("apptest.MapField", ops[0], ops[1:]...)
	if err != nil {
		return err
	}
	_, err = c.MapField().Apply(ctx, pb.MapFieldApplyRequest_builder{
		Ref: pb.MapFieldById(id), Patch: p,
	}.Build())
	return err
}

func applyMessageField(ctx context.Context, c *Client, id string, ops ...patch.Op) error {
	p, err := patch.New("apptest.MessageField", ops[0], ops[1:]...)
	if err != nil {
		return err
	}
	_, err = c.MessageField().Apply(ctx, pb.MessageFieldApplyRequest_builder{
		Ref: pb.MessageFieldById(id), Patch: p,
	}.Build())
	return err
}

func TestApplyTestsAnElementOfAList(t *testing.T) {
	// The one that could never hold: Add stores "3q2+7w==" and the comparison
	// used to name the four bytes themselves.
	t.Run("bytes hold against the base64 the row stores", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		want := []byte{0xde, 0xad, 0xbe, 0xef}
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitBytess:         [][]byte{want, {0x01}},
		}.Build())
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_bytess")).Test(patch.Bytes(want)))
		x.NoError(err)

		// And the assertion is worth something: another value does not hold.
		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_bytess")).Test(patch.Bytes([]byte{0x02})))
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))

	// What a test that cannot hold really costs: the rest of the document.
	t.Run("a held test lets the rest of the document through", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		want := []byte{0xde, 0xad, 0xbe, 0xef}
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitBytess:         [][]byte{want},
		}.Build())
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_bytess")).Test(patch.Bytes(want)),
			patch.Target(patch.Name("implicit_string")).Assign(patch.Str("Ada")))
		x.NoError(err)

		got, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("v1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal("Ada", got.GetImplicitString())
	}))

	t.Run("a message holds against the protojson the row stores", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id:       z.Ptr("g1"),
			Explicit: jsonStruct(x),
			Repeated: []*structpb.Struct{jsonStruct(x)},
		}.Build())
		x.NoError(err)

		err = applyMessageField(ctx, c, "g1",
			patch.Target(patch.Index(0)).In(patch.Name("repeated")).Test(jsonStructValue("a<b")))
		x.NoError(err)

		err = applyMessageField(ctx, c, "g1",
			patch.Target(patch.Index(0)).In(patch.Name("repeated")).Test(jsonStructValue("other")))
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))

	// A float is written with the precision of its own width and the row parses
	// that text as a double, so the comparison has to name the same double.
	t.Run("a float32 holds against the text the row parses", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitF32S:           []float32{3.14},
		}.Build())
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_f32s")).Test(patch.Float32(3.14)))
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_f32s")).Test(patch.Float32(3.15)))
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))

	t.Run("a string holds, as it always did", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitStrings:        []string{"a<b"},
		}.Build())
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_strings")).Test(patch.Str("a<b")))
		x.NoError(err)
	}))

	t.Run("an int holds, as it always did", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitI64S:           []int64{1 << 40},
		}.Build())
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(1)).In(patch.Name("implicit_i64s")).Test(patch.Int64(1<<40)))
		x.Equal(codes.FailedPrecondition, status.Code(err), "there is no second element")

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_i64s")).Test(patch.Int64(1<<40)))
		x.NoError(err)
	}))
}

func TestApplyTestsAValueOfAMap(t *testing.T) {
	t.Run("a message holds against the protojson the row stores", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MapField().Add(ctx, pb.MapFieldAddRequest_builder{
			Id:           z.Ptr("m1"),
			ImplicitJson: map[string]*structpb.Struct{"j": jsonStruct(x)},
		}.Build())
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("j")).In(patch.Name("implicit_json")).Test(jsonStructValue("a<b")))
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("j")).In(patch.Name("implicit_json")).Test(jsonStructValue("other")))
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))

	t.Run("an enum holds against the number the row stores", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MapField().Add(ctx, pb.MapFieldAddRequest_builder{
			Id:           z.Ptr("m1"),
			ImplicitEnum: map[string]pb.Level{"lvl": pb.Level_LEVEL_HARD},
		}.Build())
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("lvl")).In(patch.Name("implicit_enum")).
				Test(patch.Enum(int32(pb.Level_LEVEL_HARD))))
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("lvl")).In(patch.Name("implicit_enum")).
				Test(patch.Enum(int32(pb.Level_LEVEL_EASY))))
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))
}

// TestApplyTestsWhatItJustWrote closes the loop the other way: the value the
// document put in the column through a partial write is the value a later
// document can name. Both spellings are encoding/json's, so this holds for the
// same reason the ones above do.
func TestApplyTestsWhatItJustWrote(t *testing.T) {
	t.Run("bytes written by a document", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitBytess:         [][]byte{{0x01}},
		}.Build())
		x.NoError(err)

		want := []byte{0xde, 0xad, 0xbe, 0xef}
		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_bytess")).Assign(patch.Bytes(want)))
		x.NoError(err)

		err = applyValueField(ctx, c, "v1",
			patch.Target(patch.Index(0)).In(patch.Name("implicit_bytess")).Test(patch.Bytes(want)))
		x.NoError(err)

		got, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("v1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal([][]byte{want}, got.GetImplicitBytess())
	}))

	t.Run("a message written by a document", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.MapField().Add(ctx, pb.MapFieldAddRequest_builder{
			Id: z.Ptr("m1"),
		}.Build())
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("j")).In(patch.Name("implicit_json")).Assign(jsonStructValue("a<b")))
		x.NoError(err)

		err = applyMapField(ctx, c, "m1",
			patch.Target(patch.MapStr("j")).In(patch.Name("implicit_json")).Test(jsonStructValue("a<b")))
		x.NoError(err)

		got, err := c.MapField().Get(ctx, pb.MapFieldGetById("m1").WithSelect(func(s *pb.MapFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal("a<b", got.GetImplicitJson()["j"].GetFields()["foo"].GetStringValue())
	}))
}
