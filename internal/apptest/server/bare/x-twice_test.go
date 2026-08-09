package bare_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/note"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent/predicate"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// scope is a Scope that answers with one predicate for Note and nothing for
// anything else.
type scope struct {
	bare.Unscoped
	p predicate.Note
}

func (s scope) NoteScope(context.Context) (predicate.Note, error) { return s.p, nil }

// TestAHookGivenTwiceIsRefused is the rule, and the reason is that neither
// answer to "what did you mean" is safe to pick.
//
// Replacing loses one silently -- a recorder that was going to write the trail,
// a scope that was the tenant wall -- and neither failure says anything at the
// time. Combining silently is a rule nobody wrote, in an order nobody chose.
func TestAHookGivenTwiceIsRefused(t *testing.T) {
	for _, tt := range []struct {
		what string
		opts []bare.Option
		says string
	}{{
		what: "two scopes",
		opts: []bare.Option{bare.WithScope(bare.Unscoped{}), bare.WithScope(bare.Unscoped{})},
		says: "Scopes{...}",
	}, {
		what: "two recorders",
		opts: []bare.Option{bare.WithRecorder(bare.Recorders{}), bare.WithRecorder(bare.Recorders{})},
		says: "Recorders{...}",
	}, {
		// There is no plural of a minter and there will not be: a row has one
		// key, so two of them is not a composition, it is a disagreement.
		what: "two minters",
		opts: []bare.Option{bare.WithMinter(mint), bare.WithMinter(mint)},
		says: "and they do not",
	}, {
		// Two clocks is two answers to "now", and a write that stamped
		// `date_created` from one and the version from the other would be a row
		// whose own two times disagree.
		what: "two clocks",
		opts: []bare.Option{bare.WithClock(time.Now), bare.WithClock(time.Now)},
		says: "there is one now",
	}} {
		t.Run(tt.what, func(t *testing.T) {
			x := require.New(t)

			// The options themselves, since that is where the rule is. The
			// case below says NewServer hands it back.
			var s bare.Server
			x.NoError(tt.opts[0](&s))

			err := tt.opts[1](&s)
			x.ErrorIs(err, bare.ErrTwice)
			x.ErrorContains(err, tt.says, "the refusal does not say where to say it")
		})
	}
}

// TestNewServerHandsTheRefusalBack, which is what makes it a build-time answer
// rather than something an app has to check for itself.
func TestNewServerHandsTheRefusalBack(t *testing.T) {
	x := require.New(t)

	_, err := bare.NewServer(&ent.Client{},
		bare.WithScope(bare.Unscoped{}),
		bare.WithScope(bare.Unscoped{}),
	)
	x.ErrorIs(err, bare.ErrTwice)
}

// mint is a Minter that is not nil, which is the only thing this needs of it.
var mint = bare.MinterFunc(func(context.Context, string, uuid.UUID, bool) (uuid.UUID, error) {
	return uuid.Nil, nil
})

// TestANilHookIsNotAHook.
//
// Passing nil is the same as not passing: a nil Scope narrows nothing and a nil
// Recorder is told nothing, which is what a server with neither already does.
// So it neither sets nor collides -- refusing it would be refusing somebody for
// saying the default out loud.
func TestANilHookIsNotAHook(t *testing.T) {
	x := require.New(t)

	var s bare.Server
	x.NoError(bare.WithScope(nil)(&s))
	x.NoError(bare.WithScope(bare.Unscoped{})(&s), "saying the default out loud was refused")

	// The other way round still collides, because the first one was a hook.
	var t2 bare.Server
	x.NoError(bare.WithScope(bare.Unscoped{})(&t2))
	x.ErrorIs(bare.WithScope(nil)(&t2), bare.ErrTwice)
}

// TestScopesMeet is the other half of that refusal, and the half that makes it
// usable rather than merely strict.
func TestScopesMeet(t *testing.T) {
	t.Run("a row is in scope when every one of them says so", func(t *testing.T) {
		x := require.New(t)

		p, err := bare.Scopes{
			scope{p: note.AliasEQ("kept")},
			scope{p: note.BodyEQ("kept")},
		}.NoteScope(t.Context())
		x.NoError(err)
		x.NotNil(p)
	})

	// The nil is the reason this is worth generating rather than left to each
	// app. A scope says "narrows nothing" with a nil predicate, so combining is
	// not And(a, b) -- it is And of whichever are not nil, and nil when none
	// are. Getting that wrong turns "narrows nothing" into "matches nothing",
	// which hides every row, or the other way about, which shows every row.
	t.Run("one that narrows nothing does not narrow anything", func(t *testing.T) {
		x := require.New(t)

		p, err := bare.Scopes{scope{p: nil}, scope{p: note.AliasEQ("kept")}}.NoteScope(t.Context())
		x.NoError(err)
		x.NotNil(p, "the one that said something was dropped")
	})

	t.Run("all of them narrowing nothing is nothing", func(t *testing.T) {
		x := require.New(t)

		p, err := bare.Scopes{scope{}, scope{}}.NoteScope(t.Context())
		x.NoError(err)
		x.Nil(p, "no narrowing became a predicate matching no rows")
	})

	t.Run("none at all is nothing", func(t *testing.T) {
		x := require.New(t)

		p, err := bare.Scopes{}.NoteScope(t.Context())
		x.NoError(err)
		x.Nil(p)
	})
}
