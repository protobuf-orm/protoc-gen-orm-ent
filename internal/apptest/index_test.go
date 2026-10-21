package apptest_test

import (
	"context"
	"slices"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/migrate"
)

// TestAnIndexIsInTheOrderItDeclares: an index is read from its first column,
// so its columns are in the order its refs are, an edge's among the fields'
// -- `User.tenant_created` is `(tenant, date_created)`, and not the edge
// after every field, which is what ent makes of `Fields(...).Edges(...)`.
func TestAnIndexIsInTheOrderItDeclares(t *testing.T) {
	columns := func(table *schema.Table) [][]string {
		out := [][]string{}
		for _, i := range table.Indexes {
			cs := []string{}
			for _, c := range i.Columns {
				cs = append(cs, c.Name)
			}
			out = append(out, cs)
		}
		return out
	}

	user := columns(migrate.UserTable)
	require.True(t, slices.ContainsFunc(user, func(cs []string) bool { return slices.Equal(cs, []string{"tenant_id", "date_created"}) }), "user: %v", user)
	require.True(t, slices.ContainsFunc(user, func(cs []string) bool { return slices.Equal(cs, []string{"alias", "tenant_id"}) }), "user: %v", user)

	seat := columns(migrate.SeatTable)
	require.True(t, slices.ContainsFunc(seat, func(cs []string) bool { return slices.Equal(cs, []string{"holder_id", "org_id"}) }), "seat: %v", seat)
}

// TestAnIndexKeepsTheNameItDeclares: an index the schema names is stored under
// that name -- the table's and its own, `user_tenant_created` -- and not under
// one ent makes up out of its columns, `user_tenant_id_date_created`.
//
// The name is what a migration finds an index by. Made of the columns, an
// index whose columns changed had a new name, so a migration made a second
// index beside the first and left the first, since it drops none: written on
// every insert, read by nothing, until somebody listed it by hand. Under the
// name it declares, it is the same index changed, and rebuilt where it stands.
func TestAnIndexKeepsTheNameItDeclares(t *testing.T) {
	x := require.New(t)

	names := func(table *schema.Table) []string {
		out := []string{}
		for _, i := range table.Indexes {
			out = append(out, i.Name)
		}
		return out
	}
	x.Subset(names(migrate.UserTable), []string{"user_alias", "user_tenant_created"})
	x.Subset(names(migrate.SeatTable), []string{"seat_place", "seat_deputy"})

	// And a unique field, promoted to a partial index because the entity
	// erases softly, declares no name and keeps the one ent gives it.
	x.Contains(names(migrate.NoteTable), "note_slug")

	t.Run("and one whose columns changed is changed where it stands", func(t *testing.T) {
		x := require.New(t)
		ctx := t.Context()

		drv, err := sql.Open("sqlite3", "file:index-keeps-its-name?mode=memory&cache=shared&_fk=1")
		x.NoError(err)
		t.Cleanup(func() { drv.Close() })
		drv.DB().SetMaxOpenConns(1)

		db := ent.NewClient(ent.Driver(drv))
		x.NoError(db.Schema.Create(ctx))

		// What an earlier build made of it: the same index, with its columns
		// the other way round.
		for _, q := range []string{
			`DROP INDEX "user_tenant_created"`,
			`CREATE INDEX "user_tenant_created" ON "user" ("date_created", "tenant_id")`,
		} {
			_, err := drv.DB().ExecContext(ctx, q)
			x.NoError(err, q)
		}
		x.Equal([]string{"date_created", "tenant_id"}, indexColumns(ctx, x, drv, "user_tenant_created"))

		x.NoError(db.Schema.Create(ctx))
		x.Equal([]string{"tenant_id", "date_created"}, indexColumns(ctx, x, drv, "user_tenant_created"))

		// And it is the one index on those columns, not one of two.
		for _, name := range indexesOf(ctx, x, drv, "user") {
			if name == "user_tenant_created" {
				continue
			}
			x.NotEqual([]string{"date_created", "tenant_id"}, indexColumns(ctx, x, drv, name), "%s is the earlier build's index, left beside the new one", name)
		}
	})
}

// indexesOf are the names of the SQLite indexes on `table`.
func indexesOf(ctx context.Context, x *require.Assertions, drv *sql.Driver, table string) []string {
	return texts(ctx, x, drv, `SELECT name FROM pragma_index_list(?)`, table)
}

// indexColumns are the columns of the SQLite index called `name`, in order.
func indexColumns(ctx context.Context, x *require.Assertions, drv *sql.Driver, name string) []string {
	return texts(ctx, x, drv, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, name)
}

// texts answers a query of one text column, read to its end before it
// returns, since the connection is the only one there is.
func texts(ctx context.Context, x *require.Assertions, drv *sql.Driver, q string, args ...any) []string {
	rows, err := drv.DB().QueryContext(ctx, q, args...)
	x.NoError(err)
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var c string
		x.NoError(rows.Scan(&c))
		out = append(out, c)
	}
	x.NoError(rows.Err())

	return out
}
