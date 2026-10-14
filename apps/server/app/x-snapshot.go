package app

import (
	"strings"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

// xSnapshots is independent of Get: a write-only resource still has recorders.
func (w *fileWork) xSnapshots() {
	name_x := w.Entity.Name()
	// Lock the row before reading the preimage so concurrent writers cannot
	// make the recorded Previous differ from the row this mutation replaced.
	w.P("func (s ", name_x, "ServiceServer) snapshot(ctx ", work.IdentContext,
		", p ", w.entPkg().Ident(name_x), ") (*", w.ent.Ident(name_x), ", error) {")
	w.P("	q := s.Db.", name_x, ".Query().Where(p)")
	for edge := range w.Entity.Edges() {
		target := edge.Target().Name()
		pkg := w.ent + "/" + protogen.GoImportPath(strings.ToLower(target))
		w.P("q.With", work.Name(edge.Name()).Ent(), "(func(q *", w.ent.Ident(target+"Query"), "){q.Select(", pkg.Ident("FieldId"), ")})")
	}
	w.P("	q.Modify(func(q *", work.PkgEntSQLBuilder.Ident("Selector"), ") {")
	w.P("		if s.Db.Dialect() != ", work.PkgEntDialect.Ident("SQLite"), " { q.ForUpdate() }")
	w.P("	})")
	w.P("	return q.Only(ctx)")
	w.P("}")

	w.P("func (s ", name_x, "ServiceServer) image(v *", w.Ident, ") *", w.Ident, " {")
	w.P("if s.Rec == nil || v == nil { return nil }")
	w.P("out := ", work.PkgProto.Ident("Clone"), "(v).(*", w.Ident, ")")
	if w.Entity.HasEdges() {
		w.P("m := out.ProtoReflect()")
		for edge := range w.Entity.Edges() {
			w.P("{ f := m.Descriptor().Fields().ByNumber(", edge.Number(), "); if m.Has(f) {")
			w.P("trim := func(r ", work.PkgProtoReflect.Ident("Message"), ") {r.Range(func(f ", work.PkgProtoReflect.Ident("FieldDescriptor"), ", _ ", work.PkgProtoReflect.Ident("Value"), ") bool {if f.Number()!=", edge.Target().Key().Number(), " {r.Clear(f)};return true})}")
			if edge.IsList() {
				w.P("vs:=m.Get(f).List();for i:=0;i<vs.Len();i++ {trim(vs.Get(i).Message())}")
			} else {
				w.P("trim(m.Get(f).Message())")
			}
			w.P("}}")
		}
	}
	w.P("return out")
	w.P("}")

}
