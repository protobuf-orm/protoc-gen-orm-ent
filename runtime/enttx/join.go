package enttx

import "context"

// Txn is a transaction of a generated ent client: it ends, and while it is open
// it hands out a client that runs through it.
type Txn[C any] interface {
	Commit() error
	Rollback() error
	Client() C
}

// Db is a generated ent client. It says whether it is already running inside a
// transaction, and starts one if it is not.
//
// The type parameter is the client's own type rather than an interface, so that
// [Join] can answer with the client it was given when it starts nothing.
type Db[C any, T Txn[C]] interface {
	InTx() bool
	Tx(context.Context) (T, error)
}

// Joined is a transaction to work on -- or the absence of one, said the same
// way, so that a caller writes one shape either way.
type Joined[C any] struct {
	// Db is what to do the work with: the transaction's client when one was
	// started, and the client [Join] was given when it was not. It is never
	// the zero value of C for a Joined that Join answered with.
	Db C

	// tx is nil unless this started one, which is the whole of the difference
	// between the two cases.
	tx interface {
		Commit() error
		Rollback() error
	}
}

// Commit makes the work hold, and does nothing at all when this started no
// transaction of its own.
//
// A transaction is ended by whoever began it. Joining one somebody else began
// and committing it here would end it under them, half done.
func (j Joined[C]) Commit() error {
	if j.tx == nil {
		return nil
	}

	return j.tx.Commit()
}

// Close is what a caller defers. It takes the work back unless [Joined.Commit]
// already made it hold, and does nothing when this started no transaction.
//
// What a transaction that was already committed answers is not looked at: it is
// "you have ended this", which is the case this is written for.
func (j Joined[C]) Close() {
	if j.tx == nil {
		return
	}

	_ = j.tx.Rollback()
}

// Join answers with a transaction to do the work on.
//
//	tx, err := enttx.Join[*ent.Client, *ent.Tx](ctx, s.Db, true)
//	if err != nil {
//		return err
//	}
//	defer tx.Close()
//
//	// ... work, with tx.Db ...
//
//	return tx.Commit()
//
// It starts nothing, and answers with the client it was given, in two cases.
// One is a client already inside a transaction: whoever began that one ends it,
// and a write made here belongs to it. The other is `want` being false, which is
// for a caller whose need for a transaction depends on something -- a write
// alone is one statement and needs none, and the same write together with a
// record of it is two and does. Either way the caller commits and closes the
// same way, and both do nothing.
func Join[C Db[C, T], T Txn[C]](ctx context.Context, db C, want bool) (Joined[C], error) {
	if !want || db.InTx() {
		return Joined[C]{Db: db}, nil
	}

	tx, err := db.Tx(ctx)
	if err != nil {
		return Joined[C]{}, err
	}

	return Joined[C]{Db: tx.Client(), tx: tx}, nil
}
