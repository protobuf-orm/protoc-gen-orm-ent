package apptest_test

import (
	"slices"
	"testing"

	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/migrate"
)

// TestAnIndexIsInTheOrderItDeclares: an index is read from its first column,
// so its columns are in the order its refs are, an edge's among the fields'
// -- `User.tenant_created` is `(tenant, date_created)`, and not the edge
// after every field, which is what ent makes of `Fields(...).Edges(...)`.
func TestAnIndexIsInTheOrderItDeclares(t *testing.T) {
	columns := func(table *schema.Table) [][]string {
		out := [][]string{}
		for _, i := range table.Indexes {
			cs := []string{}
			for _, c := range i.Columns {
				cs = append(cs, c.Name)
			}
			out = append(out, cs)
		}
		return out
	}

	user := columns(migrate.UserTable)
	require.True(t, slices.ContainsFunc(user, func(cs []string) bool { return slices.Equal(cs, []string{"tenant_id", "date_created"}) }), "user: %v", user)
	require.True(t, slices.ContainsFunc(user, func(cs []string) bool { return slices.Equal(cs, []string{"alias", "tenant_id"}) }), "user: %v", user)

	seat := columns(migrate.SeatTable)
	require.True(t, slices.ContainsFunc(seat, func(cs []string) bool { return slices.Equal(cs, []string{"holder_id", "org_id"}) }), "seat: %v", seat)
}
