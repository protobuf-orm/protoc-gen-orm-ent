package bare_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entuuid"
)

// foreignKeys answers, for each foreign key column of `table`, the action the
// database itself says it takes on delete, as the database spells it.
func foreignKeys(t *testing.T, db *ent.Client, table string) map[string]string {
	t.Helper()
	ctx := t.Context()

	var q string
	switch db.Dialect() {
	case dialect.SQLite:
		q = `SELECT "from", on_delete FROM pragma_foreign_key_list(?)`
	case dialect.Postgres:
		q = `SELECT kcu.column_name, rc.delete_rule
			FROM information_schema.referential_constraints rc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_name = rc.constraint_name
			 AND kcu.constraint_schema = rc.constraint_schema
			WHERE kcu.table_name = $1 AND kcu.table_schema = current_schema()`
	default:
		t.Fatalf("no query for %s", db.Dialect())
	}

	rows, err := db.Driver().(interface {
		DB() *sql.DB
	}).DB().QueryContext(ctx, q, table)
	require.NoError(t, err)
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var column, action string
		require.NoError(t, rows.Scan(&column, &action))
		out[column] = action
	}
	require.NoError(t, rows.Err())
	return out
}

// The table's foreign keys say what each edge said, read back from the
// database that made them. An edge that says nothing is the fork's default, as
// before: SET NULL where the key is nullable and NO ACTION where it is not.
func TestOnDeleteForeignKeys(t *testing.T) {
	stores(t, func(t *testing.T, db *ent.Client) {
		x := require.New(t)

		x.Equal(map[string]string{
			"kept_id":   "RESTRICT",
			"filed_id":  "CASCADE",
			"lent_id":   "SET NULL",
			"near_id":   "SET NULL",
			"pinned_id": "RESTRICT",
		}, foreignKeys(t, db, "book"))

		x.Equal(map[string]string{"tenant_id": "NO ACTION"}, foreignKeys(t, db, "user"))
	})
}

// shelve adds a Shelf, which has no Add of its own, and a Book that names it by
// the edge `set` sets.
func shelve(t *testing.T, s bare.Server, db *ent.Client, set func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef)) (shelf string, book string) {
	t.Helper()
	ctx := t.Context()

	shelf = unique("shelf")
	_, err := db.Shelf.Create().SetId(shelf).Save(ctx)
	require.NoError(t, err)

	book = unique("book")
	req := pb.BookAddRequest_builder{Id: book}
	set(&req, pb.ShelfById(shelf))
	_, err = s.Book().Add(ctx, req.Build())
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := t.Context()
		_ = db.Book.DeleteOneId(book).Exec(ctx)
		_ = db.Shelf.DeleteOneId(shelf).Exec(ctx)
	})
	return shelf, book
}

func getBook(t *testing.T, s bare.Server, id string) (*pb.Book, error) {
	return s.Book().Get(t.Context(), pb.BookById(id).Pick().WithSelect(func(s *pb.BookSelect) {
		s.SetAll(true)
	}))
}

