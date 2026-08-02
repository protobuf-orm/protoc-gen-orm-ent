package bare_test

import (
	"context"
	"testing"
	"time"

	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// These tests pin down what Patch does TODAY, before it is rewritten to delegate
// to Apply. Every assertion here is a behavior somebody could be relying on; a
// change that breaks one is a change that has to be decided, not discovered.
//
// Patch had no test at all before this file.

// version reads the entity's version field, which is what a Patch has to echo
// back to be accepted.
func version(ctx context.Context, x *require.Assertions, c *Client, u *pb.User) *timestamppb.Timestamp {
	return get(ctx, x, c, u).GetDateUpdated()
}

func TestPatchVersion(t *testing.T) {
	// The version field is not optional input: omitting it without saying
	// "force" is a client mistake, not an unconditional write.
	t.Run("omitting the version without force is rejected", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:  u.Ref(),
			Name: z.Ptr("Ada"),
		}.Build())
		x.Equal(codes.InvalidArgument, status.Code(err))
		x.Contains(status.Convert(err).Message(), "date_updated")

		x.Empty(get(ctx, x, c, u).GetName(), "nothing may have been written")
	}))

	t.Run("a matching version lets the write through", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:         u.Ref(),
			Name:        z.Ptr("Ada"),
			DateUpdated: version(ctx, x, c, u),
		}.Build())
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	t.Run("a stale version is refused", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		stale := timestamppb.New(version(ctx, x, c, u).AsTime().Add(-time.Hour))

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:         u.Ref(),
			Name:        z.Ptr("Ada"),
			DateUpdated: stale,
		}.Build())
		x.Equal(codes.FailedPrecondition, status.Code(err))
		x.Empty(get(ctx, x, c, u).GetName())
	}))

	// A successful patch moves the version forward, which is what makes the
	// next patch with the old value fail.
	t.Run("a successful patch advances the version", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		before := version(ctx, x, c, u)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:         u.Ref(),
			Name:        z.Ptr("Ada"),
			DateUpdated: before,
		}.Build())
		x.NoError(err)
		x.True(version(ctx, x, c, u).AsTime().After(before.AsTime()))
	}))

	t.Run("force writes without a version", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		before := version(ctx, x, c, u)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Name:             z.Ptr("Ada"),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
		x.True(version(ctx, x, c, u).AsTime().After(before.AsTime()))
	}))

	// Force plus an explicit version is the one path that lets a client choose
	// the stored version rather than having the server stamp now(). Replaying a
	// recorded change needs exactly this.
	t.Run("force with a version stores the given version", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		want := timestamppb.New(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC))

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Name:             z.Ptr("Ada"),
			DateUpdated:      want,
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal(want.AsTime().UTC(), version(ctx, x, c, u).AsTime().UTC())
	}))

	t.Run("force on a missing row is NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              pb.UserById(make([]byte, 16)),
			Name:             z.Ptr("Ada"),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))

	// Today a missing row and a stale version are indistinguishable when not
	// forced: both are zero rows and the code reports the version. This is
	// recorded as-is, not endorsed.
	t.Run("a missing row without force reports the version, not NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:         pb.UserById(make([]byte, 16)),
			Name:        z.Ptr("Ada"),
			DateUpdated: timestamppb.Now(),
		}.Build())
		x.Equal(codes.FailedPrecondition, status.Code(err))
	}))
}

