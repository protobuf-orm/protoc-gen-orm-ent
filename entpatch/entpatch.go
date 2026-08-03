// Package entpatch turns a compiled patch document into one ent UPDATE.
//
// [ormpatch.Compile] resolves a document against the schema and returns column
// writes and predicates without touching storage. This package renders that
// [ormpatch.Plan] as the predicate and statement modifier an ent update builder
// takes, so the whole document is one statement: the predicates join the WHERE
// clause that already identifies the row, and the writes become its SET list.
//
// One statement is the point. A read-modify-write loses a concurrent update
// between the read and the write, and a `test` entry that became an in-memory
// comparison would no longer be a compare-and-swap. Here a `test` is a WHERE
// predicate, so a document that does not hold matches no row and nothing is
// written -- the format's atomicity, obtained from the database.
//
// # Partial JSON writes
//
// A map or a list is one JSON column, and ent's generated builder can only
// replace it wholesale. Editing one entry therefore goes through
// `Modify(func(*sql.UpdateBuilder))`, which exists only when ent is generated
// with `--feature sql/modifier`. Check gen-ent.sh before wondering why Modify
// is undefined.
//
// # Dialects
//
// The JSON functions are not portable, so the expressions are built per
// dialect at statement time. SQLite and PostgreSQL are implemented; MySQL is
// refused rather than approximated, because its JSON literal and merge
// spellings differ from both.
//
// # What Modify does not enforce
//
// A write rendered here reaches the statement through Modify, which appends to
// the UPDATE builder and never fills the ent mutation. Everything ent hangs off
// the mutation is therefore vacuous on this path. The generated check() guards
// every field validator with `if v, ok := mutation.Field(); ok`, and a column
// only Modify assigned is never ok, so no validator ever sees the value. A hook
// registered with client.Use is handed that same mutation, whose Fields() lists
// only the fields set through the builder -- through Apply, none of the
// document's.
//
// It is harmless today because this generator emits no schema for either to
// act on: the orm.field vocabulary is disabled/type/key/unique/nullable/
// immutable/default/version, and none of those becomes an ent validator. It
// starts to matter the moment the vocabulary grows a constraint -- a length, a
// pattern, an allowed set -- or an application registers a hook that reads
// mutation.Fields() to audit or to derive a column, because through Apply
// neither would run while both still run for Add and for a generated setter.
// Setting the mutation as well would not fix it either: SetNull appends without
// looking, so a document's `remove` and the mutation's clear would both be
// emitted, which is the duplicate assignment PostgreSQL rejects.
package entpatch

import (
	"errors"
	"fmt"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpatch"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// errMySQL is the refusal from the Dialects section of the package doc. It is
// raised at statement time, since the dialect belongs to the connection rather
// than to the document.
var errMySQL = errors.New("entpatch: MySQL spells JSON literals and merges differently; " +
	"it is refused rather than approximated")

// Columns names the database column of each prop, keyed by proto field number.
//
// It is generated alongside the server, because only the generator knows how
// ent named things: a field's column is its own name, but an edge's is the
// foreign key ent chose for the relation, which is neither the edge's name nor
// the target's.
type Columns map[protoreflect.FieldNumber]string

// Build renders a plan as the two things an ent update builder takes.
//
// The predicate is nil when the plan asserts nothing. The modifier is nil when
// it writes nothing -- which a caller must notice, because an update with
// neither issues no SQL and reports zero rows affected, and reading that as
// "no such row" would be wrong about a row that is there.
//
// Everything the plan decides is resolved before Build returns: values are
// converted and predicates are constructed here, so the closures only assemble
// what is already known and cannot fail. That is not tidiness. A predicate that
// reported its failure with Selector.AddError would be dropped on the floor:
// ent's UpdateBuilder.FromSelect copies the selector's WHERE clause and nothing
// else, so the error disappears and the UPDATE runs with the row predicate
// alone -- the `test` silently gone and the write applied. A `test` is this
// format's compare-and-swap, so it must never be droppable, and a value this
// engine cannot render has to be an error here, before any statement exists.
func Build(plan *ormpatch.Plan, cols Columns) (func(*sql.Selector), func(*sql.UpdateBuilder), error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("entpatch: no plan")
	}

	for _, t := range plan.Tests {
		if _, ok := cols[t.Prop.Number()]; !ok {
			return nil, nil, fmt.Errorf("entpatch: no column for %s", t.Prop.Name())
		}
	}
	for _, w := range plan.Writes {
		if _, ok := cols[w.Prop.Number()]; !ok {
			return nil, nil, fmt.Errorf("entpatch: no column for %s", w.Prop.Name())
		}
	}

	tests := make([]func(*sql.Selector) *sql.Predicate, 0, len(plan.Tests))
	for _, t := range plan.Tests {
		p, err := predicate(t, cols[t.Prop.Number()])
		if err != nil {
			return nil, nil, err
		}
		tests = append(tests, p)
	}

	writes := make([]func(*sql.UpdateBuilder), 0, len(plan.Writes))
	for _, w := range plan.Writes {
		set, err := write(w, cols[w.Prop.Number()])
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, set)
	}

	var pred func(*sql.Selector)
	if len(tests) > 0 {
		pred = func(s *sql.Selector) {
			for _, t := range tests {
				s.Where(t(s))
			}
		}
	}

	var mod func(*sql.UpdateBuilder)
	if len(writes) > 0 {
		mod = func(u *sql.UpdateBuilder) {
			for _, set := range writes {
				set(u)
			}
		}
	}

	return pred, mod, nil
}

