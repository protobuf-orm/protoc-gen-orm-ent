package bare_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	_ "github.com/mattn/go-sqlite3"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// stores runs `run` once on each store these tests run against: SQLite always,
// and PostgreSQL when [Postgres] names one. Each is handed a client of its own,
// with the schema created; on PostgreSQL the rows outlive the run, so what a
// test writes there is named by [unique].
func stores(t *testing.T, run func(t *testing.T, db *ent.Client)) {
	t.Run("sqlite", func(t *testing.T) {
		drv, err := entsql.Open(dialect.SQLite, "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1")
		require.NoError(t, err)
		drv.DB().SetMaxOpenConns(1)
		t.Cleanup(func() { drv.Close() })

		db := ent.NewClient(ent.Driver(drv))
		require.NoError(t, db.Schema.Create(t.Context()))
		run(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv(Postgres)
		if dsn == "" {
			t.Skipf("%s is not set; see the const for what it wants", Postgres)
		}
		drv, err := entsql.Open(dialect.Postgres, dsn)
		require.NoError(t, err)
		t.Cleanup(func() { drv.Close() })

		db := ent.NewClient(ent.Driver(drv))
		require.NoError(t, db.Schema.Create(t.Context()))
		run(t, db)
	})
}

// unique is a name no earlier run has used, for a store whose rows outlive it.
func unique(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func hasMethod(v any, name string) bool {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Interface {
		t = t.Elem()
	}
	_, ok := t.MethodByName(name)
	return ok
}

// admitted keeps what admission was asked about, and admits all of it.
type admitted struct {
	mu      sync.Mutex
	Changes []bare.Change
}

func (a *admitted) Admit(_ context.Context, c bare.Change) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Changes = append(a.Changes, c)
	return nil
}

// Tenant asks for Patch and not for Apply. It has no Apply, and its Patch runs
// the write Apply runs elsewhere, so a recorder and admission are told of a
// Patch and of nothing else.
func TestVerbPatchWithoutApply(t *testing.T) {
	x := require.New(t)
	x.True(hasMethod((*pb.TenantServiceServer)(nil), "Patch"))
	x.False(hasMethod((*pb.TenantServiceServer)(nil), "Apply"), "the contract has no Apply")
	x.False(hasMethod(bare.TenantServiceServer{}, "Apply"), "the server writes no Apply")
	x.False(hasMethod((*pb.TenantServiceClient)(nil), "Apply"))

	stores(t, func(t *testing.T, db *ent.Client) {
		x := require.New(t)
		ctx := t.Context()

		rec := &recorder{Images: bare.SnapshotOptions{Previous: true, Value: true}}
		adm := &admitted{}
		s, err := bare.NewServer(db, bare.WithRecorder(rec), bare.WithAdmission(adm, "apptest.Tenant"))
		x.NoError(err)

		v, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{Alias: z.Ptr(unique("before"))}.Build())
		x.NoError(err)
		t.Cleanup(func() { _, _ = s.Tenant().Erase(context.Background(), v.Ref()) })

		after := unique("after")
		out, err := s.Tenant().Patch(ctx, pb.TenantPatchRequest_builder{
			Ref:   v.Ref(),
			Alias: z.Ptr(after),
		}.Build())
		x.NoError(err)
		x.Equal(after, out.GetAlias())

		got, err := s.Tenant().Get(ctx, v.Ref().Pick())
		x.NoError(err)
		x.Equal(after, got.GetAlias())

		last := rec.Changes[len(rec.Changes)-1]
		x.Equal(pb.TenantService_Patch_FullMethodName, last.By)
		x.NotNil(last.Patch, "a Patch is recorded as the document it converts to")
		x.Equal(after, last.Value.(*pb.Tenant).GetAlias())
		x.NotEqual(after, last.Previous.(*pb.Tenant).GetAlias())

		bys := []string{}
		for _, c := range adm.Changes {
			bys = append(bys, c.By)
		}
		x.Equal([]string{pb.TenantService_Add_FullMethodName, pb.TenantService_Patch_FullMethodName}, bys)
	})
}

// Tally asks for Apply and not for Patch. It has no Patch and no PatchRequest,
// and its Apply writes, records and is admitted as an Apply.
func TestVerbApplyWithoutPatch(t *testing.T) {
	x := require.New(t)
	x.True(hasMethod((*pb.TallyServiceServer)(nil), "Apply"))
	x.False(hasMethod((*pb.TallyServiceServer)(nil), "Patch"), "the contract has no Patch")
	x.False(hasMethod(bare.TallyServiceServer{}, "Patch"), "the server writes no Patch")
	x.False(hasMethod((*pb.TallyServiceClient)(nil), "Patch"))
	x.Nil(pb.File_apptest_shift_proto.Messages().ByName("TallyPatchRequest"))

	stores(t, func(t *testing.T, db *ent.Client) {
		x := require.New(t)
		ctx := t.Context()

		rec := &recorder{Images: bare.SnapshotOptions{Previous: true, Value: true}}
		adm := &admitted{}
		s, err := bare.NewServer(db, bare.WithRecorder(rec), bare.WithAdmission(adm, "apptest.Tally"))
		x.NoError(err)

		v, err := s.Tally().Add(ctx, pb.TallyAddRequest_builder{
			Id:     unique("tally"),
			Counts: map[string]int64{"a": 1, "b": 2},
		}.Build())
		x.NoError(err)
		t.Cleanup(func() { _, _ = s.Tally().Erase(context.Background(), v.Ref()) })

		p, err := patch.New("apptest.Tally", patch.Target(patch.MapStr("c")).In(patch.Name("counts")).Assign(patch.Int64(3)))
		x.NoError(err)
		out, err := s.Tally().Apply(ctx, pb.TallyApplyRequest_builder{Ref: v.Ref(), Patch: p}.Build())
		x.NoError(err)
		x.Equal(map[string]int64{"a": 1, "b": 2, "c": 3}, out.GetCounts())

		got, err := s.Tally().Get(ctx, v.Ref().Pick())
		x.NoError(err)
		x.Equal(map[string]int64{"a": 1, "b": 2, "c": 3}, got.GetCounts())

		last := rec.Changes[len(rec.Changes)-1]
		x.Equal(pb.TallyService_Apply_FullMethodName, last.By)
		x.Equal(map[string]int64{"a": 1, "b": 2}, last.Previous.(*pb.Tally).GetCounts())

		bys := []string{}
		for _, c := range adm.Changes {
			bys = append(bys, c.By)
		}
		x.Equal([]string{pb.TallyService_Add_FullMethodName, pb.TallyService_Apply_FullMethodName}, bys)
	})
}
