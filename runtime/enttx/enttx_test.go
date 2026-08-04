package enttx_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
)

// The doubles below record what was asked of them rather than talking to a
// database. What this package promises is about who may end a transaction, and
// that is a question about calls, not about rows.

type recTx struct {
	commits   int
	rollbacks int
	execs     int
	queries   int
}

func (t *recTx) Commit() error   { t.commits++; return nil }
func (t *recTx) Rollback() error { t.rollbacks++; return nil }

func (t *recTx) Exec(context.Context, string, any, any) error  { t.execs++; return nil }
func (t *recTx) Query(context.Context, string, any, any) error { t.queries++; return nil }

type recDriver struct {
	tx    recTx
	began int

	dialect string
	err     error

	execs   int
	queries int
}

func (d *recDriver) Tx(context.Context) (dialect.Tx, error) {
	if d.err != nil {
		return nil, d.err
	}
	d.began++
	return &d.tx, nil
}

func (d *recDriver) Dialect() string { return d.dialect }
func (d *recDriver) Close() error    { return nil }

func (d *recDriver) Exec(context.Context, string, any, any) error  { d.execs++; return nil }
func (d *recDriver) Query(context.Context, string, any, any) error { d.queries++; return nil }

var _ dialect.Driver = (*recDriver)(nil)
var _ dialect.Tx = (*recTx)(nil)

func TestBegin(t *testing.T) {
	ctx := context.Background()

	t.Run("the transaction is the caller's", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		_, tx, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if drv.began != 1 {
			t.Fatalf("began = %d, want 1", drv.began)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if drv.tx.commits != 1 {
			t.Errorf("commits = %d, want 1", drv.tx.commits)
		}
	})

	t.Run("a transaction begun inside cannot end it", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}

		// What ent does on `Client.Tx`: ask the driver for a transaction and
		// end it. The generated Apply does exactly this, and it must not reach
		// the transaction the caller is holding.
		inner, err := d.Tx(ctx)
		if err != nil {
			t.Fatalf("inner tx: %s", err)
		}
		if err := inner.Commit(); err != nil {
			t.Errorf("inner commit: %s", err)
		}
		if err := inner.Rollback(); err != nil {
			t.Errorf("inner rollback: %s", err)
		}

		if drv.tx.commits != 0 {
			t.Errorf("commits = %d, want 0: an inner call ended the outer transaction", drv.tx.commits)
		}
		if drv.tx.rollbacks != 0 {
			t.Errorf("rollbacks = %d, want 0: an inner call ended the outer transaction", drv.tx.rollbacks)
		}
		// And it was the one transaction throughout.
		if drv.began != 1 {
			t.Errorf("began = %d, want 1", drv.began)
		}
	})

	t.Run("a transaction begun inside still reads and writes through it", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		inner, err := d.Tx(ctx)
		if err != nil {
			t.Fatalf("inner tx: %s", err)
		}
		if err := inner.Exec(ctx, "", nil, nil); err != nil {
			t.Fatalf("exec: %s", err)
		}
		if err := inner.Query(ctx, "", nil, nil); err != nil {
			t.Fatalf("query: %s", err)
		}

		if drv.tx.execs != 1 || drv.tx.queries != 1 {
			t.Errorf("tx got %d execs and %d queries, want 1 and 1", drv.tx.execs, drv.tx.queries)
		}
		if drv.execs != 0 || drv.queries != 0 {
			t.Errorf("the connection was used directly: %d execs, %d queries", drv.execs, drv.queries)
		}
	})

	t.Run("nesting Begin does not end the outer one either", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		outer, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("outer: %s", err)
		}
		_, inner, err := enttx.Begin(ctx, outer)
		if err != nil {
			t.Fatalf("inner: %s", err)
		}
		if err := inner.Commit(); err != nil {
			t.Fatalf("inner commit: %s", err)
		}

		if drv.tx.commits != 0 {
			t.Errorf("commits = %d, want 0", drv.tx.commits)
		}
		if drv.began != 1 {
			t.Errorf("began = %d, want 1", drv.began)
		}
	})

	t.Run("reads and writes on the driver go through the transaction", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if err := d.Exec(ctx, "", nil, nil); err != nil {
			t.Fatalf("exec: %s", err)
		}
		if err := d.Query(ctx, "", nil, nil); err != nil {
			t.Fatalf("query: %s", err)
		}

		if drv.tx.execs != 1 || drv.tx.queries != 1 {
			t.Errorf("tx got %d execs and %d queries, want 1 and 1", drv.tx.execs, drv.tx.queries)
		}
		if drv.execs != 0 || drv.queries != 0 {
			t.Errorf("the connection was used directly: %d execs, %d queries", drv.execs, drv.queries)
		}
	})

	t.Run("the dialect is the connection's", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.Postgres}

		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if got := d.Dialect(); got != dialect.Postgres {
			t.Errorf("dialect = %q, want %q", got, dialect.Postgres)
		}
	})

	t.Run("closing is not the driver's to do", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if err := d.Close(); err != nil {
			t.Errorf("close: %s", err)
		}
	})

	t.Run("a transaction that cannot be begun is reported", func(t *testing.T) {
		want := errors.New("no")
		drv := &recDriver{dialect: dialect.SQLite, err: want}

		d, tx, err := enttx.Begin(ctx, drv)
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
		if d != nil || tx != nil {
			t.Errorf("got a driver or a transaction along with the error")
		}
	})
}

func TestInTx(t *testing.T) {
	ctx := context.Background()

	t.Run("a plain driver is not in one", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		if enttx.InTx(drv) {
			t.Error("InTx = true")
		}
	})

	t.Run("one from Begin is", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if !enttx.InTx(d) {
			t.Error("InTx = false")
		}
	})

	t.Run("through the wrappers a driver picks up", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		d, _, err := enttx.Begin(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if !enttx.InTx(dialect.Debug(d)) {
			t.Error("InTx = false through Debug")
		}
		if enttx.InTx(dialect.Debug(drv)) {
			t.Error("InTx = true for a debugged plain driver")
		}
	})
}
