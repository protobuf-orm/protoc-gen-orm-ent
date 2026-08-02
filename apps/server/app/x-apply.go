package app

import (
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
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
	w.P("	return s.apply(ctx, req.GetRef(), req.GetPatch())")
	w.P("}")
	w.P("")

	// apply is the whole write path, and the only one: Patch converts its
	// request into a document and arrives here too.
	w.P("func (s ", name_x, "ServiceServer) apply(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "ref *", w.Src.GoImportPath.Ident(name_x+"Ref"), ",",
		/* */ "doc *", work.PkgPatchPb.Ident("Patch"),
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

	w.P("	pred, mod, err := ", work.PkgEntPatch.Ident("Build"), "(plan, ", v, "PatchColumns)")
	w.P("	if err != nil {")
	w.Pf("		return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Internal")))
	w.P("	}")
	w.P("")

	w.P("	p, err := ", name_x, "Pick(ref)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")

	w.P("	q := s.Db.", name_x, ".Update().Where(p)")
	w.P("	if pred != nil {")
	w.P("		q.Where(", w.entPkg().Ident(name_x), "(pred))")
	w.P("	}")
	w.P("	if mod != nil {")
	w.P("		q.Modify(mod)")
	w.P("	}")

	ver := w.Entity.GetVersionField()
	if ver != nil {
		// The version is the server's to stamp, unless the document stamped it:
		// a client that supplies the version is what Patch spells as `_force`,
		// and replaying a stored delta must reproduce the version it recorded
		// rather than the time of the replay.
		//
		// Asking first is also what keeps the column assigned once. The
		// document's write arrives through Modify and the stamp through the
		// builder, so both reach the same statement: a `remove` lands in the
		// builder's NULL list and the stamp in its value list, and it emits
		// both -- `SET v = NULL, v = ?` -- which PostgreSQL rejects as a
		// duplicate assignment and SQLite resolves by taking the last one. An
		// assign collapses into one only because the builder overwrites a
		// column it already holds, and then it is the stamp that silently
		// disappears.
		//
		// Either way the update still has at least one SET, so the "wrote
		// nothing" branch below stays unreachable while a version field exists.
		w.Pf("	if !plan.WritesTo(%d) {", ver.Number())
		w.P("		q.Set", work.Name(ver.Name()).Ent(), "(", w.QualifiedGoIdent(work.PkgTime.Ident("Now")), "().UTC())")
		w.P("	}")
	}
	w.P("")

	if ver == nil {
		// An update with no writes issues no SQL and reports zero rows, which
		// would look exactly like a missing row. A document can legitimately
		// write nothing -- one made only of tests asserts something -- so ask
		// the question the statement would have answered.
		w.P("	if mod == nil {")
		w.P("		q := s.Db.", name_x, ".Query().Where(p)")
		w.P("		if pred != nil {")
		w.P("			q.Where(", w.entPkg().Ident(name_x), "(pred))")
		w.P("		}")
		w.P("		if ok, err := q.Exist(ctx); err != nil {")
		w.P("			return nil, err")
		w.P("		} else if !ok {")
		w.P("			if _, err := s.Get(ctx, ref.Pick()); err != nil {")
		w.P("				return nil, err")
		w.P("			}")
		w.Pf("			return nil, %s(%s, \"a test in the patch did not hold\")",
			w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Error")),
			w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("FailedPrecondition")))
		w.P("		}")
		w.P("		return s.Get(ctx, ref.Pick())")
		w.P("	}")
		w.P("")
	}

	w.P("	if n, err := q.Save(ctx); err != nil {")
	w.P("		return nil, err")
	w.P("	} else if n == 0 {")
	// The row may be absent, or present with a test that did not hold. One
	// statement cannot say which, so ask only when the answer is needed.
	w.P("		if _, err := s.Get(ctx, ref.Pick()); err != nil {")
	w.P("			return nil, err")
	w.P("		}")
	w.Pf("		return nil, %s(%s, \"a test in the patch did not hold\")",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Error")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("FailedPrecondition")))
	w.P("	}")
	w.P("")
	w.P("	return s.Get(ctx, ref.Pick())")
	w.P("}")
	w.P("")
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
