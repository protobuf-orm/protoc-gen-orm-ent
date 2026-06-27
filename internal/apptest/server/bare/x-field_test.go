package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// These tests exercise the part the question is really about: how the generated
// server marshals proto scalar/enum/json/map/repeated values into ent and back.

func TestValueFieldRoundTrip(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		req := pb.ValueFieldAddRequest_builder{
			Id:             z.Ptr("v1"),
			ImplicitI32:    42,
			ImplicitI64:    1 << 40,
			ImplicitU32:    7,
			ImplicitF64:    3.5,
			ImplicitBool:   true,
			ImplicitString: "hello",
			ImplicitBytes:  []byte{0xde, 0xad, 0xbe, 0xef},
			// implicit_immutable_bytes is also required (NOT NULL, no default):
			// an omitted bytes field is inserted as NULL and fails. See the
			// dedicated TestRequiredBytesOmitted below.
			ImplicitImmutableBytes: []byte{0x01},
			ImplicitEnum:           pb.Level_LEVEL_NORM,
			ExplicitI32:            z.Ptr(int32(-7)),
			ImplicitI32S:           []int32{1, 2, 3},
			ImplicitStrings:        []string{"a", "b"},
			ImplicitLevels:         []pb.Level{pb.Level_LEVEL_EASY, pb.Level_LEVEL_HARD},
			NullableString:         z.Ptr("set"),
		}.Build()
		_, err := c.ValueField().Add(ctx, req)
		x.NoError(err)

		got, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("v1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.EqualValues(42, got.GetImplicitI32())
		x.EqualValues(1<<40, got.GetImplicitI64())
		x.EqualValues(7, got.GetImplicitU32())
		x.InDelta(3.5, got.GetImplicitF64(), 1e-9)
		x.True(got.GetImplicitBool())
		x.Equal("hello", got.GetImplicitString())
		x.Equal([]byte{0xde, 0xad, 0xbe, 0xef}, got.GetImplicitBytes())
		x.Equal(pb.Level_LEVEL_NORM, got.GetImplicitEnum()) // enum stored as int32 in ent, restored to the proto enum
		x.EqualValues(-7, got.GetExplicitI32())
		x.Equal([]int32{1, 2, 3}, got.GetImplicitI32S())
		x.Equal([]string{"a", "b"}, got.GetImplicitStrings())
		x.Equal([]pb.Level{pb.Level_LEVEL_EASY, pb.Level_LEVEL_HARD}, got.GetImplicitLevels())
		x.Equal("set", got.GetNullableString())
		x.Equal([]byte{0x01}, got.GetImplicitImmutableBytes())
	})(t)
}

// FINDING: a required (non-nullable, no-default) bytes field that the client
// omits is sent to ent as a nil []byte, which becomes a SQL NULL and trips the
// NOT NULL constraint. Unlike int/bool/string (whose zero values are non-NULL),
// only bytes has this problem. The failure leaks as codes.Unknown; ideally the
// generated Add would either coerce nil to empty bytes or reject with
// InvalidArgument. This test pins the current behavior.
func TestRequiredBytesOmitted(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		// implicit_bytes / implicit_immutable_bytes are required but omitted.
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{Id: z.Ptr("rb")}.Build())
		x.Error(err)
	})(t)
}

func TestMapFieldRoundTrip(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		json, err := structpb.NewStruct(map[string]any{"n": 1.0})
		x.NoError(err)
		req := pb.MapFieldAddRequest_builder{
			Id:             z.Ptr("m1"),
			ImplicitString: map[string]string{"k": "v"},
			ImplicitEnum:   map[string]pb.Level{"lvl": pb.Level_LEVEL_HARD},
			ImplicitJson:   map[string]*structpb.Struct{"j": json},
		}.Build()
		_, err = c.MapField().Add(ctx, req)
		x.NoError(err)

		got, err := c.MapField().Get(ctx, pb.MapFieldGetById("m1").WithSelect(func(s *pb.MapFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal(map[string]string{"k": "v"}, got.GetImplicitString())
		x.Equal(pb.Level_LEVEL_HARD, got.GetImplicitEnum()["lvl"])
		x.InDelta(1.0, got.GetImplicitJson()["j"].GetFields()["n"].GetNumberValue(), 1e-9)
	})(t)
}

func TestMessageFieldRoundTrip(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		json, err := structpb.NewStruct(map[string]any{"foo": "bar"})
		x.NoError(err)
		req := pb.MessageFieldAddRequest_builder{
			Id:       z.Ptr("g1"),
			Explicit: json,
		}.Build()
		_, err = c.MessageField().Add(ctx, req)
		x.NoError(err)

		got, err := c.MessageField().Get(ctx, pb.MessageFieldGetById("g1").WithSelect(func(s *pb.MessageFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal("bar", got.GetExplicit().GetFields()["foo"].GetStringValue())
	})(t)
}

func TestSelectScalarMask(t *testing.T) {
	T(func(ctx context.Context, x *require.Assertions, c *Client) {
		req := pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("s1"),
			ImplicitString:         "keep",
			ImplicitI32:            99,
			ImplicitBytes:          []byte{0x01},
			ImplicitImmutableBytes: []byte{0x02},
		}.Build()
		_, err := c.ValueField().Add(ctx, req)
		x.NoError(err)

		got, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("s1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetImplicitString(true)
		}))
		x.NoError(err)
		x.Equal("keep", got.GetImplicitString())
		x.EqualValues(0, got.GetImplicitI32()) // not selected -> zero value
	})(t)
}
