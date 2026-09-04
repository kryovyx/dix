// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: (c) 2026 Kryovyx

package dix

import (
	"errors"
	"reflect"
	"testing"
)

// The lifetime name appears in error messages, so an unnamed one turns a
// diagnostic into a puzzle.
func TestLifetime_String(t *testing.T) {
	cases := []struct {
		in   lifetime
		want string
	}{
		{lifetimeSingleton, "Singleton"},
		{lifetimeScoped, "Scoped"},
		{lifetimeTransient, "Transient"},
		{lifetime(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("lifetime(%d).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

type animal interface{ Speak() string }

type dog struct{ name string }

func (d dog) Speak() string { return "woof" }

type cat struct{}

func (cat) Speak() string { return "meow" }

// ---------------------------------------------------------------------------
// assign
// ---------------------------------------------------------------------------

func TestAssign(t *testing.T) {
	t.Run("assignable value", func(t *testing.T) {
		var got animal
		if err := assign(&got, dog{name: "rex"}); err != nil {
			t.Fatalf("assign: %v", err)
		}
		if got.Speak() != "woof" {
			t.Fatalf("got %v", got)
		}
	})

	// A nil resolution zeroes the target rather than leaving whatever was
	// there — otherwise a failed resolve looks like a successful one.
	t.Run("nil zeroes the target", func(t *testing.T) {
		var got animal = cat{}
		if err := assign(&got, nil); err != nil {
			t.Fatalf("assign: %v", err)
		}
		if got != nil {
			t.Fatalf("target not zeroed: %v", got)
		}
	})

	// A type mismatch is an error naming both types, not a panic inside
	// reflect.
	t.Run("unassignable type", func(t *testing.T) {
		var got *dog
		err := assign(&got, cat{})
		if err == nil {
			t.Fatal("expected an error assigning a cat to a *dog")
		}
		if !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("err = %v, want ErrInvalidTarget", err)
		}
	})
}

// ---------------------------------------------------------------------------
// appendValue
// ---------------------------------------------------------------------------

func TestAppendValue(t *testing.T) {
	var out []animal
	slice := reflect.ValueOf(&out).Elem()

	appendValue(slice, dog{name: "a"})
	appendValue(slice, cat{})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}

	// A nil is skipped rather than appended: a nil entry in a ResolveAll
	// result is indistinguishable from a registered nil, and every caller
	// would have to filter it.
	appendValue(slice, nil)
	if len(out) != 2 {
		t.Fatalf("a nil was appended: len = %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// sameInstance
// ---------------------------------------------------------------------------

// uncomparable has a slice field, so == on it panics.
type uncomparable struct{ items []string }

func TestSameInstance(t *testing.T) {
	a := dog{name: "rex"}
	b := dog{name: "rex"}
	c := dog{name: "fido"}

	cases := []struct {
		name string
		x, y any
		want bool
	}{
		{"both nil", nil, nil, true},
		{"left nil", nil, a, false},
		{"right nil", a, nil, false},
		{"equal comparable values", a, b, true},
		{"different comparable values", a, c, false},
		{"different types", a, cat{}, false},
		// The reason this helper exists: == on these panics, so it must
		// answer false rather than crash.
		{"uncomparable type", uncomparable{items: []string{"x"}}, uncomparable{items: []string{"x"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameInstance(tc.x, tc.y); got != tc.want {
				t.Fatalf("sameInstance = %v, want %v", got, tc.want)
			}
		})
	}
}

// The whole point: it must not panic on an uncomparable type.
func TestSameInstance_does_not_panic_on_uncomparable_values(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sameInstance panicked on an uncomparable type: %v", r)
		}
	}()
	v := uncomparable{items: []string{"a", "b"}}
	_ = sameInstance(v, v)
}
