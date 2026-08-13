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
func (w *fileWork) xConstraintError() {
	name := w.Ident.GoName

	w.P("		if err, ok := err.(*", w.ent.Ident("ConstraintError"), "); ok {")
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsUniqueConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("AlreadyExists"), ", \"", name, " already exists: %s\", err.Unwrap())")
	w.P("			}")
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsForeignKeyConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("NotFound"), ", \"", name, ": referenced entity not found: %s\", err.Unwrap())")
	w.P("			}")
	w.P("		}")
}
