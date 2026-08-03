package entpatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ErrValue marks a conversion that failed on the value the document carried --
// a UUID that is not sixteen bytes, a timestamp that is not one, a message that
// will not marshal. It exists so a server can tell the two kinds of [Build]
// failure apart: this one is the client's to fix and deserves to be said so,
// where anything else is ours.
//
// A column missing from [Columns] deliberately is NOT this. It means the
// generator and the schema disagree about the entity, which no request can
// correct, so it stays a plain error.
var ErrValue = errors.New("entpatch: value cannot be stored in this column")

// argPos is where a value lands in the statement, which is what decides how a
// JSON column's value binds.
type argPos int

const (
	// posColumn is a whole column's new value in a SET clause.
	posColumn argPos = iota
	// posCompare is a whole column's value on the right of a comparison.
	posCompare
	// posInner is a value inside a JSON expression -- one entry of a map, one
	// element of a list.
	posInner
)

// arg renders a plan value as a statement argument.
//
// A JSON column binds differently in each position, and following ent in each
// is the whole point: a value written where ent would write one has to look
// like ent's, and a value compared against a column ent wrote has to look like
// whatever the driver will compare successfully. They are not the same. See
// [jsonArg] and [jsonText].
//
// The conversions mirror what the ent schema generator chose for each ormpb
// type, which is why they are keyed on the prop's Type rather than on the
// protobuf kind: a UUID is bytes on the wire and a uuid.UUID in the column, and
// a version field is a Timestamp on the wire and a time.Time in the column.
func arg(p graph.Prop, v protoreflect.Value, pos argPos) (any, error) {
	if pos == posInner {
		return jsonText(v)
	}

	if graph.IsCollection(p) || p.Type() == ormpb.Type_TYPE_JSON {
		if pos == posCompare {
			// A whole-column comparison does not come through here -- it needs
			// the dialect, so predicate() builds both forms and chooses at
			// render time. This is the path for anything else compared against
			// a JSON column, and text is what ent binds in that position
			// (sqljson.marshalArg) in every dialect.
			return jsonText(v)
		}
		return jsonArg(v)
	}
	return scalarArg(p, v)
}

// scalarArg converts one value of the prop's own type.
//
// Two kinds of failure live here and they are told apart by [ErrValue]. A value
// that is the right shape but not a usable one -- bytes that are not sixteen --
// is the client's, and wraps it. A value whose SHAPE is wrong means the ORM
// type and the field descriptor disagree, which a request cannot cause, because
// the document's value was checked against that descriptor before it arrived;
// those stay plain.
func scalarArg(p graph.Prop, v protoreflect.Value) (any, error) {
	switch p.Type() {
	case ormpb.Type_TYPE_UUID:
		b, ok := v.Interface().([]byte)
		if !ok {
			return nil, fmt.Errorf("entpatch: %s is declared a UUID but holds %T", p.Name(), v.Interface())
		}
		u, err := uuid.FromBytes(b)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrValue, p.Name(), err)
		}
		return u, nil

	case ormpb.Type_TYPE_TIME:
		// Neither refusal below wraps ErrValue. A document's value is checked
		// against the field's own descriptor long before it reaches here, so a
		// value that is not a Timestamp means the schema called something a
		// time that is not one. The client cannot correct that and should not
		// be told to.
		m, ok := v.Interface().(protoreflect.Message)
		if !ok {
			return nil, fmt.Errorf("entpatch: %s is declared a time but holds %T", p.Name(), v.Interface())
		}
		ts, ok := m.Interface().(*timestamppb.Timestamp)
		if !ok {
			// A dynamic message carries the same fields; read them directly.
			// Ask what it is first: nothing checks that a prop declared
			// TYPE_TIME is backed by a Timestamp, and reading a field the
			// message does not declare would panic on a nil descriptor.
			d := m.Descriptor()
			if d.FullName() != "google.protobuf.Timestamp" {
				return nil, fmt.Errorf("entpatch: %s is declared a time but holds a %s", p.Name(), d.FullName())
			}
			secs := m.Get(d.Fields().ByName("seconds")).Int()
			nanos := m.Get(d.Fields().ByName("nanos")).Int()
			return time.Unix(secs, nanos).UTC(), nil
		}
		return ts.AsTime().UTC(), nil

	case ormpb.Type_TYPE_ENUM:
		// The schema stores an enum as int32; protobuf gives its number.
		return int32(v.Enum()), nil
	}

	return v.Interface(), nil
}

