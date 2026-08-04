package bare_test

import (
	"context"
	"testing"

	"github.com/lesomnus/protobuf-patch/patch"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"github.com/stretchr/testify/require"
)

// Apply writes and reads back inside one transaction of its own. A caller that
// has already opened one -- to make this write and something else a single
// write -- must not have it committed or thrown away underneath them.
//
// These go through the servers rather than a gRPC client, because what is being
// asked about is which driver the servers run on.

// seedAt adds a tenant and a user through the given server.
func seedAt(ctx context.Context, x *require.Assertions, s pb.Server) *pb.User {
	tenant, err := s.Tenant().Add(ctx, pb.TenantAddRequest_builder{}.Build())
	x.NoError(err)

	u, err := s.User().Add(ctx, pb.UserAddRequest_builder{Tenant: tenant.Ref()}.Build())
	x.NoError(err)

	return u
}

// nameAt reads one user's name through the given server.
func nameAt(ctx context.Context, x *require.Assertions, s pb.Server, u *pb.User) string {
	v, err := s.User().Get(ctx, u.Ref().Pick().WithSelect(func(sel *pb.UserSelect) {
		sel.SetAll(true)
	}))
	x.NoError(err)
	return v.GetName()
}

// setName is an Apply that assigns one name.
func setName(x *require.Assertions, u *pb.User, name string) *pb.UserApplyRequest {
	req := doc(x, patch.Target(patch.Name("name")).Assign(patch.Str(name)))
	req.SetRef(u.Ref())
	return req
}

func TestApplyInAnOpenTransaction(t *testing.T) {
	ctx := context.Background()

	// The connection is single and shared, so nothing may be read from outside
	// the transaction while it is open: it would wait for a connection the
	// transaction is holding. Every check below reads inside, or after the end.

	t.Run("a rollback outside undoes what it wrote", func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()
		x := require.New(t)

		u := seedAt(ctx, x, s)

		drv, tx, err := enttx.Begin(ctx, s.Driver)
		x.NoError(err)

		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		got, err := sv.User().Apply(ctx, setName(x, u, "Ada"))
		x.NoError(err)
		// It read back its own write, so it did not quietly do nothing.
		x.Equal("Ada", got.GetName())
		x.Equal("Ada", nameAt(ctx, x, sv, u))

		x.NoError(tx.Rollback())

		// And outside, it never happened -- which it could not have said if
		// Apply had committed the transaction it was handed.
		x.Equal("", nameAt(ctx, x, s, u))
	})

	t.Run("a commit outside is what makes it stick", func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()
		x := require.New(t)

		u := seedAt(ctx, x, s)

		drv, tx, err := enttx.Begin(ctx, s.Driver)
		x.NoError(err)

		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		_, err = sv.User().Apply(ctx, setName(x, u, "Ada"))
		x.NoError(err)

		x.NoError(tx.Commit())
		x.Equal("Ada", nameAt(ctx, x, s, u))
	})

	t.Run("a later failure takes the whole thing down", func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()
		x := require.New(t)

		u := seedAt(ctx, x, s)

		drv, tx, err := enttx.Begin(ctx, s.Driver)
		x.NoError(err)

		sv, err := enttx.Rebind(pb.Server(s.Server), drv)
		x.NoError(err)

		// Two writes the caller means as one.
		_, err = sv.User().Apply(ctx, setName(x, u, "Ada"))
		x.NoError(err)
		_, err = sv.User().Apply(ctx, setName(x, u, "Grace"))
		x.NoError(err)
		x.Equal("Grace", nameAt(ctx, x, sv, u))

		// The caller decides the pair did not hold.
		x.NoError(tx.Rollback())
		x.Equal("", nameAt(ctx, x, s, u))
	})

	// Ent refuses to nest a transaction of its own, so a caller that opened one
	// the Ent way hands down a client that is already bound to it. Apply has to
	// notice rather than fail.
	t.Run("one opened Ent's own way is joined too", func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()
		x := require.New(t)

		u := seedAt(ctx, x, s)

		tx, err := s.Db.Tx(ctx)
		x.NoError(err)

		sv, err := bare.NewServer(tx.Client())
		x.NoError(err)

		got, err := sv.User().Apply(ctx, setName(x, u, "Ada"))
		x.NoError(err)
		x.Equal("Ada", got.GetName())

		x.NoError(tx.Rollback())
		x.Equal("", nameAt(ctx, x, s, u))
	})

	// Nothing above should have changed what Apply does when it is the only one
	// there: it still commits on its own.
	t.Run("on its own it still commits", func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()
		x := require.New(t)

		u := seedAt(ctx, x, s)

		_, err := s.User().Apply(ctx, setName(x, u, "Ada"))
		x.NoError(err)
		x.Equal("Ada", nameAt(ctx, x, s, u))
	})
}
