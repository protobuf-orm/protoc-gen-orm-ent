package app

import (
	"strings"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xSelectKey() {
	name_x := w.Entity.Name()

	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))
	w.P("func select", name_x, "Key(",
		/* */ "q *", w.ent.Ident(name_x+"Query"),
		") {")
	w.P("	q.Select(", x.Ident("Field"+ent.Pascal(w.Entity.Key().Name())), ")")
	w.P("}")
	w.P("")
}

func (w *fileWork) xSelectedFields() {
	name_x := w.Entity.Name()

	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))
	w.P("func ", name_x, "SelectedFields(",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") []string {")
	w.P("	if m.GetAll() {")
	w.P("		return ", x.Ident("Columns"))
	w.P("	}")
	w.P("")
	w.P("	vs := make([]string, 0, len(", x.Ident("Columns"), "))")
	for p := range w.Entity.Fields() {
		name_p := work.Name(p.Name())
		if p != w.Entity.Key() {
			w.P("	if m.Get", name_p.Go(), "() {")
		} else {
			w.P("	{")
		}
		w.P("		vs = append(vs, ", x.Ident("Field"+name_p.Ent()), ")")
		w.P("	}")
	}
	w.P("")
	w.P("	return vs")
	w.P("}")
	w.P("")
}

func (w *fileWork) xSelect() {
	name_x := w.Entity.Name()
	w.P("func ", name_x, "Select(",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") {")
	w.P("	if !m.GetAll() {")
	w.P("		fields := ", name_x, "SelectedFields(m)")
	w.P("		q.Select(fields...)")
	w.P("	}")
	for p := range w.Entity.Edges() {
		name_p := work.Name(p.Name()).Ent()
		name_y := p.Target().Name()
		w.P("	if m.Has", name_p, "() {")
		w.P("		q.With", name_p, "(func(q *", w.ent.Ident(name_y+"Query"), ") {")
		w.P("			", name_y, "Select(q, m.Get", name_p, "())")
		w.P("		})")
		w.P("	}")
	}
	w.P("}")
	w.P("")
}

func (w *fileWork) xSelectInit() {
	name_x := w.Entity.Name()
	w.P("func ", name_x, "SelectInit(",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") {")
	w.P("	if m != nil {")
	w.P("		", name_x, "Select(q, m)")
	w.P("	} else {")
	for p := range w.Entity.Edges() {
		name_p := work.Name(p.Name()).Ent()
		name_y := p.Target().Name()
		w.P("		q.With", name_p, "(select", name_y, "Key)")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
