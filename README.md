# protoc-gen-orm-ent

A `protoc`/`buf` plugin that generates a **working
[ent](https://entgo.io)-backed gRPC server** from
[protobuf-orm](https://github.com/protobuf-orm/protobuf-orm) entities.

Where [protoc-gen-orm-service](../protoc-gen-orm-service) emits the service
contract and [protoc-gen-orm-go](../protoc-gen-orm-go) emits the Go glue, this
plugin emits the **implementation**: the ent schema for your tables, the
proto↔ent conversion code, and gRPC service servers that run CRUD against an
ent client.

## What it generates

The handler runs four sub-generators in order:

| App        | Output                              | Contents                                                                 |
| ---------- | ----------------------------------- | ------------------------------------------------------------------------ |
| **Schema** | `schema/<name>.go`                  | ent schema types — `Fields()`, `Edges()`, `Indexes()`, `Annotations()`.  |
| **Ent**    | `ent/<name>.go`, `ent/orm.g.go`     | proto ↔ ent conversion helpers for each entity, and what the ent client will not say about itself. |
| **Server** | `server/.../<name>.g.go`            | `<Entity>ServiceServer{ Db *ent.Client }` implementing Add/Get/Patch/Apply/Erase. |
| **Store**  | `store.g.go`                        | server/client registration wiring (like protoc-gen-orm-go's store).      |

The generated ent schema (`schema/*.go`) is then consumed by ent's own code
generator (`ent generate`) to produce the runtime ent package the servers use.

### What is written into the ent package

`ent/orm.g.go` is the one output that is about the package rather than about an
entity, and it is there because it has nowhere else to be: ent keeps the driver
on an unexported field, so only a file inside that package can read it.

| | |
| --- | --- |
| `Client.Dialect()`   | the SQL this client speaks — the servers write raw SQL for `Apply` and have to know which. |
| `Client.Driver()`    | what the client runs through, which is what a shared transaction is begun on. |
| `Client.WithDriver()`| the same client on another driver, hooks and interceptors intact. |
| `Client.InTx()`      | whether the client is already bound to a transaction of ent's own. |

Without these a caller would have to repeat, to every server it builds, what it
already said when it opened the connection — and a caller that repeats itself
can disagree with itself.

### The runtime module

The generated servers import
[`protoc-gen-orm-ent/runtime`](./runtime), a module of its own so that the
plugin does not depend on ent and a consumer of the plugin does not build it:

| | |
| --- | --- |
| `runtime/entpatch` | renders a compiled patch document as ent statements. |
| `runtime/enttx`    | lets a transaction stand in for a driver, so a whole server stack shares one. |
| `runtime/entpage`  | the half of a list that is not about the domain: a keyset cursor, and a size a request cannot blow past. |

`entpage` is the one of those a generated server does not import. Nothing
generates a list — what a list filters by is the app's, and there is no general
answer to it — but the paging is the same for every entity and is the half
people get wrong: offsets that skip a row when something is inserted ahead of
the page, an order with ties and no tiebreaker, a limit a request can set to a
million. So the filtering is written out and the paging is borrowed.

It is one module rather than one per package because a generated server imports
all of it: split up, a consumer would carry a pin per package and would have to
keep those pins agreeing with each other and with the plugin that wrote the
imports.

> `runtime/entpatch` was `protoc-gen-orm-ent/entpatch`. The path appears only in
> generated code, so moving costs one regeneration — which upgrading the plugin
> asks for anyway.

Example of the generated ent schema for a `User` entity:

```go
func (User) Fields() []ent.Field {
  return []ent.Field{
    field.UUID("id", uuid.UUID{}).Unique().Immutable(),
    field.String("alias").Optional(),
    field.String("name").Optional(),
    field.JSON("labels", map[string]string{}).Optional(),
    ...
  }
}
```

## Two hooks, and why they are here rather than in front

An app usually has something to say about writes and something to say about
reads: keep an audit trail, invalidate a cache, publish an event; show a caller
only their own rows, hide what was soft-deleted. Both look like work for a layer
stacked in front of these servers, and both are worse there.

**A write cannot be watched from in front.** `Add`, `Get`, `Patch`, `Apply` and
`Erase` are five RPCs and four code paths: `Patch` turns its request into a
patch document and joins `Apply`'s path *inside* this server, below anything
that can be stacked on top. A layer in front sees two shapes, has to convert one
of them itself, once per entity and once more for every entity added afterwards
— and still cannot tell a document that wrote something from one that only
asserted.

**A read can be narrowed from in front, and that is the problem.** It means
overriding `Get`, `Patch`, `Apply` and `Erase`, once per entity, forever. It is
the same override written out again and again, and the copies drift: a rule
fixed in one of them stays wrong in the next.

So both are hooks on the generated server.

| | | |
| --- | --- | --- |
| `Recorder` | told about every write | inside the transaction that makes it, before it is committed |
| `Scope`    | asked before every read | and its predicate goes into the query |

```go
// A Scope with something to say about one entity and nothing about the others.
// Embedding Unscoped is what keeps that from being a method per entity -- and
// what keeps an entity added to the schema later from being a compile error.
type wall struct{ bare.Unscoped }

func (wall) HolderScope(ctx context.Context) (predicate.Holder, error) { ... }

s, err := bare.NewServer(db,
    bare.WithRecorder(trail),        // given twice it adds, rather than replacing
    bare.WithScope(wall{}),
)
```

Neither hook teaches this generator what it is for. It emits them and calls
them; whether the predicate is about tenancy, ownership or a soft delete, and
whether the recorder writes a trail or drops a cache, is the app's to say.

Three things follow from where they are:

- **A write and what is recorded about it are one write.** `Add` and `Erase`
  open a transaction they would not otherwise need, and only while a recorder is
  configured. `Erase` also reads the row before deleting it, since a request may
  have named it by an alias and an alias is not what a trail is read back with.
- **Narrowing is not refusing.** A row out of scope is a row the query does not
  match, so a `Get` of it is `NotFound` and an `Apply` of it says no row was
  matched. That is usually the right answer: that something exists is itself
  something a caller who may not see it should not be told.
- **Neither applies to `Add`.** It builds a row rather than finding one. Whether
  a caller may create something, or point an edge at something they cannot see,
  is not a predicate, and it belongs in front.

A `Change` carries two names, and they answer different questions. `Method` is
what the caller asked for — the RPC gRPC dispatched, which is the whole
request's and not this leg of it, so an RPC written by hand that ends in an
`Apply` is on the trail under its own name. `By` is the RPC of *this* server
that made the write, which says which entity and which kind of write without a
field for either. A trail takes `Method`; anything that acts on the row — a
cache to drop, an event to publish — takes `By`.

The server a recorder is handed carries neither hook: it does not record, so a
trail cannot audit itself into a loop, and it is not narrowed, so it can read
what it has just been told about.

`<Entity>Narrow` is where the scope is asked, and it is a function of the
package rather than only a method, because the read most likely to forget is the
one nothing generates. A `List` is not CRUD, so it is written by hand, and what
it has at hand is a client and a scope — given only a method it reaches for the
hook directly and misses whatever narrowing means *besides* the hook.

## Soft deletion

An entity that declares an
[erased field](https://github.com/protobuf-orm/protobuf-orm#erased-fields-soft-deletion)
is not deleted. `Erase` stamps the column instead, and everything else follows
from one line in `<Entity>Narrow`:

```proto
google.protobuf.Timestamp date_erased = 13 [(orm.field) = {erased: {}}];
```

```go
// A row that was erased is not a row a read answers with.
ps = append(ps, note.DateErasedIsNil())
```

That predicate is unconditional and deliberately not something the scope was
asked for. A scope says what *this caller* may see; this says what there is to
see at all, and an app that could leave it out would be an app that could leave
it out by accident.

| | |
| --- | --- |
| `Get`, `Patch`, `Apply` | `NotFound`, because the row is not matched — no second rule |
| `Erase` of an erased row | succeeds and stamps nothing, which is what erasing what is not there has always done |
| `Add` | has no field for it: a row is added alive |
| a unique index | covers only the rows still there, so an erased row gives up its name |

Two of those are worth a second look.

**The version moves with an erase**, if the entity has one. Nothing can
compare-and-swap against the row afterwards — it is out of reach — but a row
brought back by hand would otherwise return holding a version that was current
before it left, and a client that had read it then would find its test still
passing across a gap it never saw.

**MySQL is refused.** A unique index that covers only the live rows is a partial
index; MySQL has none, and `entsql.IndexWhere` is simply not written out for it,
so the schema would come up with a plain unique index and an alias freed by an
erasure would stay taken with nothing anywhere to say so. `NewServer` refuses
that dialect for a schema in which anything erases softly, beside the refusal
that is already there for the dialects `Apply` writes no SQL for.

**Both spellings of `unique` free their value.** A field marked `unique` is
ordinarily a constraint over every row of the table, which would hold the value
of an erased row for ever while a declared index of the same entity gave it up.
One of the two behaving differently is worse than either, so for a soft-erasing
entity the field one is promoted: `.Unique()` comes off the field and a partial
index goes on beside the declared ones. Nothing above notices -- which props are
keys, and so what shape their `Ref` has, is `graph`'s to say and it reads the
field's own `unique`. Only the SQL moves.

**There is no way to read an erased row through these servers**, and no option
to ask for one. An app that needs to restore something, or to look at what was
erased, writes that by hand against the client — which is what it does for every
other RPC nothing generates.

## Field mapping

`apps/schema/app/x-fields.go` maps each prop's ORM `Type` to an ent field
builder:

| ORM type                               | ent builder           |
| -------------------------------------- | --------------------- |
| `BOOL`                                 | `field.Bool`          |
| `INT32`/`SINT32`/`SFIXED32`, `ENUM`    | `field.Int32`         |
| `UINT32`/`FIXED32`                     | `field.Uint32`        |
| `INT64`/`SINT64`/`SFIXED64`            | `field.Int64`         |
| `UINT64`/`FIXED64`                     | `field.Uint64`        |
| `FLOAT` / `DOUBLE`                     | `field.Float32` / `field.Float` |
| `STRING` / `BYTES`                     | `field.String` / `field.Bytes`  |
| `UUID` / `TIME`                        | `field.UUID` / `field.Time`     |
| `JSON`, `map<>`, repeated              | `field.JSON`          |

Constraints map to ent modifiers: key → `.Unique().Immutable()`, `unique` →
`.Unique()`, nullable (non-key, non-JSON) → `.Nillable()`, `immutable` →
`.Immutable()`, optional (non-key) → `.Optional()`.

## Usage

This plugin is the last stage of a multi-plugin `buf.gen.yaml` (it relies on the
proto, gRPC, service, and Go-helper outputs existing):

```yaml
version: v2
plugins:
  - local: [go, tool, google.golang.org/protobuf/cmd/protoc-gen-go]      # messages
  - local: [go, tool, google.golang.org/grpc/cmd/protoc-gen-go-grpc]     # gRPC stubs
  - local: [go, run, github.com/protobuf-orm/protoc-gen-orm-service]     # service .proto
    out: ./proto
  - local: [go, run, github.com/protobuf-orm/protoc-gen-orm-go]          # Go helpers
  - local: [go, run, "."]                                                # this plugin
    opt:
      - ent.namer=ent/{{ .Name }}.go
```

After `buf generate`, run `ent generate` over the produced `schema/` package to
materialize the ent runtime. It needs the `sql/modifier` feature: `Apply` writes
a patch document as one statement, through column expressions ent has no builder
for.

```sh
ent generate ./schema --target ./ent --feature sql/modifier
```

Options:

| Option       | Default              | Meaning                                       |
| ------------ | -------------------- | --------------------------------------------- |
| `ent.namer`  | `ent/{{ .Name }}.go` | template for the proto↔ent helper filename.   |
| `ent.client` | `orm.g.go`           | filename, beside those, for what is about the package rather than an entity. |

## Structure

```
main.go / handler.go    flag parsing; parses files into a graph.Graph; runs the 4 apps
apps/schema/app/        ent schema (x-fields.go, x-edges.go, x-annotations.go, type.go)
apps/ent/app/           proto ↔ ent conversion (x-proto.go), client accessors (x-client.go)
apps/server/app/        gRPC servers backed by ent (x-add/get/patch/apply/erase/select/pick.go)
apps/store/app/         registration wiring
internal/work/          file/name/import bookkeeping (Name.Go(), Name.Ent())
internal/ent/           PascalCase helper (vendored from ent)
internal/strs/          protobuf name-casing helpers (vendored from protobuf-go)
runtime/                SEPARATE MODULE — what the generated servers import (see below)
internal/apptest/       SEPARATE MODULE — integration fixtures + tests (see below)
```

### Three modules: the plugin, its runtime, and its integration tests

The plugin itself depends only on `protobuf-orm`, `google.golang.org/protobuf`,
and `go-openapi/inflect` — so a consumer that just runs the generator pulls a
tiny dependency set (the plugin binary compiles ~4 modules).

`runtime/` is its **own Go module** for the same reason from the other side: it
needs ent because it renders ent statements and wraps ent drivers, and a
consumer imports it from the generated servers rather than from the generator.
Keeping it apart is what lets the plugin stay free of ent.

The heavy test-only dependencies — `ariga.io/atlas`, `mattn/go-sqlite3` (cgo),
`google.golang.org/grpc`, `testify`, … — are used only by the generated
integration suite. To keep them out of consumers' builds, `internal/apptest/` is
a **third module** (`…/internal/apptest`); those deps live in its `go.mod`, not
the plugin's. It `replace`s the runtime with the working tree, so a change to
either the generator or the runtime is exercised by the same `go test`.

Because they are nested modules, `go build ./...` / `go test ./...` from the
repo root operate on the **plugin only**. Work on the other two from inside
their directories.

## Development

A (git-ignored) `go.work` ties the plugin module, `runtime`, the
`internal/apptest` module, and the sibling `protobuf-orm` / `protoc-gen-orm-go`
/ `protoc-gen-orm-service` checkouts together so local changes flow through.

```sh
# the plugin
go build ./...               # plugin packages only (the other two are separate modules)
go vet ./...

# what the generated servers import
cd runtime && go test ./...

# regenerate + exercise the integration suite
buf generate                 # full plugin pipeline over proto/ (writes into internal/apptest)
./gen-ent.sh                 # run `ent generate` for the schema
cd internal/apptest && go test ./...   # server integration tests against sqlite
```

> `internal/apptest` only compiles once the codegen pipeline (buf generate + ent
> generate) has produced the proto and ent runtime; a `go mod tidy` there is
> meaningful only after that.
