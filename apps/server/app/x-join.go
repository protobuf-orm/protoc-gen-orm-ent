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
// But one thing, which [fileWork.xWriting] is about: a write that joined a
// transaction cannot take back what it wrote, so when it fails partway the
// transaction it joined is told it cannot be committed.
//
// `want` is the expression that decides whether a transaction is wanted at all.
// Apply always wants one -- its write and its read-back have to be the same
// statement's -- while Add and Erase want one only when there is a recorder,
// since a write with nothing to record alongside it is a single statement.
func (w *fileWork) xJoin(want string) {
	w.P("	tx, err := ", work.PkgEnt.Ident("JoinTx"), "[*", w.ent.Ident("Client"), ", *", w.ent.Ident("Tx"),
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

// xWriting emits what a write says before its first statement that writes:
// from here, failing may leave part of it in the transaction.
//
// When the write began its own transaction, its Close takes back whatever it
// wrote and this changes nothing. When it joined one -- an app's operation, a
// batch -- that is not its to end, and there are no savepoints to take a part
// of it back by: a record of a row that could not be written, an INSERT of
// several hundred rows refused after the first few hundred held, a read after
// an UPDATE that found the row gone. So a Close with no Commit after this tells
// the transaction it joined that its commit is refused (`ent.Joined.Writing`),
// which is the answer PostgreSQL already gives a failed statement, given on
// every engine and for every way of failing.
//
// Before the statement and not after it, so that a statement refused on its
// own is answered the same on SQLite, which undoes only that statement, as on
// PostgreSQL, which takes the transaction with it.
func (w *fileWork) xWriting() {
	w.P("	tx.Writing()")
}