// predicate resolves one test into its contribution to the WHERE clause.
//
// What is left for the returned closure is building the predicate against the
// selector, which cannot fail -- see [Build] for why that matters. Everything
// that can, which is the value conversion, happens here.
//
// The predicate object itself is deliberately not built here. It looks
// reusable, since rendering one resets its buffer and a second statement
// re-renders rather than appends, but the sqljson helpers close over their
// option slice and write back to it as they render -- so a predicate that has
// met one dialect carries its adjustments into the next. Building one per
// application costs nothing and removes the question.
func predicate(t ormpatch.Test, col string) (func(*sql.Selector) *sql.Predicate, error) {
	// An address inside the column is asked about with the path BOUND, which
	// is why these do not go through ent's sqljson helpers. Those write the
	// path into the statement text, and a map key is whatever the client put
	// in the document -- one carrying a quote would close the literal and
	// continue as SQL. It also keeps the question symmetric with the answer:
	// the write side quotes and escapes a key for the JSON path grammar, and
	// sqljson does not, so a key the write side could reach was one no test
	// could ask about.
	//
	// Building per application rather than once is deliberate too. A predicate
	// closes over the dialect it first met, and these spell differently in
	// each.
	// raw is the address as the document gave it: a map key, or a list index in
	// decimal. Each dialect spells the path from it in its own way, and both
	// bind it.
	inside := func(raw string, isList bool, want func(*sql.Builder)) func(*sql.Selector) *sql.Predicate {
		return func(s *sql.Selector) *sql.Predicate {
			return sql.P(func(b *sql.Builder) {
				switch b.Dialect() {
				case dialect.Postgres:
					b.Ident(col).WriteString(" #> ARRAY[").Arg(raw).WriteString("]::text[]")
				default:
					b.WriteString("JSON_EXTRACT(").Ident(col).Comma().
						Arg(sqlitePath(raw, isList)).WriteString(")")
				}
				want(b)
			})
		}
	}
	exists := func(raw string, isList, absent bool) func(*sql.Selector) *sql.Predicate {
		return inside(raw, isList, func(b *sql.Builder) {
			if absent {
				b.WriteString(" IS NULL")
				return
			}
			b.WriteString(" IS NOT NULL")
		})
	}

	if t.HasKey || t.HasIndex {
		raw := fmt.Sprint(t.Key.Interface())
		if t.HasIndex {
			if t.Index < 0 {
				// A negative index counts from the end, which needs the length
				// the row holds. Refusing beats guessing.
				return nil, fmt.Errorf("entpatch: a negative list index needs the row's length")
			}
			raw = fmt.Sprint(t.Index)
		}
		switch t.Want {
		case ormpatch.TestExists:
			return exists(raw, t.HasIndex, false), nil
		case ormpatch.TestAbsent:
			return exists(raw, t.HasIndex, true), nil
		}
		v, err := insideArg(t.Value)
		if err != nil {
			return nil, err
		}
		return inside(raw, t.HasIndex, func(b *sql.Builder) {
			b.WriteString(" = ").Arg(v)
		}), nil
	}

	switch t.Want {
	case ormpatch.TestExists:
		return func(s *sql.Selector) *sql.Predicate { return sql.NotNull(s.C(col)) }, nil
	case ormpatch.TestAbsent:
		return func(s *sql.Selector) *sql.Predicate { return sql.IsNull(s.C(col)) }, nil
	}

	// An edge's column is the foreign key, and what it holds is a value of the
	// TARGET's key -- so the comparison converts as that key, the substitution
	// SetEdge already makes on the write side. Converting it as the edge itself
	// would bind the wire form (raw bytes, for a UUID key) against a column
	// holding the converted one, and the test could never hold.
	p := t.Prop
	if ed, ok := p.(graph.Edge); ok {
		p = ed.Target().Key()
	}

	if graph.IsCollection(p) || p.Type() == ormpb.Type_TYPE_JSON {
		// A whole JSON column has no portable comparison, so this is the one
		// place the argument's form depends on the dialect rather than on the
		// column. SQLite stores what ent wrote as a BLOB and compares storage
		// classes before contents, so text would never match a row Add
		// created; PostgreSQL holds jsonb, and a []byte arrives as bytea, for
		// which no comparison against jsonb exists. Both forms are built here,
		// where a failure to marshal is still an error; only the choice waits.
		//
		// Neither makes whole-column equality dependable. Two documents with
		// the same entries can differ in key order -- SQLite keeps what it was
		// given, Go marshals sorted -- and nothing here reorders them. Test an
		// entry instead; this exists so that the case which can work does.
		blob, err := jsonArg(t.Value)
		if err != nil {
			return nil, err
		}
		text, err := jsonText(t.Value)
		if err != nil {
			return nil, err
		}
		return func(s *sql.Selector) *sql.Predicate {
			if s.Dialect() == dialect.Postgres {
				return sql.EQ(s.C(col), text)
			}
			return sql.EQ(s.C(col), blob)
		}, nil
	}

	v, err := arg(p, t.Value, posCompare)
	if err != nil {
		return nil, err
	}
	return func(s *sql.Selector) *sql.Predicate { return sql.EQ(s.C(col), v) }, nil
}

