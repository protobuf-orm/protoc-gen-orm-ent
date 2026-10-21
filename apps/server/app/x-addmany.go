package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// addManyBinds is the most values one INSERT of an AddMany binds, which is what
// decides how many rows go into one statement.
//
// SQLite binds at most 32766 values in a statement and PostgreSQL and MySQL at
// most 65535, so this is under the smallest of them with room to spare. A
// thousand rows of a table with forty columns would bind forty thousand, which
// SQLite refuses; split at this, they are two statements.
const addManyBinds = 32000

// addManyRows is the most rows one INSERT of an AddMany writes, whatever the
// columns allow: a few hundred to a statement is where the round trips stop
// mattering, and a statement much longer than that is one the database plans
// and logs for no gain.
const addManyRows = 500

// xAddMany emits AddMany: every item written as an Add of it would be, with
// one INSERT for every few hundred of them, in one transaction.
//
// One transaction always, recorder or not: there may be more than one
// statement, and the call is all of its rows or none of them.
//
// A refusal about one item -- a key that is not sixteen bytes, a reference
// that names nothing -- is refused naming the item, before anything is
// written. A refusal by the database is about the whole statement and cannot
// say which row: a key two items share is an AlreadyExists for the call.
func (w *fileWork) xAddMany() {
	name := w.Ident.GoName
	many := w.Entity.Rpcs().GetAddMany()

	cols := 0
	for p := range w.Entity.Props() {
		// A list edge is a column of the other table.
		if e, ok := p.(graph.Edge); ok && e.IsList() {
			continue
		}
		cols++
	}
	rows := addManyRows
	if per := addManyBinds / max(cols, 1); per < rows {
		rows = max(per, 1)
	}

	w.P("// ", name, "AddManyMax is the most items one AddMany of ", name, " carries.")
	w.P("const ", name, "AddManyMax = ", many.Max())
	w.P("")
	w.P("// ", lowerFirst(name), "AddManyRows is how many rows one INSERT of an AddMany")
	w.P("// writes: as many as fit in what a statement binds on every engine, and")
	w.P("// no more than a few hundred.")
	w.P("const ", lowerFirst(name), "AddManyRows = ", rows)
	w.P("")

	w.P("// AddMany adds every item as Add would add it, in one transaction: all of")
	w.P("// the rows or none of them. The rows come back in the order of the items.")
	w.P("//")
	w.P("// An item that cannot be a row is refused naming the item, before anything")
	w.P("// is written. A refusal by the database is the whole statement's, and does")
	w.P("// not say which row it was about.")
	w.P("func (s ", name, "ServiceServer) AddMany(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"AddManyRequest"),
		") (*", w.Src.GoImportPath.Ident(name+"AddManyResponse"), ", error) {")
	w.P("	items := req.GetItems()")
	w.P("	if len(items) > ", name, "AddManyMax {")
	w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"),
		", \"items: at most %d in one call, and there are %d\", ", name, "AddManyMax, len(items))")
	w.P("	}")
	// Nothing asked, nothing written and nothing recorded -- and no
	// transaction begun for it.
	w.P("	if len(items) == 0 {")
	w.P("		return ", w.Src.GoImportPath.Ident(name+"AddManyResponse_builder"), "{}.Build(), nil")
	w.P("	}")
	w.P("")

	w.xJoin("true")

	w.P("	images := snapshotOptions(s.Rec, ", w.Src.GoImportPath.Ident(name+"Service_AddMany_FullMethodName"), ").Value")
	w.P("	qs := make([]*", w.ent.Ident(name+"Create"), ", len(items))")
	w.P("	dss := make([][]func(v *", w.Ident, "), len(items))")
	w.P("	stored := make([]*", w.ent.Ident(name), ", len(items))")
	w.P("	for i, item := range items {")
	w.P("		q, ds, err := st.create(ctx, item)")
	w.P("		if err != nil {")
	w.P("			return nil, ItemError(i, err)")
	w.P("		}")
	// The row as the database stored it, read by the INSERT itself where the
	// dialect has RETURNING and matched back to its builder -- one statement
	// for the rows and their images both. See xAdd for why an image is the
	// stored row and not the builder's.
	w.P("		if images {")
	w.P("			q.Stored(&stored[i])")
	w.P("		}")
	w.P("		qs[i], dss[i] = q, ds")
	w.P("	}")
	w.P("")
	w.P("	us := make([]*", w.ent.Ident(name), ", 0, len(qs))")
	w.P("	for lo := 0; lo < len(qs); lo += ", lowerFirst(name), "AddManyRows {")
	w.P("		vs, err := st.Db.", name, ".CreateBulk(qs[lo:min(lo+", lowerFirst(name), "AddManyRows, len(qs))]...).Save(ctx)")
	w.P("		if err != nil {")
	w.xConstraintError()
	w.P("			return nil, err")
	w.P("		}")
	w.P("		us = append(us, vs...)")
	w.P("	}")
	w.P("")
	w.P("	out := make([]*", w.Ident, ", len(us))")
	w.P("	for i, u := range us {")
	w.P("		v := u.Proto()")
	w.P("		for _, d := range dss[i] { d(v) }")
	w.P("		out[i] = v")
	w.P("")
	w.P("		var value *", w.Ident)
	w.P("		if stored[i] != nil {")
	w.P("			value = s.image(stored[i].Proto())")
	w.P("		}")
	// A row each, as an Add of each would have been told: whatever keeps a
	// trail, drops a cache or publishes an event goes on doing it a row at a
	// time without knowing there was a batch.
	w.P("		if err := record(ctx, s.Rec, st.Db, Change{")
	w.P("			By: ", w.Src.GoImportPath.Ident(name+"Service_AddMany_FullMethodName"), ",")
	w.P("			Key: u.Id,")
	w.P("			Value: value,")
	w.P("		}); err != nil {")
	w.P("			return nil, err")
	w.P("		}")
	w.P("	}")
	w.P("	if err := tx.Commit(); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	return ", w.Src.GoImportPath.Ident(name+"AddManyResponse_builder"), "{Items: out}.Build(), nil")
	w.P("}")
	w.P("")
}

