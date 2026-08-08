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
				switch p.Type() {
				case ormpb.Type_TYPE_UUID:
					// uuid.UUID is an array not a slice so `v[:]` just work without deref.
				case ormpb.Type_TYPE_JSON:

				default:
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

			// And when nobody loaded it, the key -- which this row is holding
			// either way, in the column the edge is kept in.
			//
			// It answers with a **reference**: the neighbour's identifier and
			// nothing else. That is not a half-loaded row pretending to be one.
			// It is the only thing this row actually knows about its neighbour,
			// and it is what a client that keeps its own copy of both needs in
			// order to join them without asking again.
			//
			// A list edge has no key here -- it is kept on the other table --
			// so there is nothing to answer with and the loaded case is all
			// there is.
			if !p.IsList() {
				to := p.Target()
				key := to.Key()

				// `*new(T)` rather than a literal, because the key's type is
				// whatever the target declared and only its zero value is the
				// same question in every one of them.
				zero := fmt.Sprintf("*new(%s)", graph.GoTypeOf(key, w.QualifiedGoIdent))

				v := "v"
				if key.Type() == ormpb.Type_TYPE_UUID {
					v = "v[:]"
				}

				w.P("	} else if v := e.", work.EdgeFieldGo(p.Name()), "; v != ", zero, " {")
				w.P("		r := &", w.Root.Ident(to), "{}")
				w.P("		r.Set", work.Name(key.Name()).Go(), "(", v, ")")
				w.P("		x.Set", name.Go(), "(r)")
			}

			w.P("	}")
		default:
			panic("unknown type of graph prop")
		}
	}
	w.P("	return x")
	w.P("}")
}