func TestOnDeleteErase(t *testing.T) {
	// Refused while a Book names it, and both rows are as they were. The answer
	// says that much and nothing about what names it or how the store put it.
	t.Run("restrict", func(t *testing.T) {
		stores(t, func(t *testing.T, db *ent.Client) {
			x := require.New(t)
			ctx := t.Context()
			s, err := bare.NewServer(db)
			x.NoError(err)

			shelf, book := shelve(t, s, db, func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef) { b.Kept = ref })

			_, err = s.Shelf().Erase(ctx, pb.ShelfById(shelf))
			x.Error(err)
			st, _ := status.FromError(err)
			x.Equal(codes.FailedPrecondition, st.Code(), st.Message())
			x.Equal("Shelf is still named by another row and cannot be erased", st.Message())

			_, err = s.Shelf().Get(ctx, pb.ShelfById(shelf).Pick())
			x.NoError(err, "the Shelf is still there")
			v, err := getBook(t, s, book)
			x.NoError(err)
			x.Equal(shelf, v.GetKept().GetId(), "the Book still names it")
		})
	})

	// The same answer with a recorder, where the delete runs in a transaction
	// of its own, and PostgreSQL takes the transaction down with the statement.
	t.Run("restrict in a transaction", func(t *testing.T) {
		stores(t, func(t *testing.T, db *ent.Client) {
			x := require.New(t)
			ctx := t.Context()
			rec := &recorder{}
			s, err := bare.NewServer(db, bare.WithRecorder(rec))
			x.NoError(err)

			shelf, book := shelve(t, s, db, func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef) { b.Kept = ref })
			n := len(rec.Changes)

			_, err = s.Shelf().Erase(ctx, pb.ShelfById(shelf))
			x.Equal(codes.FailedPrecondition, status.Code(err), err)
			x.Len(rec.Changes, n, "a refused erase records nothing")

			_, err = s.Shelf().Get(ctx, pb.ShelfById(shelf).Pick())
			x.NoError(err)
			v, err := getBook(t, s, book)
			x.NoError(err)
			x.Equal(shelf, v.GetKept().GetId())
		})
	})

	t.Run("cascade", func(t *testing.T) {
		for _, edge := range []struct {
			name string
			set  func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef)
		}{
			{"filed", func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef) { b.Filed = ref }},
		} {
			t.Run(edge.name, func(t *testing.T) {
				stores(t, func(t *testing.T, db *ent.Client) {
					x := require.New(t)
					ctx := t.Context()
					s, err := bare.NewServer(db)
					x.NoError(err)

					shelf, book := shelve(t, s, db, edge.set)

					res, err := s.Shelf().Erase(ctx, pb.ShelfById(shelf))
					x.NoError(err)
					x.True(res.GetErased())

					_, err = getBook(t, s, book)
					x.Equal(codes.NotFound, status.Code(err), "the Book went with it")
				})
			})
		}
	})

	t.Run("set null", func(t *testing.T) {
		for _, edge := range []struct {
			name string
			set  func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef)
		}{
			{"lent", func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef) { b.Lent = ref }},
			// Says nothing: the fork's default for a nullable edge, as before.
			{"near", func(b *pb.BookAddRequest_builder, ref *pb.ShelfRef) { b.Near = ref }},
		} {
			t.Run(edge.name, func(t *testing.T) {
				stores(t, func(t *testing.T, db *ent.Client) {
					x := require.New(t)
					ctx := t.Context()
					s, err := bare.NewServer(db)
					x.NoError(err)

					shelf, book := shelve(t, s, db, edge.set)

					res, err := s.Shelf().Erase(ctx, pb.ShelfById(shelf))
					x.NoError(err)
					x.True(res.GetErased())

					v, err := getBook(t, s, book)
					x.NoError(err, "the Book stays")
					x.False(v.HasLent())
					x.False(v.HasNear())
				})
			})
		}
	})

	// A row that erases softly is not deleted, so nothing that names it is
	// asked: the erase succeeds, the Note is still a row, and the Book still
	// names it.
	t.Run("an erased field deletes nothing", func(t *testing.T) {
		stores(t, func(t *testing.T, db *ent.Client) {
			x := require.New(t)
			ctx := t.Context()
			s, err := bare.NewServer(db)
			x.NoError(err)

			note, err := s.Note().Add(ctx, pb.NoteAddRequest_builder{}.Build())
			x.NoError(err)
			book := unique("book")
			_, err = s.Book().Add(ctx, pb.BookAddRequest_builder{Id: book, Pinned: note.Ref()}.Build())
			x.NoError(err)
			t.Cleanup(func() { _ = db.Book.DeleteOneId(book).Exec(t.Context()) })

			res, err := s.Note().Erase(ctx, note.Ref())
			x.NoError(err)
			x.True(res.GetErased())

			id, err := entuuid.FromBytes(note.GetId())
			x.NoError(err)
			row, err := db.Note.Get(ctx, id)
			x.NoError(err, "the Note is still a row")
			x.NotNil(row.DateErased)

			v, err := getBook(t, s, book)
			x.NoError(err)
			x.Equal(note.GetId(), v.GetPinned().GetId(), "the Book still names it")
		})
	})

	// What Erase answers is the same for an edge that says nothing and is
	// required, whose key the fork gives NO ACTION: it was the store's own
	// error, under Unknown.
	t.Run("a required edge that says nothing", func(t *testing.T) {
		stores(t, func(t *testing.T, db *ent.Client) {
			x := require.New(t)
			ctx := t.Context()
			s, err := bare.NewServer(db)
			x.NoError(err)

			tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
			x.NoError(err)
			user, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
			x.NoError(err)
			t.Cleanup(func() {
				_, _ = s.User().Erase(t.Context(), user.Ref())
				_, _ = s.Tenant().Erase(t.Context(), tenant.Ref())
			})

			_, err = s.Tenant().Erase(ctx, tenant.Ref())
			st, _ := status.FromError(err)
			x.Equal(codes.FailedPrecondition, st.Code(), st.Message())
			x.Equal("Tenant is still named by another row and cannot be erased", st.Message())
			for _, leak := range []string{"FOREIGN", "foreign", "constraint", "user", "pq:", "sqlite"} {
				x.False(strings.Contains(st.Message(), leak), "%q in %q", leak, st.Message())
			}
		})
	})
}
