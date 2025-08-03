package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xEdges(w *work.FileWork) {
	edge := work.PkgEdge

	w.P("func (", w.Ident.GoName, ") Edges() []", work.PkgEnt.Ident("Edge"), " {")
	w.P("	return []", work.PkgEnt.Ident("Edge"), "{")
	for p := range w.Entity.Props() {
		p_, ok := p.(graph.Edge)
		if !ok {
			continue
		}

		name_edge := string(p.FullName().Name())
		name_target := string(p_.Target().FullName().Name())
		if inv := p_.Inverse(); inv == nil {
			w.Pf("		%s(%q, %s.Type)", edge.Ident("To"), name_edge, name_target)
		} else {
			name_inv := inv.FullName().Name()
			w.Pf("		%s(%q, %s.Type).Ref(%q)", edge.Ident("From"), name_edge, name_target, name_inv)
		}
		if p.IsUnique() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")
		}
		if !p.IsNullable() {
			w.P(".")
			fmt.Fprint(w, "			Required()")
		}
		if p.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
