// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix unit tests for default container implementation.
package dix

import (
	"reflect"
	"testing"
)

type containerTestService struct {
	value int
}

// Close is a no-op Close implementation to satisfy closerInterface.
func (s *containerTestService) Close() error {
	return nil
}

// otherTestService is a second concrete type, so tests can register several
// lifetimes at once — one type may carry only one registration.
type otherTestService struct {
	value int
}

// closerInterface models any type with a Close method for interface resolution tests.
type closerInterface interface {
	Close() error
}

func TestDefaultContainer_New(t *testing.T) {
	c := New()
	// Ensure New returns an initialized container.
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.singletons == nil || c.scoped == nil || c.transient == nil {
		t.Fatal("Container maps not initialized")
	}
}

func TestDefaultContainer_Singleton(t *testing.T) {
	cases := []struct {
		name        string
		factory     any
		expectError bool
	}{
		{
			// Valid factory should register a singleton without error.
			name:    "valid factory",
			factory: func() *containerTestService { return &containerTestService{value: 1} },
		},
		{
			// Non-func inputs must be rejected by validation.
			name:        "not a func",
			factory:     "not a func",
			expectError: true, // expect error
		},
		{
			// Factories that do not return a value should fail.
			name:        "no output",
			factory:     func() {},
			expectError: true, // expect error
		},
		{
			// Multiple outputs are disallowed to keep registration deterministic.
			name:        "multiple outputs",
			factory:     func() (*containerTestService, error) { return nil, nil },
			expectError: true, // expect error
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			// Attempt to register the singleton and validate behavior per case.
			err := c.Singleton(tc.factory)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.singletons[reflect.TypeOf(&containerTestService{})] == nil {
				t.Fatal("singleton not registered")
			}
		})
	}
}

func TestDefaultContainer_Scoped(t *testing.T) {
	cases := []struct {
		name        string
		factory     any
		expectError bool
	}{
		{
			// Valid scoped factory should register without error.
			name:    "valid factory",
			factory: func() *containerTestService { return &containerTestService{value: 2} },
		},
		{
			// Non-function inputs must be rejected for scoped registration.
			name:        "not a func",
			factory:     "not a func",
			expectError: true, // expect error
		},
		{
			// Factories without outputs should fail validation.
			name:        "no output",
			factory:     func() {},
			expectError: true, // expect error
		},
		{
			// Creating multiple outputs is not supported for scoped factories.
			name:        "multiple outputs",
			factory:     func() (*containerTestService, error) { return nil, nil },
			expectError: true, // expect error
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			// Attempt to register the scoped factory and validate behavior per case.
			err := c.Scoped(tc.factory)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.scoped[reflect.TypeOf(&containerTestService{})] == nil {
				t.Fatal("scoped not registered")
			}
		})
	}
}

func TestDefaultContainer_Transient(t *testing.T) {
	cases := []struct {
		name        string
		factory     any
		expectError bool
	}{
		{
			// Transient factory should be accepted so future resolves produce new instances.
			name:    "valid factory",
			factory: func() *containerTestService { return &containerTestService{value: 3} },
		},
		{
			// Invalid factories (non-func) should fail.
			name:        "not a func",
			factory:     "not a func",
			expectError: true, // expect error
		},
		{
			// Factories without outputs must be rejected.
			name:        "no output",
			factory:     func() {},
			expectError: true, // expect error
		},
		{
			// Functions returning multiple values violate the factory contract.
			name:        "multiple outputs",
			factory:     func() (*containerTestService, error) { return nil, nil },
			expectError: true, // expect error
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			// Attempt to register the transient factory and validate behavior per case.
			err := c.Transient(tc.factory)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.transient[reflect.TypeOf(&containerTestService{})] == nil {
				t.Fatal("transient not registered")
			}
		})
	}
}

