// Package entpb stores a protobuf message in a column.
//
// # What it is for
//
// A field of a message type that is not an edge -- a value that belongs to the
// row rather than a row of its own. An address, a set of preferences, whatever
// an identity provider said about somebody. It has no key, no lifecycle and
// nothing points at it; it is written and read with its owner.
//
// # Why it is not `field.JSON`
//
// That is what a message field became before this, and it silently stored
// nothing. `field.JSON` marshals with `encoding/json`, and a message generated
// with the opaque API has no exported fields at all:
//
//	type Profile struct {
//		state                  protoimpl.MessageState `protogen:"opaque.v1"`
//		xxx_hidden_DisplayName string
//	}
//
// So every such value round-tripped as `{}` -- an insert that reported success,
// a row that compared equal to empty, and no error anywhere. The failure was
// invisible from Go, from the schema and from the database.
//
// So the field needs a codec of its own, and this is it. A JSON field takes
// one: ent wraps what the codec returns in a `json.RawMessage` on the way to
// the column, so it is written as it is rather than encoded a second time.
// The field is a `field.JSON`, and the column type comes from the builder.
//
// # What is stored
//
// The canonical protobuf JSON, as text:
//
//	{"displayName":"Ada Lovelace","email":"ada@acme.example"}
//
// Text rather than the binary encoding because the value is small, and because
// a column somebody can read in a database client is worth a great deal when
// the question is "what is actually in there".
//
// **The cost is that the field names are the storage.** Renaming a field of a
// message stored this way does not break a build and does not break the wire --
// it silently stops finding the old value. Renumbering is what the wire cares
// about and it is guarded elsewhere; renaming is what this cares about, and it
// is a migration. There is nothing here that can check it.
package entpb

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/protobuf-orm/ent/schema/field"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ValueScanner stores `T` as its canonical protobuf JSON.
//
//	field.String("profile").
//		GoType(&Profile{}).
//		ValueScanner(entpb.ValueScanner[*Profile]{}).
//		Optional()
//
// It is one type for every message rather than one per message, which is the
// whole reason it can be generated: what it needs to know is that the value is
// a [proto.Message], and that is the only thing it asks.
type ValueScanner[T proto.Message] struct{}

// Value is the message as text, and nil for a message that is not there.
//
// Nil rather than `{}` so that "no value" and "a value with nothing in it" are
// different rows. A column that cannot tell those apart cannot answer whether
// somebody has a profile.
func (ValueScanner[T]) Value(v T) (driver.Value, error) {
	if !v.ProtoReflect().IsValid() {
		return nil, nil
	}

	b, err := protojson.Marshal(v)
	if err != nil {
		return nil, err
	}

	return string(b), nil
}

// ScanValue is what the database value is read into before [ValueScanner.FromValue]
// turns it back into a message.
func (ValueScanner[T]) ScanValue() field.ValueScanner { return &sql.NullString{} }

// FromValue is the text as a message, and the zero `T` for SQL NULL.
func (ValueScanner[T]) FromValue(v driver.Value) (T, error) {
	var zero T

	s, ok := v.(*sql.NullString)
	if !ok {
		return zero, fmt.Errorf("entpb: expected *sql.NullString, got %T", v)
	}
	if !s.Valid {
		return zero, nil
	}

	// A new one through the descriptor rather than `new(E)` behind a type
	// parameter: `T` is the pointer type, and there is no way to write "the
	// thing it points at" in a constraint. The zero value is a typed nil, whose
	// ProtoReflect still answers with the descriptor -- which is the whole
	// reason a generic like this can build a message at all.
	m := zero.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal([]byte(s.String), m); err != nil {
		return zero, err
	}

	return m.(T), nil
}

// ListValueScanner stores `[]T` as a JSON array of canonical protobuf JSON.
//
//	field.Json("attachments", []*Attachment{}).
//		ValueScanner(entpb.ListValueScanner[*Attachment]{})
//
// A list needs one of its own because [ValueScanner] converts a message and a
// list is not one. Without it a list of messages is an ordinary `field.Json`,
// marshalled by `encoding/json` -- which is what could not see an opaque
// message to begin with, and which finds its unexported bookkeeping instead:
//
//	[{"XXX_raceDetectHookData":{},"XXX_presence":[3]}]
//
// The elements are assembled by hand rather than handed to `encoding/json`,
// because that is the thing being avoided: protojson writes each element and
// the brackets and commas are written here.
type ListValueScanner[T proto.Message] struct{}

