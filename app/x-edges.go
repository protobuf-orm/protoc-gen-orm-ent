package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
)

func (w *fileWork) xEdges() {
	name := string(w.entity.FullName().Name())

	w.P("func (", name, ")", "Edges() []", pkgEnt.Ident("Edge"), " {")
	w.P("	return []", pkgEnt.Ident("Edge"), "{")
	for p := range w.entity.Props() {
		p_, ok := p.(graph.Edge)
		if !ok {
			continue
		}

		name_edge := string(p.FullName().Name())
		name_target := string(p_.Target().FullName().Name())
		if inv := p_.Inverse(); inv == nil {
			fmt.Fprintf(w, "		%s(%q, %s.Type)",
				w.QualifiedGoIdent(pkgEdge.Ident("To")), name_edge, name_target)
		} else {
			name_inv := inv.FullName().Name()
			fmt.Fprintf(w, "		%s(%q, %s.Type).Ref(%q)",
				w.QualifiedGoIdent(pkgEdge.Ident("From")), name_edge, name_target, name_inv)
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
