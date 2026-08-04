package entpatch_test

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpatch"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entpatch"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The schema below is described here rather than taken from the app tests,
// which have a generated one already: that module depends on this one, and
// depending back on it -- even only to test -- would put ent's integration
// dependencies into the go.mod of a package every generated server imports.
// All these tests need is a graph.Entity, and a descriptor is enough to make
// one.
//
//	message Point {
//	  int32 x = 1;
//	  string s = 2;
//	}
//
//	message User {
//	  bytes id = 1 [(orm.field) = {type: TYPE_UUID, key: true}];
//	  Tenant tenant = 2 [(orm.edge) = {}];
//	  string name = 3;
//	  map<string, string> labels = 4;
//	  repeated int32 scores = 5;
//	  repeated bytes blobs = 6;
//	  repeated float ratios = 7;
//	  repeated Point points = 8;
//	  map<string, bytes> tokens = 9;
//	  map<string, Point> shapes = 10;
//	}
var userColumns = entpatch.Columns{
	1: "id",
	// An edge's column is the foreign key ent named for the relation, which is
	// why Columns exists at all.
	2:  "user_tenant",
	3:  "name",
	4:  "labels",
	5:  "scores",
	6:  "blobs",
	7:  "ratios",
	8:  "points",
	9:  "tokens",
	10: "shapes",
}

func TestBuildRefusesATestItCannotRender(t *testing.T) {
	e := userEntity(t)

	// A negative list index needs the length the row holds, so this test cannot
	// become a predicate. It used to be reported from inside the closure, where
	// it was dropped, and the assign below landed unconditioned.
	plan := &ormpatch.Plan{
		Entity: e,
		Tests: []ormpatch.Test{{
			Prop:     prop(t, e, "scores"),
			HasIndex: true,
			Index:    -1,
			Want:     ormpatch.TestEqual,
			Value:    protoreflect.ValueOfInt32(3),
		}},
		Writes: []ormpatch.Write{{
			Prop: prop(t, e, "name"),
			Op:   ormpatch.SetColumn{Value: protoreflect.ValueOfString("SHOULD NOT LAND")},
		}},
	}

	pred, mod, err := entpatch.Build(plan, userColumns, dialect.SQLite)
	if err == nil {
		t.Fatal("Build accepted a test it cannot render")
	}
	if pred != nil {
		t.Error("a predicate came back with the error, and a caller that ignores one gets an unconditioned UPDATE")
	}
	if mod != nil {
		t.Error("a modifier came back with the error, and the write it carries is the one that must not land")
	}
}

// TestEntDropsSelectorErrors pins the ent behavior that makes Build's eager
// resolution mandatory rather than tidy. If this ever fails, ent grew a way to
// report a predicate failure from inside the closure -- until then there is
// none, and a `test` reported that way disappears from the WHERE clause while
// the UPDATE goes ahead.
func TestEntDropsSelectorErrors(t *testing.T) {
	s := sql.Dialect(dialect.SQLite).Select().From(sql.Table("user"))
	s.Where(sql.EQ(s.C("name"), "old"))
	s.AddError(errors.New("a test this engine cannot evaluate"))

	u := sql.Dialect(dialect.SQLite).Update("user").Where(sql.EQ("id", 1))
	u.Set("name", "new")
	u.FromSelect(s)

	if err := u.Err(); err != nil {
		t.Fatalf("ent now carries the selector's error into the update (%v)", err)
	}
	if q, _ := u.Query(); !strings.Contains(q, "`name` = ?") {
		t.Fatalf("the selector's WHERE clause was not copied either: %s", q)
	}
}

