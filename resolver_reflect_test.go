// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix unit tests for reflection-based resolver implementation.
package dix

import (
	"reflect"
	"testing"
)

// reflectTestService is a sample type used for reflection-based resolver tests.
type reflectTestService struct{}

func TestReflectResolver_New(t *testing.T) {
	r := NewReflectResolver()
	if r == nil {
		t.Fatal("NewReflectResolver returned nil")
	}
	if r.values == nil {
		t.Fatal("resolver values map not initialized")
	}
}

func TestReflectResolver_Register(t *testing.T) {
	r := NewReflectResolver()
	typ := reflect.TypeOf((*reflectTestService)(nil))
	r.Register(typ, &reflectTestService{})
	if r.values[typ] == nil {
		t.Fatal("register failed to store value")
	}
}

func TestReflectResolver_Resolve(t *testing.T) {
	t.Run("resolves registered type", func(t *testing.T) {
		// Resolve should inject a previously registered value.
		r := NewReflectResolver()
		svc := &reflectTestService{}
		r.Register(reflect.TypeOf((*reflectTestService)(nil)), svc)
		var resolved *reflectTestService
		if err := r.Resolve(&resolved); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if resolved != svc {
			t.Fatal("resolved value differs from registration")
		}
	})

	t.Run("errors for non-pointer target", func(t *testing.T) {
		// Resolve must reject non-pointer targets.
		r := NewReflectResolver()
		var svc reflectTestService
		if err := r.Resolve(svc); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for missing registration", func(t *testing.T) {
		// Resolve should fail when the type was not registered.
		r := NewReflectResolver()
		var unknown int
		if err := r.Resolve(&unknown); err == nil {
			t.Fatal("expected error for unregistered type")
		}
	})
}

func TestReflectResolver_ResolveAll(t *testing.T) {
	t.Run("collects all values", func(t *testing.T) {
		// ResolveAll should return every registered value.
		r := NewReflectResolver()
		svc := &reflectTestService{}
		r.Register(reflect.TypeOf(svc), svc)
		var deps []interface{}
		if err := r.ResolveAll(&deps); err != nil {
			t.Fatalf("resolve all failed: %v", err)
		}
		if len(deps) != 1 {
			t.Fatalf("expected 1 dependency, got %d", len(deps))
		}
	})

	t.Run("errors for non-pointer target", func(t *testing.T) {
		// ResolveAll must reject non-pointer targets.
		r := NewReflectResolver()
		var deps []interface{}
		if err := r.ResolveAll(deps); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for non-[]interface{} target", func(t *testing.T) {
		// ResolveAll must target a *[]interface{} slice.
		r := NewReflectResolver()
		var i int
		if err := r.ResolveAll(&i); err == nil {
			t.Fatal("expected error for non-[]interface{} target")
		}
	})
}
