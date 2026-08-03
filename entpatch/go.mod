// entpatch is a separate module because it is RUNTIME code -- generated
// servers import it -- and it needs ent, which the plugin module deliberately
// does not depend on.
module github.com/protobuf-orm/protoc-gen-orm-ent/entpatch

go 1.26.2

require (
	entgo.io/ent v0.14.5
	github.com/google/uuid v1.6.0
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260803175457-3d185635f291
	google.golang.org/protobuf v1.36.11
)

require github.com/lesomnus/protobuf-patch v0.0.0-20260803175157-e1b7a0c2804f // indirect
