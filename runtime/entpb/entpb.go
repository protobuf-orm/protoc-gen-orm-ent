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
// `field.JSON` cannot be given a codec either: ent hangs `ValueScanner` off the
// string and bytes builders and not off the JSON one. So the field is a string
// and the codec is this.
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
	"fmt"

	"entgo.io/ent/schema/field"
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