// write resolves one column's new value into the assignment that writes it.
//
// The value is converted here, not in the returned closure -- see [Build].
func write(w ormpatch.Write, col string) (func(*sql.UpdateBuilder), error) {
	switch op := w.Op.(type) {
	case ormpatch.ClearColumn, ormpatch.ClearEdge:
		// SetNull and Set land in different lists and both would be emitted,
		// which PostgreSQL rejects as a duplicate assignment. Only ever one.
		return func(u *sql.UpdateBuilder) { u.SetNull(col) }, nil

	case ormpatch.SetColumn:
		v, err := arg(w.Prop, op.Value, posColumn)
		if err != nil {
			return nil, err
		}
		return func(u *sql.UpdateBuilder) { u.Set(col, v) }, nil

	case ormpatch.SetEdge:
		ed, ok := w.Prop.(graph.Edge)
		if !ok {
			return nil, fmt.Errorf("entpatch: %s is not an edge", w.Prop.Name())
		}
		v, err := scalarArg(ed.Target().Key(), op.Key)
		if err != nil {
			return nil, err
		}
		return func(u *sql.UpdateBuilder) { u.Set(col, v) }, nil

	case ormpatch.EditJSON:
		return editJSON(w, col, op)
	}

	return nil, fmt.Errorf("entpatch: unrecognized operation %s on %s", w.Op.Describe(), w.Prop.Name())
}

