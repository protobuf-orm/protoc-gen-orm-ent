package bare_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/user"
)

// A row answers with its neighbour's identifier without anybody having loaded
// the neighbour.
//
// The key is in the column the edge is kept in, so the row is holding it
// already -- the read that fetched the row fetched it. What used to happen is
// that ent kept it in an unexported member, so `Proto()` could only answer with
// an edge somebody had eagerly loaded, and a read that loaded none answered
// `tenant: null`: not the tenant, and not even which tenant.
//
// The difference matters to whatever keeps its own copy of both rows. It does
// not want the tenant nested inside the user; it wants to hold the two
// separately and join them itself, and for that it needs the reference. Without
// it, learning who a row's neighbour is costs a second read **per row**.
func TestAnEdgeAnswersWithItsKeyUnloaded(t *testing.T) {
	x := require.New(t)
	s := NewServer(t)
	ctx := t.Context()

	// The harness shares one in-memory database between tests, so the aliases
	// have to differ.
	alias := t.Name()

	tenant, err := s.Db.Tenant.Create().
		SetId(uuid.New()).
		SetAlias(alias).
		Save(ctx)
	x.NoError(err)

	created, err := s.Db.User.Create().
		SetId(uuid.New()).
		SetTenantId(tenant.Id).
		SetAlias("root-" + alias).
		SetDateUpdated(time.Now()).
		SetDateCreated(time.Now()).
		Save(ctx)
	x.NoError(err)

	// Read back with nothing eager-loaded, which is what a list does.
	got, err := s.Db.User.Query().Where(user.Id(created.Id)).Only(ctx)
	x.NoError(err)
	x.Nil(got.Edges.Tenant, "the test is about the case where nobody loaded it")

	v := got.Proto()
	x.Equal(tenant.Id[:], v.GetTenant().GetId())

	// And it is a reference rather than a half-loaded row: the identifier, and
	// nothing the query never asked for.
	x.Empty(v.GetTenant().GetAlias())
}

// And the loaded edge still wins, because a row that was asked for whole
// answers whole.
func TestALoadedEdgeIsStillTheWholeThing(t *testing.T) {
	x := require.New(t)
	s := NewServer(t)
	ctx := t.Context()

	// The harness shares one in-memory database between tests, so the aliases
	// have to differ.
	alias := t.Name()

	tenant, err := s.Db.Tenant.Create().
		SetId(uuid.New()).
		SetAlias(alias).
		Save(ctx)
	x.NoError(err)

	created, err := s.Db.User.Create().
		SetId(uuid.New()).
		SetTenantId(tenant.Id).
		SetAlias("root-" + alias).
		SetDateUpdated(time.Now()).
		SetDateCreated(time.Now()).
		Save(ctx)
	x.NoError(err)

	got, err := s.Db.User.Query().Where(user.Id(created.Id)).WithTenant().Only(ctx)
	x.NoError(err)

	v := got.Proto()
	x.Equal(tenant.Id[:], v.GetTenant().GetId())
	x.Equal(alias, v.GetTenant().GetAlias())
}
