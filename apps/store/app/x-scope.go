package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xScopes emits what narrows the reads, which is the read-side counterpart of
// [Work.xRecorder].
//
// A write reports itself from inside the server that makes it, because a layer
// in front cannot see all of them: Patch and Apply are two RPCs and one write,
// and they become one below anything that can be stacked on top. A read has the
// same shape of problem for the opposite reason. Every server in front can see
// a read, and that is exactly what makes it expensive: narrowing what a caller
// may see means overriding Get, Patch, Apply and Erase, once per entity, and
// once more for every entity added afterwards. It is the same override written
// out again and again, and the copies drift -- a rule fixed in one of them is
// still wrong in the next.
//
// So the narrowing is a predicate these servers put into every query they
// build. What it means is not this generator's to know: it emits the hook and
// calls it, and whether the answer is about tenancy, ownership, a soft delete
// or something nobody here has thought of is the app's to say. That is the same
// line [Work.xRecorder] draws -- these servers know a write happened and
// nothing about audit.
//
// It is an interface and not a struct of functions, which is the shape
// [Recorder] has and is the shape this had to grow into. What narrows a read is
// one decision an app makes about who is asking, and the entities are the
// several answers it has; a struct of closures made each entity a separate
// thing that had to capture the same state for itself. [Unscoped] is what keeps
// that from costing an app a method per entity it has nothing to say about, and
// is also what keeps an entity added later from being a compile error in every
// app that implements this.
//
// What it does *not* cover is worth saying, because the boundary is not
// obvious. It narrows reads of an entity by the server for that entity. It does
// not narrow:
//
//   - Add, which builds a row rather than finding one. Whether a caller may
//     create something is not a predicate, and it belongs in front.
//   - <Entity>GetKey, which resolves a reference to another entity's row so
//     that an edge can be pointed at it. Pointing an edge at something a caller
//     cannot see is a rule about what may be written, which is the same
//     question as Add and has the same answer.
func (w *Work) xScopes() {
	pkg := w.Ent + "/predicate"

	w.P("// Scope narrows what the servers of a [Server] can see, one entity at a")
	w.P("// time. A nil Scope, and a method that answers with a nil predicate,")
	w.P("// narrow nothing: every row is in scope.")
	w.P("//")
	w.P("// A method is called once per query and is handed the context of the")
	w.P("// call, which is where whatever it needs to decide has to be -- who is")
	w.P("// calling, what they are allowed. An error it answers with is the")
	w.P("// caller's answer, so it may be a status.")
	w.P("//")
	w.P("// Narrowing is not refusing, and the difference is the point. A row out")
	w.P("// of scope is a row the query does not match, so a Get of it is NotFound")
	w.P("// and an Apply of it says no row was matched -- which is usually what")
	w.P("// should be said. That something exists is itself something a caller who")
	w.P("// may not see it should not be told.")
	w.P("//")
	w.P("// A call with nothing in its context to decide by -- a deployment writing")
	w.P("// to itself before anybody exists, a job with no request behind it -- is")
	w.P("// the case to be deliberate about. A method that answers `nil, nil` there")
	w.P("// lets that call see everything, which is usually right and is never")
	w.P("// safe to arrive at by accident.")
	w.P("//")
	w.P("// Embed [Unscoped] to write out only the entities there is something to")
	w.P("// say about.")
	w.P("type Scope interface {")
	for _, v := range w.Entities {
		w.P("	", v.Name(), "Scope(ctx ", work.IdentContext, ") (", pkg.Ident(v.Name()), ", error)")
	}
	w.P("}")
	w.P("")

	w.P("// Unscoped is a [Scope] that narrows nothing. Embed it and write out the")
	w.P("// entities there is something to say about; the rest go on seeing every")
	w.P("// row, and so does an entity added to the schema afterwards.")
	w.P("//")
	w.P("// That last part is what it is really for. Without it, every app that")
	w.P("// narrows anything would stop compiling the day an entity is declared,")
	w.P("// and the fix would be a method that says \"no opinion\" -- which is this,")
	w.P("// written out by hand once per app.")
	w.P("type Unscoped struct{}")
	w.P("")
	w.P("var _ Scope = Unscoped{}")
	w.P("")
	for _, v := range w.Entities {
		w.P("func (Unscoped) ", v.Name(), "Scope(_ ", work.IdentContext, ") (", pkg.Ident(v.Name()), ", error) {")
		w.P("	return nil, nil")
		w.P("}")
	}
	w.P("")
}
