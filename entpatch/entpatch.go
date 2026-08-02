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
package entpatch

import (
	"encoding/json"
	"fmt"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpatch"
	"google.golang.org/protobuf/reflect/protoreflect"
)

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

	var pred func(*sql.Selector)
	if len(plan.Tests) > 0 {
		tests := plan.Tests
		pred = func(s *sql.Selector) {
			for _, t := range tests {
				p, err := predicate(s, t, cols[t.Prop.Number()])
				if err != nil {
					s.AddError(err)
					return
				}
				s.Where(p)
			}
		}
	}

	var mod func(*sql.UpdateBuilder)
	if len(plan.Writes) > 0 {
		writes := plan.Writes
		mod = func(u *sql.UpdateBuilder) {
			for _, w := range writes {
				if err := write(u, w, cols[w.Prop.Number()]); err != nil {
					u.AddError(err)
					return
				}
			}
		}
	}

	return pred, mod, nil
}

func predicate(s *sql.Selector, t ormpatch.Test, col string) (*sql.Predicate, error) {
	qualified := s.C(col)

	switch {
	case t.HasKey:
		path := sqljson.Path(fmt.Sprint(t.Key.Interface()))
		switch t.Want {
		case ormpatch.TestExists:
			return sqljson.HasKey(col, path), nil
		case ormpatch.TestAbsent:
			return sql.Not(sqljson.HasKey(col, path)), nil
		}
		v, err := insideArg(t.Value)
		if err != nil {
			return nil, err
		}
		return sqljson.ValueEQ(col, v, path), nil

	case t.HasIndex:
		if t.Index < 0 {
			// A negative index counts from the end, which needs the length the
			// row holds. Refusing beats guessing.
			return nil, fmt.Errorf("entpatch: a negative list index needs the row's length")
		}
		path := sqljson.Path(fmt.Sprintf("[%d]", t.Index))
		switch t.Want {
		case ormpatch.TestExists:
			return sqljson.HasKey(col, path), nil
		case ormpatch.TestAbsent:
			return sql.Not(sqljson.HasKey(col, path)), nil
		}
		v, err := insideArg(t.Value)
		if err != nil {
			return nil, err
		}
		return sqljson.ValueEQ(col, v, path), nil
	}

	switch t.Want {
	case ormpatch.TestExists:
		return sql.NotNull(qualified), nil
	case ormpatch.TestAbsent:
		return sql.IsNull(qualified), nil
	}

	v, err := arg(t.Prop, t.Value, false)
	if err != nil {
		return nil, err
	}
	return sql.EQ(qualified, v), nil
}

func write(u *sql.UpdateBuilder, w ormpatch.Write, col string) error {
	switch op := w.Op.(type) {
	case ormpatch.ClearColumn, ormpatch.ClearEdge:
		// SetNull and Set land in different lists and both would be emitted,
		// which PostgreSQL rejects as a duplicate assignment. Only ever one.
		u.SetNull(col)
		return nil

	case ormpatch.SetColumn:
		v, err := arg(w.Prop, op.Value, false)
		if err != nil {
			return err
		}
		u.Set(col, v)
		return nil

	case ormpatch.SetEdge:
		ed, ok := w.Prop.(graph.Edge)
		if !ok {
			return fmt.Errorf("entpatch: %s is not an edge", w.Prop.Name())
		}
		v, err := scalarArg(ed.Target().Key(), op.Key)
		if err != nil {
			return err
		}
		u.Set(col, v)
		return nil

	case ormpatch.EditJSON:
		return editJSON(u, w, col, op)
	}

	return fmt.Errorf("entpatch: unrecognized operation %s on %s", w.Op.Describe(), w.Prop.Name())
}

// editJSON folds a column's sub-document operations into one expression.
//
// They nest rather than chain: each operation wraps the previous one, so the
// column is assigned exactly once. Assigning it twice would be a duplicate
// assignment in PostgreSQL, and in SQLite the later one would silently win.
func editJSON(u *sql.UpdateBuilder, w ormpatch.Write, col string, op ormpatch.EditJSON) error {
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
		u.SetNull(col)
		return nil
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
				return fmt.Errorf("entpatch: a negative list index needs the row's length")
			}
			path = fmt.Sprint(o.Index)
		}

		s := step{path: path}
		switch o.Kind {
		case ormpatch.JSONSet:
			s.fn = "set"
			v, err := arg(w.Prop, o.Value, true)
			if err != nil {
				return err
			}
			s.v = v
		case ormpatch.JSONRemove:
			s.fn = "remove"
		case ormpatch.JSONAppend:
			s.fn = "append"
			v, err := arg(w.Prop, o.Value, true)
			if err != nil {
				return err
			}
			s.v = v
		default:
			return fmt.Errorf("entpatch: unrecognized json operation %s", o.Kind)
		}
		steps = append(steps, s)
	}

	cleared := len(ops) > 0 && ops[0].Kind == ormpatch.JSONClear
	isList := w.Prop.IsList()

	u.Set(col, sql.ExprFunc(func(b *sql.Builder) {
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
					b.Comma().WriteString(pgPath(s.path, isList)).Comma()
					b.Argf("?::jsonb", s.v)
					b.WriteString(", true)")
				case "remove":
					b.WriteString("(")
					emit(i - 1)
					if isList {
						b.WriteString(" - " + s.path + ")")
					} else {
						b.WriteString(" - ").Arg(s.path).WriteString(")")
					}
				case "append":
					b.WriteString("(")
					emit(i - 1)
					b.WriteString(" || ").Argf("jsonb_build_array(?::jsonb)", s.v).WriteString(")")
				}

			default: // SQLite, and MySQL is refused before we get here.
				switch s.fn {
				case "set":
					b.WriteString("JSON_SET(")
					emit(i - 1)
					b.Comma().WriteString(sqlitePath(s.path, isList)).Comma()
					b.Argf("JSON(?)", s.v)
					b.WriteString(")")
				case "remove":
					b.WriteString("JSON_REMOVE(")
					emit(i - 1)
					b.Comma().WriteString(sqlitePath(s.path, isList))
					b.WriteString(")")
				case "append":
					b.WriteString("JSON_INSERT(")
					emit(i - 1)
					b.WriteString(", '$[#]', ").Argf("JSON(?)", s.v)
					b.WriteString(")")
				}
			}
		}

		if b.Dialect() == dialect.MySQL {
			b.AddError(fmt.Errorf("entpatch: MySQL spells JSON literals and merges differently; " +
				"it is refused rather than approximated"))
			return
		}
		emit(len(steps) - 1)
	}))

	return nil
}

func sqlitePath(path string, isList bool) string {
	if isList {
		return "'$[" + path + "]'"
	}
	// A key is quoted so that a dot or a bracket in it is data, not syntax.
	return `'$."` + jsonEscape(path) + `"'`
}

func pgPath(path string, isList bool) string {
	b, _ := json.Marshal(path)
	_ = b
	return "'{" + pgElem(path) + "}'"
}

func pgElem(s string) string {
	out := make([]rune, 0, len(s)+2)
	for _, r := range s {
		if r == '"' || r == '\\' || r == ',' || r == '{' || r == '}' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
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