func TestDefaultContainer_Instance(t *testing.T) {
	cases := []struct {
		name        string
		value       any
		expectError bool
	}{
		{
			// Instance registration should accept concrete values.
			name:  "valid instance",
			value: &containerTestService{value: 4},
		},
		{
			// Nil values are invalid for instance registration.
			name:        "nil",
			value:       nil,
			expectError: true, // expect error
		},
		{
			// Non-values such as functions should be rejected.
			name:        "function",
			value:       func() {},
			expectError: true, // expect error
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			// Attempt to register the instance and validate behavior per case.
			err := c.Instance(tc.value)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.singletons[reflect.TypeOf(&containerTestService{})] == nil {
				t.Fatal("instance not registered")
			}
		})
	}
}

func TestDefaultContainer_NewScope(t *testing.T) {
	c := New()
	s := c.NewScope()
	// Verify the new scope is properly initialized and linked.
	if s == nil {
		t.Fatal("NewScope returned nil")
	}
	ds, ok := s.(*defaultScope)
	if !ok {
		t.Fatal("expected defaultScope")
	}
	if ds.container != c {
		t.Fatal("scope container not set")
	}
	if ds.scopedInstances == nil {
		t.Fatal("scope map not initialized")
	}
}

func TestDefaultContainer_get(t *testing.T) {
	type testCase struct {
		name        string
		setup       func(*defaultContainer)
		typ         reflect.Type
		expectError bool
		assert      func(*testing.T, any)
	}

	closerType := reflect.TypeOf((*closerInterface)(nil)).Elem()
	direct := &containerTestService{value: 1}
	interfaceImpl := &containerTestService{value: 2}
	cases := []testCase{
		{
			// Direct instance lookups should return the registered value.
			name:  "direct singleton",
			typ:   reflect.TypeOf(direct),
			setup: func(c *defaultContainer) { _ = c.Instance(direct) },
			assert: func(t *testing.T, got any) {
				if got != direct {
					t.Fatalf("expected direct singleton, got %#v", got)
				}
			},
		},
		{
			// Interface lookups should return the concrete implementation.
			name:  "interface lookup",
			typ:   closerType,
			setup: func(c *defaultContainer) { _ = c.Instance(interfaceImpl) },
			assert: func(t *testing.T, got any) {
				if got != interfaceImpl {
					t.Fatalf("expected interface implementation, got %#v", got)
				}
			},
		},
		{
			// Transient factories should still produce the first instance on initial get.
			name: "transient factory",
			typ:  reflect.TypeOf(&containerTestService{}),
			setup: func(c *defaultContainer) {
				callCount := 0
				_ = c.Transient(func() *containerTestService {
					callCount++
					return &containerTestService{value: callCount}
				})
			},
			assert: func(t *testing.T, got any) {
				svc, ok := got.(*containerTestService)
				if !ok || svc.value != 1 {
					t.Fatalf("expected transient factory result, got %#v", got)
				}
			},
		},
		{
			// Missing registrations must produce an error.
			name:        "missing registration",
			typ:         reflect.TypeOf(&containerTestService{}),
			expectError: true, // expect error
		},
		{
			// Missing interface bindings should also error.
			name:        "missing interface",
			typ:         closerType,
			expectError: true, // expect error
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			// Prepare registrations and attempt container get for the requested type.
			if tc.setup != nil {
				tc.setup(c)
			}
			got, err := c.get(tc.typ)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.assert != nil {
				tc.assert(t, got)
				return
			}
			if got == nil {
				t.Fatal("expected non-nil value")
			}
		})
	}
}

