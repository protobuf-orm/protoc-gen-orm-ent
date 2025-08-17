package app

import (
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

			t := p.Type()
			if !p.IsList() {
				// Note that repeated field is store in the DB with JSON type.
				// So the type is already aligned with the proto type.
				switch t {
				case ormpb.Type_TYPE_ENUM:
					if p.IsList() {

					}
					s := p.Shape().(graph.MessageShape)
					p := work.MustGetGoImportPath(s.Descriptor.ParentFile())
					v = w.QualifiedGoIdent(p.Ident(string(s.FullName.Name()))) + "(" + v + ")"
				case ormpb.Type_TYPE_UUID:
					v = v + "[:]"
				case ormpb.Type_TYPE_TIME:
					v = w.QualifiedGoIdent(work.PkgProtoTimestamp.Ident("New")) + "(" + v + ")"
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
