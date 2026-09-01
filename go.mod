module github.com/protobuf-orm/protoc-gen-orm-ent

go 1.27.0

require (
	github.com/go-openapi/inflect v0.21.3
	github.com/protobuf-orm/protobuf-orm v0.0.0-20260807003431-ce1156ba9f29
	github.com/stretchr/testify v1.11.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ettle/strcase v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/protobuf-orm/protoc-gen-orm-go v0.0.0-20260808062124-7336db3ccda7 // indirect
	github.com/protobuf-orm/protoc-gen-orm-service v0.0.0-20260820235114-efff3accbfe1 // indirect
	golang.org/x/net v0.47.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251111163417-95abcf5c77ba // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool (
	github.com/protobuf-orm/protoc-gen-orm-go
	github.com/protobuf-orm/protoc-gen-orm-service
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
