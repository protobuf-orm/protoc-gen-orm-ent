// Package enttx lets a transaction stand in for a driver, so that everything
// built on that driver runs inside the one transaction.
//
// It is how a caller that spans several servers -- add the row, then write the
// audit entry -- makes them one write. The servers do not have to know: they
// keep talking to the client they were given, and the client talks to the
// transaction.
//
// The rule the whole package rests on is that a transaction is ended by
// whoever began it. [Begin] returns the transaction to its caller and hands
// everyone else a driver that cannot end it, so a server that opens a
// transaction of its own -- a generated Apply does -- joins this one rather
// than committing it early or taking it down.
package enttx

import (
	"context"

	"entgo.io/ent/dialect"
)

// Begin starts a transaction on drv and answers with a driver that runs
// through it, and with the transaction itself.
//
// The transaction is the caller's to commit or roll back, and nobody else's:
// the driver is not it, and what the driver hands out when something asks it
// for a transaction is a no-op. That is not a nicety. ent ends a transaction
// through whatever `Driver.Tx` returned, so a driver that returned the real
// one would let any inner call's `defer tx.Rollback()` throw away the work of
// the call that wrapped it, and report success for it.
//
// The driver is not safe for concurrent use, because a transaction is not.
func Begin(ctx context.Context, drv dialect.Driver) (dialect.Driver, dialect.Tx, error) {
	tx, err := drv.Tx(ctx)
	if err != nil {
		return nil, nil, err
	}

	return &txDriver{drv: drv, tx: tx}, tx, nil
}

// InTx reports whether drv is already running inside a transaction [Begin]
// started, looking through the wrappers a driver picks up on the way.
//
// Nothing depends on this for correctness: a transaction begun inside another
// is a no-op participant in it either way. It is for a caller that would
// rather not wrap a wrapper, and for one that wants to say out loud that it
// joined rather than started.
func InTx(drv dialect.Driver) bool {
	for {
		switch v := drv.(type) {
		case *txDriver:
			return true
		case *dialect.DebugDriver:
			drv = v.Driver
		default:
			return false
		}
	}
}

// txDriver is a transaction wearing a driver's clothes.
type txDriver struct {
	// drv is the driver the transaction was begun from. It is kept for the
	// dialect, which is a property of the connection and not of the
	// transaction, and which callers ask for through however many wrappers.
	drv dialect.Driver

	// tx is the transaction. Only the caller of [Begin] holds it as a
	// transaction; from here it is only ever read and written through.
	tx dialect.Tx
}

var _ dialect.Driver = (*txDriver)(nil)

// Tx answers with a transaction that reads and writes through this one but
// cannot end it -- which is the whole point, so it is not an implementation
// detail to be tidied away. [dialect.NopTx] is ent's own wrapper for exactly
// this, and ent's generated txDriver answers the same way for the same reason.
func (d *txDriver) Tx(context.Context) (dialect.Tx, error) {
	return dialect.NopTx(d), nil
}

// Dialect is the dialect of the driver the transaction was begun from.
func (d *txDriver) Dialect() string { return d.drv.Dialect() }

// Close is a nop: the connection is not this driver's to close, and the
// transaction is not this driver's to end.
func (*txDriver) Close() error { return nil }

func (d *txDriver) Exec(ctx context.Context, query string, args, v any) error {
	return d.tx.Exec(ctx, query, args, v)
}

func (d *txDriver) Query(ctx context.Context, query string, args, v any) error {
	return d.tx.Query(ctx, query, args, v)
}