func TestTestOnAnEdgeBindsTheTargetsKey(t *testing.T) {
	e := userEntity(t)
	id := []byte{
		0x9e, 0xed, 0x09, 0xd9, 0x4a, 0x59, 0x43, 0xc9,
		0x8e, 0xf3, 0xb6, 0x1b, 0xb6, 0xb4, 0x5a, 0x25,
	}

	plan := &ormpatch.Plan{
		Entity: e,
		Tests: []ormpatch.Test{{
			Prop:  prop(t, e, "tenant"),
			Want:  ormpatch.TestEqual,
			Value: protoreflect.ValueOfBytes(id),
		}},
	}

	pred, _, err := entpatch.Build(plan, userColumns, dialect.SQLite)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	s := sql.Dialect(dialect.SQLite).Select().From(sql.Table("user"))
	pred(s)
	q, args := s.Query()

	if !strings.Contains(q, "`user`.`user_tenant` = ?") {
		t.Errorf("the test did not land on the foreign key column: %s", q)
	}
	if len(args) != 1 {
		t.Fatalf("bound %d arguments, want 1: %#v", len(args), args)
	}
	// The wire form of a UUID key is bytes, but the column holds the converted
	// value, exactly as SetEdge writes it. Binding the bytes matches nothing.
	want := uuid.Must(uuid.FromBytes(id))
	got, ok := args[0].(uuid.UUID)
	if !ok {
		t.Fatalf("bound %#v (%T), want a uuid.UUID", args[0], args[0])
	}
	if got != want {
		t.Errorf("bound %v, want %v", got, want)
	}

	// The predicate is resolved once and applied per statement, so it has to
	// survive a second application unchanged.
	again := sql.Dialect(dialect.SQLite).Select().From(sql.Table("user"))
	pred(again)
	if q2, _ := again.Query(); q2 != q {
		t.Errorf("applying the predicate twice rendered\n\t%s\nthen\n\t%s", q, q2)
	}
}

func TestWholeMapBindsWhatEntBinds(t *testing.T) {
	e := userEntity(t)
	labels := prop(t, e, "labels")
	v := mapValue(t, e, "labels", map[string]string{"b": "2", "a": "1"})

	// What ent binds for a JSON field: sqlgraph.setTableColumns marshals the
	// value and hands the driver a json.RawMessage. go-sqlite3 stores that as a
	// BLOB, and a Go string as TEXT, and SQLite calls values of two classes
	// unequal -- so a row written by Add and one written here would not compare
	// equal if this bound a string.
	want, err := json.Marshal(map[string]string{"a": "1", "b": "2"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("write", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Writes: []ormpatch.Write{{Prop: labels, Op: ormpatch.SetColumn{Value: v}}},
		}
		_, mod, err := entpatch.Build(plan, userColumns, dialect.SQLite)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		u := sql.Dialect(dialect.SQLite).Update("user")
		mod(u)
		if err := u.Err(); err != nil {
			t.Fatalf("the modifier failed: %v", err)
		}
		_, args := u.Query()
		assertRawJSON(t, args, want)
	})

	t.Run("test", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Tests:  []ormpatch.Test{{Prop: labels, Want: ormpatch.TestEqual, Value: v}},
		}
		pred, _, err := entpatch.Build(plan, userColumns, dialect.SQLite)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		s := sql.Dialect(dialect.SQLite).Select().From(sql.Table("user"))
		pred(s)
		_, args := s.Query()
		assertRawJSON(t, args, want)
	})

	// PostgreSQL is the one place the write form is the wrong one to compare
	// with: the column is jsonb and a []byte arrives as bytea, which jsonb has
	// no operator against. So the comparison -- and only the comparison --
	// binds text there.
	t.Run("test on postgres binds text", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Tests:  []ormpatch.Test{{Prop: labels, Want: ormpatch.TestEqual, Value: v}},
		}
		pred, _, err := entpatch.Build(plan, userColumns, dialect.Postgres)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		s := sql.Dialect(dialect.Postgres).Select().From(sql.Table("user"))
		pred(s)
		_, args := s.Query()
		if len(args) != 1 {
			t.Fatalf("got %d args, want 1", len(args))
		}
		got, ok := args[0].(string)
		if !ok {
			t.Fatalf("bound %#v (%T), want a string", args[0], args[0])
		}
		if got != string(want) {
			t.Fatalf("bound %q, want %q", got, want)
		}
	})

	// What Build was told is what it writes. The same plan yields either
	// spelling depending only on which dialect it was built for, and a builder
	// is never consulted -- which is what lets a caller on a compatible engine
	// name one deliberately.
	t.Run("the dialect Build was given is the one it writes", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Tests:  []ormpatch.Test{{Prop: labels, Want: ormpatch.TestEqual, Value: v}},
		}
		for _, d := range []string{dialect.Postgres, dialect.SQLite} {
			pred, _, err := entpatch.Build(plan, userColumns, d)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			// Rendered against a builder that disagrees, on purpose.
			s := sql.Dialect(dialect.SQLite).Select().From(sql.Table("user"))
			pred(s)
			_, args := s.Query()
			switch d {
			case dialect.Postgres:
				if _, ok := args[0].(string); !ok {
					t.Fatalf("%s: bound %T, want a string", d, args[0])
				}
			default:
				assertRawJSON(t, args, want)
			}
		}
	})
}

