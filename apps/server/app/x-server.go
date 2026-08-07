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

// xNarrow emits the one place a read of this entity is narrowed, so that every
// query narrows the same way and none of them can be the one that forgot.
//
// It takes the predicate the request already produced and answers with
// everything together, rather than answering with the parts for a caller to
// combine: a caller that combined them would be a caller that could leave one
// out.
//
// It is a function of the package and not only a method of the server, because
// the read most likely to forget is the one this generator does not write. A
// List is not a CRUD operation, so it is written by hand, and what it has at
// hand is a client and a scope rather than a service server. Given only the
// method, such a list reaches for the scope hook directly and gets whatever
// this function does *besides* calling it -- today nothing, tomorrow the rows
// that have not been erased -- silently wrong in the one place nobody
// generated.
func (w *fileWork) xNarrow() {
	name := w.Ident.GoName
	p := w.entPkg().Ident(name)
	scope := "func(" + w.QualifiedGoIdent(work.IdentContext) + ") (" + w.QualifiedGoIdent(p) + ", error)"

	del := w.Entity.GetErasedField()

	w.P("// ", name, "Narrow answers with `p` and everything else that narrows a")
	if del == nil {
		w.P("// read of a ", name, ", which is whatever `scope` says.")
	} else {
		w.P("// read of a ", name, ": the rows that have not been erased, and whatever")
		w.P("// `scope` says of those.")
	}
	w.P("//")
	w.P("// Every read this package makes goes through it, and a read written by")
	w.P("// hand should too -- a List is the one read nothing generates, and so the")
	w.P("// one that would otherwise answer with rows nobody should be given.")
	w.P("func ", name, "Narrow(ctx ", work.IdentContext, ", scope ", scope, ", p ", p, ") (", p, ", error) {")
	if del == nil {
		w.P("	ps := make([]", p, ", 0, 2)")
	} else {
		w.P("	ps := make([]", p, ", 0, 3)")
		// Unconditional, and deliberately not something the scope was asked
		// for. A scope says what this caller may see; this says what there is
		// to see at all. An app that could leave it out would be an app that
		// could leave it out by accident, in the one place -- a hand-written
		// list -- where nothing would say so.
		w.P("")
		w.P("	// A row that was erased is not a row a read answers with.")
		w.P("	ps = append(ps, ", w.xEntPkg().Ident(work.Name(del.Name()).Ent()+"IsNil"), "())")
	}
	w.P("	if p != nil {")
	w.P("		ps = append(ps, p)")
	w.P("	}")
	w.P("	if scope != nil {")
	w.P("		q, err := scope(ctx)")
	w.P("		if err != nil {")
	w.P("			return nil, err")
	w.P("		}")
	w.P("		if q != nil {")
	w.P("			ps = append(ps, q)")
	w.P("		}")
	w.P("	}")
	w.P("")
	// Answering with And of one is not the same as answering with the one: the
	// SQL grows a pair of parentheses per read for nothing. Answering with And
	// of none would be worse -- ent renders it as a predicate that holds.
	w.P("	switch len(ps) {")
	w.P("	case 0:")
	w.P("		return nil, nil")
	w.P("	case 1:")
	w.P("		return ps[0], nil")
	w.P("	default:")
	w.P("		return ", w.xEntPkg().Ident("And"), "(ps...), nil")
	w.P("	}")
	w.P("}")
	w.P("")

	w.P("// narrow is [", name, "Narrow] with this server's own scope.")
	w.P("func (s ", name, "ServiceServer) narrow(ctx ", work.IdentContext, ", p ", p, ") (", p, ", error) {")
	w.P("	return ", name, "Narrow(ctx, s.Scope, p)")
	w.P("}")
	w.P("")
}
