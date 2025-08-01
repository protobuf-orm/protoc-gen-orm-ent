package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
)

func (w *fileWork) xIndexes() {
	name := string(w.entity.FullName().Name())

	w.P("func (", name, ")", "Indexes() []", pkgEnt.Ident("Index"), " {")
	w.P("	return []", pkgEnt.Ident("Index"), "{")
	for v := range w.entity.Indexes() {
		props := slices.Collect(v.Props())
		if len(props) == 0 {
			continue
		}

		fields := []string{}
		edges := []string{}
		for _, p := range props {
			name := fmt.Sprintf("%q", string(p.FullName().Name()))
			switch p.(type) {
			case graph.Field:
				fields = append(fields, name)
			case graph.Edge:
				edges = append(edges, name)
			default:
				panic("unknown type of graph prop")
			}
		}
		if len(fields) > 0 {
			fmt.Fprintf(w, "		%s(%s)",
				w.QualifiedGoIdent(pkgIndex.Ident("Fields")),
				strings.Join(fields, ", "),
			)
		}
		if len(edges) > 0 {
			if len(fields) > 0 {
				w.P(".")
				fmt.Fprint(w, "			Edges(", strings.Join(edges, ","), ")")
			} else {
				fmt.Fprintf(w, "		%s(%s)",
					w.QualifiedGoIdent(pkgIndex.Ident("Edges")),
					strings.Join(fields, ", "),
				)
			}
		}
		if v.IsUnique() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")
		}
		if v.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
