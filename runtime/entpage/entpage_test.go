package entpage_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entpage"
)

// where renders what After adds to a query, which is the thing under test:
// these are predicates and the only way to read one is to build the statement.
func where(t *testing.T, by []entpage.Order, at []any) (string, []any) {
	t.Helper()

	p, err := entpage.After(by, at)
	require.NoError(t, err)

	s := sql.Select("*").From(sql.Table("user"))
	p(s)

	q, args := s.Query()
	return q, args
}

func TestAfter(t *testing.T) {
	t.Run("one column reads as one comparison", func(t *testing.T) {
		x := require.New(t)

		q, args := where(t, []entpage.Order{{Column: "id"}}, []any{7})
		x.Contains(q, "`user`.`id` > ?")
		x.Equal([]any{7}, args)
	})

	t.Run("descending turns the comparison round", func(t *testing.T) {
		x := require.New(t)

		q, _ := where(t, []entpage.Order{{Column: "id", Desc: true}}, []any{7})
		x.Contains(q, "`user`.`id` < ?")
	})

	t.Run("a tiebreaker only applies where the first column ties", func(t *testing.T) {
		x := require.New(t)

		// The whole of keyset paging: rows strictly past the first column, and
		// where it is equal, rows strictly past the tiebreaker. Anything
		// looser repeats a row and anything tighter skips one.
		q, args := where(t,
			[]entpage.Order{{Column: "date_created", Desc: true}, {Column: "id", Desc: true}},
			[]any{"t", 7})

		x.Contains(q, "`user`.`date_created` < ?")
		x.Contains(q, "OR")
		x.Contains(q, "`user`.`date_created` = ?")
		x.Contains(q, "`user`.`id` < ?")
		x.Equal([]any{"t", "t", 7}, args)
	})

	t.Run("mixed directions are each their own", func(t *testing.T) {
		x := require.New(t)

		q, _ := where(t,
			[]entpage.Order{{Column: "name"}, {Column: "id", Desc: true}},
			[]any{"a", 7})
		x.Contains(q, "`user`.`name` > ?")
		x.Contains(q, "`user`.`id` < ?")
	})

	t.Run("nothing to order by narrows nothing", func(t *testing.T) {
		x := require.New(t)

		p, err := entpage.After(nil, nil)
		x.NoError(err)

		s := sql.Select("*").From(sql.Table("user"))
		p(s)

		q, args := s.Query()
		x.NotContains(q, "WHERE")
		x.Empty(args)
	})

	t.Run("a cursor of the wrong width is not a cursor", func(t *testing.T) {
		x := require.New(t)

		_, err := entpage.After([]entpage.Order{{Column: "id"}}, []any{1, 2})
		x.ErrorIs(err, entpage.ErrCursor)
	})
}

func TestCursor(t *testing.T) {
	t.Run("comes back as what it went in as", func(t *testing.T) {
		x := require.New(t)

		at := time.Date(2026, 8, 6, 11, 22, 33, 0, time.UTC)
		id := uuid.MustParse("726f6f74-0000-0000-0000-000000000000")

		s, err := entpage.Encode(at, id)
		x.NoError(err)
		x.NotEmpty(s)

		var (
			gotAt time.Time
			gotId uuid.UUID
		)
		x.NoError(entpage.Decode(s, &gotAt, &gotId))
		x.True(at.Equal(gotAt))
		x.Equal(id, gotId)
	})

	t.Run("survives a URL", func(t *testing.T) {
		x := require.New(t)

		// A cursor is handed back in a request, and a request may travel as a
		// query string on the way to one.
		s, err := entpage.Encode("a/b+c?d", 1)
		x.NoError(err)
		x.NotContains(s, "/")
		x.NotContains(s, "+")
		x.NotContains(s, "=")
	})

	t.Run("what is not a cursor says so", func(t *testing.T) {
		x := require.New(t)

		var v int
		for _, s := range []string{"!!!!", "", "aGVsbG8"} {
			x.True(errors.Is(entpage.Decode(s, &v), entpage.ErrCursor), s)
		}
	})

	t.Run("one made for another order is not this one's", func(t *testing.T) {
		x := require.New(t)

		s, err := entpage.Encode(1, 2, 3)
		x.NoError(err)

		var a, b int
		x.ErrorIs(entpage.Decode(s, &a, &b), entpage.ErrCursor)
	})
}

func TestSize(t *testing.T) {
	x := require.New(t)

	x.Equal(10, entpage.Size(10, 20, 100))
	x.Equal(20, entpage.Size(0, 20, 100), "asking for nothing is asking for the usual")
	x.Equal(20, entpage.Size(-1, 20, 100), "and so is asking for nonsense")
	x.Equal(100, entpage.Size(1_000_000, 20, 100), "the cap is not a suggestion")
	x.Equal(100, entpage.Size(100, 20, 100))
}
