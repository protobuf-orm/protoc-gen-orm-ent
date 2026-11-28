package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xClock emits the hook that says what time it is, which is the fourth place an
// app can reach into a generated server.
//
// The other three -- [Work.xRecorder], [Work.xScopes], [Work.xMinter] -- cover
// the write, the read and the key, and this one covers the clock for the same
// reason the minter covers the key: a generated server stamps a time in five
// places, an app cannot reach any of them, and what a test needs is to say what
// that time was.
//
// # What it is not for
//
// Not clock skew, and not synchronising anything. A deployment's clocks are the
// deployment's business and nothing here tries to correct them. This is about a
// **test** being able to say "now is this", the way [Minter] let one say "the
// key is this".
//
// The two together are what make a whole response predictable. With a key that
// is made up and a time that is read off the wall, a test can assert one field
// at a time and never the answer itself; with both handed in, the answer is a
// value that can be compared to one written down.
//
// # Why the default is `time.Now` rather than nothing
//
// [Minter] is the other way round: no minter means no key is made up, and a
// server without one still works, so nil is a real answer. A clock has no such
// state -- a row has to be stamped with something, and there is exactly one
// sensible something. So nil means `time.Now` rather than meaning nothing.
//
// # Why it is asked, rather than being a time handed in
//
// Because a request writes more than once. An Apply reads, writes and reads
// back; an Erase stamps two fields. A time handed in at the top would be one
// value threaded through every generated method, and the thread is the part
// that goes wrong -- a method added later takes the wall clock and nothing
// says so. A function reaches all of them by being on the [Store].
//
// It is asked once per stamp rather than once per request, which is a choice
// worth naming: two stamps in one write may differ by a moment. That is what a
// real clock does, so a fake one that answers with a constant is the sharper
// test and the one an app will write.
func (w *Work) xClock() {
	w.P("// Clock is what these servers ask when they stamp a time -- `date_created`,")
	w.P("// the version field, the one an erase marks.")
	w.P("//")
	w.P("// It exists so that a test can say what time it is, the way [Minter] lets")
	w.P("// one say what a key is. Those two are the whole of what makes a generated")
	w.P("// server's answer unpredictable, and an answer that cannot be predicted is")
	w.P("// one a test has to take apart field by field.")
	w.P("//")
	w.P("// It is **not** about clock skew. What a deployment's clocks say to each")
	w.P("// other is the deployment's business, and nothing here corrects it.")
	w.P("//")
	w.P("// A nil Clock is `time.Now`, which is the difference between this and")
	w.P("// [Minter]: a row has to be stamped with something, so there is no state")
	w.P("// in which answering nothing is right.")
	w.P("//")
	w.P("// What it answers is stamped in UTC and at the microsecond, as the wall")
	w.P("// clock is: see [Store.now].")
	w.P("//")
	w.P("//	s, err := bare.NewServer(db, bare.WithClock(func() ", w.QualifiedGoIdent(work.PkgTime.Ident("Time")), " { return at }))")
	w.P("type Clock func() ", w.QualifiedGoIdent(work.PkgTime.Ident("Time")))
	w.P("")

	// A method rather than a bare `s.Now()` at each site, so the nil case is
	// answered once. Written out five times it is five chances to write
	// `s.Now()` on a Store that has none.
	w.P("// now is what a stamp reads, and `time.Now` when nothing was handed in.")
	w.P("//")
	w.P("// UTC because a stamp is compared and ordered rather than shown, and a")
	w.P("// time carrying a zone is one that compares equal to itself written")
	w.P("// another way -- which is true and is not what anybody reading the")
	w.P("// comparison expects. A Clock is free to answer in any zone; this is")
	w.P("// what is stored.")
	w.P("//")
	w.P("// And at the microsecond, through [written], because that is what")
	w.P("// PostgreSQL keeps: a stamp finer than that is answered by an Add and")
	w.P("// never by a read. A Clock is cut the same way as the wall clock, so a")
	w.P("// test that hands in nanoseconds is answered what is stored.")
	w.P("//")
	w.P("// So a version moves in steps of a microsecond, on SQLite as on")
	w.P("// PostgreSQL, and two writes of one row within the same microsecond")
	w.P("// leave the same version. That is known and accepted: a clock that fine")
	w.P("// was never one the database kept.")
	w.P("func (s Store) now() ", w.QualifiedGoIdent(work.PkgTime.Ident("Time")), " {")
	w.P("	if s.Now == nil {")
	w.P("		return written(", w.QualifiedGoIdent(work.PkgTime.Ident("Now")), "())")
	w.P("	}")
	w.P("")
	w.P("	return written(s.Now())")
	w.P("}")
	w.P("")

	// One function every time a server writes goes through -- the stamps by
	// way of now, a time the caller handed to an Add, a time a patch document
	// assigns -- so that what the database keeps is decided once. A setter
	// written later that hands ent a time takes it from now or wraps it in
	// written; nothing else in the generated package truncates.
	w.P("// written is `t` as these servers write it: UTC, and cut to the")
	w.P("// microsecond, which is what a `timestamptz` keeps. Everything they")
	w.P("// write goes through it -- what [Store.now] stamps, a time an Add is")
	w.P("// handed, a time a patch document assigns -- on every database, so")
	w.P("// that what is stored is the same on SQLite as on PostgreSQL and an")
	w.P("// Add answers what a read will.")
	w.P("//")
	w.P("// Cut, not rounded: a time is never moved later than it was.")
	w.P("//")
	w.P("// Only what is written. A time that is compared -- a version a patch")
	w.P("// tests, a reference by a time, a filter -- is the caller's, as it was")
	w.P("// sent: a row stored before this, which on SQLite can hold")
	w.P("// nanoseconds, is matched by the version a read gives of it, and a cut")
	w.P("// comparison would never match it. Nothing stored changes shape: the")
	w.P("// column is the time it was, with fewer digits written into it.")
	w.P("func written(t ", w.QualifiedGoIdent(work.PkgTime.Ident("Time")), ") ", w.QualifiedGoIdent(work.PkgTime.Ident("Time")), " {")
	w.P("	return t.UTC().Truncate(", w.QualifiedGoIdent(work.PkgTime.Ident("Microsecond")), ")")
	w.P("}")
	w.P("")

	msg := w.QualifiedGoIdent(work.PkgProtoReflect.Ident("Message"))
	val := w.QualifiedGoIdent(work.PkgProtoReflect.Ident("Value"))
	w.P("// writtenTimestamp is [written] for a Timestamp a patch document")
	w.P("// assigns to a column: a new message, so the document and the request")
	w.P("// it came from keep what the caller sent. Anything that is not a")
	w.P("// Timestamp is handed back as it is, for the renderer to refuse.")
	w.P("func writtenTimestamp(v ", val, ") ", val, " {")
	w.P("	m, ok := v.Interface().(", msg, ")")
	w.P("	if !ok || m.Descriptor().FullName() != \"google.protobuf.Timestamp\" {")
	w.P("		return v")
	w.P("	}")
	w.P("	fs := m.Descriptor().Fields()")
	w.P("	secs, nanos := fs.ByName(\"seconds\"), fs.ByName(\"nanos\")")
	w.P("	t := written(", w.QualifiedGoIdent(work.PkgTime.Ident("Unix")), "(m.Get(secs).Int(), m.Get(nanos).Int()))")
	w.P("	u := m.New()")
	w.P("	u.Set(secs, ", w.QualifiedGoIdent(work.PkgProtoReflect.Ident("ValueOfInt64")), "(t.Unix()))")
	w.P("	u.Set(nanos, ", w.QualifiedGoIdent(work.PkgProtoReflect.Ident("ValueOfInt32")), "(int32(t.Nanosecond())))")
	w.P("	return ", w.QualifiedGoIdent(work.PkgProtoReflect.Ident("ValueOfMessage")), "(u)")
	w.P("}")
	w.P("")
}
