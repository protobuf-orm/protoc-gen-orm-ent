package bare_test

import (
	"context"
	"os"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/lesomnus/z"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// Postgres is the environment variable naming a database to run against.
//
//	APPTEST_POSTGRES='postgres://app:app@127.0.0.1/apptest?sslmode=disable' go test ./...
//
// Everything else here runs on SQLite, which is right: it needs no server and
// it exercises the same generated code. What it cannot exercise is the part of
// this that is **only** true on postgres -- a column declared `jsonb`, and a
// value scanner that hands the driver a Go string for it.
const Postgres = "APPTEST_POSTGRES"

// TestPostgresMessageField is the half of the message-field story that SQLite
// cannot answer.
//
// A message field is a string column with a value scanner, and its schema type
// is put back to `jsonb` -- because that is what `field.JSON` gave it before,
// and narrowing it would be a table rewrite for every deployment that already
// has one.
//
// Which raises a question SQLite is no help with: the scanner answers with a Go
// `string`, and the column is `jsonb`. Whether postgres accepts that depends on
// the driver, on parameter type inference, and on nothing this repository
// controls. It is the sort of thing that is assumed and then found in
// production, so it is asserted here instead.
func TestPostgresMessageField(t *testing.T) {
	dsn := os.Getenv(Postgres)
	if dsn == "" {
		t.Skipf("%s is not set; see the const for what it wants", Postgres)
	}

	x := require.New(t)
	ctx := context.TODO()

	drv, err := entsql.Open(dialect.Postgres, dsn)
	x.NoError(err)
	t.Cleanup(func() { drv.Close() })

	db := ent.NewClient(ent.Driver(drv))
	x.NoError(db.Schema.Create(ctx))

	s, err := bare.NewServer(db)
	x.NoError(err)

	// Before as well as after. This is a database that outlives the run, so a
	// test that only cleaned up on the way out is one that passes once and then
	// fails for whoever inherits the rows -- including the run that crashed.
	clear := func() {
		_, err := drv.DB().ExecContext(ctx, `DELETE FROM messagefield WHERE id LIKE 'pg-%'`)
		x.NoError(err)
	}
	clear()
	t.Cleanup(clear)

	t.Run("the column is jsonb and not text", func(t *testing.T) {
		x := require.New(t)

		var kind string
		err := drv.DB().QueryRowContext(ctx, `
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'messagefield' AND column_name = 'held'`).Scan(&kind)
		x.NoError(err)
		x.Equal("jsonb", kind)
	})

	t.Run("a string from the scanner is accepted by a jsonb column", func(t *testing.T) {
		x := require.New(t)

		v, err := s.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id:   z.Ptr("pg-a"),
			Held: pb.Held_builder{What: z.Ptr("a thing"), HowMany: z.Ptr(int32(3))}.Build(),
		}.Build())
		x.NoError(err)
		x.Equal("a thing", v.GetHeld().GetWhat())

		row, err := db.MessageField.Get(ctx, "pg-a")
		x.NoError(err)
		x.Equal("a thing", row.Held.GetWhat())
		x.EqualValues(3, row.Held.GetHowMany())
	})

	// The reason to want jsonb rather than text, and the one thing that is
	// actually better rather than merely unchanged: postgres normalises what it
	// stores. protojson deliberately varies its whitespace, so two writes of an
	// equal value are not equal as text -- and are equal as jsonb.
	t.Run("what is stored is normalised", func(t *testing.T) {
		x := require.New(t)

		_, err := s.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id:   z.Ptr("pg-b"),
			Held: pb.Held_builder{What: z.Ptr("a thing"), HowMany: z.Ptr(int32(3))}.Build(),
		}.Build())
		x.NoError(err)

		var same bool
		err = drv.DB().QueryRowContext(ctx, `
			SELECT (SELECT held FROM messagefield WHERE id = 'pg-a')
			     = (SELECT held FROM messagefield WHERE id = 'pg-b')`).Scan(&same)
		x.NoError(err)
		x.True(same, "two writes of an equal message did not store equal")
	})

	// And that jsonb is genuinely jsonb rather than a string that happens to
	// hold JSON: an operator over it works.
	t.Run("jsonb operators reach inside it", func(t *testing.T) {
		x := require.New(t)

		var what string
		err := drv.DB().QueryRowContext(ctx,
			`SELECT held->>'what' FROM messagefield WHERE id = 'pg-a'`).Scan(&what)
		x.NoError(err)
		x.Equal("a thing", what)
	})

	t.Run("a message that was not given is NULL", func(t *testing.T) {
		x := require.New(t)

		_, err := s.MessageField().Add(ctx, pb.MessageFieldAddRequest_builder{
			Id: z.Ptr("pg-c"),
		}.Build())
		x.NoError(err)

		var n int
		err = drv.DB().QueryRowContext(ctx,
			`SELECT count(*) FROM messagefield WHERE id = 'pg-c' AND nullable_held IS NULL`).Scan(&n)
		x.NoError(err)
		x.Equal(1, n)
	})
}
