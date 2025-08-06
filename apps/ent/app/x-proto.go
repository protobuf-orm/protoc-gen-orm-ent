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

		switch p_ := p.(type) {
		case graph.Field:
			v := "e." + name.Ent()
			if p_.IsNullable() {
				w.P("	if ", v, " != nil {")
				v = "*" + v
			}

			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				v = v + "[:]"
			case ormpb.Type_TYPE_TIME:
				v = w.QualifiedGoIdent(work.PkgProtoTimestamp.Ident("New")) + "(" + v + ")"
			}
			w.P("	x.Set", name.Go(), "(", v, ")")
			if p_.IsNullable() {
				w.P("	}")
			}

		case graph.Edge:
			w.P("	if v := e.Edges.", name.Ent(), "; v != nil {")
			w.P("		x.Set", name.Ent(), "(v.Proto())")
			w.P("	}")
		default:
			panic("unknown type of graph prop")
		}
	}
	w.P("	return x")
	w.P("}")
}