// jsonArg renders a value as the argument a whole JSON COLUMN takes.
//
// It binds what ent binds. ent marshals a JSON field and wraps the result in
// json.RawMessage before it reaches the driver, and the wrapper is not
// cosmetic: go-sqlite3 stores []byte as a BLOB and a string as TEXT, and SQLite
// compares values of different storage classes as unequal without complaint. A
// string here would therefore write a column that no whole-map `test` against
// an Add-written row could ever match, and the test would fail with nothing to
// point at.
//
// It does NOT make whole-column JSON tests reliable, and this package cannot:
//
//   - A partial edit assigns the output of JSON_SET and friends, and SQLite's
//     JSON functions return TEXT. So a row that was edited through [editJSON]
//     has flipped class anyway, and the next whole-column test misses it.
//   - Equality is over serializations, and the two writers do not agree on one.
//     SQLite keeps the key order it was handed, while encoding/json sorts a
//     map's keys, so a document that went through JSON_SET and one written
//     whole differ by key order alone. protojson, which renders any message
//     inside the document, also varies its spacing from build to build on
//     purpose -- encoding/json compacts that back out here, but nothing
//     promises it stays that way.
//
// A test against one entry has none of these problems, because sqljson extracts
// the element first and the comparison is on the element.
func jsonArg(v protoreflect.Value) (any, error) {
	b, err := jsonBytes(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// jsonText renders a value as the JSON text a JSON EXPRESSION takes.
//
// This one stays a string on purpose, unlike [jsonArg]. Its value is an
// argument to the JSON(...) or the ...::jsonb around it in the expression
// [editJSON] builds, and there the surrounding function parses what it is
// handed: PostgreSQL has no cast from bytea to jsonb, and SQLite reads a BLOB
// as its own binary JSONB encoding whenever that parses, falling back to the
// text reading only when it does not -- an ambiguity with no upside. ent's own
// sqljson binds this position as a string for every dialect, for the same
// reason.
func jsonText(v protoreflect.Value) (any, error) {
	b, err := jsonBytes(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func jsonBytes(v protoreflect.Value) ([]byte, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Map:
		out := map[string]any{}
		var err error
		x.Range(func(k protoreflect.MapKey, e protoreflect.Value) bool {
			var ev any
			ev, err = jsonScalar(e)
			if err != nil {
				return false
			}
			out[fmt.Sprint(k.Interface())] = ev
			return true
		})
		if err != nil {
			return nil, err
		}
		return marshal(out)

	case protoreflect.List:
		out := make([]any, 0, x.Len())
		for i := range x.Len() {
			ev, err := jsonScalar(x.Get(i))
			if err != nil {
				return nil, err
			}
			out = append(out, ev)
		}
		return marshal(out)
	}

	e, err := jsonScalar(v)
	if err != nil {
		return nil, err
	}
	return marshal(e)
}

func jsonScalar(v protoreflect.Value) (any, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Message:
		// A message inside a JSON column is stored as its protojson form, the
		// same shape the ent conversion helpers write.
		b, err := protojson.Marshal(x.Interface())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrValue, err)
		}
		return json.RawMessage(b), nil
	case protoreflect.EnumNumber:
		return int32(x), nil
	case []byte:
		return x, nil
	default:
		return x, nil
	}
}

func marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValue, err)
	}
	return b, nil
}

// insideArg renders a value for COMPARISON against something already inside a
// JSON document.
//
// It is not jsonArg: the surrounding predicate extracts the element first, so
// the comparison is against the element's own type. Binding the marshalled form
// would compare a quoted string to an unquoted one and never match.
func insideArg(v protoreflect.Value) (any, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Message:
		b, err := protojson.Marshal(x.Interface())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrValue, err)
		}
		return string(b), nil
	case protoreflect.EnumNumber:
		return int32(x), nil
	case []byte:
		return string(x), nil
	default:
		return x, nil
	}
}
