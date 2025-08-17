package app

import (
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func entField(t ormpb.Type) protogen.GoIdent {
	id := ""
	switch t {
	case ormpb.Type_TYPE_BOOL:
		id = "Bool"
	case ormpb.Type_TYPE_ENUM:
		// See https://protobuf.dev/programming-guides/editions/#enum
		// Enumerator constants must be in the range of a 32-bit integer.
		id = "Int32"
	case ormpb.Type_TYPE_INT32,
		ormpb.Type_TYPE_SINT32,
		ormpb.Type_TYPE_SFIXED32:
		id = "Int32"
	case ormpb.Type_TYPE_UINT32,
		ormpb.Type_TYPE_FIXED32:
		id = "Uint32"
	case ormpb.Type_TYPE_INT64,
		ormpb.Type_TYPE_SINT64,
		ormpb.Type_TYPE_SFIXED64:
		id = "Int64"
	case ormpb.Type_TYPE_UINT64,
		ormpb.Type_TYPE_FIXED64:
		id = "Uint64"
	case ormpb.Type_TYPE_FLOAT:
		id = "Float32"
	case ormpb.Type_TYPE_DOUBLE:
		id = "Float"
	case ormpb.Type_TYPE_STRING:
		id = "String"
	case ormpb.Type_TYPE_BYTES:
		id = "Bytes"
	case ormpb.Type_TYPE_MESSAGE:
		id = "JSON"
	case ormpb.Type_TYPE_GROUP:
		panic("not implemented")
	case ormpb.Type_TYPE_UUID:
		id = "UUID"
	case ormpb.Type_TYPE_TIME:
		id = "Time"
	case ormpb.Type_TYPE_JSON:
		id = "JSON"
	}

	return work.PkgField.Ident(id)
}
