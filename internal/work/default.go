package work

import (
	"fmt"
	"math"
	"strconv"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Default is the value a field with `(orm.field).default` holds when nobody
// said otherwise, as the Go literal that writes it -- or false for a field
// whose default is made each time rather than written down: a uuid key is
// minted, a time is now, a message is absent.
//
// # One answer, for two places that used to give two
//
// The server writes it into an `Add` that leaves the field out, and the schema
// gives it to the column, so that a row which predates the column holds it
// too. Asked here once, so that the two cannot disagree -- which they did
// before the schema asked at all: the server wrote the zero value whatever the
// option said, `default: "7"` included, and the schema wrote nothing, so a
// column added to a table with rows in it left them NULL.
//
// # What a default may say
//
// The value, written as the field's type reads it: a string as it is, a number
// in decimal, a bool as `true` or `false`, an enum by the name of its value or
// its number. Empty is the zero value, which is what every default written so
// far says.
//
// Anything else is refused rather than read as the zero value. A default that
// was not honoured is a value somebody chose and nothing kept, and that is the
// one mistake a default exists to prevent. So is a non-empty default on a
// field whose default is made rather than written -- bytes, a key, a time, a
// message -- since there is nothing for it to have said.
func Default(p graph.Field) (string, bool) {
	if !p.HasDefault() {
		return "", false
	}

	opts, _ := proto.GetExtension(p.Descriptor().Options(), ormpb.E_Field).(*ormpb.FieldOptions)
	v, ok, err := ParseDefault(p.Type(), p.Descriptor().Enum(), opts.GetDefault())
	if err != nil {
		panic(fmt.Errorf("%s: %w", p.FullName(), err))
	}

	return v, ok
}

// ParseDefault is [Default] for a type, the enum its values are named by when it
// is one, and what the option said.
func ParseDefault(t ormpb.Type, enum protoreflect.EnumDescriptor, v string) (string, bool, error) {
	switch t {
	case ormpb.Type_TYPE_STRING:
		return strconv.Quote(v), true, nil

	case ormpb.Type_TYPE_BOOL:
		switch v {
		case "", "false":
			return "false", true, nil
		case "true":
			return "true", true, nil
		}

		return "", false, fmt.Errorf("default %q: a bool is `true` or `false`", v)

	case ormpb.Type_TYPE_ENUM:
		if v == "" {
			return "0", true, nil
		}
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			return strconv.FormatInt(n, 10), true, nil
		}
		if enum != nil {
			if e := enum.Values().ByName(protoreflect.Name(v)); e != nil {
				return strconv.FormatInt(int64(e.Number()), 10), true, nil
			}
		}

		return "", false, fmt.Errorf("default %q: no value of the enum is called that", v)

	case ormpb.Type_TYPE_INT32, ormpb.Type_TYPE_SINT32, ormpb.Type_TYPE_SFIXED32:
		return parseInt(v, 32)
	case ormpb.Type_TYPE_INT64, ormpb.Type_TYPE_SINT64, ormpb.Type_TYPE_SFIXED64:
		return parseInt(v, 64)
	case ormpb.Type_TYPE_UINT32, ormpb.Type_TYPE_FIXED32:
		return parseUint(v, 32)
	case ormpb.Type_TYPE_UINT64, ormpb.Type_TYPE_FIXED64:
		return parseUint(v, 64)
	case ormpb.Type_TYPE_FLOAT:
		return parseFloat(v, 32)
	case ormpb.Type_TYPE_DOUBLE:
		return parseFloat(v, 64)

	case ormpb.Type_TYPE_BYTES:
		// A value, and one the schema cannot give a column -- ent writes a
		// `[]byte` default into the DDL as `[]` -- so the server is the only
		// place it is written, and empty is the only one there is.
		if v != "" {
			return "", false, fmt.Errorf("default %q: bytes default to empty and to nothing else", v)
		}

		return "[]byte{}", true, nil
	}

	if v != "" {
		return "", false, fmt.Errorf("default %q: a %s is made rather than written, so its default says nothing", v, t)
	}

	return "", false, nil
}

func parseInt(v string, bits int) (string, bool, error) {
	if v == "" {
		return "0", true, nil
	}

	n, err := strconv.ParseInt(v, 10, bits)
	if err != nil {
		return "", false, fmt.Errorf("default %q: not an integer of %d bits", v, bits)
	}

	return strconv.FormatInt(n, 10), true, nil
}

func parseUint(v string, bits int) (string, bool, error) {
	if v == "" {
		return "0", true, nil
	}

	n, err := strconv.ParseUint(v, 10, bits)
	if err != nil {
		return "", false, fmt.Errorf("default %q: not an unsigned integer of %d bits", v, bits)
	}

	return strconv.FormatUint(n, 10), true, nil
}

func parseFloat(v string, bits int) (string, bool, error) {
	if v == "" {
		return "0", true, nil
	}

	f, err := strconv.ParseFloat(v, bits)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", false, fmt.Errorf("default %q: not a finite number", v)
	}

	return strconv.FormatFloat(f, 'g', -1, bits), true, nil
}
