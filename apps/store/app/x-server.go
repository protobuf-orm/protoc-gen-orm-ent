package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xServer emits the aggregate that hands out one service server per entity.
//
// It carries the dialect because a patch document becomes JSON functions and
// those are not portable. It has to be resolved before a statement is built:
// the refusal has to keep its type so a handler can answer with a code, and
// anything raised inside a builder does not -- ent's Builder.Err rebuilds its
// errors from their text, so errors.Is can never see through it.
//
// The driver is asked rather than the caller, because the caller has it in hand
// already and a string they retype can disagree with reality. ent's own client
// keeps its driver unexported, so it cannot be the thing we ask.
func (w *Work) xServer() {
	w.P("type Server struct {")
	w.P("	Db *", w.Ent.Ident("Client"))
	w.P("	// Dialect is the SQL this server writes.")
	w.P("	Dialect string")
	w.P("}")
	w.P("")

	w.P("// Option adjusts a [Server] as it is built.")
	w.P("type Option func(*Server)")
	w.P("")
	w.P("// WithDialect writes SQL for a dialect other than the driver's own.")
	w.P("//")
	w.P("// It is for an engine that speaks one of the written dialects under a")
	w.P("// different name -- a PostgreSQL-compatible server told to be postgres.")
	w.P("// Whether it really is compatible is the caller's to know; a statement")
	w.P("// is where a wrong answer shows up. It cannot name a dialect nothing")
	w.P("// was written for.")
	w.P("func WithDialect(dialect string) Option {")
	w.P("	return func(s *Server) { s.Dialect = dialect }")
	w.P("}")
	w.P("")

	w.P("// NewServer refuses a dialect this backend does not write SQL for.")
	w.P("//")
	w.P("// The driver says which one that is. Pass the same one the client was")
	w.P("// built with, or say otherwise with [WithDialect].")
	w.P("func NewServer(db *", w.Ent.Ident("Client"), ", drv ",
		work.PkgEntDialect.Ident("Driver"), ", opts ...Option) (Server, error) {")
	w.P("	s := Server{Db: db, Dialect: drv.Dialect()}")
	w.P("	for _, opt := range opts {")
	w.P("		opt(&s)")
	w.P("	}")
	w.P("	if !", work.PkgEntPatch.Ident("Supports"), "(s.Dialect) {")
	w.P("		return Server{}, ", work.PkgFmt.Ident("Errorf"),
		"(\"%w: %s\", ", work.PkgEntPatch.Ident("ErrDialect"), ", s.Dialect)")
	w.P("	}")
	w.P("	return s, nil")
	w.P("}")
	w.P("")

	for _, v := range w.Entities {
		w.P("func (s Server) ", v.Name(), "() ", w.Package.Ident(v.Name()+"ServiceServer"),
			" { return New", v.Name(), "ServiceServer(s.Db, s.Dialect) }")
	}
	w.P("")
}
