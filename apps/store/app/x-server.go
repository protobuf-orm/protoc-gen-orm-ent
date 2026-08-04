package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xServer emits the aggregate that hands out one service server per entity.
//
// It refuses a dialect nothing was written for. A patch document becomes JSON
// functions and those are not portable, so the answer has to be settled before
// a statement is built: the refusal has to keep its type for a handler to
// answer with a code, and anything raised inside a builder does not -- ent's
// Builder.Err rebuilds its errors from their text, so errors.Is can never see
// through it.
//
// The client is asked which dialect that is, rather than the caller being made
// to say. The caller already said it once, when it opened the connection, and a
// second telling is a second claim that can disagree with the first. Ent keeps
// its driver unexported, so the asking is possible only because this generator
// also writes a file inside that package; see apps/ent/app/x-client.go.
func (w *Work) xServer() {
	w.P("type Server struct {")
	w.P("	Db *", w.Ent.Ident("Client"))
	w.P("")
	w.P("	// Rec is told about every write these servers make, and nothing is")
	w.P("	// told if it is nil. See [Recorder].")
	w.P("	Rec Recorder")
	w.P("}")
	w.P("")

	w.P("// Option adjusts a [Server] as it is built.")
	w.P("type Option func(*Server)")
	w.P("")

	w.P("// WithRecorder answers with the option that has every write reported to")
	w.P("// `v`.")
	w.P("//")
	w.P("// It is not free. A write and what a recorder makes of it have to be one")
	w.P("// write, so Add and Erase, which are a single statement without a")
	w.P("// recorder, open a transaction and hold it for as long as the recorder")
	w.P("// takes; and Erase reads the row it is about to delete, to be able to say")
	w.P("// which one it was. Nothing of the sort happens while this is unset.")
	w.P("func WithRecorder(v Recorder) Option {")
	w.P("	return func(s *Server) { s.Rec = v }")
	w.P("}")
	w.P("")

	w.xRecorder()

	w.P("// NewServer refuses a client whose dialect this backend does not write")
	w.P("// SQL for.")
	w.P("//")
	w.P("// An engine that speaks one of the written dialects under a different")
	w.P("// name -- a PostgreSQL-compatible server -- is named when the connection")
	w.P("// is opened, which is where saying so belongs: everything the client does")
	w.P("// is rendered for that dialect, not just what this server writes.")
	w.P("func NewServer(db *", w.Ent.Ident("Client"), ", opts ...Option) (Server, error) {")
	w.P("	s := Server{Db: db}")
	w.P("	for _, opt := range opts {")
	w.P("		opt(&s)")
	w.P("	}")
	w.P("	if d := db.Dialect(); !", work.PkgEntPatch.Ident("Supports"), "(d) {")
	w.P("		return Server{}, ", work.PkgFmt.Ident("Errorf"),
		"(\"%w: %s\", ", work.PkgEntPatch.Ident("ErrDialect"), ", d)")
	w.P("	}")
	w.P("	return s, nil")
	w.P("}")
	w.P("")

	// WithDriver is where the swap actually happens: every server in front of
	// this one rebinds what is behind it and hands the call down, and this is
	// the end of that chain. It is an [enttx.Binder], which is what lets a
	// caller rebind a stack without knowing what the stack is made of.
	w.P("// WithDriver answers with a server that runs through drv, and is this one")
	w.P("// in every other way.")
	w.P("//")
	w.P("// It is how a caller puts several servers on one transaction: begin one")
	w.P("// with enttx and rebind the stack onto the driver it answers with.")
	w.P("//")
	w.P("// The dialect is checked again rather than assumed. A transaction wraps")
	w.P("// the connection it was begun on, so it carries the same dialect -- but")
	w.P("// this takes a driver from anywhere, and what NewServer refused at the")
	w.P("// start should not become reachable by going around it.")
	w.P("func (s Server) WithDriver(drv ", work.PkgEntDialect.Ident("Driver"),
		") (", w.Package.Ident("Server"), ", error) {")
	w.P("	db := s.Db.WithDriver(drv)")
	w.P("	if d := db.Dialect(); !", work.PkgEntPatch.Ident("Supports"), "(d) {")
	w.P("		return nil, ", work.PkgFmt.Ident("Errorf"),
		"(\"%w: %s\", ", work.PkgEntPatch.Ident("ErrDialect"), ", d)")
	w.P("	}")
	w.P("	s.Db = db")
	w.P("	return s, nil")
	w.P("}")
	w.P("")

	// Built here rather than through the constructor, which takes a client and
	// nothing else: a service server reached through the store carries what the
	// store was built with, the recorder included.
	for _, v := range w.Entities {
		w.P("func (s Server) ", v.Name(), "() ", w.Package.Ident(v.Name()+"ServiceServer"),
			" { return ", v.Name(), "ServiceServer{Db: s.Db, Rec: s.Rec} }")
	}
	w.P("")
}