// TestAnEntryIsComparedTheWayItWasWritten pins the spelling of a comparison
// against one entry of a JSON column, kind by kind.
//
// The column holds what encoding/json wrote -- through ent's Add, or through
// this package's own partial write, which binds the same text -- and the
// predicate extracts the entry before comparing, so what the argument must
// equal is that entry as the extraction hands it back. Two kinds used to be
// spelled a second way here and could never match: bytes, which the writer
// stores as base64 and this compared as the raw bytes, and a message, which
// reaches the column compacted and HTML-escaped by encoding/json and was
// compared as protojson left it.
//
// A mismatch here is not a test that fails. It is a test that cannot hold, and
// a document whose test does not hold is abandoned whole -- so every other
// entry in it disappears with no error to point at.
func TestAnEntryIsComparedTheWayItWasWritten(t *testing.T) {
	e := userEntity(t)

	for _, tt := range []struct {
		name string
		test ormpatch.Test
		// sqlite is the entry decoded, which is what JSON_EXTRACT yields: a
		// JSON string arrives without its quotes, an object as its own text.
		sqlite any
		// postgres is the entry's JSON text, because #> yields jsonb -- and it
		// is also exactly what the write binds there.
		postgres string
	}{
		{
			name:     "bytes are the base64 encoding/json wrote, not the bytes",
			test:     at(t, e, "blobs", 0, protoreflect.ValueOfBytes([]byte{0xde, 0xad, 0xbe, 0xef})),
			sqlite:   "3q2+7w==",
			postgres: `"3q2+7w=="`,
		},
		{
			// A value is spelled the same wherever it sits, so this pins that
			// the map path did not grow a spelling of its own. The app tests
			// have no map of bytes to reach it through.
			name:     "and the same base64 as a map value",
			test:     in(t, e, "tokens", "k", protoreflect.ValueOfBytes([]byte{0xde, 0xad, 0xbe, 0xef})),
			sqlite:   "3q2+7w==",
			postgres: `"3q2+7w=="`,
		},
		{
			// The escape is the point: protojson leaves the three HTML bytes
			// alone and encoding/json does not, and encoding/json is what wrote
			// the column.
			name:     "a message is compacted and HTML-escaped, as it went in",
			test:     at(t, e, "points", 0, pointValue(t, e, "a<b")),
			sqlite:   `{"s":"a\u003cb"}`,
			postgres: `{"s":"a\u003cb"}`,
		},
		{
			name:     "and the same message as a map value",
			test:     in(t, e, "shapes", "k", pointValue(t, e, "a<b")),
			sqlite:   `{"s":"a\u003cb"}`,
			postgres: `{"s":"a\u003cb"}`,
		},
		{
			// A JSON string comes back from the extraction decoded, which is
			// why this one is the plain string and the escaping is not its
			// business. It is jsonb's business in PostgreSQL, which parses.
			name:     "a string arrives decoded",
			test:     in(t, e, "labels", "k", protoreflect.ValueOfString("a<b")),
			sqlite:   "a<b",
			postgres: `"a\u003cb"`,
		},
		{
			name:     "a number stays a number",
			test:     at(t, e, "scores", 0, protoreflect.ValueOfInt32(7)),
			sqlite:   int32(7),
			postgres: "7",
		},
		{
			// The written text is what the row parses, and it is written with
			// 32-bit precision; the widened value is a different double.
			name:     "a float32 is the double its written text parses to",
			test:     at(t, e, "ratios", 0, protoreflect.ValueOfFloat32(3.14)),
			sqlite:   float64(3.14),
			postgres: "3.14",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Two arguments in each dialect: the path, then the value.
			if got := entryArgs(t, e, dialect.SQLite, tt.test); got != tt.sqlite {
				t.Errorf("sqlite bound %#v (%T), want %#v", got, got, tt.sqlite)
			}
			if got := entryArgs(t, e, dialect.Postgres, tt.test); got != any(tt.postgres) {
				t.Errorf("postgres bound %#v (%T), want %q", got, got, tt.postgres)
			}
		})
	}
}

