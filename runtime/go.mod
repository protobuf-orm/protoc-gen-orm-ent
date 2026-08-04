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

go 1.26.2

require (
	entgo.io/ent v0.14.5
	github.com/google/uuid v1.6.0
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260803175457-3d185635f291
	google.golang.org/protobuf v1.36.11
)

require github.com/lesomnus/protobuf-patch v0.0.0-20260803175157-e1b7a0c2804f // indirect
