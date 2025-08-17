package work

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
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

	switch s := s.(type) {
	case graph.ScalarShape:
		panic("it must not be a scalar")
	case graph.MapShape:
		t := w.GoType(s.V, s.S)
		if _, ok := s.S.Descriptor.(protoreflect.MessageDescriptor); ok {
			t = "*" + t
		}
		return fmt.Sprintf("map[%s]%s", w.goTypeScalar(s.K), t)
	case graph.MessageShape:
		p := MustGetGoImportPath(s.Descriptor.ParentFile())
		return w.QualifiedGoIdent(p.Ident(string(s.FullName.Name())))
	default:
		panic(fmt.Sprintf("unknown shape: %s", reflect.TypeOf(s).Name()))
	}
}

func (w *FileWork) GoTypeOf(f graph.Field) string {
	return w.GoType(f.Type(), f.Shape())
}

func (w *FileWork) entTypeScalar(t ormpb.Type) string {
	switch t {
	case ormpb.Type_TYPE_UUID:
		return w.QualifiedGoIdent(IdentUuid)
	}

	return w.goTypeScalar(t)
}

func (w *FileWork) EntType(t ormpb.Type, s graph.Shape) string {
	if t == ormpb.Type_TYPE_GROUP {
		panic("TODO")
	}
	if t.IsScalar() {
		return w.entTypeScalar(t)
	}

	switch t {
	case ormpb.Type_TYPE_MESSAGE, ormpb.Type_TYPE_JSON:
		s, ok := s.(graph.MessageShape)
		if !ok {
			panic("field message or json type must have a message shape")
		}

		p, ok := w.Root.Imports[s.FullName]
		if !ok {
			panic(fmt.Sprintf("unknown message: %s", s.FullName))
		}

		return w.QualifiedGoIdent(p.Ident(string(s.FullName.Name())))
	}

	return w.GoType(t, s)
}

func (w *FileWork) EntTypeOf(f graph.Field) string {
	return w.EntType(f.Type(), f.Shape())
}

func GetGoImportPath(d protoreflect.FileDescriptor) (protogen.GoImportPath, bool) {
	opts := d.Options().(*descriptorpb.FileOptions)
	v := opts.GetGoPackage()
	if v == "" {
		return "", false
	}

	es := strings.SplitN(v, ";", 2)
	return protogen.GoImportPath(es[0]), true
}

func MustGetGoImportPath(d protoreflect.FileDescriptor) protogen.GoImportPath {
	v, ok := GetGoImportPath(d.ParentFile())
	if !ok {
		panic(fmt.Sprintf("Go import path for %s not found", d.FullName))
	}

	return v
}
