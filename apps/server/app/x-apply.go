package app

import (
	"fmt"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

// xApply emits Apply, which takes a patch document instead of a field per prop.
//
// Where Patch spends one request field on each prop and can only say "set this
// to that", a document also addresses one entry of a map, one element of a
// list, and asserts a value before writing it. That last one is why the whole
// thing is worth a second RPC: a `test` compiles to a WHERE predicate, so the
// document is one statement and stays a compare-and-swap, and editing a single
// map entry no longer costs a read-modify-write that loses a concurrent writer.
//
// The compile step is in ormpatch, which knows the schema but nothing about
// SQL, and the rendering is in entpatch, which knows ent. Nothing here does
// either: this emits the wiring, the column table, and the error mapping.
func (w *fileWork) xApply() {
	name_x := w.Ident.GoName
	v := lowerFirst(name_x)

	w.xApplyColumns()

	w.P("func (s ", name_x, "ServiceServer) Apply(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name_x+"ApplyRequest"),
		") (*", w.Ident, ", error) {")
	// Apply requires the document; Patch is what may have nothing to say.
	// Saying so here keeps that difference at the RPC boundary rather than
	// inside the one path both of them run.
	w.P("	if !req.HasPatch() {")
	w.Pf("		return nil, %s(%s, \"%%s\", %s)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("InvalidArgument")),
		w.QualifiedGoIdent(work.PkgOrmPatch.Ident("ErrNoPatch")))
	w.P("	}")
	w.P("	return s.apply(ctx, req.GetRef(), req.GetPatch(), ",
		w.Src.GoImportPath.Ident(name_x+"Service_Apply_FullMethodName"), ")")
	w.P("}")
	w.P("")

	// apply is the whole write path, and the only one: Patch converts its
	// request into a document and arrives here too.
	//
	// Which of the two it was is carried in rather than worked out, because by
	// the time the write happens there is nothing left to work it out from --
	// the document is the same either way, which is the point of converting.
	// It is the RPC's own name rather than a word this generator made up,
	// which is the only spelling of it that outlives this package.
	w.P("func (s ", name_x, "ServiceServer) apply(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "ref *", w.Src.GoImportPath.Ident(name_x+"Ref"), ",",
		/* */ "doc *", work.PkgPatchPb.Ident("Patch"), ",",
		/* */ "method string",
		") (*", w.Ident, ", error) {")

	// No document means no delta -- a request that asked for nothing. It is
	// not an empty one: a Delta must carry an entry, so "change nothing" has to
	// be absence, and an empty plan is what it compiles to.
	w.P("	plan := &", work.PkgOrmPatch.Ident("Plan"), "{Entity: ", v, "OrmEntity}")
	w.P("	if doc != nil {")
	// Compiling refuses a document it cannot honor. Which refusal it is decides
	// what the client should do, so the codes stay apart: a format violation is
	// the producer's to fix, an engine limit is not.
	w.P("		v, err := ", work.PkgOrmPatch.Ident("Compile"), "(", v, "OrmEntity, doc)")
	w.P("		if err != nil {")
	w.Pf("			if %s(err, %s) {",
		w.QualifiedGoIdent(work.PkgErrors.Ident("Is")),
		w.QualifiedGoIdent(work.PkgOrmPatch.Ident("ErrUnsupported")))
	w.Pf("				return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Unimplemented")))
	w.P("			}")
	w.Pf("			return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("InvalidArgument")))
	w.P("		}")
	w.P("		plan = v")
	w.P("	}")
	w.P("")

	// Rendering fails for two different reasons and they are not the client's
	// to tell apart. A value this engine cannot store is the document's, the
	// same answer compiling would have given; anything else means the column
	// table and the schema disagree, which no request can correct.
	w.P("	pred, mod, err := ", work.PkgEntPatch.Ident("Build"), "(plan, ", v, "PatchColumns, s.Db.Dialect())")
	w.P("	if err != nil {")
	w.Pf("		if %s(err, %s) {",
		w.QualifiedGoIdent(work.PkgErrors.Ident("Is")),
		w.QualifiedGoIdent(work.PkgEntPatch.Ident("ErrValue")))
	w.Pf("			return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("InvalidArgument")))
	w.P("		}")
	w.Pf("		return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Internal")))
	w.P("	}")
	w.P("")

	// The write and the read-back are one transaction, so the row a caller is
	// handed is the one its own statement left behind. As two statements the
	// final SELECT is an independent read: under concurrency a caller can be
	// given a row -- and a version -- that some other writer stamped, and a
	// version that is not yours is not a token you can compare-and-swap with.
	//
	// A caller may already have put this call inside a transaction, to make it
	// one write together with something else. Then this one joins rather than
	// starts, and ending it is not this server's to do: whoever began it says
	// whether the whole thing holds. That is enttx's to settle; see xJoin.
	// Always, here, because the two statements are this server's own need and
	// have nothing to do with whether anybody is watching.
	w.xJoin("true")

	// The ref is resolved to the key before anything is written, because the
	// document may assign the very column the ref selects on. Read back through
	// the original ref, a write that committed finds nothing and is reported to
	// its own author as NotFound.
	w.P("	k, err := ", name_x, "GetKey(ctx, st.Db, ref)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	at := &", w.Src.GoImportPath.Ident(name_x+"Ref"), "{}")
	key := w.Entity.Key()
	w.P("	at.Set", work.Name(key.Name()).Go(), "(", w.xKeyGoValue("k", key), ")")
	// The scope is folded into the key rather than checked before it. Every
	// statement below runs through this predicate -- the write, the existence
	// question a document of nothing but tests asks, and the one that explains
	// a write that matched nothing -- so a row out of scope is reported as a
	// row that is not there. That is the same answer Get gives, and it comes
	// from the same absent match rather than from a second rule.
	//
	// GetKey above is not narrowed. It resolves the reference, which may be an
	// alias, and narrowing it would answer NotFound one query earlier while
	// telling the caller nothing different.
	w.P("	p, err := s.narrow(ctx, ", w.xEntPkg().Ident("IDEQ"), "(k))")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")

	// A document can legitimately write nothing: one made only of tests asserts
	// something. An update with no assignments issues no SQL and reports zero
	// rows, which is indistinguishable from a missing row, so ask the question
	// the statement would have answered -- and stamp nothing, because an
	// assertion is not a write. Stamping would move the token every other
	// holder is waiting on, for a request that changed no data.
	w.P("	if mod == nil {")
	w.P("		q := st.Db.", name_x, ".Query().Where(p)")
	w.P("		if pred != nil {")
	w.P("			q.Where(", w.entPkg().Ident(name_x), "(pred))")
	w.P("		}")
	w.P("		if ok, err := q.Exist(ctx); err != nil {")
	w.P("			return nil, err")
	w.P("		} else if !ok {")
	w.P("			return nil, ", w.xNoRowError())
	w.P("		}")
	w.P("	} else {")
	w.P("		q := st.Db.", name_x, ".Update().Where(p)")
	w.P("		if pred != nil {")
	w.P("			q.Where(", w.entPkg().Ident(name_x), "(pred))")
	w.P("		}")
	w.P("		q.Modify(mod)")

	if ver := w.Entity.GetVersionField(); ver != nil {
		// The stamp is the server's, always: ormpatch refuses a document that
		// writes the version, so there is no assignment of its own to collide
		// with. WritesTo is asked anyway -- it costs a map lookup, and it is
		// what would keep this honest if that rule were ever relaxed. Both
		// writes reach the same statement, the document's through Modify and
		// the stamp through the builder, and they land in different lists:
		// PostgreSQL rejects the pair as a duplicate assignment, and SQLite
		// silently keeps one.
		w.Pf("		if !plan.WritesTo(%d) {", ver.Number())
		w.P("			q.Set", work.Name(ver.Name()).Ent(), "(", w.QualifiedGoIdent(work.PkgTime.Ident("Now")), "().UTC())")
		w.P("		}")
	}

	w.P("		if n, err := q.Save(ctx); err != nil {")
	w.P("			return nil, err")
	w.P("		} else if n == 0 {")
	w.P("			return nil, ", w.xNoRowError())
	w.P("		}")
	w.P("	}")
	w.P("")

	// Only the branch that wrote. A document made of tests asserts something
	// and leaves the row as it was, so there is no change to be told about --
	// and a recorder told about one would be saying that a row was touched on
	// the strength of a request that read it.
	w.P("	if mod != nil {")
	w.P("		if err := record(ctx, s.Rec, st.Db, Change{")
	w.P("			Method: method,")
	w.P("			Key: k,")
	w.P("			Patch: doc,")
	w.P("		}); err != nil {")
	w.P("			return nil, err")
	w.P("		}")
	w.P("	}")
	w.P("")

	w.P("	out, err := st.Get(ctx, at.Pick())")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	if err := tx.Commit(); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	return out, nil")
	w.P("}")
	w.P("")
}