// TestARefusalThisEngineOwnsIsNotAServerFault covers the one thing Build
// declines that the document is entitled to ask for. Everything else it refuses
// is either the value's fault or ours; this is neither, and the difference is
// the difference between telling a client to fix a correct document and telling
// it we cannot honor one.
func TestARefusalThisEngineOwnsIsNotAServerFault(t *testing.T) {
	e := userEntity(t)

	for name, plan := range map[string]*ormpatch.Plan{
		"a test on a negative index": {
			Entity: e,
			Tests: []ormpatch.Test{{
				Prop:     prop(t, e, "scores"),
				HasIndex: true,
				Index:    -1,
				Want:     ormpatch.TestEqual,
				Value:    protoreflect.ValueOfInt32(3),
			}},
		},
		"a write to a negative index": {
			Entity: e,
			Writes: []ormpatch.Write{{
				Prop: prop(t, e, "scores"),
				Op: ormpatch.EditJSON{Ops: []ormpatch.JSONOp{{
					Kind: ormpatch.JSONSet, Index: -1, HasIndex: true,
					Value: protoreflect.ValueOfInt32(3), HasValue: true,
				}}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := entpatch.Build(plan, userColumns, dialect.SQLite)
			if err == nil {
				t.Fatal("Build accepted an index it has to read the row to resolve")
			}
			if !errors.Is(err, ormpatch.ErrUnsupported) {
				t.Errorf("%v does not wrap ErrUnsupported, so a server answers Internal "+
					"and blames itself for something the document asked for", err)
			}
			if errors.Is(err, entpatch.ErrValue) {
				t.Errorf("%v wraps ErrValue, which would tell the client to correct a "+
					"value that is perfectly good", err)
			}
		})
	}
}

func TestBuildTellsAClientMistakeFromABug(t *testing.T) {
	e := userEntity(t)

	// ormpatch refuses a wrong-width UUID before Build ever sees one, so this
	// pins the second line of defence rather than a path a request can take.
	// The reachable case is below: a float the column can hold but JSON cannot
	// spell.
	t.Run("a UUID of the wrong width is the value's fault", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Writes: []ormpatch.Write{{
				Prop: prop(t, e, "id"),
				Op:   ormpatch.SetColumn{Value: protoreflect.ValueOfBytes([]byte("fifteen bytes!!"))},
			}},
		}

		_, _, err := entpatch.Build(plan, userColumns, dialect.SQLite)
		if err == nil {
			t.Fatal("Build accepted a 15 byte UUID")
		}
		if !errors.Is(err, entpatch.ErrValue) {
			t.Errorf("%v does not wrap ErrValue, so a server cannot answer InvalidArgument", err)
		}
	})

	// This one no earlier layer catches. +Inf is a legal float64 and a legal
	// document, and it reaches Build; encoding/json is where it stops, because
	// JSON has no infinity.
	t.Run("a value JSON cannot spell is the value's fault", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Writes: []ormpatch.Write{{
				Prop: prop(t, e, "labels"),
				Op:   ormpatch.SetColumn{Value: protoreflect.ValueOfFloat64(math.Inf(1))},
			}},
		}

		_, _, err := entpatch.Build(plan, userColumns, dialect.SQLite)
		if err == nil {
			t.Fatal("Build accepted +Inf")
		}
		if !errors.Is(err, entpatch.ErrValue) {
			t.Errorf("%v does not wrap ErrValue, so a server cannot answer InvalidArgument", err)
		}
	})

	t.Run("a column the table does not name is ours", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Writes: []ormpatch.Write{{
				Prop: prop(t, e, "name"),
				Op:   ormpatch.SetColumn{Value: protoreflect.ValueOfString("Ada")},
			}},
		}

		_, _, err := entpatch.Build(plan, entpatch.Columns{}, dialect.SQLite)
		if err == nil {
			t.Fatal("Build accepted a prop with no column")
		}
		// The generator and the schema disagree, which no request can correct.
		if errors.Is(err, entpatch.ErrValue) {
			t.Errorf("%v wraps ErrValue, which would blame the client for a build-time bug", err)
		}
	})
}

