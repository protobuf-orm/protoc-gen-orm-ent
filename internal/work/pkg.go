package work

import "google.golang.org/protobuf/compiler/protogen"

var (
	PkgContext = protogen.GoImportPath("context")
	PkgTime    = protogen.GoImportPath("time")

	PkgEnt         = protogen.GoImportPath("entgo.io/ent")
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