func TestPatchFields(t *testing.T) {
	t.Run("a scalar is assigned", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Name:             z.Ptr("Ada"),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal("Ada", get(ctx, x, c, u).GetName())
	}))

	// Presence decides, not truthiness: an explicitly-set empty string clears
	// the name, and an omitted one leaves it alone.
	t.Run("an explicit zero value is assigned", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Name: z.Ptr("Ada"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		_, err = c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Name: z.Ptr(""), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Empty(get(ctx, x, c, u).GetName())
	}))

	t.Run("an omitted field is left alone", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Name: z.Ptr("Ada"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		_, err = c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Alias: z.Ptr("ada"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		got := get(ctx, x, c, u)
		x.Equal("Ada", got.GetName())
		x.Equal("ada", got.GetAlias())
	}))

	// A map is replaced wholesale -- this is the limitation Apply exists to
	// remove, and it must keep working until Patch is retired.
	t.Run("a map is replaced wholesale", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod", "team": "infra"})

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Labels:           map[string]string{"tier": "gold"},
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal(map[string]string{"tier": "gold"}, get(ctx, x, c, u).GetLabels())
	}))

	// An empty map is indistinguishable from an omitted one, so a map cannot be
	// emptied through Patch. Recorded because the converted path must not
	// accidentally start clearing it.
	t.Run("an empty map is a no-op, not a clear", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, map[string]string{"env": "prod"})

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Labels:           map[string]string{},
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal(map[string]string{"env": "prod"}, get(ctx, x, c, u).GetLabels())
	}))
}

func TestPatchNullable(t *testing.T) {
	t.Run("a nullable field is set", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Lock: z.Ptr("held"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal("held", get(ctx, x, c, u).GetLock())
	}))

	t.Run("the null flag clears it", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), Lock: z.Ptr("held"), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		_, err = c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref: u.Ref(), LockNull: z.Ptr(true), DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.False(get(ctx, x, c, u).HasLock())
	}))

	// The generator emits `if null { clear } else if has { set }`, so the flag
	// wins when both are given.
	t.Run("the null flag wins over a value", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Lock:             z.Ptr("held"),
			LockNull:         z.Ptr(true),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.False(get(ctx, x, c, u).HasLock())
	}))
}

func TestPatchEdge(t *testing.T) {
	t.Run("an edge is repointed by key", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		other, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		_, err = c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Tenant:           other.Ref(),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)

		got, err := c.User().Get(ctx, u.Ref().Pick())
		x.NoError(err)
		x.Equal(other.GetId(), got.GetTenant().GetId())
	}))

	// TenantGetKey short-circuits when the ref carries the key, so a bogus id
	// is not caught while resolving -- it reaches the statement and the foreign
	// key rejects it. Patch does not map constraint errors the way Add does, so
	// what surfaces is the raw one. Recorded as "an error", because the code it
	// carries today is an accident rather than a decision.
	t.Run("an edge to a missing target fails", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Tenant:           pb.TenantById(make([]byte, 16)),
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.Error(err)

		got, err := c.User().Get(ctx, u.Ref().Pick())
		x.NoError(err)
		x.NotEqual(make([]byte, 16), got.GetTenant().GetId(), "the edge must not have moved")
	}))
}

// Tenant has no version field, so it takes neither a version nor a force flag,
// and a missing row is reported as such.
func TestPatchWithoutVersion(t *testing.T) {
	t.Run("a patch needs no version", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		v, err := c.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
		x.NoError(err)

		_, err = c.Tenant().Patch(ctx, pb.TenantPatchRequest_builder{
			Ref:  v.Ref(),
			Name: z.Ptr("Acme"),
		}.Build())
		x.NoError(err)

		got, err := c.Tenant().Get(ctx, v.Ref().Pick().WithSelect(func(s *pb.TenantSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		x.Equal("Acme", got.GetName())
	}))

	t.Run("a missing row is NotFound", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.Tenant().Patch(ctx, pb.TenantPatchRequest_builder{
			Ref:  pb.TenantById(make([]byte, 16)),
			Name: z.Ptr("Acme"),
		}.Build())
		x.Equal(codes.NotFound, status.Code(err))
	}))
}

