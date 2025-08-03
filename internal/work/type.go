package work

import (
	"fmt"
	"reflect"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
)

func (w *FileWork) goTypeScalar(t ormpb.Type) string {
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
		return w.QualifiedGoIdent(PkgTime.Ident("Time"))
	}

	panic(fmt.Sprintf("must be a scalar type: %v", t.String()))
}

func (w *FileWork) GoType(t ormpb.Type, s graph.Shape) string {
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
		t := w.GoType(s_.V, s_.S)
		return fmt.Sprintf("map[%s]%s", w.goTypeScalar(s_.K), t)
	case graph.MessageShape:
		panic("not implemented")
	default:
		panic(fmt.Sprintf("unknown shape: %s", reflect.TypeOf(s).Name()))
	}
}

func (w *FileWork) GoTypeOf(f graph.Field) string {
	return w.GoType(f.Type(), f.Shape())
}
