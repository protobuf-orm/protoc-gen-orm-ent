package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xProto(w *work.FileWork) {
	w.P("func (e *", w.Ident.GoName, ") Proto() *", w.Ident, "{")
	w.P("	x := &", w.Ident, "{}")
	for p := range w.Entity.Props() {
		name := work.Name(p.Name())

		switch p := p.(type) {
		case graph.Field:
			v := "e." + name.Ent()

			is_nillable := p.IsNullable() && p != w.Entity.Key()
			if is_nillable {
				w.P("	if ", v, " != nil {")
				if p.Type() != ormpb.Type_TYPE_JSON {
					v = "*" + v
				}
			}
			if !p.IsList() {
				// Some types of the field defined in Ent are not same with the proto type.
				// However, repeated field is store in the DB with JSON type so the type of
				// the repeated fields is already aligned with the proto type.
				switch p.Type() {
				case ormpb.Type_TYPE_ENUM:
					v = fmt.Sprintf("%s(%s)", graph.GoTypeOf(p, w.QualifiedGoIdent), v)
				case ormpb.Type_TYPE_UUID:
					v = v + "[:]"
				case ormpb.Type_TYPE_TIME:
					v = fmt.Sprintf("%s(%s)", w.QualifiedGoIdent(work.PkgProtoTimestamp.Ident("New")), v)
				}
			}
			w.P("	x.Set", name.Go(), "(", v, ")")
			if is_nillable {
				w.P("	}")
			}

		case graph.Edge:
			w.P("	if v := e.Edges.", name.Ent(), "; v != nil {")
			w.P("		x.Set", name.Go(), "(v.Proto())")
			w.P("	}")
		default:
			panic("unknown type of graph prop")
		}
	}
	w.P("	return x")
	w.P("}")
}