// xNoRowError renders the answer to "the statement matched no row".
//
// The key was resolved before the write, so the row was there a moment ago and
// a failed test is much the likelier cause -- but it can also have been erased
// in between, and answering the wrong one sends a client to retry something
// that will never succeed. One extra query, only on the failing path.
func (w *fileWork) xNoRowError() string {
	name_x := w.Ident.GoName
	return fmt.Sprintf(
		"func() error {\n"+
			"\t\t\tif ok, err := st.Db.%s.Query().Where(p).Exist(ctx); err != nil {\n"+
			"\t\t\t\treturn err\n"+
			"\t\t\t} else if !ok {\n"+
			"\t\t\t\treturn %s(%s, \"%s not found\")\n"+
			"\t\t\t}\n"+
			"\t\t\treturn %s(%s, \"a test in the patch did not hold\")\n"+
			"\t\t}()",
		name_x,
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Error")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("NotFound")),
		name_x,
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Error")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("FailedPrecondition")))
}

// xEntPkg is ent's own package for this entity, where IDEQ lives -- distinct
// from the predicate package [fileWork.entPkg] returns.
func (w *fileWork) xEntPkg() protogen.GoImportPath {
	return w.ent + "/" + protogen.GoImportPath(strings.ToLower(w.Ident.GoName))
}

// xKeyGoValue renders a key held in a Go variable as the argument the Ref's
// setter takes: a UUID is a fixed array in Go and bytes in the message.
func (w *fileWork) xKeyGoValue(v string, k graph.Field) string {
	if k.Type() == ormpb.Type_TYPE_UUID {
		return v + "[:]"
	}
	return v
}

// xApplyColumns emits the entity and the prop-to-column table Apply needs.
//
// The table cannot be derived at runtime: a field's column is its own name, but
// an edge's is the foreign key ent named for the relation, which matches
// neither the edge nor the target.
func (w *fileWork) xApplyColumns() {
	name_x := w.Ident.GoName
	v := lowerFirst(name_x)
	pkg := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))

	w.P("var ", v, "OrmEntity = ", work.PkgOrmPatch.Ident("MustEntityOf"), "(",
		w.Src.GoImportPath.Ident("File_"+fileVar(w.Src.Desc.Path())), ", ", quote(name_x), ")")
	w.P("")

	w.P("var ", v, "PatchColumns = ", work.PkgEntPatch.Ident("Columns"), "{")
	for p := range w.Entity.Props() {
		name := work.Name(p.Name())
		if _, ok := p.(graph.Edge); ok {
			w.Pf("\t%d: %s,", p.Number(), w.QualifiedGoIdent(pkg.Ident(name.Ent()+"Column")))
			continue
		}
		w.Pf("\t%d: %s,", p.Number(), w.QualifiedGoIdent(pkg.Ident("Field"+name.Ent())))
	}
	w.P("}")
	w.P("")
}

func (w *fileWork) entPkg() protogen.GoImportPath {
	return w.ent + "/predicate"
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func quote(s string) string { return `"` + s + `"` }

// fileVar renders a proto path as the generated File_ variable's suffix:
// "apptest/user.proto" -> "apptest_user_proto".
func fileVar(path string) string {
	r := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	return r.Replace(path)
}
