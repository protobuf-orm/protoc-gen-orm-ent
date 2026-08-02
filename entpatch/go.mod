// entpatch is a separate module because it is RUNTIME code -- generated
// servers import it -- and it needs ent, which the plugin module deliberately
// does not depend on.
module github.com/protobuf-orm/protoc-gen-orm-ent/entpatch

go 1.26.2

require (
	entgo.io/ent v0.14.5
	github.com/google/uuid v1.6.0
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260627113410-c97ccf1e9419
	google.golang.org/protobuf v1.36.11
)
