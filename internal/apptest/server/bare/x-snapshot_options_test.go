package bare_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lesomnus/z"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/stretchr/testify/require"
)

// This implements only the original Recorder interface.
type metadataRecorder func(bare.Change)

func (r metadataRecorder) Record(_ context.Context, _ bare.Server, c bare.Change) error {
	r(c)
	return nil
}

type snapshotRecorder struct {
	bare.Recorder
	options bare.SnapshotOptions
}

func (r snapshotRecorder) Snapshots(string) bare.SnapshotOptions { return r.options }

func snapshotDB(t testing.TB, postgres bool, opts ...ent.Option) *ent.Client {
	t.Helper()
	name, dsn := dialect.SQLite, ":memory:?_fk=1"
	if postgres {
		name, dsn = dialect.Postgres, os.Getenv(Postgres)
		if dsn == "" {
			t.Skip("APPTEST_POSTGRES is not set")
		}
	}
	drv, err := entsql.Open(name, dsn)
	require.NoError(t, err)
	drv.DB().SetMaxOpenConns(1)
	db := ent.NewClient(append(opts, ent.Driver(drv))...)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.Schema.Create(context.Background()))
	return db
}

func TestRecorderSnapshotCosts(t *testing.T)         { testSnapshotCosts(t, false) }
func TestPostgresRecorderSnapshotCosts(t *testing.T) { testSnapshotCosts(t, true) }

func testSnapshotCosts(t *testing.T, postgres bool) {
	for _, mode := range []string{"metadata", "value", "both"} {
		t.Run(mode, func(t *testing.T) {
			x := require.New(t)
			var log []string
			db := snapshotDB(t, postgres, ent.Debug(), ent.Log(func(v ...any) { log = append(log, fmt.Sprint(v...)) }))
			var changes []bare.Change
			var rec bare.Recorder = metadataRecorder(func(c bare.Change) { changes = append(changes, c) })
			options := bare.SnapshotOptions{Value: mode != "metadata", Previous: mode == "both"}
			if mode != "metadata" {
				rec = snapshotRecorder{rec, options}
			}
			// Aggregation, including nested groups, must preserve opt-in requests.
			s, err := bare.NewServer(db, bare.WithRecorder(bare.Recorders{bare.Recorders{rec}}))
			x.NoError(err)
			check := func(selects, locks int) {
				t.Helper()
				var n, l int
				for _, line := range log {
					if strings.Contains(line, "query=SELECT") {
						n++
					}
					if strings.Contains(line, "FOR UPDATE") {
						l++
					}
				}
				x.Equal(selects, n, strings.Join(log, "\n"))
				if !postgres {
					locks = 0
				}
				x.Equal(locks, l, strings.Join(log, "\n"))
				log = nil
			}
			log = nil
			v, err := s.Tenant().Add(t.Context(), pb.TenantAddRequest_builder{Name: z.Ptr("before")}.Build())
			x.NoError(err)
			wantReads := 0
			if options.Value {
				wantReads = 1
			}
			check(wantReads, 0) // Postimages never request a preceding row lock.
			_, err = s.Tenant().Patch(t.Context(), pb.TenantPatchRequest_builder{Ref: v.Ref(), Name: z.Ptr("after")}.Build())
			x.NoError(err)
			wantReads, locks := 1, 0
			if options.Previous {
				wantReads++
				locks++
			}
			check(wantReads, locks)
			_, err = s.Tenant().Erase(t.Context(), v.Ref())
			x.NoError(err)
			check(1, locks) // Metadata-only delete still reads only its key.
			x.Len(changes, 3)
			x.Nil(changes[0].Previous)
			x.Equal(options.Value, changes[0].Value != nil)
			x.Equal(options.Value, changes[1].Value != nil)
			x.Equal(options.Previous, changes[1].Previous != nil)
			x.Equal(options.Previous, changes[2].Previous != nil)
			x.Nil(changes[2].Value)
		})
	}
}

func TestRecorderSnapshotRequirementsUnion(t *testing.T) {
	noop := metadataRecorder(func(bare.Change) {})
	rs := bare.Recorders{noop, bare.Recorders{snapshotRecorder{noop, bare.SnapshotOptions{Previous: true}}}, snapshotRecorder{noop, bare.SnapshotOptions{Value: true}}}
	require.Equal(t, bare.SnapshotOptions{Previous: true, Value: true}, rs.Snapshots(pb.TenantService_Patch_FullMethodName))
}

// Measures only the ORM write path: one row, no edges, no audit/outbox writes,
// no subscribers and no lock contention. Use APPTEST_POSTGRES for the network DB.
func BenchmarkRecorderSnapshotsPatch(b *testing.B) {
	for _, mode := range []string{"metadata", "value", "both"} {
		b.Run(mode, func(b *testing.B) {
			db := snapshotDB(b, os.Getenv(Postgres) != "")
			var rec bare.Recorder = metadataRecorder(func(bare.Change) {})
			if mode != "metadata" {
				rec = snapshotRecorder{rec, bare.SnapshotOptions{Value: true, Previous: mode == "both"}}
			}
			s, err := bare.NewServer(db, bare.WithRecorder(rec))
			require.NoError(b, err)
			ctx := context.Background()
			v, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{Name: z.Ptr("before")}.Build())
			require.NoError(b, err)
			b.Cleanup(func() { _, err := s.Tenant().Erase(ctx, v.Ref()); require.NoError(b, err) })
			req := pb.TenantPatchRequest_builder{Ref: v.Ref(), Name: z.Ptr("after")}.Build()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.Tenant().Patch(ctx, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
