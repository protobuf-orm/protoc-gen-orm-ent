package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

func (w *fileWork) xServer() {
	name := w.Ident.GoName + "ServiceServer"
	w.P("type ", name, " struct {")
	w.P("	Db *", w.ent.Ident("Client"))
	w.P("")
	w.P("	// Rec is told about every write this server makes, and nothing is told")
	w.P("	// if it is nil. See [Recorder].")
	w.P("	Rec Recorder")
	w.P("")
	w.P("	// Scope narrows every query this server builds, and it sees every row")
	w.P("	// if it is nil. See [Scopes].")
	w.P("	Scope func(ctx ", work.IdentContext, ") (", w.entPkg().Ident(w.Ident.GoName), ", error)")
	w.P("")
	w.P("	", w.Src.GoImportPath.Ident("Unimplemented"+name))
	w.P("}")
	w.P("")
	// The options are the store's, and they are taken here for one reason: a
	// server built without them records nothing, silently, which is the one
	// failure of this feature nobody would notice. Whoever builds a service
	// server by hand should be able to say what it reports to in the same
	// words the store does.
	w.P("// New", name, " answers with a server that runs its queries with `db`.")
	w.P("//")
	w.P("// It takes the options of [Server] so that what is built here can be told")
	w.P("// where to report its writes and what it may see. Built without them, it")
	w.P("// reports nowhere and sees everything.")
	w.P("func New", name, "(db *", w.ent.Ident("Client"), ", opts ...Option) ",
		w.Src.GoImportPath.Ident(name), "{")
	w.P("	s := Server{Db: db}")
	w.P("	for _, opt := range opts {")
	w.P("		opt(&s)")
	w.P("	}")
	w.P("	return ", name, "{Db: s.Db, Rec: s.Rec, Scope: s.Scope.", w.Ident.GoName, "}")
	w.P("}")
	w.P("")

	w.xNarrow()
}

// xNarrow emits the one place the scope is asked, so that every query this
// server builds narrows the same way and none of them can be the one that
// forgot.
//
// It takes the predicate the request already produced and answers with both
// together, rather than answering with the scope for a caller to combine: a
// caller that combined them would be a caller that could leave one out.
func (w *fileWork) xNarrow() {
	name := w.Ident.GoName
	p := w.entPkg().Ident(name)

	w.P("// narrow answers with `p` and whatever [", name, "ServiceServer.Scope]")
	w.P("// adds to it, which is `p` itself where nothing is out of scope.")
	w.P("func (s ", name, "ServiceServer) narrow(ctx ", work.IdentContext, ", p ", p, ") (", p, ", error) {")
	w.P("	if s.Scope == nil {")
	w.P("		return p, nil")
	w.P("	}")
	w.P("")
	w.P("	q, err := s.Scope(ctx)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	switch {")
	w.P("	case q == nil:")
	w.P("		return p, nil")
	w.P("	case p == nil:")
	w.P("		return q, nil")
	w.P("	default:")
	w.P("		return ", w.xEntPkg().Ident("And"), "(p, q), nil")
	w.P("	}")
	w.P("}")
	w.P("")
}
