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
| **Ent**    | `ent/<name>.go`                     | proto ↔ ent conversion helpers for each entity.                          |
| **Server** | `server/.../<name>.g.go`            | `<Entity>ServiceServer{ Db *ent.Client }` implementing Add/Get/Patch/Erase. |
| **Store**  | `store.g.go`                        | server/client registration wiring (like protoc-gen-orm-go's store).      |

The generated ent schema (`schema/*.go`) is then consumed by ent's own code
generator (`ent generate`) to produce the runtime ent package the servers use.

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
materialize the ent runtime.

Options:

| Option       | Default              | Meaning                                       |
| ------------ | -------------------- | --------------------------------------------- |
| `ent.namer`  | `ent/{{ .Name }}.go` | template for the proto↔ent helper filename.   |

## Structure

```
main.go / handler.go    flag parsing; parses files into a graph.Graph; runs the 4 apps
apps/schema/app/        ent schema (x-fields.go, x-edges.go, x-annotations.go, type.go)
apps/ent/app/           proto ↔ ent conversion (x-proto.go)
apps/server/app/        gRPC servers backed by ent (x-add/get/patch/erase/select/pick.go)
apps/store/app/         registration wiring
internal/work/          file/name/import bookkeeping (Name.Go(), Name.Ent())
internal/ent/           PascalCase helper (vendored from ent)
internal/strs/          protobuf name-casing helpers (vendored from protobuf-go)
internal/apptest/       SEPARATE MODULE — integration fixtures + tests (see below)
```

### Two modules: the plugin and its integration tests

The plugin itself depends only on `protobuf-orm`, `google.golang.org/protobuf`,
and `go-openapi/inflect` — so a consumer that just runs the generator pulls a
tiny dependency set (the plugin binary compiles ~4 modules).

The heavy runtime dependencies — `entgo.io/ent`, `ariga.io/atlas`,
`mattn/go-sqlite3` (cgo), `google.golang.org/grpc`, `testify`, … — are used
only by the generated integration suite. To keep them out of consumers'
builds, `internal/apptest/` is its **own Go module**
(`github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest`); those deps live
in its `go.mod`, not the plugin's.

Because it is a nested module, `go build ./...` / `go test ./...` from the repo
root operate on the **plugin only** and never touch `apptest`. Work on the
integration suite from inside its directory.

## Development

A (git-ignored) `go.work` ties the plugin module, the `internal/apptest`
module, and the sibling `protobuf-orm` / `protoc-gen-orm-go` /
`protoc-gen-orm-service` checkouts together so local changes flow through.

```sh
# the plugin
go build ./...               # plugin packages only (apptest is a separate module)
go vet ./...

# regenerate + exercise the integration suite
buf generate                 # full plugin pipeline over proto/ (writes into internal/apptest)
go generate ./...            # run `ent generate` for the schema
cd internal/apptest && go test ./...   # server integration tests against sqlite
```

> `internal/apptest` only compiles once the codegen pipeline (buf generate + ent
> generate) has produced the proto and ent runtime; a `go mod tidy` there is
> meaningful only after that.
