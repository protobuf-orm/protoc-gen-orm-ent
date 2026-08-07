package app

import (
	"slices"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// xErasedDialect emits the refusal of an engine that cannot keep the promise a
// soft erasure makes.
//
// An entity that erases softly frees the names it held: its unique indexes
// cover only the rows that are still there, which a backend writes as a partial
// index. MySQL has none, and ent quietly writes the annotation out for it -- so
// the schema would come up with a plain unique index and everything would look
// right until somebody erased a row and could not use its alias again. That is
// a wrong answer with nothing to say it is wrong, which is the kind worth
// refusing at the start.
//
// It is a runtime check because it is the connection that says which engine
// this is, and the connection is not there when a schema is written. It sits
// beside the dialect check that is already here, for the same reason: what a
// server cannot do it should say before it serves, not at the first request
// that needs it.
//
// Nothing is emitted for a schema in which nothing erases softly.
func (w *Work) xErasedDialect() {
	vs := []graph.Entity{}
	for _, v := range w.Entities {
		if v.HasErasedField() {
			vs = append(vs, v)
		}
	}
	if len(vs) == 0 {
		return
	}

	names := make([]string, len(vs))
	for i, v := range vs {
		names[i] = v.Name()
	}
	slices.Sort(names)

	w.P("")
	w.P("	// ", join(names), " erase", plural(len(names)), " softly, and that is only true where a")
	w.P("	// unique index can be made partial. MySQL has no such thing, so the")
	w.P("	// index would come up covering every row and an alias freed by an")
	w.P("	// erasure would stay taken -- with nothing anywhere to say so.")
	w.P("	if d := db.Dialect(); d == ", work.PkgEntDialect.Ident("MySQL"), " {")
	w.P("		return Server{}, ", work.PkgFmt.Ident("Errorf"),
		"(\"%s cannot make a unique index partial, which is what an erased row needs to give up its name: %s\", d, ",
		quoteJoin(names), ")")
	w.P("	}")
}

func plural(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

func join(vs []string) string {
	switch len(vs) {
	case 1:
		return vs[0]
	case 2:
		return vs[0] + " and " + vs[1]
	}

	out := ""
	for i, v := range vs[:len(vs)-1] {
		if i > 0 {
			out += ", "
		}
		out += v
	}

	return out + " and " + vs[len(vs)-1]
}

func quoteJoin(vs []string) string {
	return `"` + join(vs) + `"`
}
