package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func xFields(w *work.FileWork) {
	w.P("func (", w.Ident.GoName, ") Fields() []", work.PkgEnt.Ident("Field"), " {")
	w.P("	return []", work.PkgEnt.Ident("Field"), "{")
	for p := range w.Entity.Fields() {
		id := "" // Name of builder
		ctor := ""

		t := p.Type()
		switch p.Type() {
		case ormpb.Type_TYPE_MESSAGE:
			panic("field cannot be typed as message")
		case ormpb.Type_TYPE_JSON:
			id = "JSON"
			ctor = graph.GoTypeOf(p, func(v protogen.GoIdent) string {
				ident := w.QualifiedGoIdent(v)

				d := p.Descriptor()
				if !d.IsMap() || d.MapValue().Kind() == protoreflect.EnumKind {
					return ident
				}
				return "*" + ident

			})
			if p.Descriptor().IsMap() {
				ctor += "{}"
			} else if p.IsList() {
				ctor = "*" + ctor
			} else {
				ctor = "&" + ctor + "{}"
			}
		case ormpb.Type_TYPE_UUID:
			ctor = graph.GoTypeOf(p, w.QualifiedGoIdent) + "{}"
		}

		if p.IsList() {
			id = "JSON"
			if ctor == "" {
				ctor = graph.GoTypeOf(p, w.QualifiedGoIdent)
			}
			ctor = "[]" + ctor + "{}"
		} else {
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
			// case ormpb.Type_TYPE_MESSAGE:
			case ormpb.Type_TYPE_GROUP:
				panic("not implemented")
			case ormpb.Type_TYPE_UUID:
				id = "UUID"
			case ormpb.Type_TYPE_TIME:
				id = "Time"
				// case ormpb.Type_TYPE_JSON:
				// 	builder = "JSON"
			}
		}

		name := p.Name()
		builder := w.QualifiedGoIdent(work.PkgField.Ident(id))
		if ctor == "" {
			fmt.Fprintf(w, "		%s(%q)", builder, name)
		} else {
			fmt.Fprintf(w, "		%s(%q, %s)", builder, name, ctor)
		}

		is_key := p == w.Entity.Key()
		if p.IsUnique() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")
		}
		if p.IsNullable() && !is_key && t != ormpb.Type_TYPE_JSON {
			w.P(".")
			fmt.Fprint(w, "			Nillable()")
		}
		if p.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		if p.IsOptional() && !is_key {
			w.P(".")
			fmt.Fprint(w, "			Optional()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
