package app

import (
	"fmt"
	"reflect"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/compiler/protogen"
)

var (
	identTime = protogen.GoImportPath("time").Ident("Time")

	pkgEnt   = protogen.GoImportPath("entgo.io/ent")
	pkgField = protogen.GoImportPath("entgo.io/ent/schema/field")
	pkgEdge  = protogen.GoImportPath("entgo.io/ent/schema/edge")
	pkgIndex = protogen.GoImportPath("entgo.io/ent/schema/index")

	pkgGoogleUuid = protogen.GoImportPath("github.com/google/uuid")
)

func (w *fileWork) goTypeScalar(t ormpb.Type) string {
	switch t {
	case ormpb.Type_TYPE_BOOL:
		return "bool"
	case ormpb.Type_TYPE_INT32:
		return "int32"
	case ormpb.Type_TYPE_SINT32:
		return "int32"
	case ormpb.Type_TYPE_UINT32:
		return "uint32"
	case ormpb.Type_TYPE_INT64:
		return "int64"
	case ormpb.Type_TYPE_SINT64:
		return "int64"
	case ormpb.Type_TYPE_UINT64:
		return "uint64"
	case ormpb.Type_TYPE_SFIXED32:
		return "int32"
	case ormpb.Type_TYPE_FIXED32:
		return "uint32"
	case ormpb.Type_TYPE_FLOAT:
		return "float32"
	case ormpb.Type_TYPE_SFIXED64:
		return "int64"
	case ormpb.Type_TYPE_FIXED64:
		return "uint64"
	case ormpb.Type_TYPE_DOUBLE:
		return "float64"
	case ormpb.Type_TYPE_STRING:
		return "string"
	case ormpb.Type_TYPE_BYTES:
		return "[]byte"
	case ormpb.Type_TYPE_UUID:
		return "[]byte"
	case ormpb.Type_TYPE_TIME:
		return w.QualifiedGoIdent(identTime)
	}

	panic(fmt.Sprintf("must be a scalar type: %v", t.String()))
}

func (w *fileWork) goType(t ormpb.Type, s graph.Shape) string {
	if t == ormpb.Type_TYPE_GROUP {
		panic("TODO")
	}
	if t.IsScalar() {
		return w.goTypeScalar(t)
	}

	switch s_ := s.(type) {
	case graph.ScalarShape:
		panic("it must not be a scalar")
	case graph.MapShape:
		t := w.goType(s_.V, s_.S)
		return fmt.Sprintf("map[%s]%s", w.goTypeScalar(s_.K), t)
	case graph.MessageShape:
		panic("not implemented")
	default:
		panic(fmt.Sprintf("unknown shape: %s", reflect.TypeOf(s).Name()))
	}
}

func (w *fileWork) GoType(f graph.Field) string {
	return w.goType(f.Type(), f.Shape())
}

func goTypeScalar(t ormpb.Type) string {
	switch t {
	case ormpb.Type_TYPE_BOOL:
		return "bool"
	case ormpb.Type_TYPE_INT32:
		return "int32"
	case ormpb.Type_TYPE_SINT32:
		return "int32"
	case ormpb.Type_TYPE_UINT32:
		return "uint32"
	case ormpb.Type_TYPE_INT64:
		return "int64"
	case ormpb.Type_TYPE_SINT64:
		return "int64"
	case ormpb.Type_TYPE_UINT64:
		return "uint64"
	case ormpb.Type_TYPE_SFIXED32:
		return "int32"
	case ormpb.Type_TYPE_FIXED32:
		return "uint32"
	case ormpb.Type_TYPE_FLOAT:
		return "float32"
	case ormpb.Type_TYPE_SFIXED64:
		return "int64"
	case ormpb.Type_TYPE_FIXED64:
		return "uint64"
	case ormpb.Type_TYPE_DOUBLE:
		return "float64"
	case ormpb.Type_TYPE_STRING:
		return "string"
	case ormpb.Type_TYPE_BYTES:
		return "[]byte"
	case ormpb.Type_TYPE_UUID:
		return "[]byte"
		// case ormpb.Type_TYPE_TIME:
		// 	return w.QualifiedGoIdent(identTime)
	}

	panic(fmt.Sprintf("must be a scalar type: %v", t.String()))
}

func entField(t ormpb.Type) protogen.GoIdent {
	id := ""
	switch t {
	case ormpb.Type_TYPE_BOOL:
		id = "Bool"
	case ormpb.Type_TYPE_ENUM:
		panic("not implemented")
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

	return pkgField.Ident(id)
}