// xAddRule emits the rule a layer in front of this server puts on the RPCs
// that add, and the server it is put on.
//
// What a layer has to say about a row an Add would write -- may this caller
// put it there, which tenant is it in, what is it named -- is a thing to say
// about each item of an AddMany as well. Written as an Add the layer overrides,
// it is said in one place and silently not in the other the day the entity
// asks for AddMany: the layer's Add refuses what its AddMany, inherited from
// the server behind, lets through. Written as a rule, both RPCs ask it, and an
// entity that gains AddMany gains the rule on it without the layer changing.
//
// The rule hands back the request to write. A layer that changes a request
// changes a copy, since the one it was handed belongs to whoever called, and an
// AddMany hands on a request of its own with the items the rule answered with.
// What reaches the server behind is one call either way, so an AddMany stays
// one AddMany and one statement below.
func (w *fileWork) xAddRule() {
	name := w.Ident.GoName
	rule := name + "AddRule"
	impl := lowerFirst(name) + "AddRuled"
	svc := w.Src.GoImportPath.Ident(name + "ServiceServer")
	req := w.Src.GoImportPath.Ident(name + "AddRequest")

	w.P("// ", rule, " is what a layer in front says about one row an Add of ", name)
	w.P("// would write: the request to write it with -- the one it was handed, or a")
	w.P("// copy it changed -- or a refusal. Nil is the request it was handed.")
	w.P("type ", rule, " func(ctx ", work.PkgContext.Ident("Context"), ", req *", req, ") (*", req, ", error)")
	w.P("")
	w.P("// ", name, "WithAddRule answers with `next` behind `rule` for the RPCs that add.")
	w.P("// An Add is asked about, and every item of an AddMany in order, and the")
	w.P("// call is refused for the first one that is -- an AddMany naming the item.")
	w.P("// What reaches `next` is one call either way: an AddMany stays one AddMany.")
	w.P("//")
	w.P("// A layer that overrides Add itself, for something an Add does that a rule")
	w.P("// cannot say, still has its AddMany asked through this.")
	w.P("func ", name, "WithAddRule(next ", svc, ", rule ", rule, ") ", svc, " {")
	w.P("	return ", impl, "{next, rule}")
	w.P("}")
	w.P("")
	w.P("type ", impl, " struct {")
	w.P("	", svc)
	w.P("	rule ", rule)
	w.P("}")
	w.P("")
	w.P("func (s ", impl, ") Add(ctx ", work.PkgContext.Ident("Context"), ", req *", req, ") (*", w.Ident, ", error) {")
	w.P("	r, err := s.rule(ctx, req)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	if r == nil {")
	w.P("		r = req")
	w.P("	}")
	w.P("	return s.", name, "ServiceServer.Add(ctx, r)")
	w.P("}")
	w.P("")
	if !w.Entity.Rpcs().HasAddMany() {
		return
	}

	w.P("func (s ", impl, ") AddMany(ctx ", work.PkgContext.Ident("Context"), ", req *", w.Src.GoImportPath.Ident(name+"AddManyRequest"),
		") (*", w.Src.GoImportPath.Ident(name+"AddManyResponse"), ", error) {")
	w.P("	items := req.GetItems()")
	// Before the rule is asked about a single one of them: a call the server
	// behind refuses is not one to read the wall a thousand times for.
	w.P("	if len(items) > ", name, "AddManyMax {")
	w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"),
		", \"items: at most %d in one call, and there are %d\", ", name, "AddManyMax, len(items))")
	w.P("	}")
	w.P("")
	w.P("	rs := make([]*", req, ", len(items))")
	w.P("	for i, item := range items {")
	w.P("		r, err := s.rule(ctx, item)")
	w.P("		if err != nil {")
	w.P("			return nil, ItemError(i, err)")
	w.P("		}")
	w.P("		if r == nil {")
	w.P("			r = item")
	w.P("		}")
	w.P("		rs[i] = r")
	w.P("	}")
	// What the call asks besides its items -- an app's own fields on it, a dry
	// run, an idempotency key -- is the server behind's to read as much as the
	// items are: carried over as it came, beside the items the rule made.
	w.P("	out := &", w.Src.GoImportPath.Ident(name+"AddManyRequest"), "{}")
	w.P("	req.ProtoReflect().Range(func(fd ", work.PkgProtoReflect.Ident("FieldDescriptor"), ", v ", work.PkgProtoReflect.Ident("Value"), ") bool {")
	w.P("		if fd.Name() != \"items\" {")
	w.P("			out.ProtoReflect().Set(fd, v)")
	w.P("		}")
	w.P("		return true")
	w.P("	})")
	w.P("	out.ProtoReflect().SetUnknown(req.ProtoReflect().GetUnknown())")
	w.P("	out.SetItems(rs)")
	w.P("")
	w.P("	return s.", name, "ServiceServer.AddMany(ctx, out)")
	w.P("}")
	w.P("")
}
