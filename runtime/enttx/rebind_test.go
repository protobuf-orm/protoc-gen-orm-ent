package enttx_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
)

// A stack in miniature: an interface the layers answer as, a sink that holds
// the driver, and middlewares in front of it.
type srv interface {
	Name() string
	Next() srv
}

type sink struct{ drv dialect.Driver }

func (sink) Name() string { return "sink" }
func (sink) Next() srv    { return nil }

func (s sink) WithDriver(drv dialect.Driver) (srv, error) {
	s.drv = drv
	return s, nil
}

type layer struct {
	name string
	next srv
}

func (s layer) Name() string { return s.name }
func (s layer) Next() srv    { return s.next }

func (s layer) WithDriver(drv dialect.Driver) (srv, error) {
	next, err := enttx.Rebind(s.next, drv)
	if err != nil {
		return nil, err
	}
	s.next = next
	return s, nil
}

// deaf is a layer that never learned to be rebound.
type deaf struct{ next srv }

func (deaf) Name() string { return "deaf" }
func (s deaf) Next() srv  { return s.next }

// broken holds the driver but cannot take another one.
type broken struct{ err error }

func (broken) Name() string { return "broken" }
func (broken) Next() srv    { return nil }

func (s broken) WithDriver(dialect.Driver) (srv, error) { return nil, s.err }

func drivers(s srv) []dialect.Driver {
	var vs []dialect.Driver
	for ; s != nil; s = s.Next() {
		if v, ok := s.(sink); ok {
			vs = append(vs, v.drv)
		}
	}
	return vs
}

func names(s srv) []string {
	var vs []string
	for ; s != nil; s = s.Next() {
		vs = append(vs, s.Name())
	}
	return vs
}

func TestRebind(t *testing.T) {
	want := &recDriver{dialect: dialect.SQLite}

	t.Run("every layer is kept, and the sink gets the driver", func(t *testing.T) {
		s := srv(layer{"gate", layer{"core", sink{}}})

		v, err := enttx.Rebind(s, want)
		if err != nil {
			t.Fatalf("rebind: %s", err)
		}
		if got := strings.Join(names(v), ","); got != "gate,core,sink" {
			t.Errorf("names = %s, want gate,core,sink", got)
		}
		ds := drivers(v)
		if len(ds) != 1 || ds[0] != dialect.Driver(want) {
			t.Errorf("the sink is not on the new driver: %v", ds)
		}
	})

	t.Run("the original is left alone", func(t *testing.T) {
		s := srv(layer{"core", sink{}})

		if _, err := enttx.Rebind(s, want); err != nil {
			t.Fatalf("rebind: %s", err)
		}
		if ds := drivers(s); len(ds) != 1 || ds[0] != nil {
			t.Errorf("the original sink moved: %v", ds)
		}
	})

	// The whole reason this is not a Find: a layer that cannot be rebound must
	// stop the rebinding, not be quietly left out of the result.
	t.Run("a layer that cannot be rebound is refused, not skipped", func(t *testing.T) {
		s := srv(layer{"core", deaf{sink{}}})

		_, err := enttx.Rebind(s, want)
		if !errors.Is(err, enttx.ErrNotBindable) {
			t.Fatalf("err = %v, want ErrNotBindable", err)
		}
		// It says which layer refused.
		if !strings.Contains(err.Error(), "deaf") {
			t.Errorf("err = %q, which does not name the layer", err)
		}
	})

	t.Run("what a layer says about itself comes back", func(t *testing.T) {
		mine := errors.New("no connection to give")
		s := srv(layer{"core", broken{mine}})

		_, err := enttx.Rebind(s, want)
		if !errors.Is(err, mine) {
			t.Fatalf("err = %v, want %v", err, mine)
		}
	})

	t.Run("nothing at all is refused rather than answered", func(t *testing.T) {
		var s srv
		if _, err := enttx.Rebind(s, want); !errors.Is(err, enttx.ErrNotBindable) {
			t.Errorf("err = %v, want ErrNotBindable", err)
		}
	})
}
