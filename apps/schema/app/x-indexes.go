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

		// Every part by its column, in the order the index declares them. An
		// index is read from its first column, so the order is what it is
		// for -- and ent's `Fields(...).Edges(...)` puts the edges' columns
		// after every field whatever order they were declared in, so that
		// an index declared `(tenant, created)` was `(created, tenant_id)`.
		// An edge's column is its key's field, the one [xEdgeFields] wrote.
		columns := []string{}
		for _, p := range props {
			switch p := p.(type) {
			case graph.Field:
				columns = append(columns, fmt.Sprintf("%q", p.Name()))
			case graph.Edge:
				if p.IsList() {
					panic(fmt.Sprintf("an index over %s, an edge to many, which has no column to index", p.FullName()))
				}
				columns = append(columns, fmt.Sprintf("%q", work.EdgeField(p.Name())))
			default:
				panic("unknown type of graph prop")
			}
		}
		w.Pf("		%s(%s)", index.Ident("Fields"), strings.Join(columns, ", "))
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
