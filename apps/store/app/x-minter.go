package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xMinter emits the hook that decides the key of a row about to be added,
// which is the third of the three places an app can reach into a generated
// server.
//
// The other two are [Work.xRecorder] and [Work.xScopes], and the three cover
// the write, the read and the key. Each exists for the same reason: the thing
// it does is the same for every entity, and without a hook an app writes it out
// once per entity and once more for every entity added afterwards -- which is
// how a rule ends up fixed in one copy and still wrong in the next.
//
// What a key should be is not this generator's to know. It emits the hook and
// calls it; whether the answer is a plain uuid, one that carries what kind of
// thing it names, one an allocator handed out, or one a test wants to be able
// to predict is the app's to say. That is the line the other two hooks draw as
// well.
//
// # Why it takes the entity by name
//
// [Scope] has a method per entity because a predicate is typed per entity and
// one signature cannot carry them all. A key is not like that: every entity
// keyed by a uuid has the same key type, and the only thing that varies is
// *which entity this is*. So it is one method, and the entity arrives as its
// full name -- "app.Robot" -- which is what an app that keeps a table of them
// already keys on.
//
// # Only for uuid keys
//
// An entity keyed by a string or a number is left alone: what the default
// should be for those is the schema's business and there is nothing general to
// decide. A uuid is different -- something has to make it up, and *that* is the
// decision worth handing over.
func (w *Work) xMinter() {
	w.P("// Minter decides the key a row is stored under. It is asked once per Add,")
	w.P("// for an entity whose key is a uuid, and only for those.")
	w.P("//")
	w.P("// `given` is the key the request named and `ok` says whether it named")
	w.P("// one. A minter is free to refuse what it was given -- an error it")
	w.P("// answers with is the caller's answer, so it may be a status -- and free")
	w.P("// to ignore it, though a request whose key is quietly replaced is a")
	w.P("// request that was answered with something it did not ask for.")
	w.P("//")
	w.P("// `entity` is the full name of the message, such as \"app.Robot\".")
	w.P("//")
	w.P("// A nil Minter keeps what a request named and makes up a v4 for a request")
	w.P("// that named nothing, which is what these servers did before there was")
	w.P("// anywhere to say otherwise.")
	w.P("type Minter interface {")
	w.P("	Mint(ctx ", work.IdentContext, ", entity string, given ", work.IdentUuid, ", ok bool) (", work.IdentUuid, ", error)")
	w.P("}")
	w.P("")

	w.P("// MinterFunc is a [Minter] written as a function.")
	w.P("type MinterFunc func(ctx ", work.IdentContext, ", entity string, given ", work.IdentUuid, ", ok bool) (", work.IdentUuid, ", error)")
	w.P("")
	w.P("func (f MinterFunc) Mint(ctx ", work.IdentContext, ", entity string, given ", work.IdentUuid, ", ok bool) (", work.IdentUuid, ", error) {")
	w.P("	return f(ctx, entity, given, ok)")
	w.P("}")
	w.P("")

	w.P("// mint is what the generated Add asks. It is here rather than at each")
	w.P("// call site so that the answer to a nil Minter is written once.")
	w.P("func mint(ctx ", work.IdentContext, ", m Minter, entity string, given ", work.IdentUuid, ", ok bool) (", work.IdentUuid, ", error) {")
	w.P("	if m == nil {")
	w.P("		if ok {")
	w.P("			return given, nil")
	w.P("		}")
	w.P("		return ", work.PkgUuid.Ident("New"), "(), nil")
	w.P("	}")
	w.P("")
	w.P("	return m.Mint(ctx, entity, given, ok)")
	w.P("}")
	w.P("")
}
