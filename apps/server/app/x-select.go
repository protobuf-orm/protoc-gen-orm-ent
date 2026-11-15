package app

import (
	"strings"

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

// xSelect emits what a select asks for of a row: its columns, and each edge it
// names read through the Narrow of the entity at the other end.
//
// # Why it takes a scope
//
// `<Entity>Narrow` is "the one place a read of this entity is narrowed, so
// that every query narrows the same way and none of them can be the one that
// forgot" -- and a select through an edge was one that forgot. The row a Get
// names is narrowed; the row at the other end of an edge it asks for was loaded
// with the target's `Select` and no predicate but liveness. So a caller a wall
// keeps from a row read it whole, by naming any row they may read that points
// at it: `select: {holder: {all: true}}` on a child, and the parent of another
// tenant came back with it. A reference to a row is a column and is answered
// either way; the row is what the scope was for.
//
// It took a query and a select, and there was nothing to narrow by. It takes
// the context and the scope now, and there is no form of it without them: a
// read with no scope says so, by passing nil.
//
// # What it answers for an edge out of scope
//
// The edge is not loaded, which `Proto()` answers as it answers an edge nobody
// asked for: the reference, and nothing of the row. That is what the caller
// holds already by reading the child, so it tells them nothing more -- and it
// is not an error, for the reason a Get out of scope is NotFound and not a
// refusal.
//
// The scope is asked once for each edge a select names and again for each edge
// under it, with the context of the read that began it: an edge is read by the
// rule of what it reaches, as the caller who asked. That predicate carries the
// target's liveness with it, so an erased row is kept out of an edge as it is
// kept out of a Get.
//
// # Why it answers with an error
//
// A scope may fail -- it reads who is asking, and that can be a database that
// would not answer. Loaded anyway, the edge would be the row the scope was
// about to keep back.
func (w *fileWork) xSelect() {
	name_x := w.Entity.Name()
	w.P("// ", name_x, "Select loads what `m` asks for, each edge through the Narrow")
	w.P("// of the entity it reaches: an edge whose target `scope` keeps from this")
	w.P("// caller is answered as its reference alone, as an edge nobody asked for")
	w.P("// is. A nil scope narrows by nothing but whether the row is still there.")
	w.P("func ", name_x, "Select(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "scope Scope, ",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") error {")
	w.P("	if !m.GetAll() {")
	w.P("		fields := ", name_x, "SelectedFields(m)")
	w.P("		q.Select(fields...)")
	w.P("	}")
	for p := range w.Entity.Edges() {
		name_p := work.Name(p.Name()).Ent()
		name_y := p.Target().Name()
		w.P("	if m.Has", name_p, "() {")
		w.P("		p, err := ", name_y, "Narrow(ctx, scope, nil)")
		w.P("		if err != nil {")
		w.P("			return err")
		w.P("		}")
		w.P("		q.With", name_p, "(func(q *", w.ent.Ident(name_y+"Query"), ") {")
		w.P("			if p != nil {")
		w.P("				q.Where(p)")
		w.P("			}")
		w.P("			err = ", name_y, "Select(ctx, scope, q, m.Get", name_p, "())")
		w.P("		})")
		w.P("		if err != nil {")
		w.P("			return err")
		w.P("		}")
		w.P("	}")
	}
	w.P("	return nil")
	w.P("}")
	w.P("")
}

// xSelectInit is [fileWork.xSelect] for a select that may be nil.
//
// With no select, only a list edge is loaded, and only its keys. A row holds
// the key of every other neighbour in the column the edge is kept in, and
// `Proto()` answers with it when nothing was loaded -- which is the whole of
// what a Get with no select answers for an edge. Loading it anyway was a query
// for each edge of every row read, and an answer that was worse: the neighbour
// read with its key and nothing else, so every other field of it at its zero
// value and its timestamps in the year 1, as though that were the row.
//
// The keys of a list edge are in no column of the row, so they are loaded --
// and narrowed, as a select is: a key in that list says the row it names is
// there and is this one's, which is what a scope keeps from a caller who may
// not read it.
func (w *fileWork) xSelectInit() {
	name_x := w.Entity.Name()
	w.P("// ", name_x, "SelectInit is [", name_x, "Select] for a select that may be")
	w.P("// nil, which loads the keys of a list edge and nothing else.")
	w.P("func ", name_x, "SelectInit(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "scope Scope, ",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") error {")
	w.P("	if m != nil {")
	w.P("		return ", name_x, "Select(ctx, scope, q, m)")
	w.P("	}")
	for p := range w.Entity.Edges() {
		if !p.IsList() {
			continue
		}
		name_p := work.Name(p.Name()).Ent()
		name_y := p.Target().Name()
		w.P("	{")
		w.P("		p, err := ", name_y, "Narrow(ctx, scope, nil)")
		w.P("		if err != nil {")
		w.P("			return err")
		w.P("		}")
		w.P("		q.With", name_p, "(func(q *", w.ent.Ident(name_y+"Query"), ") {")
		w.P("			select", name_y, "Key(q)")
		w.P("			if p != nil {")
		w.P("				q.Where(p)")
		w.P("			}")
		w.P("		})")
		w.P("	}")
	}
	w.P("	return nil")
	w.P("}")
	w.P("")
}