// editJSON folds a column's sub-document operations into one expression.
//
// They nest rather than chain: each operation wraps the previous one, so the
// column is assigned exactly once. Assigning it twice would be a duplicate
// assignment in PostgreSQL, and in SQLite the later one would silently win.
//
// The paths and the values are resolved here, but the spelling stays deferred:
// which JSON functions to write is the connection's dialect to decide, and no
// one knows it until there is a statement. So this is the one place where a
// closure still does work -- rendering, which cannot fail -- and the only
// decision left to statement time is the refusal of a dialect this cannot
// spell.
func editJSON(w ormpatch.Write, col string, op ormpatch.EditJSON) (func(*sql.UpdateBuilder), error) {
	empty := "'{}'"
	if w.Prop.IsList() {
		empty = "'[]'"
	}

	// A JSONClear discards whatever came before it, exactly as emptying the
	// document would.
	ops := op.Ops
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i].Kind == ormpatch.JSONClear {
			ops = ops[i:]
			break
		}
	}

	if len(ops) == 1 && ops[0].Kind == ormpatch.JSONClear {
		// Nothing dialect-specific is left, so this one does not go through the
		// expression below and MySQL has no reason to be refused.
		return func(u *sql.UpdateBuilder) { u.SetNull(col) }, nil
	}

	type step struct {
		fn   string
		path string
		v    any
	}
	steps := make([]step, 0, len(ops))
	for _, o := range ops {
		if o.Kind == ormpatch.JSONClear {
			continue // handled by starting from the empty document
		}

		var path string
		switch {
		case o.HasKey:
			path = fmt.Sprint(o.Key.Interface())
		case o.HasIndex:
			if o.Index < 0 {
				return nil, fmt.Errorf("entpatch: a negative list index needs the row's length")
			}
			path = fmt.Sprint(o.Index)
		}

		s := step{path: path}
		switch o.Kind {
		case ormpatch.JSONSet:
			s.fn = "set"
			v, err := arg(w.Prop, o.Value, posInner)
			if err != nil {
				return nil, err
			}
			s.v = v
		case ormpatch.JSONRemove:
			s.fn = "remove"
		case ormpatch.JSONAppend:
			s.fn = "append"
			v, err := arg(w.Prop, o.Value, posInner)
			if err != nil {
				return nil, err
			}
			s.v = v
		default:
			return nil, fmt.Errorf("entpatch: unrecognized json operation %s", o.Kind)
		}
		steps = append(steps, s)
	}

	cleared := len(ops) > 0 && ops[0].Kind == ormpatch.JSONClear
	isList := w.Prop.IsList()

	expr := sql.ExprFunc(func(b *sql.Builder) {
		base := func() {
			if cleared {
				b.WriteString(empty)
				return
			}
			// An unset column is NULL, and every JSON function returns NULL for
			// a NULL document -- the patch would vanish with no error. Start
			// from an empty document instead.
			switch b.Dialect() {
			case dialect.Postgres:
				b.WriteString("COALESCE(").Ident(col).WriteString(", " + empty + "::jsonb)")
			default:
				b.WriteString("COALESCE(").Ident(col).WriteString(", " + empty + ")")
			}
		}

		// Every value goes in through Arg, never through Argf: Argf writes the
		// format it is handed verbatim, so a `?` inside one reaches PostgreSQL
		// as a `?` rather than the $n it numbers its arguments with. Arg is
		// what knows the dialect, so the wrapping is written around it.
		var emit func(i int)
		emit = func(i int) {
			if i < 0 {
				base()
				return
			}
			s := steps[i]

			switch b.Dialect() {
			case dialect.Postgres:
				switch s.fn {
				case "set":
					b.WriteString("jsonb_set(")
					emit(i - 1)
					b.Comma().WriteString("ARRAY[").Arg(s.path).WriteString("]::text[]").Comma()
					b.Arg(s.v).WriteString("::jsonb")
					b.WriteString(", true)")
				case "remove":
					b.WriteString("(")
					emit(i - 1)
					if isList {
						b.WriteString(" - ").Arg(s.path).WriteString("::int)")
					} else {
						b.WriteString(" - ").Arg(s.path).WriteString(")")
					}
				case "append":
					b.WriteString("(")
					emit(i - 1)
					b.WriteString(" || jsonb_build_array(").Arg(s.v).WriteString("::jsonb))")
				}

			default: // SQLite; the modifier refused MySQL before we got here.
				switch s.fn {
				case "set":
					b.WriteString("JSON_SET(")
					emit(i - 1)
					b.Comma().Arg(sqlitePath(s.path, isList)).Comma()
					b.WriteString("JSON(").Arg(s.v).WriteString(")")
					b.WriteString(")")
				case "remove":
					b.WriteString("JSON_REMOVE(")
					emit(i - 1)
					b.Comma().Arg(sqlitePath(s.path, isList))
					b.WriteString(")")
				case "append":
					b.WriteString("JSON_INSERT(")
					emit(i - 1)
					b.WriteString(", '$[#]', JSON(").Arg(s.v).WriteString(")")
					b.WriteString(")")
				}
			}
		}

		emit(len(steps) - 1)
	})

	return func(u *sql.UpdateBuilder) {
		// The refusal belongs on the update builder rather than inside the
		// expression above, even though both learn the dialect at the same
		// moment. ent renders an ExprFunc into a clone of the builder and keeps
		// only its text and its arguments, so an error raised in there is
		// dropped and what reaches the database is an assignment with no value
		// on its right-hand side. UpdateBuilder.Err is checked before the
		// statement is issued, so this one is a refusal rather than a syntax
		// error from the driver.
		if u.Dialect() == dialect.MySQL {
			u.AddError(errMySQL)
			return
		}
		u.Set(col, expr)
	}, nil
}

// sqlitePath is the JSON path a step addresses, as a VALUE to be bound.
//
// It is not a SQL literal and must never become one. A map key is whatever the
// client put in the document, so writing it into the statement text -- which is
// what this used to do -- let a key carrying a quote close the literal and
// continue as SQL. Bound, the worst a key can do is address nothing.
func sqlitePath(path string, isList bool) string {
	if isList {
		return "$[" + path + "]"
	}
	// A key is quoted so that a dot or a bracket in it is data, not syntax.
	// This escaping is the JSON path grammar's, not SQL's.
	return `$."` + jsonEscape(path) + `"`
}

func jsonEscape(s string) string {
	out := make([]rune, 0, len(s)+2)
	for _, r := range s {
		if r == '"' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}
