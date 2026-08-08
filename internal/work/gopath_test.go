package work_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// TestANamerMayClimbOut is the whole reason these are joined.
//
// A `--namer` says where a generated file goes relative to the package the
// messages are in, and not everything generated belongs below it: an app whose
// `go_package` is `<module>/api` still wants its ent runtime beside `go.mod`.
// Concatenated, `../internal/ent` makes `<module>/api../internal/ent` -- which
// is not a package, and is not an error either. It is an import of something
// that does not exist, written into every generated file.
func TestANamerMayClimbOut(t *testing.T) {
	x := require.New(t)

	pkg := protogen.GoImportPath("github.com/acme/thing/api")
	x.Equal(protogen.GoImportPath("github.com/acme/thing/internal/ent"),
		work.At(pkg, "../internal/ent"))
	x.Equal(protogen.GoImportPath("github.com/acme/thing/server/bare"),
		work.At(pkg, "../server/bare"))

	// And the ordinary case, where the messages are at the module root and a
	// namer says a plain path.
	root := protogen.GoImportPath("github.com/acme/thing")
	x.Equal(protogen.GoImportPath("github.com/acme/thing/internal/ent"),
		work.At(root, "internal/ent"))
}

// TestAFileNamesItsOwnPackage, which is what decides whether an identifier is
// qualified: a generated file is named by its whole import path, so the package
// it is in is the directory of that name.
func TestAFileNamesItsOwnPackage(t *testing.T) {
	x := require.New(t)

	x.Equal(protogen.GoImportPath("github.com/acme/thing/internal/ent/schema"),
		work.Dir("github.com/acme/thing/internal/ent/schema/robot.go"))
	x.Equal("schema", work.Base(work.Dir("github.com/acme/thing/internal/ent/schema/robot.go")))
}

// TestAProtoNameIsASlashPath on every platform, so it is taken apart with
// `path` and not `filepath` -- one joined with the host separator is an import
// path that only builds where it was generated.
func TestAProtoNameIsASlashPath(t *testing.T) {
	x := require.New(t)

	dir, name := work.Split("github.com/acme/thing/api/robot")
	x.Equal("github.com/acme/thing/api/", dir)
	x.Equal("robot", name)
}
