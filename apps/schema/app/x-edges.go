package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
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
		if v := onDeleteOf(p); v != "" {
			w.P(".")
			fmt.Fprintf(w, "			Annotations(%s(%s))",
				w.QualifiedGoIdent(work.PkgEntSql.Ident("OnDelete")),
				w.QualifiedGoIdent(work.PkgEntSql.Ident(v)))
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}

// onDeleteOf is the entsql action for the foreign key of the ent edge `p` is
// written as, or "" where the schema says nothing, so that the fork's own
// default holds: SET NULL for a nullable column and NO ACTION otherwise.
//
// protobuf-orm says it on the edge that holds the key, and ent reads it from
// the edge written `To` -- the one that is not an inverse -- whichever table
// the key is in. The two are one edge when the key holder is written `To`:
// a single edge with no inverse. When it is written `From` -- the single side
// of a many-to-one declared from its list, or the side of a one-to-one
// written from the other -- ent reads the action from the edge it is written
// from, the Reverse of the one written `To` here. An edge written `From`
// carries nothing: ent would not read it there.
func onDeleteOf(p graph.Edge) string {
	if p.Inverse() != nil {
		return ""
	}
	v := p.OnDelete()
	if v == ormpb.OnDelete_ON_DELETE_UNSPECIFIED {
		if rev := p.Reverse(); rev != nil && !rev.IsList() {
			v = rev.OnDelete()
		}
	}
	switch v {
	case ormpb.OnDelete_ON_DELETE_RESTRICT:
		return "Restrict"
	case ormpb.OnDelete_ON_DELETE_CASCADE:
		return "Cascade"
	case ormpb.OnDelete_ON_DELETE_SET_NULL:
		return "SetNull"
	case ormpb.OnDelete_ON_DELETE_UNSPECIFIED:
		return ""
	default:
		panic(fmt.Sprintf("%s: unknown on_delete %s", p.FullName(), v))
	}
}
