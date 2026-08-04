package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xJoin emits the transaction a write is done inside, and the `st` that names
// the client to do it with.
//
// Three of these servers need one and they need it for the same reason, so it
// is said once here and settled once in enttx: a client already inside a
// transaction joins it rather than nesting, whoever began a transaction is who
// ends it, and a commit or a close on one that was only joined does nothing.
// Emitting the whole dance three times meant three copies of that reasoning,
// in a place no test could reach except through a generated server.
//
// `want` is the expression that decides whether a transaction is wanted at all.
// Apply always wants one -- its write and its read-back have to be the same
// statement's -- while Add and Erase want one only when there is a recorder,
// since a write with nothing to record alongside it is a single statement.
func (w *fileWork) xJoin(want string) {
	w.P("	tx, err := ", work.PkgEntTx.Ident("Join"), "[*", w.ent.Ident("Client"), ", *", w.ent.Ident("Tx"),
		"](ctx, s.Db, ", want, ")")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	defer tx.Close()")
	w.P("")
	w.P("	st := s")
	w.P("	st.Db = tx.Db")
	w.P("")
}
