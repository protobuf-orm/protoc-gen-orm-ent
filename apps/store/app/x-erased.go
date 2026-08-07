package app

import "slices"

// xErasedDialect writes down what a soft erasure needs of the engine, and why
// nothing here checks it.
//
// An entity that erases softly frees the names it held: its unique indexes
// cover only the rows that are still there, which a backend writes as a partial
// index. MySQL has none, and ent quietly writes the annotation out for it -- so
// the schema would come up with a plain unique index and everything would look
// right until somebody erased a row and could not use its alias again. That is
// a wrong answer with nothing to say it is wrong, which is the kind worth
// refusing at the start.
//
// It used to be refused here, by a branch of its own beside the dialect check.
// The branch could not run: entpatch renders SQL for the two dialects that do
// have partial indexes and refuses everything else, so the check above had
// already returned by the time it was reached. A refusal that cannot happen is
// not a safeguard, it is a comment that looks like one -- so this is a comment.
//
// What keeps it true is written where it could stop being true: the dialect set
// in runtime/entpatch says that a dialect added to it must be able to make a
// unique index partial, because these servers take that check for the whole
// answer.
//
// Nothing is emitted for a schema in which nothing erases softly.
func (w *Work) xErasedDialect() {
	vs := []string{}
	for _, v := range w.Entities {
		if v.HasErasedField() {
			vs = append(vs, v.Name())
		}
	}
	if len(vs) == 0 {
		return
	}
	slices.Sort(vs)

	w.P("//")
	w.P("// That set is also what a soft erasure needs, so this is the whole")
	w.P("// check. ", join(vs), " free", plural(len(vs)), " the names ", they(len(vs)), " held when a row")
	w.P("// is erased, which is a unique index covering only the rows that are")
	w.P("// still there -- a partial index, and the dialects above are the ones")
	w.P("// that have one. MySQL does not, and ent writes the annotation out for")
	w.P("// it rather than refusing, so the index would come up covering every")
	w.P("// row and a freed name would stay taken with nothing to say so.")
}

func plural(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

func they(n int) string {
	if n == 1 {
		return "it"
	}
	return "they"
}

func join(vs []string) string {
	switch len(vs) {
	case 1:
		return vs[0]
	case 2:
		return vs[0] + " and " + vs[1]
	}

	out := ""
	for i, v := range vs[:len(vs)-1] {
		if i > 0 {
			out += ", "
		}
		out += v
	}

	return out + " and " + vs[len(vs)-1]
}
