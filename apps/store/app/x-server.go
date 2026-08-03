package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xServer emits the aggregate that hands out one service server per entity.
//
// It carries the dialect because a patch document becomes JSON functions and
// those are not portable. The dialect is what SQL to write rather than what
// driver is in use, so ent is never asked -- a caller on a compatible engine
// can name one of the written dialects deliberately and take the risk.
func (w *Work) xServer() {
	w.P("type Server struct {")
	w.P("	Db *", w.Ent.Ident("Client"))
	w.P("	// Dialect is the SQL this server writes.")
	w.P("	Dialect string")
	w.P("}")
	w.P("")

	// Refusing here is the fail-fast half. entpatch.Build refuses too, so a
	// server assembled some other way still cannot write SQL nobody wrote --
	// this one just says so at startup rather than at the first request that
	// needs it.
	w.P("// NewServer refuses a dialect this backend does not write SQL for.")
	w.P("//")
	w.P("// The dialect is the SQL to emit, not a claim about the driver. Naming one")
	w.P("// the connection does not speak is allowed and is the caller's risk: an")
	w.P("// engine compatible with it will work, and one that is not will fail at the")
	w.P("// statement rather than here.")
	w.P("func NewServer(db *", w.Ent.Ident("Client"), ", dialect string) (Server, error) {")
	w.P("	if !", work.PkgEntPatch.Ident("Supports"), "(dialect) {")
	w.P("		return Server{}, ", work.PkgFmt.Ident("Errorf"),
		"(\"%w: %s\", ", work.PkgEntPatch.Ident("ErrDialect"), ", dialect)")
	w.P("	}")
	w.P("	return Server{Db: db, Dialect: dialect}, nil")
	w.P("}")
	w.P("")

	for _, v := range w.Entities {
		w.P("func (s Server) ", v.Name(), "() ", w.Package.Ident(v.Name()+"ServiceServer"),
			" { return New", v.Name(), "ServiceServer(s.Db, s.Dialect) }")
	}
	w.P("")
}
