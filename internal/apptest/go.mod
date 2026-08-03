// This is a separate module so the protoc-gen-orm-ent plugin module does not
// drag the integration-test dependencies (ent, atlas, sqlite, grpc runtime,
// testify, ...) into every consumer that only runs the generator.
//
// It contains generated code (proto, ent schema/runtime, servers) plus the
// integration tests that exercise them. Regenerate with the repo's codegen
// pipeline (buf generate + ent generate); a `go mod tidy` here only fully
// resolves once that generated code is present.
module github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest

go 1.26.2

require (
	entgo.io/ent v0.14.5
	github.com/google/uuid v1.6.0
	github.com/lesomnus/protobuf-patch v0.0.0-20260803070125-75159a5efcba
	github.com/lesomnus/z v0.0.0-20250923111312-437bd8f8f4cf
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260803070323-3d16565c3a32
	github.com/protobuf-orm/protoc-gen-orm-ent/entpatch v0.0.0-20260803075511-d4686eec918f
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.11
)

require (
	ariga.io/atlas v0.38.0 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-openapi/inflect v0.19.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/hashicorp/hcl/v2 v2.24.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/zclconf/go-cty v1.17.0 // indirect
	github.com/zclconf/go-cty-yaml v1.1.0 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/tools v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251111163417-95abcf5c77ba // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
