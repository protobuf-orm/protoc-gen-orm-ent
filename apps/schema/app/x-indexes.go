package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xIndexes(w *work.FileWork) {
	index := work.PkgIndex

	w.P("func (", w.Ident.GoName, ")", "Indexes() []", work.PkgEnt.Ident("Index"), " {")
	w.P("	return []", work.PkgEnt.Ident("Index"), "{")
	for v := range w.Entity.Indexes() {
		props := slices.Collect(v.Props())
		if len(props) == 0 {
			continue
		}

		fields := []string{}
		edges := []string{}
		for _, p := range props {
			name := fmt.Sprintf("%q", p.Name())
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
			w.Pf("		%s(%s)", index.Ident("Fields"), strings.Join(fields, ", "))
		}
		if len(edges) > 0 {
			if len(fields) > 0 {
				w.P(".")
				w.Pf("			Edges(%s)", strings.Join(edges, ","))
			} else {
				w.Pf("		%s(%s)", index.Ident("Edges"), strings.Join(fields, ", "))
			}
		}
		if v.IsUnique() {
			w.P(".")
			w.Pf("			Unique()")
		}
		if v.IsImmutable() {
			w.P(".")
			w.Pf("			Immutable()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