// Value is the list as a JSON array, and nil for a list that is not there.
//
// Nil rather than `[]` for the same reason [ValueScanner.Value] answers nil for
// an absent message: a column that cannot tell "no list" from "an empty list"
// cannot answer whether anything was ever set.
func (ListValueScanner[T]) Value(vs []T) (driver.Value, error) {
	if vs == nil {
		return nil, nil
	}

	var b []byte
	b = append(b, '[')
	for i, v := range vs {
		if i > 0 {
			b = append(b, ',')
		}
		if !v.ProtoReflect().IsValid() {
			b = append(b, "null"...)
			continue
		}
		e, err := protojson.Marshal(v)
		if err != nil {
			return nil, err
		}
		b = append(b, e...)
	}
	b = append(b, ']')

	return string(b), nil
}

// ScanValue is what the database value is read into.
func (ListValueScanner[T]) ScanValue() field.ValueScanner { return &sql.NullString{} }

// FromValue is the array as a list, and nil for SQL NULL.
func (ListValueScanner[T]) FromValue(v driver.Value) ([]T, error) {
	s, ok := v.(*sql.NullString)
	if !ok {
		return nil, fmt.Errorf("entpb: expected *sql.NullString, got %T", v)
	}
	if !s.Valid {
		return nil, nil
	}

	// Split into elements by `encoding/json`, which can be trusted with the
	// shape of a document; each element is then read by protojson, which is
	// the only thing that can read a message. json.RawMessage is what says
	// "hold this one aside rather than decoding it".
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(s.String), &raw); err != nil {
		return nil, err
	}

	vs := make([]T, 0, len(raw))
	for _, e := range raw {
		v, err := messageFrom[T](e)
		if err != nil {
			return nil, err
		}
		vs = append(vs, v)
	}

	return vs, nil
}

// MapValueScanner stores `map[K]T` as a JSON object of canonical protobuf JSON.
//
//	field.Json("by_locale", map[string]*Greeting{}).
//		ValueScanner(entpb.MapValueScanner[string, *Greeting]{})
//
// See [ListValueScanner] for why a map cannot be an ordinary `field.Json`.
//
// K is the protobuf map key, which the language limits to an integer, a bool or
// a string; all three are keys `encoding/json` writes, so the object half is
// left to it.
type MapValueScanner[K comparable, T proto.Message] struct{}

// Value is the map as a JSON object, and nil for a map that is not there.
func (MapValueScanner[K, T]) Value(vs map[K]T) (driver.Value, error) {
	if vs == nil {
		return nil, nil
	}

	raw := make(map[K]json.RawMessage, len(vs))
	for k, v := range vs {
		if !v.ProtoReflect().IsValid() {
			raw[k] = json.RawMessage("null")
			continue
		}
		e, err := protojson.Marshal(v)
		if err != nil {
			return nil, err
		}
		raw[k] = json.RawMessage(e)
	}

	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	return string(b), nil
}

// ScanValue is what the database value is read into.
func (MapValueScanner[K, T]) ScanValue() field.ValueScanner { return &sql.NullString{} }

// FromValue is the object as a map, and nil for SQL NULL.
func (MapValueScanner[K, T]) FromValue(v driver.Value) (map[K]T, error) {
	s, ok := v.(*sql.NullString)
	if !ok {
		return nil, fmt.Errorf("entpb: expected *sql.NullString, got %T", v)
	}
	if !s.Valid {
		return nil, nil
	}

	var raw map[K]json.RawMessage
	if err := json.Unmarshal([]byte(s.String), &raw); err != nil {
		return nil, err
	}

	vs := make(map[K]T, len(raw))
	for k, e := range raw {
		v, err := messageFrom[T](e)
		if err != nil {
			return nil, err
		}
		vs[k] = v
	}

	return vs, nil
}

// messageFrom reads one element of a collection.
//
// The new message comes through the descriptor of the zero value, for the
// reason [ValueScanner.FromValue] gives: T is the pointer type, and a
// constraint cannot name what it points at.
func messageFrom[T proto.Message](b []byte) (T, error) {
	var zero T
	if string(b) == "null" {
		return zero, nil
	}

	m := zero.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(b, m); err != nil {
		return zero, err
	}

	return m.(T), nil
}
