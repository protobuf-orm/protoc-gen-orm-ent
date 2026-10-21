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

	// What every index of this entity is stored as, so that two called the
	// same are refused here rather than by the database, which refuses the
	// second when a migration makes it. That is the ones a name is declared
	// for, and the ones ent names after their columns -- an index that declares
	// none, and the unique fields promoted to partial indexes below -- since a
	// declared `slug` and a unique field `slug` would both be `<table>_slug`.
	keys := map[string]string{}
	for _, f := range promotedUniques(w.Entity) {
		keys[work.IndexKey(w.Entity, f.Name())] = "the unique field " + f.Name()
	}

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
		columns, names := []string{}, []string{}
		for _, p := range props {
			switch p := p.(type) {
			case graph.Field:
				names = append(names, p.Name())
			case graph.Edge:
				if p.IsList() {
					panic(fmt.Sprintf("an index over %s, an edge to many, which has no column to index", p.FullName()))
				}
				names = append(names, work.EdgeField(p.Name()))
			default:
				panic("unknown type of graph prop")
			}
			columns = append(columns, fmt.Sprintf("%q", names[len(names)-1]))
		}
		w.Pf("		%s(%s)", index.Ident("Fields"), strings.Join(columns, ", "))

		// Under the name it declares, and not the one ent makes up out of its
		// columns. That name is the index: a migration finds the index by it,
		// so one whose columns change is the same index changed -- rebuilt
		// where it stands -- where a name made of the columns is a new index
		// beside the old one, which a migration never drops, and which goes on
		// being written on every insert and read by nothing.
		name, what := v.Name(), fmt.Sprintf("index %q", v.Name())
		if name == "" {
			// Nothing declared, so ent's name, which is what it always was.
			name, what = strings.Join(names, "_"), "an index of no name"
		}
		key := work.IndexKey(w.Entity, name)
		if was, ok := keys[key]; ok {
			panic(fmt.Sprintf("%s: %s and %s are both stored as %q, and a database holds one index of a name",
				w.Entity.FullName(), was, what, key))
		}
		keys[key] = what
		if v.Name() != "" {
			if len(key) > work.MaxIndexKey {
				panic(fmt.Sprintf("%s: %s is stored as %q, which is %d bytes, and PostgreSQL keeps the first %d of a name: two that begin alike would be one",
					w.Entity.FullName(), what, key, len(key), work.MaxIndexKey))
			}

			w.P(".")
			w.Pf("			StorageKey(%q)", key)
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
