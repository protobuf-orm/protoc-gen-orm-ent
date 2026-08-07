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
				w.Pf("		%s(%s)", index.Ident("Edges"), strings.Join(edges, ", "))
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
		if v.ExcludesErased() {
			// A partial index: the rows that carry an erasure stamp are not in
			// it, so they hold nothing unique and the name they had can be used
			// again. Without this a soft erasure would free a row from every
			// read and go on occupying its alias forever, which is "gone" in
			// no sense anybody means.
			//
			// The predicate is written the way the engine stores it, because
			// atlas diffs the two and a mismatch is a migration it plans again
			// on every run. `<column> IS NULL` is already in normal form in
			// both PostgreSQL and SQLite; nothing more complicated is possible
			// here, since an erased field is a nullable time and the question
			// is always the same one.
			//
			// MySQL has no partial indexes at all and ent writes the annotation
			// out for the dialects that do, so a MySQL deployment would get a
			// plain unique index and the wrong behaviour with nothing said.
			// The generated store refuses that dialect; see the store app.
			del := w.Entity.GetErasedField()
			w.P(".")
			w.Pf("			Annotations(%s(%q))",
				w.QualifiedGoIdent(work.PkgEntSql.Ident("IndexWhere")),
				del.Name()+" IS NULL")
		}
		w.P(",")
	}

	// And the fields that said `unique` on themselves, for an entity that
	// erases softly. They are here rather than on the field because only an
	// index can be partial; see promotedUniques.
	if del := w.Entity.GetErasedField(); del != nil {
		for _, f := range promotedUniques(w.Entity) {
			w.Pf("		%s(%q)", index.Ident("Fields"), f.Name())
			w.P(".")
			w.Pf("			Unique()")
			w.P(".")
			w.Pf("			Annotations(%s(%q))",
				w.QualifiedGoIdent(work.PkgEntSql.Ident("IndexWhere")),
				del.Name()+" IS NULL")
			w.P(",")
		}
	}

	w.P("	}")
	w.P("}")
	w.P("")
}