func TestJSONEditRendersPerDialect(t *testing.T) {
	e := userEntity(t)
	plan := &ormpatch.Plan{
		Entity: e,
		Writes: []ormpatch.Write{{
			Prop: prop(t, e, "labels"),
			Op: ormpatch.EditJSON{Ops: []ormpatch.JSONOp{{
				Kind:     ormpatch.JSONSet,
				Key:      protoreflect.ValueOfString("a").MapKey(),
				HasKey:   true,
				Value:    protoreflect.ValueOfString("1"),
				HasValue: true,
			}}},
		}},
	}

	// One plan, built once per dialect: what Build is told is what it writes,
	// so the same plan yields SQLite's spelling or PostgreSQL's depending only
	// on what it was asked for.
	for _, tt := range []struct {
		dialect string
		want    string
		path    string
	}{
		{dialect.SQLite, "SET `labels` = JSON_SET(COALESCE(`labels`, '{}'), ?, JSON(?))", `$."a"`},
		// PostgreSQL numbers its arguments, and only Builder.Arg knows that --
		// a `?` written into a format string arrives as a `?`.
		{dialect.Postgres, `SET "labels" = jsonb_set(COALESCE("labels", '{}'::jsonb), ARRAY[$1]::text[], $2::jsonb, true)`, "a"},
	} {
		t.Run(tt.dialect, func(t *testing.T) {
			_, mod, err := entpatch.Build(plan, userColumns, tt.dialect)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			u := sql.Dialect(tt.dialect).Update("user")
			mod(u)
			if err := u.Err(); err != nil {
				t.Fatalf("the modifier failed: %v", err)
			}

			q, args := u.Query()
			if !strings.Contains(q, tt.want) {
				t.Errorf("rendered\n\t%s\nwant it to contain\n\t%s", q, tt.want)
			}
			// Two arguments, and the first is the PATH. It is bound rather
			// than written into the statement because a map key is whatever
			// the client put in the document; interpolated, one carrying a
			// quote would close the literal and continue as SQL.
			if len(args) != 2 {
				t.Fatalf("bound %d arguments, want 2: %#v", len(args), args)
			}
			if got, ok := args[0].(string); !ok || got != tt.path {
				t.Errorf("bound path %#v, want %q", args[0], tt.path)
			}
			// The entry is an argument to JSON(?) or to ?::jsonb, which parse
			// the text they are handed -- so this one is a string, unlike the
			// whole-column form.
			if got, ok := args[1].(string); !ok || got != `"1"` {
				t.Errorf("bound %#v (%T), want the JSON text `\"1\"`", args[1], args[1])
			}
		})
	}

	// Refused before a statement exists, which is the only place it can be
	// heard. A predicate's error is dropped by UpdateBuilder.FromSelect, and a
	// document made only of tests never reaches the modifier at all, so a
	// refusal raised while rendering would be silent for exactly the documents
	// that read.
	t.Run("an unwritten dialect is refused by Build", func(t *testing.T) {
		_, _, err := entpatch.Build(plan, userColumns, dialect.MySQL)
		if !errors.Is(err, entpatch.ErrDialect) {
			t.Fatalf("got %v, want ErrDialect", err)
		}
		if entpatch.Supports(dialect.MySQL) {
			t.Error("Supports says otherwise")
		}
		for _, d := range []string{dialect.SQLite, dialect.Postgres} {
			if !entpatch.Supports(d) {
				t.Errorf("%s is written for but Supports says no", d)
			}
		}
	})
}

