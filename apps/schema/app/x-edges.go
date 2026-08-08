package app

import (
	"fmt"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xEdges(w *work.FileWork) {
	edge := work.PkgEdge

	w.P("func (", w.Ident.GoName, ") Edges() []", work.PkgEnt.Ident("Edge"), " {")
	w.P("	return []", work.PkgEnt.Ident("Edge"), "{")
	for p := range w.Entity.Edges() {
		name_edge := p.Name()
		name_target := p.Target().Name()
		if inv := p.Inverse(); inv == nil {
			w.Pf("		%s(%q, %s.Type)", edge.Ident("To"), name_edge, name_target)
		} else {
			name_inv := inv.FullName().Name()
			w.Pf("		%s(%q, %s.Type).Ref(%q)", edge.Ident("From"), name_edge, name_target, name_inv)
		}
		if !p.IsList() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")

			// Bound to the field [xEdgeFields] wrote, which is what makes the
			// key readable at all. Without it ent keeps the same column in an
			// unexported member, and `Proto()` can then only answer with an
			// edge somebody eagerly loaded.
			w.P(".")
			fmt.Fprintf(w, "			Field(%q)", work.EdgeField(name_edge))
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
