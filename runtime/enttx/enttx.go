// Package enttx puts a server stack on another driver, so that everything in
// it runs inside one transaction.
//
// The transaction itself is ent's: [dialect.BeginTx] answers with a driver
// that is one, [dialect.InTx] says whether a driver is inside one, and
// [ent.JoinTx] is the shape a caller writes whether or not there is one to
// join. All three were written here first, and having been written twice they
// disagreed -- a client asked whether it was in a transaction knew only about
// the one its own package begins, and this knew only about the other. They are
// one thing in ent now, and what is left here is the half ent has no words
// for: a *server* rebound onto a driver.
//
// The rule the whole thing rests on is unchanged: a transaction is ended by
// whoever began it. Begin returns the transaction to its caller and hands
// everyone else a driver that cannot end it, so a server that opens a
// transaction of its own -- a generated Apply does -- joins this one rather
// than committing it early or taking it down.
package enttx

import (
	"context"

	"github.com/protobuf-orm/ent/dialect"
)

// Begin is [dialect.BeginTx].
//
// Deprecated: call ent's directly. This is here so that the generated servers
// of a deployment that has not been regenerated still compile.
func Begin(ctx context.Context, drv dialect.Driver) (dialect.Driver, dialect.Tx, error) {
	return dialect.BeginTx(ctx, drv)
}

// InTx is [dialect.InTx].
//
// Deprecated: call ent's directly. Unlike this one used to, it also sees a
// transaction begun by a generated client.
func InTx(drv dialect.Driver) bool { return dialect.InTx(drv) }