// TestPostgresNumbersArgumentsAcrossTheStatement puts a plain assignment ahead
// of a JSON edit, because PostgreSQL numbers its arguments by position and the
// JSON expression renders through a builder of its own.
func TestPostgresNumbersArgumentsAcrossTheStatement(t *testing.T) {
	e := userEntity(t)
	plan := &ormpatch.Plan{
		Entity: e,
		Writes: []ormpatch.Write{
			{
				Prop: prop(t, e, "name"),
				Op:   ormpatch.SetColumn{Value: protoreflect.ValueOfString("Ada")},
			},
			{
				Prop: prop(t, e, "labels"),
				Op: ormpatch.EditJSON{Ops: []ormpatch.JSONOp{{
					Kind:     ormpatch.JSONSet,
					Key:      protoreflect.ValueOfString("a").MapKey(),
					HasKey:   true,
					Value:    protoreflect.ValueOfString("1"),
					HasValue: true,
				}}},
			},
		},
	}

	_, mod, err := entpatch.Build(plan, userColumns, dialect.Postgres)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	u := sql.Dialect(dialect.Postgres).Update("user")
	mod(u)
	q, args := u.Query()

	want := `SET "name" = $1, "labels" = jsonb_set(COALESCE("labels", '{}'::jsonb), ARRAY[$2]::text[], $3::jsonb, true)`
	if !strings.Contains(q, want) {
		t.Errorf("rendered\n\t%s\nwant it to contain\n\t%s", q, want)
	}
	if len(args) != 3 {
		t.Fatalf("bound %d arguments, want 3: %#v", len(args), args)
	}
}

// TestJSONEditNestsOnAList walks a list through more than one operation, which
// is where the shape of the expression shows: the operations wrap each other so
// that the column is assigned once, and the innermost one starts from an empty
// document rather than from the NULL an unset column holds.
func TestJSONEditNestsOnAList(t *testing.T) {
	e := userEntity(t)
	plan := &ormpatch.Plan{
		Entity: e,
		Writes: []ormpatch.Write{{
			Prop: prop(t, e, "scores"),
			Op: ormpatch.EditJSON{Ops: []ormpatch.JSONOp{
				{Kind: ormpatch.JSONAppend, Value: protoreflect.ValueOfInt32(7), HasValue: true},
				{Kind: ormpatch.JSONRemove, Index: 0, HasIndex: true},
			}},
		}},
	}

	for _, tt := range []struct {
		dialect string
		want    string
	}{
		{dialect.SQLite, "SET `scores` = JSON_REMOVE(JSON_INSERT(COALESCE(`scores`, '[]'), '$[#]', JSON(?)), ?)"},
		{dialect.Postgres, `SET "scores" = ((COALESCE("scores", '[]'::jsonb) || jsonb_build_array($1::jsonb)) - $2::int)`},
	} {
		t.Run(tt.dialect, func(t *testing.T) {
			_, mod, err := entpatch.Build(plan, userColumns, tt.dialect)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			u := sql.Dialect(tt.dialect).Update("user")
			mod(u)
			if err := u.Err(); err != nil {
				t.Fatalf("the modifier failed: %v", err)
			}

			if q, _ := u.Query(); !strings.Contains(q, tt.want) {
				t.Errorf("rendered\n\t%s\nwant it to contain\n\t%s", q, tt.want)
			}
		})
	}
}