func TestDefaultContainer_Resolve(t *testing.T) {
	t.Run("resolves singleton", func(t *testing.T) {
		// Resolve should inject a registered singleton value.
		c := New()
		if err := c.Singleton(func() *containerTestService { return &containerTestService{value: 5} }); err != nil {
			t.Fatalf("singleton registration failed: %v", err)
		}
		var svc *containerTestService
		if err := c.Resolve(&svc); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if svc == nil {
			t.Fatal("resolve returned nil")
		}
	})

	t.Run("errors for non-pointer", func(t *testing.T) {
		// Resolve must reject non-pointer targets.
		c := New()
		var svc containerTestService
		if err := c.Resolve(svc); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for missing registration", func(t *testing.T) {
		// Resolve should fail when no registration exists.
		c := New()
		var unknown int
		if err := c.Resolve(&unknown); err == nil {
			t.Fatal("expected error for missing registration")
		}
	})

	t.Run("resolves interface implementation", func(t *testing.T) {
		// Resolve should match registered concrete types to interface targets.
		c := New()
		impl := &containerTestService{value: 12}
		if err := c.Instance(impl); err != nil {
			t.Fatalf("instance registration failed: %v", err)
		}
		var svc interface{}
		if err := c.Resolve(&svc); err != nil {
			t.Fatalf("interface resolve failed: %v", err)
		}
		if svc != impl {
			t.Fatal("interface did not resolve to implementation")
		}
	})

	t.Run("resolves specific interface", func(t *testing.T) {
		// Resolve should satisfy a specific interface from a registered instance.
		c := New()
		impl := &containerTestService{value: 13}
		if err := c.Instance(impl); err != nil {
			t.Fatalf("instance registration failed: %v", err)
		}
		var closer interface{ Close() error }
		if err := c.Resolve(&closer); err != nil {
			t.Fatalf("specific interface resolve failed: %v", err)
		}
		if closer != impl {
			t.Fatal("specific interface did not resolve to implementation")
		}
	})
}

func TestDefaultContainer_ResolveAll(t *testing.T) {
	t.Run("resolves singleton and transient registrations", func(t *testing.T) {
		// ResolveAll gathers singletons and transients. Each lifetime needs a
		// distinct type: one type may hold only one registration
		// (ErrAlreadyRegistered), so the old shape — the same type registered
		// three times over — is no longer expressible.
		c := New()
		if err := c.Singleton(func() *containerTestService { return &containerTestService{value: 6} }); err != nil {
			t.Fatalf("singleton registration failed: %v", err)
		}
		if err := c.Transient(func() *otherTestService { return &otherTestService{value: 7} }); err != nil {
			t.Fatalf("transient registration failed: %v", err)
		}
		var deps []interface{}
		if err := c.ResolveAll(&deps); err != nil {
			t.Fatalf("resolve all failed: %v", err)
		}
		if len(deps) != 2 {
			t.Fatalf("expected 2 dependencies, got %d", len(deps))
		}
	})

	t.Run("skips scoped registrations", func(t *testing.T) {
		// Building a scoped value from the root would leave it with no owning
		// scope, so nothing would ever Close it — the leak ErrScopedFromRoot
		// documents. ResolveAll therefore omits scoped types entirely.
		c := New()
		if err := c.Singleton(func() *containerTestService { return &containerTestService{value: 1} }); err != nil {
			t.Fatalf("singleton registration failed: %v", err)
		}
		built := false
		if err := c.Scoped(func() *otherTestService { built = true; return &otherTestService{} }); err != nil {
			t.Fatalf("scoped registration failed: %v", err)
		}
		var deps []interface{}
		if err := c.ResolveAll(&deps); err != nil {
			t.Fatalf("resolve all failed: %v", err)
		}
		if built {
			t.Fatal("ResolveAll instantiated a scoped registration from the root container")
		}
		if len(deps) != 1 {
			t.Fatalf("expected only the singleton, got %d dependencies", len(deps))
		}
	})

	t.Run("errors for non-pointer target", func(t *testing.T) {
		// ResolveAll must reject non-pointer targets.
		c := New()
		var deps []interface{}
		if err := c.ResolveAll(deps); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for non-slice target", func(t *testing.T) {
		// ResolveAll requires a slice pointer target.
		c := New()
		var i int
		if err := c.ResolveAll(&i); err == nil {
			t.Fatal("expected error for non-slice target")
		}
	})
}

func TestDefaultContainer_TransientDifferentInstances(t *testing.T) {
	c := New()
	callCount := 0
	if err := c.Transient(func() *containerTestService {
		callCount++
		return &containerTestService{value: callCount}
	}); err != nil {
		t.Fatalf("transient registration failed: %v", err)
	}
	var s1, s2 *containerTestService
	if err := c.Resolve(&s1); err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if err := c.Resolve(&s2); err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if s1 == s2 || s1.value == s2.value {
		t.Fatal("transient should return distinct instances")
	}
}
