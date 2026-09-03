// runtime is a separate module because it is what a generated server needs at
// RUN time -- the servers import it -- and it needs ent, which the plugin
// module deliberately does not depend on.
//
// It is one module rather than one per concern because a generated server
// imports every package in it. Split up, a consumer would carry a pin per
// package and would have to keep those pins agreeing with each other and with
// the plugin that emitted the imports; together, there is one version to move.
//
//	entpatch  renders a compiled patch document as ent statements
//	enttx     lets a transaction stand in for a driver, so a stack shares one
module github.com/protobuf-orm/protoc-gen-orm-ent/runtime

go 1.27.0

require (
	github.com/protobuf-orm/ent v0.0.0-20260903235335-78a935fbe882
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260807003431-ce1156ba9f29
	github.com/stretchr/testify v1.11.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/lesomnus/protobuf-patch v0.0.0-20260803175157-e1b7a0c2804f // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