func assertRawJSON(t *testing.T, args []any, want []byte) {
	t.Helper()

	if len(args) != 1 {
		t.Fatalf("bound %d arguments, want 1: %#v", len(args), args)
	}
	got, ok := args[0].(json.RawMessage)
	if !ok {
		t.Fatalf("bound %#v (%T), want a json.RawMessage as ent binds", args[0], args[0])
	}
	if string(got) != string(want) {
		t.Errorf("bound %s, want %s", got, want)
	}
}

// at is a test on one element of a list, in is one on one entry of a map.
func at(t *testing.T, e graph.Entity, field string, i int64, v protoreflect.Value) ormpatch.Test {
	t.Helper()
	return ormpatch.Test{
		Prop: prop(t, e, field), HasIndex: true, Index: i,
		Want: ormpatch.TestEqual, Value: v,
	}
}

func in(t *testing.T, e graph.Entity, field, key string, v protoreflect.Value) ormpatch.Test {
	t.Helper()
	return ormpatch.Test{
		Prop: prop(t, e, field), HasKey: true, Key: protoreflect.ValueOfString(key).MapKey(),
		Want: ormpatch.TestEqual, Value: v,
	}
}

// entryArgs renders one test against a JSON entry and returns the value it
// compares against. The path is bound first, so the value is the second
// argument in either dialect.
func entryArgs(t *testing.T, e graph.Entity, d string, test ormpatch.Test) any {
	t.Helper()

	pred, _, err := entpatch.Build(&ormpatch.Plan{Entity: e, Tests: []ormpatch.Test{test}}, userColumns, d)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	s := sql.Dialect(d).Select().From(sql.Table("user"))
	pred(s)
	_, args := s.Query()
	if len(args) != 2 {
		t.Fatalf("bound %d arguments, want the path and the value: %#v", len(args), args)
	}
	return args[1]
}

// pointValue builds a message value the way a compiled plan carries one: a
// dynamic message over the descriptor, since ormpatch has no generated type to
// allocate.
func pointValue(t *testing.T, e graph.Entity, s string) protoreflect.Value {
	t.Helper()

	md := e.Descriptor().ParentFile().Messages().ByName("Point")
	if md == nil {
		t.Fatal("the test schema declares no Point")
	}
	m := dynamicpb.NewMessage(md)
	m.Set(md.Fields().ByName("s"), protoreflect.ValueOfString(s))
	return protoreflect.ValueOfMessage(m)
}

// mapValue builds the value a compiled plan carries for a whole map column: a
// protoreflect.Map, which is what ormpatch.Compile produces over the descriptor
// since it has no instance of the entity to allocate into.
func mapValue(t *testing.T, e graph.Entity, field string, kv map[string]string) protoreflect.Value {
	t.Helper()

	md := e.Descriptor()
	fd := md.Fields().ByName(protoreflect.Name(field))
	if fd == nil {
		t.Fatalf("%s declares no %s", md.FullName(), field)
	}

	m := dynamicpb.NewMessage(md)
	entries := m.Mutable(fd).Map()
	for k, v := range kv {
		entries.Set(protoreflect.ValueOfString(k).MapKey(), protoreflect.ValueOfString(v))
	}
	return m.Get(fd)
}

func prop(t *testing.T, e graph.Entity, name string) graph.Prop {
	t.Helper()

	for p := range e.Props() {
		if p.Name() == name {
			return p
		}
	}
	t.Fatalf("%s has no prop %s", e.FullName(), name)
	return nil
}

func userEntity(t *testing.T) graph.Entity {
	t.Helper()

	e, err := ormpatch.EntityOf(testSchema(t), "User")
	if err != nil {
		t.Fatalf("resolve User: %v", err)
	}
	return e
}

