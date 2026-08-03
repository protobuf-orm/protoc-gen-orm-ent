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
	"github.com/protobuf-orm/protoc-gen-orm-ent/entpatch"
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
//	message User {
//	  bytes id = 1 [(orm.field) = {type: TYPE_UUID, key: true}];
//	  Tenant tenant = 2 [(orm.edge) = {}];
//	  string name = 3;
//	  map<string, string> labels = 4;
//	  repeated int32 scores = 5;
//	}
var userColumns = entpatch.Columns{
	1: "id",
	// An edge's column is the foreign key ent named for the relation, which is
	// why Columns exists at all.
	2: "user_tenant",
	3: "name",
	4: "labels",
	5: "scores",
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

	pred, mod, err := entpatch.Build(plan, userColumns)
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

	pred, _, err := entpatch.Build(plan, userColumns)
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
		_, mod, err := entpatch.Build(plan, userColumns)
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
		pred, _, err := entpatch.Build(plan, userColumns)
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
		pred, _, err := entpatch.Build(plan, userColumns)
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

	// The same Build must answer both, which is what makes the choice a render-
	// time one rather than something Build could have decided.
	t.Run("one plan serves both dialects", func(t *testing.T) {
		plan := &ormpatch.Plan{
			Entity: e,
			Tests:  []ormpatch.Test{{Prop: labels, Want: ormpatch.TestEqual, Value: v}},
		}
		pred, _, err := entpatch.Build(plan, userColumns)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		for _, d := range []string{dialect.Postgres, dialect.SQLite, dialect.Postgres} {
			s := sql.Dialect(d).Select().From(sql.Table("user"))
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

		_, _, err := entpatch.Build(plan, userColumns)
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

		_, _, err := entpatch.Build(plan, userColumns)
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

		_, _, err := entpatch.Build(plan, entpatch.Columns{})
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

	_, mod, err := entpatch.Build(plan, userColumns)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// One plan, three connections: the JSON spelling is the dialect's, so it
	// stays deferred to the statement even though everything else is resolved.
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

	t.Run("mysql is refused where the refusal is read", func(t *testing.T) {
		u := sql.Dialect(dialect.MySQL).Update("user")
		mod(u)

		// ent checks UpdateBuilder.Err before it issues the statement. Raising
		// this inside the expression instead would lose it: an ExprFunc renders
		// into a clone of the builder and only its text and arguments are kept.
		err := u.Err()
		if err == nil {
			t.Fatal("MySQL was accepted")
		}
		if !strings.Contains(err.Error(), "MySQL") {
			t.Errorf("unexpected error: %v", err)
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

	_, mod, err := entpatch.Build(plan, userColumns)
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

	_, mod, err := entpatch.Build(plan, userColumns)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, tt := range []struct {
		dialect string
		want    string
	}{
		{dialect.SQLite, "SET `scores` = JSON_REMOVE(JSON_INSERT(COALESCE(`scores`, '[]'), '$[#]', JSON(?)), ?)"},
		{dialect.Postgres, `SET "scores" = ((COALESCE("scores", '[]'::jsonb) || jsonb_build_array($1::jsonb)) - $2::int)`},
	} {
		t.Run(tt.dialect, func(t *testing.T) {
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
		},
		NestedType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("LabelsEntry"),
			Field: []*descriptorpb.FieldDescriptorProto{
				field("key", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
				field("value", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
			},
			Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		}},
		Options: messageOptions(),
	}

	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        proto.String("entpatchtest/schema.proto"),
		Package:     proto.String("entpatchtest"),
		Syntax:      proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{tenant, user},
	}, nil)
	if err != nil {
		t.Fatalf("build the test schema: %v", err)
	}
	return fd
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
