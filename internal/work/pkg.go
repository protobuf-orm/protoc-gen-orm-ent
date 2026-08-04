package work

import "google.golang.org/protobuf/compiler/protogen"

var (
	PkgContext = protogen.GoImportPath("context")
	PkgFmt     = protogen.GoImportPath("fmt")
	PkgTime    = protogen.GoImportPath("time")

	PkgEnt         = protogen.GoImportPath("entgo.io/ent")
	PkgEntDialect  = protogen.GoImportPath("entgo.io/ent/dialect")
	PkgEntSql      = protogen.GoImportPath("entgo.io/ent/dialect/entsql")
	PkgEntSqlGraph = protogen.GoImportPath("entgo.io/ent/dialect/sql/sqlgraph")
	PkgSchema      = protogen.GoImportPath("entgo.io/ent/schema")
	PkgField       = protogen.GoImportPath("entgo.io/ent/schema/field")
	PkgEdge        = protogen.GoImportPath("entgo.io/ent/schema/edge")
	PkgIndex       = protogen.GoImportPath("entgo.io/ent/schema/index")
	PkgZ           = protogen.GoImportPath("github.com/lesomnus/z")

	PkgGoogleUuid = protogen.GoImportPath("github.com/google/uuid")
	PkgUuid       = PkgGoogleUuid

	PkgProtoEmpty     = protogen.GoImportPath("google.golang.org/protobuf/types/known/emptypb")
	PkgProtoTimestamp = protogen.GoImportPath("google.golang.org/protobuf/types/known/timestamppb")

	PkgGrpcCodes  = protogen.GoImportPath("google.golang.org/grpc/codes")
	PkgGrpcStatus = protogen.GoImportPath("google.golang.org/grpc/status")

	IdentContext = PkgContext.Ident("Context")
	IdentUuid    = PkgUuid.Ident("UUID")
	IdentEmpty   = PkgProtoEmpty.Ident("Empty")
)

// Apply's runtime: the schema-side compiler and the ent-side renderer.
var (
	PkgErrors   = protogen.GoImportPath("errors")
	PkgOrmPatch = protogen.GoImportPath("github.com/protobuf-orm/protobuf-orm/ormpatch")
	PkgEntPatch = protogen.GoImportPath("github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entpatch")

	// Patch converts its request into a document and hands it to the same
	// path Apply uses. That needs the document type, the schema graph the
	// converter names edges with, and protoreflect to read the request.
	PkgPatchPb      = protogen.GoImportPath("github.com/lesomnus/protobuf-patch/patchpb")
	PkgOrmGraph     = protogen.GoImportPath("github.com/protobuf-orm/protobuf-orm/graph")
	PkgProtoReflect = protogen.GoImportPath("google.golang.org/protobuf/reflect/protoreflect")
)
