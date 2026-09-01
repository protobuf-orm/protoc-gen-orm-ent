package app

import (
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// xConstraintError emits what a write does with a constraint the database
// refused it on. It is written inside an `if err != nil {` that the caller
// opened and falls through to whatever that block does with an error it has
// nothing to say about.
//
// Two answers, and both are about a rule the schema declared rather than
// something that went wrong:
//
//   - a unique index already holding the value is `AlreadyExists`;
//   - an edge pointing at a row that is not there is `NotFound`, which is what
//     resolving that same edge by a unique index would have answered, since
//     that path queries.
//
// # Why it is one function
//
// Because it is one rule, and it was two places: `Add` mapped both and the
// update path -- which is what `Patch` and `Apply` are -- mapped neither. So an
// update that broke a unique index escaped as the driver's own error: `Unknown`
// to the caller, carrying the name of the index and the columns it is on, and
// **the same conflict answered differently depending on which RPC hit it**.
//
// A constraint is a property of the schema, not of the statement that ran into
// one, so the answer cannot depend on which statement that was.
//
// # What the caller is not told
//
// The driver's own rendering of the violation, which used to be the tail of
// both messages:
//
//	AlreadyExists: SshKey already exists:
//	  ERROR: duplicate key value violates unique constraint "sshkey_fingerprint" (SQLSTATE 23505)
//
// A caller is owed the fact -- the value is taken, the row pointed at is not
// there -- and that is the whole of what these two say. What followed named a
// table, an index and a SQLSTATE, which are the deployment's schema and its
// choice of database: an API whose errors carry them is one whose callers read
// them, and then a migration that renames an index is a breaking change to
// something nobody wrote down. It sent a person reading a CLI to look for a
// table they do not have.
//
// Two ways to keep it were weighed. As a `google.rpc.DebugInfo` detail: it is
// the shape meant for exactly this, and it costs every generated app a direct
// googleapis dependency for a string most of them will never read. Behind the
// error rather than in the status -- a `GRPCStatus()` that answers the fact
// while `Error()` keeps the driver's words: it costs nothing on the wire, and
// it buys nothing either, because what a server logs is the status that went
// out and not the error the handler returned.
//
// So it is dropped, and what is lost with it is which index of several was the
// one -- knowable from the request and the schema, which is where an index
// belongs.
func (w *fileWork) xConstraintError() {
	name := w.Ident.GoName

	w.P("		if err, ok := err.(*", w.ent.Ident("ConstraintError"), "); ok {")
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsUniqueConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("AlreadyExists"), ", \"", name, " already exists\")")
	w.P("			}")
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsForeignKeyConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("NotFound"), ", \"", name, ": referenced entity not found\")")
	w.P("			}")
	w.P("		}")
}
