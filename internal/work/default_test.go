package work_test

import (
	"testing"

	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// TestADefaultSaysAValueOrIsRefused is what `(orm.field).default` may say: the
// value as its type reads it, empty for the zero value, and nothing that would
// be read as the zero value without saying so.
func TestADefaultSaysAValueOrIsRefused(t *testing.T) {
	enum := structpb.NullValue_NULL_VALUE.Descriptor()

	for _, tc := range []struct {
		t    ormpb.Type
		v    string
		want string
	}{
		{ormpb.Type_TYPE_STRING, "", `""`},
		{ormpb.Type_TYPE_STRING, `say "hi"`, `"say \"hi\""`},
		{ormpb.Type_TYPE_BOOL, "", "false"},
		{ormpb.Type_TYPE_BOOL, "true", "true"},
		{ormpb.Type_TYPE_INT32, "", "0"},
		{ormpb.Type_TYPE_INT32, "-7", "-7"},
		{ormpb.Type_TYPE_SFIXED64, "9223372036854775807", "9223372036854775807"},
		{ormpb.Type_TYPE_UINT32, "4294967295", "4294967295"},
		{ormpb.Type_TYPE_DOUBLE, "0.5", "0.5"},
		{ormpb.Type_TYPE_FLOAT, "", "0"},
		{ormpb.Type_TYPE_ENUM, "", "0"},
		{ormpb.Type_TYPE_ENUM, "NULL_VALUE", "0"},
		{ormpb.Type_TYPE_ENUM, "3", "3"},
		{ormpb.Type_TYPE_BYTES, "", "[]byte{}"},
	} {
		got, ok, err := work.ParseDefault(tc.t, enum, tc.v)
		require.NoError(t, err, "%s %q", tc.t, tc.v)
		require.True(t, ok, "%s %q", tc.t, tc.v)
		require.Equal(t, tc.want, got, "%s %q", tc.t, tc.v)
	}

	// A default that is made rather than written says nothing, and is no value.
	for _, typ := range []ormpb.Type{ormpb.Type_TYPE_UUID, ormpb.Type_TYPE_TIME, ormpb.Type_TYPE_MESSAGE, ormpb.Type_TYPE_JSON} {
		_, ok, err := work.ParseDefault(typ, nil, "")
		require.NoError(t, err, "%s", typ)
		require.False(t, ok, "%s", typ)
	}

	for _, tc := range []struct {
		t ormpb.Type
		v string
	}{
		{ormpb.Type_TYPE_BOOL, "yes"},
		{ormpb.Type_TYPE_INT32, "2147483648"},
		{ormpb.Type_TYPE_INT64, "seven"},
		{ormpb.Type_TYPE_UINT32, "-1"},
		{ormpb.Type_TYPE_DOUBLE, "inf"},
		{ormpb.Type_TYPE_DOUBLE, "NaN"},
		{ormpb.Type_TYPE_ENUM, "NOPE"},
		{ormpb.Type_TYPE_BYTES, "AQ=="},
		{ormpb.Type_TYPE_UUID, "00000000-0000-0000-0000-000000000000"},
		{ormpb.Type_TYPE_TIME, "now"},
	} {
		_, _, err := work.ParseDefault(tc.t, enum, tc.v)
		require.Error(t, err, "%s %q was read as something it did not say", tc.t, tc.v)
	}
}
