module github.com/protobuf-orm/protoc-gen-orm-ent

go 1.26.2

require (
	github.com/go-openapi/inflect v0.21.3
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260807003431-ce1156ba9f29
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/net v0.47.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251111163417-95abcf5c77ba // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
