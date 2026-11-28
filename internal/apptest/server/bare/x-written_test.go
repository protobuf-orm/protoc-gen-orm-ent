package bare_test

import (
	"context"
	"testing"
	"time"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"uuid"

	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entuuid"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// fine is a time with nanoseconds in it, which is what a clock on Linux
// answers and what PostgreSQL's `timestamptz` does not keep.
var fine = time.Date(2026, 11, 28, 9, 30, 0, 123456789, time.UTC)

// kept is `fine` as the database keeps it: cut to the microsecond.
var kept = time.Date(2026, 11, 28, 9, 30, 0, 123456000, time.UTC)

// A time these servers write is cut to the microsecond on every database, so
// that an Add answers what a read gives. An Add answers the row as it was built
// -- only its key is read back -- and on PostgreSQL its version used to carry
// nanoseconds no read would give again; a version taken from that answer and
// compared in Go against the row read back was refused (lesomnus/payday#324).
// SQLite keeps nanoseconds, so a suite on SQLite never saw it, and now sees
// what PostgreSQL would.
//
// What is compared is not cut: a version a Patch tests, a reference. The last
// case is a row stored before this, with nanoseconds, which is still read with
// them and patched with them as its test.
func TestATimeWrittenIsWhatTheDatabaseKeeps(t *testing.T) {
	stores(t, func(t *testing.T, db *ent.Client) {
		ctx := t.Context()
		s, err := bare.NewServer(db, bare.WithClock(func() time.Time { return fine }))
		require.NoError(t, err)

		note := func(x *require.Assertions, req *pb.NoteAddRequest) *pb.Note {
			name := unique("written")
			req.SetAlias(name)
			req.SetBody(name)
			req.SetSlug(name)
			v, err := s.Note().Add(ctx, req)
			x.NoError(err)
			t.Cleanup(func() {
				if k, err := entuuid.FromBytes(v.GetId()); err == nil {
					_ = db.Note.DeleteOneId(k).Exec(context.Background())
				}
			})
			return v
		}
		get := func(x *require.Assertions, v *pb.Note) *pb.Note {
			got, err := s.Note().Get(ctx, v.Ref().Pick())
			x.NoError(err)
			return got
		}

		t.Run("an Add answers the stamps a Get answers", func(t *testing.T) {
			x := require.New(t)
			v := note(x, pb.NoteAddRequest_builder{}.Build())
			got := get(x, v)

			x.True(kept.Equal(v.GetDateUpdated().AsTime()), "date_updated is %s", v.GetDateUpdated().AsTime())
			x.True(proto.Equal(got.GetDateUpdated(), v.GetDateUpdated()), "an Add answered %s and a Get %s",
				v.GetDateUpdated().AsTime(), got.GetDateUpdated().AsTime())
			x.True(proto.Equal(got.GetDateCreated(), v.GetDateCreated()), "an Add answered %s and a Get %s",
				v.GetDateCreated().AsTime(), got.GetDateCreated().AsTime())

			row, err := db.Note.Get(ctx, mustId(x, v.GetId()))
			x.NoError(err)
			x.True(kept.Equal(row.DateUpdated), "stored %s", row.DateUpdated)
		})

		t.Run("a Patch whose test is an Add's answer holds", func(t *testing.T) {
			x := require.New(t)
			v := note(x, pb.NoteAddRequest_builder{}.Build())

			u, err := s.Note().Patch(ctx, pb.NotePatchRequest_builder{
				Ref:         v.Ref(),
				Body:        z.Ptr(unique("patched")),
				DateUpdated: v.GetDateUpdated(),
			}.Build())
			x.NoError(err)
			x.True(proto.Equal(get(x, v).GetDateUpdated(), u.GetDateUpdated()))
		})

		t.Run("a time handed to an Add is answered as a Get answers it", func(t *testing.T) {
			x := require.New(t)
			v := note(x, pb.NoteAddRequest_builder{DateCreated: timestamppb.New(fine.Add(-time.Hour))}.Build())

			x.True(kept.Add(-time.Hour).Equal(v.GetDateCreated().AsTime()), "date_created is %s", v.GetDateCreated().AsTime())
			x.True(proto.Equal(get(x, v).GetDateCreated(), v.GetDateCreated()))

			desk := unique("desk")
			w, err := s.Shift().Add(ctx, pb.ShiftAddRequest_builder{Desk: desk, StartsAt: timestamppb.New(fine)}.Build())
			x.NoError(err)
			t.Cleanup(func() { _, _ = s.Shift().Erase(context.Background(), w.Ref()) })
			x.True(kept.Equal(w.GetStartsAt().AsTime()), "starts_at is %s", w.GetStartsAt().AsTime())

			got, err := s.Shift().Get(ctx, w.Ref().Pick())
			x.NoError(err)
			x.True(proto.Equal(got.GetStartsAt(), w.GetStartsAt()))
		})

		t.Run("a time a patch assigns is written as an Add writes it", func(t *testing.T) {
			x := require.New(t)
			w, err := s.Shift().Add(ctx, pb.ShiftAddRequest_builder{Desk: unique("desk"), StartsAt: timestamppb.New(kept)}.Build())
			x.NoError(err)
			t.Cleanup(func() { _, _ = s.Shift().Erase(context.Background(), w.Ref()) })

			req := pb.ShiftPatchRequest_builder{Ref: w.Ref(), StartsAt: timestamppb.New(fine.Add(time.Hour))}.Build()
			u, err := s.Shift().Patch(ctx, req)
			x.NoError(err)
			x.True(kept.Add(time.Hour).Equal(u.GetStartsAt().AsTime()), "starts_at is %s", u.GetStartsAt().AsTime())
			x.True(fine.Add(time.Hour).Equal(req.GetStartsAt().AsTime()), "the request was changed")

			row, err := db.Shift.Get(ctx, mustId(x, w.GetId()))
			x.NoError(err)
			x.True(kept.Add(time.Hour).Equal(row.StartsAt), "stored %s", row.StartsAt)
		})

		t.Run("an erase marks what the database keeps", func(t *testing.T) {
			x := require.New(t)
			v := note(x, pb.NoteAddRequest_builder{}.Build())
			_, err := s.Note().Erase(ctx, v.Ref())
			x.NoError(err)

			row, err := db.Note.Get(ctx, mustId(x, v.GetId()))
			x.NoError(err)
			x.NotNil(row.DateErased)
			x.True(kept.Equal(*row.DateErased), "date_erased is %s", row.DateErased)
			x.True(kept.Equal(row.DateUpdated), "date_updated is %s", row.DateUpdated)
		})

		// JSON, in a column that keeps every digit it is handed, on both.
		t.Run("a list of times is kept as it was handed over", func(t *testing.T) {
			x := require.New(t)
			v, err := s.ListField().Add(ctx, pb.ListFieldAddRequest_builder{
				Id:    z.Ptr(unique("list")),
				Times: []*timestamppb.Timestamp{timestamppb.New(fine)},
			}.Build())
			x.NoError(err)
			t.Cleanup(func() { _, _ = s.ListField().Erase(context.Background(), v.Ref()) })

			got, err := s.ListField().Get(ctx, v.Ref().Pick().WithSelect(func(s *pb.ListFieldSelect) { s.SetAll(true) }))
			x.NoError(err)
			x.Len(got.GetTimes(), 1)
			x.True(fine.Equal(got.GetTimes()[0].AsTime()), "times[0] is %s", got.GetTimes()[0].AsTime())
		})

		// A row written before this, which on SQLite holds nanoseconds. It is
		// written through ent, as the servers wrote it then. A read answers it
		// as it is stored, and the version that read gives is a test that holds:
		// cutting what is compared would leave such a row unpatchable.
		// PostgreSQL has never held nanoseconds, so there the row is what a read
		// gives as well and the case is the one above.
		t.Run("a row stored with nanoseconds is read and patched with them", func(t *testing.T) {
			x := require.New(t)
			name := unique("old")
			old := fine.Add(-24 * time.Hour)
			row, err := db.Note.Create().
				SetId(uuid.New()).
				SetAlias(name).SetBody(name).SetSlug(name).
				SetDateCreated(old).
				SetDateUpdated(old).
				Save(ctx)
			x.NoError(err)
			t.Cleanup(func() { _ = db.Note.DeleteOneId(row.Id).Exec(context.Background()) })

			got, err := s.Note().Get(ctx, pb.NoteById(row.Id[:]).Pick())
			x.NoError(err)
			if db.Dialect() == dialect.SQLite {
				x.True(old.Equal(got.GetDateUpdated().AsTime()), "date_updated is %s", got.GetDateUpdated().AsTime())
			}

			u, err := s.Note().Patch(ctx, pb.NotePatchRequest_builder{
				Ref:         pb.NoteById(row.Id[:]),
				Body:        z.Ptr(unique("patched")),
				DateUpdated: got.GetDateUpdated(),
			}.Build())
			x.NoError(err)
			x.True(kept.Equal(u.GetDateUpdated().AsTime()), "date_updated is %s", u.GetDateUpdated().AsTime())
		})
	})
}
