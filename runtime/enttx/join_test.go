package enttx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
)

// The doubles stand in for a generated ent client and its transaction. What
// [enttx.Join] promises is about who ends a transaction and who does not, which
// is a question about calls.

type recClient struct {
	inTx bool
	err  error

	tx    recClientTx
	began int
}

func (c *recClient) InTx() bool { return c.inTx }

func (c *recClient) Tx(context.Context) (*recClientTx, error) {
	c.began++
	if c.err != nil {
		return nil, c.err
	}

	c.tx.of = c
	return &c.tx, nil
}

type recClientTx struct {
	of *recClient

	commits   int
	rollbacks int
}

func (t *recClientTx) Commit() error   { t.commits++; return nil }
func (t *recClientTx) Rollback() error { t.rollbacks++; return nil }

// Client is the client to work on: a transaction's own, so that what is written
// through it is inside the transaction.
func (t *recClientTx) Client() *recClient { return &recClient{inTx: true} }

func join(t *testing.T, c *recClient, want bool) enttx.Joined[*recClient] {
	t.Helper()

	v, err := enttx.Join[*recClient, *recClientTx](context.Background(), c, want)
	if err != nil {
		t.Fatalf("join: %s", err)
	}

	return v
}

func TestJoinStarts(t *testing.T) {
	c := &recClient{}

	v := join(t, c, true)
	if c.began != 1 {
		t.Errorf("began = %d, want 1", c.began)
	}
	if v.Db == c {
		t.Error("the work runs on the client it was given, not on the transaction's")
	}
	if !v.Db.InTx() {
		t.Error("the client it answers with is not inside the transaction")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	if c.tx.commits != 1 {
		t.Errorf("commits = %d, want 1", c.tx.commits)
	}

	// What a caller defers, on a transaction that already held. The answer of a
	// transaction that has ended is not looked at, which is the whole point of
	// deferring it.
	v.Close()
	if c.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", c.tx.rollbacks)
	}
}

func TestJoinTakesBack(t *testing.T) {
	c := &recClient{}

	v := join(t, c, true)
	v.Close()

	if c.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", c.tx.rollbacks)
	}
	if c.tx.commits != 0 {
		t.Errorf("commits = %d, want 0", c.tx.commits)
	}
}

// A client already inside a transaction is one somebody else began, and ending
// it is theirs. Committing here would end it under them, half done.
func TestJoinDoesNotEndAnothersTransaction(t *testing.T) {
	c := &recClient{inTx: true}

	v := join(t, c, true)
	if c.began != 0 {
		t.Errorf("began = %d, want 0", c.began)
	}
	if v.Db != c {
		t.Error("the work should run on the client it was given")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	v.Close()

	if c.tx.commits != 0 || c.tx.rollbacks != 0 {
		t.Errorf("commits = %d and rollbacks = %d, want 0 and 0", c.tx.commits, c.tx.rollbacks)
	}
}

// A caller whose need for a transaction depends on something writes the same
// shape either way, and the way that needs none costs none.
func TestJoinWantsNothing(t *testing.T) {
	c := &recClient{}

	v := join(t, c, false)
	if c.began != 0 {
		t.Errorf("began = %d, want 0", c.began)
	}
	if v.Db != c {
		t.Error("the work should run on the client it was given")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	v.Close()

	if c.tx.commits != 0 || c.tx.rollbacks != 0 {
		t.Errorf("commits = %d and rollbacks = %d, want 0 and 0", c.tx.commits, c.tx.rollbacks)
	}
}

func TestJoinReportsWhatItCouldNotStart(t *testing.T) {
	want := errors.New("no")
	c := &recClient{err: want}

	_, err := enttx.Join[*recClient, *recClientTx](context.Background(), c, true)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
