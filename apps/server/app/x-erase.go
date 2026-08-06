package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xErase emits Erase, which is a delete by whatever the reference selects on.
//
// Erasing what is not there succeeds. A caller that asked for a row to be gone
// and is told it is gone has got what it asked for, and answering NotFound
// would make the retry of a delete that already worked look like a failure.
//
// That is also what costs the recorder a query it would not otherwise need.
// The delete says how many rows it removed, but by then the row is gone and
// there is nothing left to name it by -- and the reference may have been an
// alias, which is not the key a trail is read back with. So when there is a
// recorder the row is looked up first, and the answer settles both questions
// at once: no row means nothing was erased, and nothing is recorded.
//
// That lookup also *replaces* the delete when it finds nothing, which is worth
// saying out loud: a client-level delete hook that used to fire for an erase of
// a row that was not there stops firing once a recorder is configured. The RPC
// answers the same either way; what changed is that no statement is issued.
func (w *fileWork) xErase() {
	name := w.Ident.GoName

	w.P("func (s ", name, "ServiceServer) Erase(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"Ref"),
		") (*", work.IdentEmpty, ", error) {")
	w.P("	p, err := ", name, "Pick(req)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	// Out of scope is out of reach, and erasing what is out of reach succeeds
	// without erasing anything -- which is the same answer this gives for a
	// row that was never there, and for the same reason.
	w.P("	p, err = s.narrow(ctx, p)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")

	// The delete and what a recorder writes about it are one write; see xJoin.
	w.xJoin("s.Rec != nil")

	// `any`, and not the key's own type spelled out. What a Change carries is
	// an `any` either way, and Add fills it from the row ent handed back --
	// so writing the type here is a second opinion about what that type is,
	// from the proto side rather than the ent side. They agree for every key
	// there is today and there is no reason for this to be the place that
	// finds out when they stop.
	w.P("	var k any")
	w.P("	if s.Rec != nil {")
	w.P("		v, err := st.Db.", name, ".Query().Where(p).OnlyID(ctx)")
	w.P("		if err != nil {")
	w.P("			if ", w.ent.Ident("IsNotFound"), "(err) {")
	w.P("				return &", work.IdentEmpty, "{}, nil")
	w.P("			}")
	w.P("			return nil, err")
	w.P("		}")
	w.P("")
	// Narrowed to the row that was found, so the statement that runs is about
	// the row that will be recorded and not about whatever the reference would
	// select a moment later.
	w.P("		k = v")
	w.P("		p = ", w.xEntPkg().Ident("IDEQ"), "(v)")
	w.P("	}")
	w.P("")

	w.P("	n, err := st.Db.", name, ".Delete().Where(p).Exec(ctx)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	// Only what the statement actually removed. The row was there a moment ago
	// -- it was read to be named -- so this is the narrow window in which
	// somebody else erased it first, and a trail that claimed this request did
	// it would be wrong about who.
	w.P("	if n > 0 {")
	w.P("		if err := record(ctx, s.Rec, st.Db, Change{")
	w.P("			Method: ", w.Src.GoImportPath.Ident(name+"Service_Erase_FullMethodName"), ",")
	w.P("			Key: k,")
	w.P("		}); err != nil {")
	w.P("			return nil, err")
	w.P("		}")
	w.P("	}")
	w.P("	if err := tx.Commit(); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	return &", work.IdentEmpty, "{}, nil")
	w.P("}")
	w.P("")
}
