package bare_test

import (
	"context"
	"fmt"
	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"os"
	"testing"

	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRecorderSnapshots(t *testing.T) {
	t.Run("successive mutations retain their own images including deleted rows", R(func(ctx context.Context, x *require.Assertions, c *Client, r *recorder) {
		tenant := seedTenant(ctx, x, c, r)
		first, err := c.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Name: z.Ptr("first")}.Build())
		x.NoError(err)
		second, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{Ref: first.Ref(), DateUpdated: first.GetDateUpdated(), Name: z.Ptr("second")}.Build())
		x.NoError(err)
		third, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{Ref: first.Ref(), DateUpdated: second.GetDateUpdated(), Name: z.Ptr("third")}.Build())
		x.NoError(err)
		_, err = c.User().Erase(ctx, first.Ref())
		x.NoError(err)
		x.Len(r.Changes, 4)
		first.SetTenant(pb.Tenant_builder{Id: tenant.GetId()}.Build())
		second.SetTenant(pb.Tenant_builder{Id: tenant.GetId()}.Build())
		third.SetTenant(pb.Tenant_builder{Id: tenant.GetId()}.Build())
		x.Nil(r.Changes[0].Previous)
		x.True(proto.Equal(first, r.Changes[0].Value))
		x.True(proto.Equal(first, r.Changes[1].Previous), "first=%s previous=%s", first, r.Changes[1].Previous)
		x.True(proto.Equal(second, r.Changes[1].Value))
		x.True(proto.Equal(second, r.Changes[2].Previous))
		x.True(proto.Equal(third, r.Changes[2].Value))
		x.True(proto.Equal(third, r.Changes[3].Previous))
		x.Nil(r.Changes[3].Value)
		x.Equal(tenant.GetId(), r.Changes[3].Previous.(*pb.User).GetTenant().GetId())
	}))
}

// The preimage must be read under the same row lock as the write. A plain SELECT
// followed by UPDATE lets concurrent writers report the same Previous value.
func TestPostgresRecorderSnapshotsSerializeWriters(t *testing.T) {
	dsn := os.Getenv(Postgres)
	if dsn == "" {
		t.Skip("APPTEST_POSTGRES is not set")
	}
	x := require.New(t)
	ctx := t.Context()
	drv, err := entsql.Open(dialect.Postgres, dsn)
	x.NoError(err)
	t.Cleanup(func() { drv.Close() })
	db := ent.NewClient(ent.Driver(drv))
	x.NoError(db.Schema.Create(ctx))
	r := &recorder{}
	s, err := bare.NewServer(db, bare.WithRecorder(r))
	x.NoError(err)
	tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)
	user, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref(), Name: z.Ptr("initial")}.Build())
	x.NoError(err)
	t.Cleanup(func() {
		_, _ = s.User().Erase(context.Background(), user.Ref())
		_, _ = s.Tenant().Erase(context.Background(), tenant.Ref())
	})
	r.Changes = nil
	const count = 16
	done := make(chan error, count)
	start := make(chan struct{})
	for i := range count {
		request := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str(fmt.Sprint(i))))
		request.SetRef(user.Ref())
		go func() { <-start; _, err := s.User().Apply(ctx, request); done <- err }()
	}
	close(start)
	for range count {
		x.NoError(<-done)
	}
	x.Len(r.Changes, count)
	previous := "initial"
	for _, c := range r.Changes {
		x.Equal(previous, c.Previous.(*pb.User).GetName())
		previous = c.Value.(*pb.User).GetName()
	}
	current, err := s.User().Get(ctx, user.Ref().Pick())
	x.NoError(err)
	x.Equal(previous, current.GetName())
}
