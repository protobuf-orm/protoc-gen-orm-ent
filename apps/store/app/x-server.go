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
	// Store is a type of its own rather than fields on [Server], and the
	// reason is what a Server is: the thing that hands out one service server
	// per entity. A service server that embedded it would answer for every
	// entity too, and would satisfy the app's own Server interface by
	// accident. Naming what they all run with lets each of them embed exactly
	// that -- and lets a field added here reach every one of them without a
	// line being written at each place one is built.
	w.P("// Store is what every server of this package runs with: the client its")
	w.P("// queries are built on, what it tells about a write, and what narrows")
	w.P("// what it can see.")
	w.P("//")
	w.P("// It is handed over whole. Copying the parts across would be a line per")
	w.P("// field per entity, and forgetting one fails quietly -- a server built")
	w.P("// without the recorder writes rows that nothing records.")
	w.P("type Store struct {")
	w.P("	Db *", w.Ent.Ident("Client"))
	w.P("")
	w.P("	// Rec is told about every write these servers make, and nothing is")
	w.P("	// told if it is nil. See [Recorder].")
	w.P("	Rec Recorder")
	w.P("")
	w.P("	// Scope narrows what these servers can see, and they see every row")
	w.P("	// if it is nil. See [Scope].")
	w.P("	Scope Scope")
	w.P("")
	w.P("	// Mint decides the key of a row about to be added, for an entity")
	w.P("	// keyed by a uuid. A nil Minter makes one up. See [Minter].")
	w.P("	Mint Minter")
	w.P("")
	w.P("	// Now is what these servers stamp a time with, and is `time.Now` when")
	w.P("	// it is nil. See [Clock].")
	w.P("	Now Clock")
	w.P("}")
	w.P("")

	w.P("// Server hands out one service server per entity, every one of them")
	w.P("// running with the same [Store].")
	w.P("type Server struct {")
	w.P("	Store")
	w.P("}")
	w.P("")

	w.P("// Option adjusts a [Server] as it is built.")
	w.P("//")
	w.P("// An option that sets a hook refuses to be given twice, and the whole of")
	w.P("// the reason is that neither answer to \"what did you mean\" is safe to")
	w.P("// pick. Replacing silently loses one -- a recorder that was going to write")
	w.P("// the trail, a scope that was the tenant wall -- and neither failure says")
	w.P("// anything at the time. Combining silently is a rule nobody wrote, in an")
	w.P("// order nobody chose, which is the same thing later.")
	w.P("//")
	w.P("// So it is refused, and how they compose is said where it can be read:")
	w.P("// [Recorders] and [Scopes] are what that looks like.")
	w.P("//")
	w.P("// A nil hook is not a hook. Passing one is the same as not passing --")
	w.P("// a nil Scope narrows nothing and a nil Recorder is told nothing, which")
	w.P("// is what a server with neither already does -- so it neither sets nor")
	w.P("// collides. Refusing it would be refusing somebody for saying the")
	w.P("// default out loud.")
	w.P("type Option func(*Server) error")
	w.P("")

	w.P("// ErrTwice is an option that sets the same hook as one already given.")
	w.P("var ErrTwice = ", work.PkgErrors.Ident("New"), "(\"given twice; say how they compose\")")
	w.P("")

	w.P("// WithRecorder answers with the option that has every write reported to")
	w.P("// `v`. Several recorders are [Recorders], written out where the order and")
	w.P("// what a refusal costs can both be read.")
	w.P("//")
	w.P("// It is not free. A write and what a recorder makes of it have to be one")
	w.P("// write, so Add and Erase, which are a single statement without a")
	w.P("// recorder, open a transaction and hold it for as long as the recorder")
	w.P("// takes; and Erase reads the row it is about to delete, to be able to say")
	w.P("// which one it was. Nothing of the sort happens while this is unset.")
	w.P("func WithRecorder(v Recorder) Option {")
	w.P("	return func(s *Server) error {")
	w.P("		if s.Rec != nil {")
	w.P("			return ", work.PkgFmt.Ident("Errorf"), "(\"recorder: %w with Recorders{...}\", ErrTwice)")
	w.P("		}")
	w.P("")
	w.P("		s.Rec = v")
	w.P("")
	w.P("		return nil")
	w.P("	}")
	w.P("}")
	w.P("")

	w.P("// WithScope answers with the option that narrows what these servers can")
	w.P("// see to what `v` says. Several scopes are [Scopes], which meet.")
	w.P("func WithScope(v Scope) Option {")
	w.P("	return func(s *Server) error {")
	w.P("		if s.Scope != nil {")
	w.P("			return ", work.PkgFmt.Ident("Errorf"), "(\"scope: %w with Scopes{...}\", ErrTwice)")
	w.P("		}")
	w.P("")
	w.P("		s.Scope = v")
	w.P("")
	w.P("		return nil")
	w.P("	}")
	w.P("}")
	w.P("")

	w.P("// WithMinter answers with the option that has `v` decide the key of every")
	w.P("// row these servers add. See [Minter].")
	w.P("//")
	w.P("// There is no plural of this and there will not be: a row has one key, so")
	w.P("// two minters is not a composition, it is a disagreement.")
	w.P("func WithMinter(v Minter) Option {")
	w.P("	return func(s *Server) error {")
	w.P("		if s.Mint != nil {")
	w.P("			return ", work.PkgFmt.Ident("Errorf"), "(\"minter: %w, and they do not\", ErrTwice)")
	w.P("		}")
	w.P("")
	w.P("		s.Mint = v")
	w.P("")
	w.P("		return nil")
	w.P("	}")
	w.P("}")
	w.P("")

	w.P("// WithClock answers with the option that has `v` say what time it is")
	w.P("// wherever these servers stamp one. See [Clock].")
	w.P("//")
	w.P("// No plural, for the reason [WithMinter] has none and a sharper one: two")
	w.P("// clocks is two answers to \"now\", and a write that stamped `date_created`")
	w.P("// from one and `date_updated` from the other would be a row whose own two")
	w.P("// times disagree.")
	w.P("func WithClock(v Clock) Option {")
	w.P("	return func(s *Server) error {")
	w.P("		if s.Now != nil {")
	w.P("			return ", work.PkgFmt.Ident("Errorf"), "(\"clock: %w, and there is one now\", ErrTwice)")
	w.P("		}")
	w.P("")
	w.P("		s.Now = v")
	w.P("")
	w.P("		return nil")
	w.P("	}")
	w.P("}")
	w.P("")

	w.xRecorder()
	w.xScopes()
	w.xMinter()
	w.xClock()

	w.P("// NewServer refuses a client whose dialect this backend does not write")
	w.P("// SQL for.")
	w.P("//")
	w.P("// An engine that speaks one of the written dialects under a different")
	w.P("// name -- a PostgreSQL-compatible server -- is named when the connection")
	w.P("// is opened, which is where saying so belongs: everything the client does")
	w.P("// is rendered for that dialect, not just what this server writes.")
	w.xErasedDialect()
	w.P("func NewServer(db *", w.Ent.Ident("Client"), ", opts ...Option) (Server, error) {")
	w.P("	s := Server{Store: Store{Db: db}}")
	w.P("	for _, opt := range opts {")
	w.P("		if err := opt(&s); err != nil {")
	w.P("			return Server{}, err")
	w.P("		}")
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
			" { return ", v.Name(), "ServiceServer{Store: s.Store} }")
	}
	w.P("")
}
