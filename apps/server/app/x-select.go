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
	w.P("// ", name_x, "Select loads what `m` asks for, each edge with no scope.")
	w.P("//")
	w.P("// Deprecated: a row at the other end of an edge is read here whatever a")
	w.P("// scope says of it, so a caller who may not read that row is answered with")
	w.P("// it by asking for it through this one. [", name_x, "SelectWithin] narrows each")
	w.P("// edge by its target's own Narrow, and is what the generated Get calls.")
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

// xSelectWithin emits the select a served read uses: the same columns and the
// same edges as [fileWork.xSelect], with every edge read through the Narrow of
// the entity at its other end.
//
// # What the other one let through
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
// # What it answers instead
//
// An edge whose target the scope keeps from this caller is not loaded, which
// `Proto()` answers as it answers an edge nobody asked for: the reference, and
// nothing of the row. That is what the caller holds already by reading the
// child, so it tells them nothing more -- and it is not an error, for the
// reason a Get out of scope is NotFound and not a refusal.
//
// The scope is asked once for each edge a select names and again for each edge
// under it, with the context of the read that began it: an edge is read by the
// rule of what it reaches, as the caller who asked.
//
// # Why it answers with an error
//
// A scope may fail -- it reads who is asking, and that can be a database that
// would not answer. Loaded anyway, the edge would be the row the scope was
// about to keep back.
func (w *fileWork) xSelectWithin() {
	name_x := w.Entity.Name()
	w.P("// ", name_x, "SelectWithin loads what `m` asks for, each edge through the")
	w.P("// Narrow of the entity it reaches: an edge whose target `scope` keeps from")
	w.P("// this caller is answered as its reference alone, as an edge nobody asked")
	w.P("// for is.")
	w.P("func ", name_x, "SelectWithin(",
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
		w.P("			err = ", name_y, "SelectWithin(ctx, scope, q, m.Get", name_p, "())")
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
	w.P("// ", name_x, "SelectInit is [", name_x, "Select] for a select that may be nil.")
	w.P("//")
	w.P("// Deprecated: it reads each edge with no scope, as [", name_x, "Select] does.")
	w.P("// [", name_x, "SelectInitWithin] is what the generated Get calls.")
	w.P("func ", name_x, "SelectInit(",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") {")
	w.P("	if m != nil {")
	w.P("		", name_x, "Select(q, m)")
	// Only a list edge is loaded. A row holds the key of every other one in
	// the column the edge is kept in, and `Proto()` answers with it when
	// nothing was loaded -- which is the whole of what a Get with no select
	// answers for an edge. Loading it anyway was a query for each edge of
	// every row read, and an answer that was worse: the neighbour read with
	// its key and nothing else, so every other field of it at its zero value
	// and its timestamps in the year 1, as though that were the row.
	opened := false
	for p := range w.Entity.Edges() {
		if !p.IsList() {
			continue
		}
		if !opened {
			w.P("	} else {")
			opened = true
		}
		name_p := work.Name(p.Name()).Ent()
		name_y := p.Target().Name()
		w.P("		q.With", name_p, "(select", name_y, "Key)")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}

// xSelectInitWithin is [fileWork.xSelectInit] through a scope.
//
// With no select, the keys of a list edge are loaded and nothing else, since
// those are in no column of the row. They are narrowed too: a key in that list
// says the row it names is there and is this one's, which is what a scope
// keeps from a caller who may not read it.
func (w *fileWork) xSelectInitWithin() {
	name_x := w.Entity.Name()
	w.P("// ", name_x, "SelectInitWithin is [", name_x, "SelectWithin] for a select that")
	w.P("// may be nil, which loads the keys of a list edge and nothing else.")
	w.P("func ", name_x, "SelectInitWithin(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "scope Scope, ",
		/* */ "q *", w.ent.Ident(name_x+"Query"), ", ",
		/* */ "m *", w.Src.GoImportPath.Ident(name_x+"Select"),
		") error {")
	w.P("	if m != nil {")
	w.P("		return ", name_x, "SelectWithin(ctx, scope, q, m)")
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
