// Package entpage is the half of a list that is not about the domain.
//
// What a list filters by is the app's, and there is no general answer to it. A
// cursor is not: naming the last row seen, comparing against it in the right
// order, and capping how many rows come back look the same for every entity,
// and they are the half people get wrong. Offsets that skip rows when something
// is inserted, an order with ties and no tiebreaker, a limit a request can set
// to a million.
//
// Nothing here is generated. A list is written by hand because its filtering
// is, and this is what that hand-written list borrows.
package entpage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/protobuf-orm/ent/dialect/sql"
)

// ErrCursor is what a cursor that cannot be read is. A caller is given one by
// this server and hands it back, so a broken one is the caller's mistake -- but
// it is also what an old cursor looks like after the order it was made for has
// changed, which is worth telling apart from a row that is not there.
var ErrCursor = errors.New("not a cursor of this list")

// Order is a column and the direction it is read in.
//
// The last one has to be unique, and that is not this package's to check. Two
// rows that compare equal in every column of the order are two rows a cursor
// cannot tell apart: the page after the first of them either repeats the second
// or skips it, and which one it is depends on the storage engine. A key is
// always safe as the last entry, and is what a tiebreaker means.
type Order struct {
	Column string
	Desc   bool
}

// After narrows a query to the rows that come strictly after `at` in the order
// `by` describes, where `at` is the value of each of those columns in the last
// row of the page before.
//
// Written out rather than as a row-value comparison -- `c1 < v1 OR (c1 = v1 AND
// c2 < v2)` rather than `(c1, c2) < (v1, v2)` -- because the second is not
// spelled the same everywhere and is missing outright in older engines. The
// first is what every one of them takes.
//
// This is a keyset and not an offset, which is the whole reason to bother. An
// offset counts rows from the start every time, so a row inserted ahead of the
// page shifts everything by one and a caller reading through a list sees a row
// twice or not at all; and the count itself gets slower the further in the
// caller reads.
func After(by []Order, at []any) (func(*sql.Selector), error) {
	if len(by) != len(at) {
		return nil, fmt.Errorf("%w: it names %d columns and the order has %d", ErrCursor, len(at), len(by))
	}
	if len(by) == 0 {
		return func(*sql.Selector) {}, nil
	}

	return func(s *sql.Selector) {
		// Built from the last column back, so that each step wraps what came
		// after it: c1 <> v1 OR (c1 = v1 AND <the rest>).
		var p *sql.Predicate
		for i := len(by) - 1; i >= 0; i-- {
			c := s.C(by[i].Column)

			cmp := sql.GT(c, at[i])
			if by[i].Desc {
				cmp = sql.LT(c, at[i])
			}

			if p == nil {
				p = cmp
				continue
			}

			p = sql.Or(cmp, sql.And(sql.EQ(c, at[i]), p))
		}

		s.Where(p)
	}, nil
}

// Encode answers with the cursor that names the row the given values came from,
// in the order of the columns they are the values of.
//
// It is opaque and it is not secret. A caller who takes one apart, or makes one
// up, asks for rows starting somewhere else -- which is a question they could
// have asked anyway, and which the same wall answers. What it is not is stable
// across a change to the order: a cursor made for one is meaningless in
// another, which is what [ErrCursor] is for.
func Encode(vs ...any) (string, error) {
	b, err := json.Marshal(vs)
	if err != nil {
		return "", fmt.Errorf("hold the cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Decode reads a cursor into `into`, which are pointers to values of the types
// [Encode] was given, in the same order.
func Decode(s string, into ...any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrCursor, err)
	}

	var vs []json.RawMessage
	if err := json.Unmarshal(b, &vs); err != nil {
		return fmt.Errorf("%w: %s", ErrCursor, err)
	}
	if len(vs) != len(into) {
		return fmt.Errorf("%w: it names %d columns and the order has %d", ErrCursor, len(vs), len(into))
	}

	for i, v := range vs {
		if err := json.Unmarshal(v, into[i]); err != nil {
			return fmt.Errorf("%w: %s", ErrCursor, err)
		}
	}

	return nil
}

// Size is how many rows to answer with: what the caller asked for, what the
// server answers when they asked for nothing, and what it will not go past
// however loudly they ask.
//
// The cap is the point. A limit a request can set is a request that can ask for
// the whole table, and the answer to that is not an error -- a caller asking
// for more than there is meant no harm -- but it is not a million rows either.
func Size(want int, def int, max int) int {
	if want <= 0 {
		want = def
	}
	if want > max {
		return max
	}

	return want
}
