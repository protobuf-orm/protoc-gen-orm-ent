package bare_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/protobuf-orm/protoc-gen-orm-ent/entpatch"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/stretchr/testify/require"
)

// A map key is whatever the caller put in the document, and it addresses a
// place inside a JSON column -- so it reaches the statement as a JSON path.
// Written into the statement text, a key carrying a quote closed the SQL
// literal and what followed was parsed as SQL. Bound, the worst a key can do is
// address nothing.
//
// These go through the whole RPC on purpose. The path is built in two places --
// the assignment and the guard predicate a remove needs -- and only one of them
// used to be reachable with an awkward key.
func TestJSONKeysAreData(t *testing.T) {
	// Each of these breaks either the SQL string literal, the JSON path
	// grammar, or both.
	for name, key := range map[string]string{
		"single quote":         "a'b",
		"double quote":         `a"b`,
		"backslash":            `a\b`,
		"closes and continues": `x"', ` + "`name`" + ` = 'PWNED' --`,
		"dot":                  "a.b",
		"bracket":              "a[0]",
		"looks quoted":         `"a"`,
		"looks indexed":        "[0]",
	} {
		t.Run(name, T(func(ctx context.Context, x *require.Assertions, c *Client) {
			u := seed(ctx, x, c, map[string]string{"env": "prod"})

			set := doc(x, patch.Target(patch.MapStr(key)).In(patch.Name("labels")).Assign(patch.Str("v")))
			set.SetRef(u.Ref())
			_, err := c.User().Apply(ctx, set)
			x.NoError(err)

			// It went in under the key as given, beside what was already there.
			x.Equal(map[string]string{"env": "prod", key: "v"}, get(ctx, x, c, u).GetLabels())

			// The other entry is untouched, which is what says nothing else in
			// the statement was reinterpreted.
			x.Equal("", get(ctx, x, c, u).GetName())

			// A test has to be able to ask about the same key the write
			// reached. It used to build the path a different way, so a key one
			// side could address the other could not.
			hold := doc(x,
				patch.Target(patch.MapStr(key)).In(patch.Name("labels")).Test(patch.Str("v")),
				patch.Target(patch.Name("name")).Assign(patch.Str("Ada")),
			)
			hold.SetRef(u.Ref())
			_, err = c.User().Apply(ctx, hold)
			x.NoError(err)
			x.Equal("Ada", get(ctx, x, c, u).GetName())

			// And remove, which carries an existence guard on that same path.
			del := doc(x, patch.Target(patch.MapStr(key)).In(patch.Name("labels")).Remove())
			del.SetRef(u.Ref())
			_, err = c.User().Apply(ctx, del)
			x.NoError(err)
			x.Equal(map[string]string{"env": "prod"}, get(ctx, x, c, u).GetLabels())
		}))
	}
}

// The same key through Patch, which converts a whole map rather than addressing
// one entry -- so it must survive the other binding too.
func TestJSONKeysThroughPatch(t *testing.T) {
	t.Run("a whole map with an awkward key", T(func(ctx context.Context, x *require.Assertions, c *Client) {
		u := seed(ctx, x, c, nil)
		want := map[string]string{"a'b": "1", `a"b`: "2"}

		_, err := c.User().Patch(ctx, pb.UserPatchRequest_builder{
			Ref:              u.Ref(),
			Labels:           want,
			DateUpdatedForce: z.Ptr(true),
		}.Build())
		x.NoError(err)
		x.Equal(want, get(ctx, x, c, u).GetLabels())
	}))
}

// The dialect a server is told is the SQL it writes, not a claim about the
// driver -- so an engine this backend has no spelling for is refused at
// construction rather than at the first request that would have needed one.
func TestServerRefusesAnUnwrittenDialect(t *testing.T) {
	s := NewServer(t)
	defer s.Close()

	for _, d := range []string{dialect.MySQL, dialect.Gremlin, "cockroach", ""} {
		_, err := bare.NewServer(s.Db, d)
		require.ErrorIs(t, err, entpatch.ErrDialect, "dialect %q", d)
	}
	for _, d := range []string{dialect.SQLite, dialect.Postgres} {
		_, err := bare.NewServer(s.Db, d)
		require.NoError(t, err, "dialect %q", d)
	}
}
