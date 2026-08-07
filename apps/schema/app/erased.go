package app

import (
	"slices"

	"github.com/protobuf-orm/protobuf-orm/graph"
)

// promotedUniques is the fields whose uniqueness cannot be written on the field
// and has to become an index of its own.
//
// A field marked `unique` is ordinarily `field.X(...).Unique()`, which ent
// writes as a unique constraint over every row of the table. For an entity that
// erases softly that is the wrong constraint: an erased row would go on holding
// its value, so an alias freed by erasing something could never be used again
// -- exactly what a declared unique index of the same entity does not do,
// because that one is written partial.
//
// One of the two spellings behaving differently is worse than either behaviour,
// so the field one is promoted: `.Unique()` comes off the field and an index
// carrying `IndexWhere` goes on beside the declared ones.
//
// Nothing above notices. Which props are keys, and so which Refs exist and what
// shape they have, is `graph`'s to say and it reads the prop's own `unique`,
// which is untouched -- a Ref named by a unique field stays the bare scalar it
// was, rather than turning into the wrapper message a declared index produces.
// What changes is the SQL and nothing else.
//
// The key is left alone. It is unique by being the key, its uniqueness is the
// primary key rather than an index, and a row that is erased is still a row
// that has to be told apart from every other.
func promotedUniques(e graph.Entity) []graph.Field {
	if !e.HasErasedField() {
		return nil
	}

	vs := []graph.Field{}
	for f := range e.Fields() {
		if !f.IsUnique() || f == e.Key() {
			continue
		}

		vs = append(vs, f)
	}

	return slices.Clip(vs)
}

// isPromotedUnique reports whether p is one of them.
func isPromotedUnique(e graph.Entity, p graph.Prop) bool {
	f, ok := p.(graph.Field)
	if !ok {
		return false
	}

	return slices.Contains(promotedUniques(e), f)
}
