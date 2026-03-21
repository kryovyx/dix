// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix unit tests for default scope implementation.
package dix

import (
	"fmt"
	"reflect"
	"testing"
)

// scopeTestService is a sample service used to verify scoped lifecycle and closing.
type scopeTestService struct {
	closed bool
}

// Close marks the service as closed (used to ensure Close is called).
func (s *scopeTestService) Close() error {
	s.closed = true
	return nil
}

// errorCloser simulates a resource whose Close returns an error.
type errorCloser struct {
	closed bool
}

// Close marks the closer as closed and returns an error to test error propagation.
func (e *errorCloser) Close() error {
	e.closed = true
	return fmt.Errorf("close error")
}

func TestDefaultScope_get(t *testing.T) {
	type testCase struct {
		name        string
		setup       func(*defaultContainer)
		typ         reflect.Type
		expectError bool
		assert      func(*testing.T, *defaultScope, any)
	}

	serviceType := reflect.TypeOf(&scopeTestService{})
	cases := []testCase{
		{
			// Should return the cached scoped instance and reuse it on subsequent gets.
			name:  "scoped singleton",
			typ:   serviceType,
			setup: func(c *defaultContainer) { _ = c.Scoped(func() *scopeTestService { return &scopeTestService{} }) },
			assert: func(t *testing.T, s *defaultScope, got any) {
				svc, ok := got.(*scopeTestService)
				if !ok {
					t.Fatalf("expected scopeTestService, got %#v", got)
				}
				cached, err := s.get(serviceType)
				if err != nil {
					t.Fatalf("second get failed: %v", err)
				}
				if cached != svc {
					t.Fatalf("expected cached instance, got %#v", cached)
				}
			},
		},
		{
			// Should fall back to container singleton when scope lacks registration.
			name:  "container fallback",
			typ:   serviceType,
			setup: func(c *defaultContainer) { _ = c.Singleton(func() *scopeTestService { return &scopeTestService{} }) },
			assert: func(t *testing.T, _ *defaultScope, got any) {
				if _, ok := got.(*scopeTestService); !ok {
					t.Fatalf("expected container value, got %#v", got)
				}
			},
		},
		{
			// Should error when the type is not registered in scope or container.
			name:        "missing registration",
			typ:         serviceType,
			expectError: true, // expect error
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			s := c.NewScope().(*defaultScope)
			if tc.setup != nil {
				tc.setup(c)
			}
			got, err := s.get(tc.typ)
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
				tc.assert(t, s, got)
				return
			}
			if got == nil {
				t.Fatal("expected non-nil value")
			}
		})
	}
}

func TestDefaultScope_Resolve(t *testing.T) {
	t.Run("resolves scoped value", func(t *testing.T) {
		// Resolve should inject a scoped registration within the scope.
		c := New()
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		s := c.NewScope().(*defaultScope)
		var svc *scopeTestService
		if err := s.Resolve(&svc); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if svc == nil {
			t.Fatal("resolve returned nil")
		}
	})

	t.Run("errors for non-pointer", func(t *testing.T) {
		// Resolve must reject non-pointer targets.
		c := New()
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		s := c.NewScope().(*defaultScope)
		var svc scopeTestService
		if err := s.Resolve(svc); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for missing registration", func(t *testing.T) {
		// Resolve should fail when the type is not registered.
		c := New()
		s := c.NewScope().(*defaultScope)
		var svc *scopeTestService
		if err := s.Resolve(&svc); err == nil {
			t.Fatal("expected error for missing registration")
		}
	})
}

func TestDefaultScope_ResolveAll(t *testing.T) {
	t.Run("aggregates all dependencies", func(t *testing.T) {
		// ResolveAll should collect singleton, transient, and scoped instances.
		c := New()
		c.Singleton(func() *scopeTestService { return &scopeTestService{} })
		c.Transient(func() *scopeTestService { return &scopeTestService{} })
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		s := c.NewScope().(*defaultScope)
		var deps []interface{}
		if err := s.ResolveAll(&deps); err != nil {
			t.Fatalf("resolve all failed: %v", err)
		}
		if len(deps) < 3 {
			t.Fatalf("expected at least 3 dependencies, got %d", len(deps))
		}
	})

	t.Run("uses cached scoped instances", func(t *testing.T) {
		// ResolveAll should include the cached scoped instance created earlier.
		c := New()
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		s := c.NewScope().(*defaultScope)
		var resolved *scopeTestService
		if err := s.Resolve(&resolved); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		var deps []interface{}
		if err := s.ResolveAll(&deps); err != nil {
			t.Fatalf("resolve all failed: %v", err)
		}
		found := false
		for _, dep := range deps {
			if dep == resolved {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected cached scoped instance to appear in ResolveAll output")
		}
	})

	t.Run("errors for non-pointer target", func(t *testing.T) {
		// ResolveAll must reject non-pointer targets.
		c := New()
		s := c.NewScope().(*defaultScope)
		var deps []interface{}
		if err := s.ResolveAll(deps); err == nil {
			t.Fatal("expected error for non-pointer target")
		}
	})

	t.Run("errors for non-[]interface{} target", func(t *testing.T) {
		// ResolveAll requires a pointer to []interface{}.
		c := New()
		s := c.NewScope().(*defaultScope)
		var i int
		if err := s.ResolveAll(&i); err == nil {
			t.Fatal("expected error for non-[]interface{} target")
		}
	})

	t.Run("errors for []non-interface slice", func(t *testing.T) {
		// ResolveAll should fail when slice elements are not interface{}.
		c := New()
		s := c.NewScope().(*defaultScope)
		var deps []int
		if err := s.ResolveAll(&deps); err == nil {
			t.Fatal("expected error for non-interface slice")
		}
	})
}

func TestDefaultScope_Close(t *testing.T) {
	t.Run("closes scoped resources", func(t *testing.T) {
		// Close should invoke Close on all scoped instances created in the scope.
		c := New()
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		c.Scoped(func() *scopeTestService { return &scopeTestService{} })
		s := c.NewScope().(*defaultScope)
		var svc1, svc2 *scopeTestService
		if err := s.Resolve(&svc1); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if err := s.Resolve(&svc2); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if svc1.closed || svc2.closed {
			t.Fatal("services should not be closed before close call")
		}
		if err := s.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
		if !svc1.closed || !svc2.closed {
			t.Fatal("all services should be closed")
		}
	})

	t.Run("propagates close error", func(t *testing.T) {
		// Close should surface errors returned by scoped resources.
		c := New()
		c.Scoped(func() *errorCloser { return &errorCloser{} })
		s := c.NewScope().(*defaultScope)
		var ec *errorCloser
		if err := s.Resolve(&ec); err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if err := s.Close(); err == nil {
			t.Fatal("expected close error")
		}
	})
}
