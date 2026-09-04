# enttx

Puts a server **stack** on another driver, so that everything in it runs inside
one transaction.

It is how a call that spans several servers — add the row, then write the audit
entry — becomes a single write. The servers do not have to know: they keep
talking to the client they were given, and the client talks to the transaction.

## The transaction is ent's

`dialect.BeginTx` answers with a driver that is a transaction, `dialect.InTx`
says whether a driver is inside one, and `ent.JoinTx` is the shape a caller
writes whether or not there is one to join. All three were here first, and
having been written twice they disagreed with ent about what counts as being in
a transaction: a client asked knew only about the one its own package begins,
and this knew only about the other.

What is left here is the half ent has no words for, since ent has no notion of a
server.

**A transaction is ended by whoever began it.** `dialect.BeginTx` hands the
transaction to its caller and hands everyone else a driver that *cannot end it*.
Ask that driver for a transaction — which `ent.Client.Tx` does, and which the
generated `Apply` does through it — and you get one that reads and writes
through the real one but whose `Commit` and `Rollback` are no-ops.

That is not a detail to be tidied away. Ent ends a transaction through whatever
`Driver.Tx` returned, so a driver that returned the real one would let any inner
call's `defer tx.Rollback()` throw away the work of the call that wrapped it —
and report success for it.

## Using it

```go
drv, tx, err := dialect.BeginTx(ctx, db.Driver())
if err != nil {
    return err
}
defer tx.Rollback() // harmless after a commit, and the only thing that undoes a failure

// Rebuild whatever talks to the database on the new driver.
sink, err := bare.NewServer(db.WithDriver(drv))
if err != nil {
    return err
}

if _, err := sink.User().Add(ctx, req); err != nil {
    return err
}
if _, err := sink.User().Apply(ctx, doc); err != nil {
    return err
}

return tx.Commit()
```

`db.Driver()`, `db.WithDriver()` and `db.Dialect()` are generated into the ent
package by protoc-gen-orm-ent (`ent/orm.g.go`); ent keeps its own driver
unexported, so they are the only way to ask.

`dialect.InTx` reports whether a driver is already inside a transaction, and
`Client.InTx` asks it of the client's own. It is **not needed for correctness**
— a transaction begun inside another is a no-op participant in it either way —
it spares the second wrapper, and it is what `ent.JoinTx` asks so that a caller
joins rather than starts.

## Putting a whole stack on it

If the servers are stacked, rebuilding only the one that holds the client leaves
the layers in front of it on the old driver. `Rebind` does the whole stack:

```go
s, err := enttx.Rebind(stack, drv)
```

It asks each layer to make itself again in front of a rebound next, so every
layer implements `WithDriver` — four lines, and the same four lines each time:

```go
func (s Server) WithDriver(drv dialect.Driver) (app.Server, error) {
	next, err := enttx.Rebind(s.Next(), drv)
	if err != nil {
		return nil, err
	}
	return NewServer(next), nil
}
```

Three things about that are worth saying out loud.

**It cannot be inherited.** A middleware that forwards everything else through
an embedded base still writes this: the base holds what is *behind* the layer
and has no way to make the layer *around* it again.

**A layer that cannot be rebound is an error, not a skip.** `Rebind` refuses
with `ErrNotBindable`, naming the type. Looking the capability up instead — with
whatever the stack's equivalent of `Find` is — would walk *past* a layer that
does not implement it, and that layer would then be missing from the rebuilt
stack: requests inside the transaction would go around it. For a layer that
decides what a caller may do, that is not a missing feature but an open door.

**It fails for the same reasons building does.** `WithDriver` is not an accessor
but a builder — "make me again, in front of this next" — so a layer that opens
something as it is made can fail to be made again, and says so.

Guarding it at compile time costs one line per layer:

```go
var _ enttx.Binder[app.Server] = Server{}
```

Without it, a layer that was never taught is found out at the first transaction
rather than at the first build.

## How an inner failure travels

**The error comes back. What the inner call already wrote does not go away.**

A server that opens a transaction of its own is atomic *by itself, and only by
itself*. Inside somebody else's transaction its rollback is a no-op, so
atomicity becomes the outer caller's — which is what the `defer tx.Rollback()`
above is for. It is the only thing that undoes a failure partway through.

Do not carry on after an inner error. Whether the transaction is still usable
depends on what the error was:

| | |
| --- | --- |
| a SQL error — a constraint violation, say | PostgreSQL marks the whole transaction aborted, and every later statement fails until it is rolled back |
| not a SQL error — `Apply` reporting that a `test` did not hold, which is an `UPDATE` that matched no row | the statement succeeded; the transaction is still fine |

Telling those apart to decide whether to continue is not worth it. Treat any
error from inside as *roll back*.

## Watch out for

- **Rebind the whole stack, not one server.** A layer left on the old driver
  writes outside the transaction, and nothing will say so. `Rebind` is what does
  the whole thing; reaching past it to the server that holds the client is the
  mistake it exists to prevent.
- **One goroutine.** A transaction is a single connection and the driver is not
  safe for concurrent use — the same as ent's own.
- **Do not use the original client while the transaction is open.** It holds a
  connection; on a small pool (a test with `SetMaxOpenConns(1)`, say) a query
  through the untransacted client waits for a connection the transaction is
  holding, and nothing arrives. Inside, use only what was rebuilt.
- **Nesting is not savepoints.** There is one transaction, and an inner failure
  does not roll back to where the inner call started. If you need that, you need
  savepoints, and this package does not have them.

## What it is not

Nothing here is about a driver any more; that is all ent's. `Rebind` and
`Binder` are generic over whatever the stack's own server type happens to be,
because that type belongs to the app and a driver has no business naming it —
which is the same reason they are not in ent.

That is also why this is not in the generated message package: it would put
`github.com/protobuf-orm/ent` in front of every consumer, including the ones that only speak
gRPC and never open a database.