// ValueField carries every scalar type twice -- once with implicit presence and
// once with explicit -- so it is the type surface a rewrite is most likely to
// break.
func TestPatchValueField(t *testing.T) {
	// Both bytes columns are NOT NULL with no default, and a bytes field with
	// implicit presence is "set" only when it is non-empty -- an empty one
	// inserts NULL and trips the constraint. See TestRequiredBytesOmitted.
	add := func(ctx context.Context, x *require.Assertions, c *Client) {
		_, err := c.ValueField().Add(ctx, pb.ValueFieldAddRequest_builder{
			Id:                     z.Ptr("v1"),
			ImplicitBytes:          []byte{0x00},
			ImplicitImmutableBytes: []byte{0x01},
		}.Build())
		x.NoError(err)
	}
	read := func(ctx context.Context, x *require.Assertions, c *Client) *pb.ValueField {
		v, err := c.ValueField().Get(ctx, pb.ValueFieldGetById("v1").WithSelect(func(s *pb.ValueFieldSelect) {
			s.SetAll(true)
		}))
		x.NoError(err)
		return v
	}

	t.Run("implicit-presence scalars of every width", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		add(ctx, x, c)

		_, err := c.ValueField().Patch(ctx, pb.ValueFieldPatchRequest_builder{
			Ref:            pb.ValueFieldById("v1"),
			ImplicitI32:    z.Ptr(int32(-7)),
			ImplicitI64:    z.Ptr(int64(1 << 40)),
			ImplicitU32:    z.Ptr(uint32(7)),
			ImplicitU64:    z.Ptr(uint64(1 << 41)),
			ImplicitF32:    z.Ptr(float32(1.5)),
			ImplicitF64:    z.Ptr(3.5),
			ImplicitBool:   z.Ptr(true),
			ImplicitString: z.Ptr("hello"),
			ImplicitBytes:  []byte{0xde, 0xad},
			ImplicitEnum:   pb.Level_LEVEL_NORM.Enum(),
		}.Build())
		x.NoError(err)

		got := read(ctx, x, c)
		x.EqualValues(-7, got.GetImplicitI32())
		x.EqualValues(1<<40, got.GetImplicitI64())
		x.EqualValues(7, got.GetImplicitU32())
		x.EqualValues(1<<41, got.GetImplicitU64())
		x.InDelta(1.5, got.GetImplicitF32(), 1e-6)
		x.InDelta(3.5, got.GetImplicitF64(), 1e-9)
		x.True(got.GetImplicitBool())
		x.Equal("hello", got.GetImplicitString())
		x.Equal([]byte{0xde, 0xad}, got.GetImplicitBytes())
		x.Equal(pb.Level_LEVEL_NORM, got.GetImplicitEnum())
	}))

	t.Run("a repeated field is replaced wholesale", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		add(ctx, x, c)

		_, err := c.ValueField().Patch(ctx, pb.ValueFieldPatchRequest_builder{
			Ref:             pb.ValueFieldById("v1"),
			ImplicitI32S:    []int32{1, 2, 3},
			ImplicitStrings: []string{"a", "b"},
			ImplicitLevels:  []pb.Level{pb.Level_LEVEL_EASY, pb.Level_LEVEL_HARD},
		}.Build())
		x.NoError(err)

		got := read(ctx, x, c)
		x.Equal([]int32{1, 2, 3}, got.GetImplicitI32S())
		x.Equal([]string{"a", "b"}, got.GetImplicitStrings())
		x.Equal([]pb.Level{pb.Level_LEVEL_EASY, pb.Level_LEVEL_HARD}, got.GetImplicitLevels())

		_, err = c.ValueField().Patch(ctx, pb.ValueFieldPatchRequest_builder{
			Ref:          pb.ValueFieldById("v1"),
			ImplicitI32S: []int32{9},
		}.Build())
		x.NoError(err)
		x.Equal([]int32{9}, read(ctx, x, c).GetImplicitI32S())
	}))

	t.Run("an explicit-presence field is set then cleared", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		add(ctx, x, c)

		_, err := c.ValueField().Patch(ctx, pb.ValueFieldPatchRequest_builder{
			Ref:         pb.ValueFieldById("v1"),
			ExplicitI32: z.Ptr(int32(-7)),
		}.Build())
		x.NoError(err)
		x.EqualValues(-7, read(ctx, x, c).GetExplicitI32())

		_, err = c.ValueField().Patch(ctx, pb.ValueFieldPatchRequest_builder{
			Ref:             pb.ValueFieldById("v1"),
			ExplicitI32Null: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.False(read(ctx, x, c).HasExplicitI32())
	}))
}