func testSchema(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()

	tenant := &descriptorpb.DescriptorProto{
		Name: proto.String("Tenant"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_BYTES, ormpb.FieldOptions_builder{
				Type: ormpb.Type_TYPE_UUID.Enum(),
				Key:  proto.Bool(true),
			}.Build()),
		},
		Options: messageOptions(),
	}

	user := &descriptorpb.DescriptorProto{
		Name: proto.String("User"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_BYTES, ormpb.FieldOptions_builder{
				Type: ormpb.Type_TYPE_UUID.Enum(),
				Key:  proto.Bool(true),
			}.Build()),
			message("tenant", 2, ".entpatchtest.Tenant", false, edgeOptions()),
			field("name", 3, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
			message("labels", 4, ".entpatchtest.User.LabelsEntry", true, nil),
			list("scores", 5, descriptorpb.FieldDescriptorProto_TYPE_INT32),
			list("blobs", 6, descriptorpb.FieldDescriptorProto_TYPE_BYTES),
			list("ratios", 7, descriptorpb.FieldDescriptorProto_TYPE_FLOAT),
			message("points", 8, ".entpatchtest.Point", true, nil),
			message("tokens", 9, ".entpatchtest.User.TokensEntry", true, nil),
			message("shapes", 10, ".entpatchtest.User.ShapesEntry", true, nil),
		},
		NestedType: []*descriptorpb.DescriptorProto{
			mapEntry("LabelsEntry", field("value", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil)),
			mapEntry("TokensEntry", field("value", 2, descriptorpb.FieldDescriptorProto_TYPE_BYTES, nil)),
			mapEntry("ShapesEntry", message("value", 2, ".entpatchtest.Point", false, nil)),
		},
		Options: messageOptions(),
	}

	// Point is deliberately not an entity: a message field that is one parses to
	// an edge, and what these tests need is a message stored inside a JSON
	// column.
	point := &descriptorpb.DescriptorProto{
		Name: proto.String("Point"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("x", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, nil),
			field("s", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
		},
	}

	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        proto.String("entpatchtest/schema.proto"),
		Package:     proto.String("entpatchtest"),
		Syntax:      proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{tenant, point, user},
	}, nil)
	if err != nil {
		t.Fatalf("build the test schema: %v", err)
	}
	return fd
}

// mapEntry is the synthetic message a map field's descriptor points at.
func mapEntry(name string, value *descriptorpb.FieldDescriptorProto) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String(name),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("key", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
			value,
		},
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
	}
}

func field(name string, number int32, kind descriptorpb.FieldDescriptorProto_Type, opts *ormpb.FieldOptions) *descriptorpb.FieldDescriptorProto {
	f := &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		JsonName: proto.String(name),
		Number:   proto.Int32(number),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:     kind.Enum(),
	}
	if opts != nil {
		f.Options = &descriptorpb.FieldOptions{}
		proto.SetExtension(f.Options, ormpb.E_Field, opts)
	}
	return f
}

func message(name string, number int32, target string, repeated bool, opts *ormpb.EdgeOptions) *descriptorpb.FieldDescriptorProto {
	f := field(name, number, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, nil)
	f.TypeName = proto.String(target)
	if repeated {
		f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	}
	if opts != nil {
		f.Options = &descriptorpb.FieldOptions{}
		proto.SetExtension(f.Options, ormpb.E_Edge, opts)
	}
	return f
}

func list(name string, number int32, kind descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	f := field(name, number, kind, nil)
	f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return f
}

func messageOptions() *descriptorpb.MessageOptions {
	opts := &descriptorpb.MessageOptions{}
	proto.SetExtension(opts, ormpb.E_Message, ormpb.MessageOptions_builder{
		Rpc: ormpb.RpcOptions_builder{Crud: proto.Bool(true)}.Build(),
	}.Build())
	return opts
}

func edgeOptions() *ormpb.EdgeOptions {
	return ormpb.EdgeOptions_builder{}.Build()
}
