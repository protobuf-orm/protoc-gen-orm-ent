package work

import (
	"path"

	"google.golang.org/protobuf/compiler/protogen"
)

// At is the import path `rel` names, read from `base`.
//
// It is a join and not a concatenation, and that is the whole of it: a `--namer`
// says where a generated file goes relative to the package the messages are in,
// and a file does not always go **below** it. An app whose `go_package` is
// `<module>/api` still wants its ent runtime at `<module>/internal/ent` rather
// than at `<module>/api/internal/ent`, and says so with `../internal/ent`.
// Concatenated that is `<module>/api../internal/ent`, which is not a package
// and is not an error either -- it is an import of something that does not
// exist, written into every generated file.
func At(base protogen.GoImportPath, rel ...string) protogen.GoImportPath {
	return protogen.GoImportPath(path.Join(append([]string{string(base)}, rel...)...))
}

// Base is the name of a package: the last segment of its import path, which is
// what a `package` clause says.
func Base(v protogen.GoImportPath) string { return path.Base(string(v)) }

// Dir is the package a generated file belongs to.
//
// A generated file is named by its whole import path -- `module=` in the plugin
// options is what strips the prefix back off at the end -- so the package it is
// in is the directory of that name, and nothing has to be added to it.
func Dir(p string) protogen.GoImportPath {
	return protogen.GoImportPath(path.Dir(p))
}

// Split takes `f.GeneratedFilenamePrefix` apart the way a `--namer` reads it:
// the directory the messages are in, and the name of this one without its
// suffix.
//
// It is `path` rather than `filepath` because a protoc file name is a slash
// path on every platform, and joining one with the host separator makes an
// import path that only builds where it was generated.
func Split(prefix string) (string, string) {
	dir, name := path.Split(prefix)

	return dir, name
}
