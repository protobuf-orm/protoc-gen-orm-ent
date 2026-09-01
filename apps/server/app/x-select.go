package app

import (
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"

	"github.com/protobuf-orm/ent/entc/gen"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xSelectKey() {
	name_x := w.Entity.Name()

	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))
	w.P("func select", name_x, "Key(",
		/* */ "q *", w.ent.Ident(name_x+"Query"),
		") {")
	w.P("	q.Select(", x.Ident("Field"+gen.Pascal(w.Entity.Key().Name())), ")")
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
		w.emitSelectLive(p)
		w.P("			", name_y, "Select(q, m.Get", name_p, "())")
		w.P("		})")
		w.P("	}")
	}
	w.P("}")
	w.P("")
}

// emitSelectLive keeps an erased row out of an edge a select asks for.
//
// # The same hole as [fileWork.xSelectKey]'s, one door along
//
// `<Entity>Pick` answers among the rows that are still here, because a
// reference to a row is composed into the reference of whatever names one and
// no narrowing of the parent is applied there. A **select** is the other way to
// reach a parent, and it went through nothing at all: the edge is loaded with
// the target's own `Select` and no predicate, so the parent of any row a caller
// may read came back whole, whatever state it was in.
//
// What that costs is an erased person read entire through a row that outlived
// them. Nothing cascades on an erase, deliberately -- so their address and
// their external identity survive them -- and asking for `select.holder.all` on
// the way past a child answered their alias, their name, their profile, their
// provider subject: everything the entity's own `Get` answers NotFound to, for
// the same caller, one call later.
//
// It is the parent's liveness and not the caller's scope. A wall narrows the
// child's path to a tenant and has nothing to say about whether the row at the
// other end of an edge is still there, which is why this is here rather than in
// whatever composed the query.
func (w *fileWork) emitSelectLive(p graph.Edge) {
	y := p.Target()

	del := y.GetErasedField()
	if del == nil {
		// An entity erased hard leaves no row, so there is nothing for an edge
		// to reach that a read should not see.
		return
	}

	pkg := w.ent + "/" + protogen.GoImportPath(strings.ToLower(y.Name()))
	w.P("			q.Where(", pkg.Ident(work.Name(del.Name()).Ent()+"IsNil"), "())")
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
