package entpatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"uuid"

	"github.com/protobuf-orm/ent/schema/field"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/entuuid"
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

// uuidCodec is what a uuid.UUID is written as, which is what a UUID column
// holds. See field.Uuid.
var uuidCodec = field.TextValueScannerOf[uuid.UUID]()

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
		u, err := entuuid.FromBytes(b)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrValue, p.Name(), err)
		}
		// A uuid.UUID says nothing to a driver on its own, so the column holds
		// whatever ent's codec for the type wrote. Asking that codec is how
		// this stays the same value the schema generator stores, rather than a
		// second opinion on how a UUID is spelled.
		return uuidCodec.Value(u)

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
//
// It is also what a comparison against one entry binds on PostgreSQL, where the
// extraction yields jsonb and the argument is parsed as jsonb beside it. See
// [insideArg] for the other dialect, which decodes instead.
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

// insideArg renders a value for COMPARISON against one element already inside a
// JSON document, as the element the EXTRACTION hands back.
//
// It goes through [jsonScalar] and [marshal] rather than converting the value
// again, and that is the whole point. The column holds whatever encoding/json
// wrote, and a comparison spelled any other way is a test that can never hold
// -- which does not merely fail, it abandons the document and silently drops
// every other entry with it. Two spellings used to differ: bytes, which the
// writer stores as base64 and this compared as the raw bytes, and a message,
// which the writer compacts and HTML-escapes on its way through encoding/json
// and this compared as protojson left it, spacing and all.
//
// What it does not keep is the marshalled form's outer quoting, because the
// surrounding predicate extracts the element first: SQLite's JSON_EXTRACT
// decodes a JSON string into a plain one, so `"3q2+7w=="` in the column arrives
// at the comparison as 3q2+7w==. An object or an array arrives as the text it
// is stored as, and a number as the number that text parses to -- which is not
// always the number Go would have bound for it.
//
// PostgreSQL extracts jsonb instead of a decoded value, so it compares against
// [jsonText] -- the same form the write binds there. See [predicate].
func insideArg(v protoreflect.Value) (any, error) {
	e, err := jsonScalar(v)
	if err != nil {
		return nil, err
	}
	// The element exactly as the write spelled it. Nothing below re-encodes;
	// they only undo the reading the extraction already did.
	b, err := marshal(e)
	if err != nil {
		return nil, err
	}

	switch e.(type) {
	case json.RawMessage:
		return string(b), nil

	case []byte, string:
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrValue, err)
		}
		if string(b) == "null" {
			// A nil []byte marshals to null, and unmarshalling null into a
			// string leaves it empty and reports nothing -- so the comparison
			// would quietly become one against "" while the write bound null.
			// Nothing reaches here with a nil slice today (ValueOf normalizes
			// one and the wire cannot carry it), which is why this says so
			// rather than guessing which of the two was meant.
			return nil, fmt.Errorf("%w: a nil bytes value has no element form", ErrValue)
		}
		return s, nil

	case float32:
		// A float32 is written with 32-bit precision -- 3.14, not the
		// 3.140000104904175 it widens to -- and the row parses that text as a
		// double. Binding the widened value compares the two and they are never
		// equal, so the value has to come back through the text as well.
		f, err := strconv.ParseFloat(string(b), 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrValue, err)
		}
		return f, nil
	}

	return e, nil
}
