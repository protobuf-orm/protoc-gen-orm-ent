package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldBuilder is the ent field builder for one declared field: the name of the
// `field.X` constructor, and the argument it takes when it takes one.
//
// Lifted out of the loop below because an edge needs it too -- the foreign key
// this table holds for an edge is a field of the **target's key** type, and
// there is one right answer per type rather than two.
func fieldBuilder(w *work.FileWork, p graph.Field) (string, string) {
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

	return id, ctor
}

func xFields(w *work.FileWork) {
	w.P("func (", w.Ident.GoName, ") Fields() []", work.PkgEnt.Ident("Field"), " {")
	w.P("	return []", work.PkgEnt.Ident("Field"), "{")
	for p := range w.Entity.Fields() {
		id, ctor := fieldBuilder(w, p)
		t := p.Type()

		name := p.Name()
		builder := w.QualifiedGoIdent(work.PkgField.Ident(id))
		if ctor == "" {
			fmt.Fprintf(w, "		%s(%q)", builder, name)
		} else {
			fmt.Fprintf(w, "		%s(%q, %s)", builder, name, ctor)
		}

		is_key := p == w.Entity.Key()
		// Not for a field whose uniqueness had to become a partial index; see
		// promotedUniques. Written here as well it would be a second, total
		// constraint over the same column, and the total one is the one that
		// would refuse the alias of an erased row.
		if p.IsUnique() && !isPromotedUnique(w.Entity, p) {
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

	xEdgeFields(w)

	w.P("	}")
	w.P("}")
	w.P("")
}

// xEdgeFields writes the foreign key of every edge this table holds one for,
// as a field of its own.
//
// ent keeps that column either way -- a unique edge puts the key on this table
// and reads it back into the row -- but without a field bound to it, the value
// lands in an **unexported** struct member. It is there, in memory, already
// paid for, and nothing outside the package can read it. So `Proto()` could
// only answer with an edge that had been eagerly loaded, and a `List`, which
// loads no edges, answered rows with `tenant: null`: not the tenant, and not
// even which tenant.
//
// That is what a client-side normalized store cannot work without. It does not
// need the tenant; it needs to know **which** tenant, so it can hold the two
// rows separately and join them itself. Without the reference it has to fetch
// the row again to learn who its neighbour is, once per row.
//
// The field costs nothing to fill: it is the same column, already selected.
func xEdgeFields(w *work.FileWork) {
	for p := range w.Entity.Edges() {
		// The unique side is the one that holds the key. A list edge is kept
		// on the other table, and a field here would name a column that is
		// not.
		if p.IsList() {
			continue
		}

		name := work.EdgeField(p.Name())
		for f := range w.Entity.Fields() {
			if f.Name() == name {
				panic(fmt.Sprintf(
					"%s: an edge named %q needs the field %q for its key, and this entity declares one",
					w.Entity.FullName(), p.Name(), name))
			}
		}

		key := p.Target().Key()
		id, ctor := fieldBuilder(w, key)

		builder := w.QualifiedGoIdent(work.PkgField.Ident(id))
		if ctor == "" {
			fmt.Fprintf(w, "		%s(%q)", builder, name)
		} else {
			fmt.Fprintf(w, "		%s(%q, %s)", builder, name, ctor)
		}

		// The edge's own nullability, because they are the same fact said
		// twice: an edge that may be absent is a key that may be NULL, and ent
		// refuses a required edge bound to an optional field.
		if p.IsNullable() {
			w.P(".")
			fmt.Fprint(w, "			Optional()")
		}
		if p.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		w.P(",")
	}
}
