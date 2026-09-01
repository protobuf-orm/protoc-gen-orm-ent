// This is a separate module so the protoc-gen-orm-ent plugin module does not
// drag the integration-test dependencies (ent, atlas, sqlite, grpc runtime,
// testify, ...) into every consumer that only runs the generator.
//
// It contains generated code (proto, ent schema/runtime, servers) plus the
// integration tests that exercise them. Regenerate with the repo's codegen
// pipeline (buf generate + ent generate); a `go mod tidy` here only fully
// resolves once that generated code is present.
module github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest

go 1.27.0

require (
	github.com/google/uuid v1.6.0
	github.com/lesomnus/protobuf-patch v0.0.0-20260803175157-e1b7a0c2804f
	github.com/lesomnus/z v0.0.0-20250923111312-437bd8f8f4cf
	github.com/lib/pq v1.12.3
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/protobuf-orm/ent v0.0.0-20260901172250-d7f47a8b836a
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260807003431-ce1156ba9f29
	github.com/protobuf-orm/protoc-gen-orm-ent/runtime v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.11
)

require (
	ariga.io/atlas v1.3.0 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/apparentlymart/go-textseg/v17 v17.0.1 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-openapi/inflect v1.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/hashicorp/hcl/v2 v2.24.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/zclconf/go-cty v1.19.0 // indirect
	github.com/zclconf/go-cty-yaml v1.2.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251111163417-95abcf5c77ba // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The runtime module lives in this repository, and these tests are about what
// the generator emits against it. Pinned to a published version they would test
// the working tree's generated code against somebody else's runtime, and would
// have nothing to say until the runtime was published; this way a change to
// either is exercised by the same `go test`.
replace github.com/protobuf-orm/protoc-gen-orm-ent/runtime => ../../runtime
