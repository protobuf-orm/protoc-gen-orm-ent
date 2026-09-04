// Package enttx puts a server stack on another driver, so that everything in
// it runs inside one transaction.
//
// The transaction itself is ent's. [dialect.BeginTx] answers with a driver
// that is one, [dialect.InTx] says whether a driver is inside one, and
// [ent.JoinTx] is the shape a caller writes whether or not there is one to
// join. All three were written here first, and having been written twice they
// disagreed: a client asked whether it was in a transaction knew only about
// the one its own package begins, and this knew only about the other.
//
// What is left is the half ent has no words for. Rebinding is about a server
// *stack* being rebuilt on another driver, and ent has no notion of a server.
//
// The rule it rests on is ent's too: a transaction is ended by whoever began
// it. [dialect.BeginTx] returns the transaction to its caller and hands
// everyone else a driver that cannot end it, so a server that opens a
// transaction of its own -- a generated Apply does -- joins that one rather
// than committing it early or taking it down.
package enttx

import (
	"errors"
	"fmt"

	"github.com/protobuf-orm/ent/dialect"
)

// ErrNotBindable is a server that cannot be put on another driver.
//
// It is an error rather than something to skip over. Putting a stack on a
// transaction rewrites the whole stack, and a layer left out of the rewrite is
// left out of the stack: requests inside the transaction would go around it,
// which for a layer that decides what a caller may do is not a missing feature
// but an open door.
var ErrNotBindable = errors.New("enttx: cannot be put on another driver")

// Binder is a server that can be put on another driver.
//
// `S` is the server type of whatever is being rebound -- the stack's own
// interface -- because a layer answers with something the layers in front of it
// can hold. This is generic for the same reason the package is separate: the
// stack's types belong to the app, and a driver does not know them.
//
// Implementing it is the same operation as building a layer in front of
// another: "make me again, in front of this next". That is why it may fail, and
// why the signature is the shape of a builder rather than of an accessor -- a
// layer that opens something as it is made can fail to be made again.
//
// A middleware implements it by rebinding what is behind it and remaking
// itself:
//
//	func (s Server) WithDriver(drv dialect.Driver) (app.Server, error) {
//		next, err := enttx.Rebind(s.Next(), drv)
//		if err != nil {
//			return nil, err
//		}
//		return NewServer(next), nil
//	}
//
// There is no default to inherit. A middleware that forwards everything else
// through an embedded base still has to write this, because the base holds what
// is behind it and does not know how to make the layer around it again.
type Binder[S any] interface {
	WithDriver(dialect.Driver) (S, error)
}

// Rebind answers with `s` running on drv.
//
// It refuses, with [ErrNotBindable], a server that is not a [Binder] -- naming
// the type that refused, since in a stack of look-alike layers that is the only
// way to tell which one it was.
func Rebind[S any](s S, drv dialect.Driver) (S, error) {
	v, ok := any(s).(Binder[S])
	if !ok {
		var zero S
		return zero, fmt.Errorf("%w: %T", ErrNotBindable, s)
	}

	return v.WithDriver(drv)
}
