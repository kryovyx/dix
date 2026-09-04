// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix concurrency and lifetime-semantics tests.
//
// These are the acceptance tests for D5–D8. They exist because the package
// previously reported 100% statement coverage while carrying a guaranteed
// crash: the defects were concurrency bugs, not untaken branches, so no amount
// of statement coverage could reach them. Everything here must be run with
// -race, which the module Makefile does by default.
package dix

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// D5 — concurrent registration and resolution
// ---------------------------------------------------------------------------

type ifaceA interface{ A() string }
type ifaceB interface{ B() string }

type implA struct{ id int }

func (i *implA) A() string { return fmt.Sprintf("A%d", i.id) }

type implB struct{}

func (implB) B() string { return "B" }

// altA is a second ifaceA implementation, for the ambiguity test.
type altA struct{}

func (altA) A() string { return "alt" }

// TestConcurrentRegisterAndResolve is the D5 acceptance test: registration on
// one goroutine while N others resolve.
//
// Before the RWMutex, container.get ranged over the singletons map on every
// interface resolve while Instance and Singleton wrote it. That is
// "fatal error: concurrent map read and map write" — the runtime kills the
// process, recover cannot catch it, and every request touches this path.
func TestConcurrentRegisterAndResolve(t *testing.T) {
	c := New()

	// Seed one registration so the resolvers have something to find.
	if err := c.Instance(&implB{}); err != nil {
		t.Fatalf("seed instance failed: %v", err)
	}

	const (
		registrars = 4
		resolvers  = 16
		iterations = 500
	)

	var writers, readers sync.WaitGroup
	stop := make(chan struct{})

	// Writers hammer the write lock. Only the first Singleton for *implA
	// succeeds; the rest hit ErrAlreadyRegistered, which still takes the write
	// lock and still touches the maps, so the interleaving under test is the
	// same. What matters is that writes land while reads are in flight.
	for w := range registrars {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for i := range iterations {
				_ = c.Singleton(func() *implA { return &implA{id: w*iterations + i} })
				_ = c.Instance(&altA{})
			}
		}(w)
	}

	// Readers resolve by interface, which is the path that ranges the maps —
	// the one that used to crash the process.
	for range resolvers {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				var b ifaceB
				if err := c.Resolve(&b); err != nil {
					t.Errorf("resolve ifaceB failed: %v", err)
					return
				}
				var deps []interface{}
				if err := c.ResolveAll(&deps); err != nil {
					t.Errorf("resolve all failed: %v", err)
					return
				}
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

// TestConcurrentScopeResolve exercises one scope from many goroutines: a
// handler that fans out and resolves is the realistic shape.
func TestConcurrentScopeResolve(t *testing.T) {
	c := New()
	var built atomic.Int64
	if err := c.Scoped(func() *implA {
		built.Add(1)
		return &implA{id: 1}
	}); err != nil {
		t.Fatalf("scoped registration failed: %v", err)
	}

	scope := c.NewScope()
	var wg sync.WaitGroup
	got := make([]*implA, 32)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var v *implA
			if err := scope.Resolve(&v); err != nil {
				t.Errorf("resolve failed: %v", err)
				return
			}
			got[i] = v
		}(i)
	}
	wg.Wait()

	// One instance per scope, whatever the concurrency.
	for i, v := range got {
		if v == nil {
			t.Fatalf("goroutine %d resolved nil", i)
		}
		if v != got[0] {
			t.Fatalf("goroutine %d got a different instance: scoped values must be one per scope", i)
		}
	}
	if err := scope.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// D6 — deterministic resolution
// ---------------------------------------------------------------------------

// TestAmbiguousInterfaceResolution is the D6 acceptance test: two types
// satisfying one interface must be an error, not a coin flip.
//
// Map iteration order is randomised per range, so the previous "return the
// first match" behaviour picked a different implementation from run to run.
// The router resolves event.Bus and log.Logger by interface, so this decided
// which logger a process used based on nothing.
func TestAmbiguousInterfaceResolution(t *testing.T) {
	c := New()
	if err := c.Instance(&implA{id: 1}); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := c.Instance(altA{}); err != nil {
		t.Fatalf("second registration failed: %v", err)
	}

	var a ifaceA
	err := c.Resolve(&a)
	if err == nil {
		t.Fatal("expected an ambiguity error, got a resolved value — the winner would depend on map iteration order")
	}
	if !errors.Is(err, ErrAmbiguousResolution) {
		t.Fatalf("expected ErrAmbiguousResolution, got %v", err)
	}
	// The message has to name both candidates, or the error is not actionable.
	for _, want := range []string{"implA", "altA"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name candidate %q, got: %v", want, err)
		}
	}

	// Resolving the concrete type stays unambiguous, which is the documented
	// way out.
	var concrete *implA
	if err := c.Resolve(&concrete); err != nil {
		t.Fatalf("concrete resolve should still work: %v", err)
	}
}

// TestAmbiguityIsStableAcrossRepeats guards the determinism claim itself: the
// same container must produce the same outcome every time, not just once.
func TestAmbiguityIsStableAcrossRepeats(t *testing.T) {
	c := New()
	_ = c.Instance(&implA{id: 1})
	_ = c.Instance(altA{})

	first := ""
	for i := range 50 {
		var a ifaceA
		err := c.Resolve(&a)
		if err == nil {
			t.Fatalf("iteration %d resolved instead of erroring", i)
		}
		if first == "" {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("error message changed between runs — candidate ordering is not stable:\n  %s\n  %s", first, err)
		}
	}
}

// TestDuplicateRegistrationRejected covers the other half of determinism: if
// one type could hold two lifetimes, the container would have to pick between
// them by lookup precedence, which is the same nondeterminism hidden behind an
// ordering instead of a map range.
func TestDuplicateRegistrationRejected(t *testing.T) {
	cases := []struct {
		name   string
		second func(*defaultContainer) error
	}{
		{"singleton then scoped", func(c *defaultContainer) error {
			return c.Scoped(func() *implA { return &implA{} })
		}},
		{"singleton then transient", func(c *defaultContainer) error {
			return c.Transient(func() *implA { return &implA{} })
		}},
		{"singleton then instance", func(c *defaultContainer) error {
			return c.Instance(&implA{})
		}},
		{"singleton then singleton", func(c *defaultContainer) error {
			return c.Singleton(func() *implA { return &implA{} })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			if err := c.Singleton(func() *implA { return &implA{} }); err != nil {
				t.Fatalf("first registration failed: %v", err)
			}
			err := tc.second(c)
			if !errors.Is(err, ErrAlreadyRegistered) {
				t.Fatalf("expected ErrAlreadyRegistered, got %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// D7 / O1 — factory semantics
// ---------------------------------------------------------------------------

// TestResolverFactory is the D7 acceptance test: a factory taking a Resolver
// receives a working one.
func TestResolverFactory(t *testing.T) {
	c := New()
	if err := c.Instance(&implB{}); err != nil {
		t.Fatalf("dependency registration failed: %v", err)
	}

	var sawResolver bool
	if err := c.Singleton(func(r Resolver) *implA {
		var b ifaceB
		if err := r.Resolve(&b); err != nil {
			t.Errorf("factory could not resolve its dependency: %v", err)
			return nil
		}
		sawResolver = b != nil
		return &implA{id: 7}
	}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	var a *implA
	if err := c.Resolve(&a); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !sawResolver {
		t.Fatal("factory did not receive a usable Resolver")
	}
	if a.id != 7 {
		t.Fatalf("unexpected value: %#v", a)
	}
}

// TestScopedFactoryReceivesScope checks the Resolver handed to a scoped factory
// is the scope, not the root — otherwise a scoped value's own scoped
// dependencies would be built against the root and leak.
func TestScopedFactoryReceivesScope(t *testing.T) {
	c := New()
	if err := c.Scoped(func() *implB { return &implB{} }); err != nil {
		t.Fatalf("inner registration failed: %v", err)
	}
	if err := c.Scoped(func(r Resolver) *implA {
		var inner *implB
		if err := r.Resolve(&inner); err != nil {
			t.Errorf("scoped factory could not resolve a sibling scoped type — it was given the root: %v", err)
		}
		return &implA{id: 1}
	}); err != nil {
		t.Fatalf("outer registration failed: %v", err)
	}

	scope := c.NewScope()
	defer func() { _ = scope.Close() }()
	var a *implA
	if err := scope.Resolve(&a); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
}

// TestBadFactorySignatureRejectedAtRegistration checks unsupported signatures
// fail where the mistake was made, not later inside reflect.Call.
func TestBadFactorySignatureRejectedAtRegistration(t *testing.T) {
	cases := []struct {
		name    string
		factory any
	}{
		{"one non-Resolver argument", func(int) *implA { return nil }},
		{"two arguments", func(Resolver, int) *implA { return nil }},
		{"variadic", func(...int) *implA { return nil }},
		{"no return", func() {}},
		{"two returns", func() (*implA, error) { return nil, nil }},
		{"not a function", "nope"},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			err := c.Singleton(tc.factory)
			if !errors.Is(err, ErrInvalidFactory) {
				t.Fatalf("expected ErrInvalidFactory, got %v", err)
			}
		})
	}
}

// TestSingletonIsLazy is the other half of D7: the factory must run on first
// resolve, not at registration.
//
// Eager construction made registration order load-bearing — a factory could
// not depend on anything registered after it, for no reason anyone chose.
func TestSingletonIsLazy(t *testing.T) {
	c := New()
	var calls atomic.Int64
	if err := c.Singleton(func() *implA {
		calls.Add(1)
		return &implA{id: 1}
	}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("factory ran at registration time (%d calls); singletons must be lazy", got)
	}

	var a1, a2 *implA
	if err := c.Resolve(&a1); err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if err := c.Resolve(&a2); err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 factory call, got %d", got)
	}
	if a1 != a2 {
		t.Fatal("singleton returned two different instances")
	}
}

// TestSingletonBuiltExactlyOnceUnderConcurrency covers the sync.Once contract.
func TestSingletonBuiltExactlyOnceUnderConcurrency(t *testing.T) {
	c := New()
	var calls atomic.Int64
	if err := c.Singleton(func() *implA {
		calls.Add(1)
		return &implA{id: 1}
	}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	var wg sync.WaitGroup
	out := make([]*implA, 32)
	for i := range out {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var a *implA
			if err := c.Resolve(&a); err != nil {
				t.Errorf("resolve failed: %v", err)
				return
			}
			out[i] = a
		}(i)
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 factory call under concurrency, got %d", got)
	}
	for i, a := range out {
		if a != out[0] {
			t.Fatalf("goroutine %d got a different singleton instance", i)
		}
	}
}

// TestScopedFromRootIsAnError is the O1 acceptance test.
//
// The root must not instantiate a scoped value: it would have no owning scope,
// so nothing would ever Close it — which recreates the exact per-request leak
// that closing scopes exists to fix.
func TestScopedFromRootIsAnError(t *testing.T) {
	c := New()
	var built atomic.Int64
	if err := c.Scoped(func() *implA {
		built.Add(1)
		return &implA{id: 1}
	}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	var a *implA
	err := c.Resolve(&a)
	if !errors.Is(err, ErrScopedFromRoot) {
		t.Fatalf("expected ErrScopedFromRoot, got %v", err)
	}
	if got := built.Load(); got != 0 {
		t.Fatalf("root container instantiated a scoped value (%d times) — it must only report the error", got)
	}

	// From a scope it resolves normally.
	scope := c.NewScope()
	defer func() { _ = scope.Close() }()
	if err := scope.Resolve(&a); err != nil {
		t.Fatalf("scope resolve failed: %v", err)
	}
	if got := built.Load(); got != 1 {
		t.Fatalf("expected 1 construction from the scope, got %d", got)
	}
}

// TestScopedFromRootByInterface covers the interface path too — the error must
// not be reachable only by exact type.
func TestScopedFromRootByInterface(t *testing.T) {
	c := New()
	if err := c.Scoped(func() *implA { return &implA{id: 1} }); err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	var a ifaceA
	if err := c.Resolve(&a); !errors.Is(err, ErrScopedFromRoot) {
		t.Fatalf("expected ErrScopedFromRoot via interface, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// D8 — scope closing
// ---------------------------------------------------------------------------

type countingCloser struct {
	closes atomic.Int64
	err    error
}

func (c *countingCloser) Close() error {
	c.closes.Add(1)
	return c.err
}

// TestScopeClosesEachInstanceOnce is the D8 acceptance test at the dix layer:
// the router-side half is asserted in the rex module.
func TestScopeClosesEachInstanceOnce(t *testing.T) {
	c := New()
	var made []*countingCloser
	var mu sync.Mutex
	if err := c.Scoped(func() *countingCloser {
		cc := &countingCloser{}
		mu.Lock()
		made = append(made, cc)
		mu.Unlock()
		return cc
	}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	scope := c.NewScope()
	var v *countingCloser
	if err := scope.Resolve(&v); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if err := scope.Resolve(&v); err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if err := scope.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	// Idempotent: a deferred Close plus an explicit one must not double-close.
	if err := scope.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}

	if len(made) != 1 {
		t.Fatalf("expected 1 construction per scope, got %d", len(made))
	}
	if got := made[0].closes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 Close, got %d", got)
	}
}

// TestScopeCloseClosesAllDespiteError checks a failing Close does not abandon
// the rest. Returning early on the first error turned one bad closer into a
// leak of every closer after it.
func TestScopeCloseClosesAllDespiteError(t *testing.T) {
	c := New()
	bad := &countingCloser{err: errors.New("boom")}
	good := &countingCloser{}
	if err := c.Instance(bad); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	scope := c.NewScope().(*defaultScope)
	// Seed both directly: the point under test is Close, not resolution.
	scope.scopedInstances[reflect.TypeOf(bad)] = bad
	scope.scopedInstances[reflect.TypeOf(&implA{})] = good

	err := scope.Close()
	if err == nil {
		t.Fatal("expected the failing Close to be reported")
	}
	if got := bad.closes.Load(); got != 1 {
		t.Fatalf("failing closer: expected 1 Close, got %d", got)
	}
	if got := good.closes.Load(); got != 1 {
		t.Fatalf("second closer was abandoned after the first error: %d closes", got)
	}
}

// TestScopeRejectsUseAfterClose keeps a closed scope from handing out values
// nothing will release.
func TestScopeRejectsUseAfterClose(t *testing.T) {
	c := New()
	if err := c.Scoped(func() *implA { return &implA{} }); err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	scope := c.NewScope()
	if err := scope.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	var a *implA
	if err := scope.Resolve(&a); !errors.Is(err, ErrScopeClosed) {
		t.Fatalf("expected ErrScopeClosed, got %v", err)
	}
	var deps []interface{}
	if err := scope.ResolveAll(&deps); !errors.Is(err, ErrScopeClosed) {
		t.Fatalf("expected ErrScopeClosed from ResolveAll, got %v", err)
	}
}

// TestConcurrentScopeCloseAndResolve checks the Close/Resolve race does not
// hand back an untracked value.
func TestConcurrentScopeCloseAndResolve(t *testing.T) {
	for range 50 {
		c := New()
		var made atomic.Int64
		var closed atomic.Int64
		if err := c.Scoped(func() *countingCloser {
			made.Add(1)
			return &countingCloser{}
		}); err != nil {
			t.Fatalf("registration failed: %v", err)
		}
		scope := c.NewScope()

		var wg sync.WaitGroup
		wg.Add(2)
		var resolved *countingCloser
		go func() {
			defer wg.Done()
			var v *countingCloser
			if err := scope.Resolve(&v); err == nil {
				resolved = v
			}
		}()
		go func() {
			defer wg.Done()
			_ = scope.Close()
		}()
		wg.Wait()
		_ = scope.Close()

		if resolved != nil {
			closed.Add(resolved.closes.Load())
			// Whatever the interleaving, a value handed to a caller must
			// eventually be closed exactly once.
			if resolved.closes.Load() != 1 {
				t.Fatalf("resolved value closed %d times, want 1", resolved.closes.Load())
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Unbind — correcting a registration
// ---------------------------------------------------------------------------

// TestUnbindAllowsOverride covers the case that motivated Unbind: a default is
// registered early, the application later supplies its own, and the two are
// different concrete types satisfying the same interface.
//
// Without removing the first, both stay registered and the interface resolve
// becomes ambiguous — which is how "set a logger" silently became "which of
// these two loggers, decided by map order".
func TestUnbindAllowsOverride(t *testing.T) {
	c := New()
	def := &implA{id: 1}
	if err := c.Instance(def); err != nil {
		t.Fatalf("default registration failed: %v", err)
	}

	// The naive override leaves both in place.
	override := altA{}
	if err := c.Instance(override); err != nil {
		t.Fatalf("override registration failed: %v", err)
	}
	var a ifaceA
	if err := c.Resolve(&a); !errors.Is(err, ErrAmbiguousResolution) {
		t.Fatalf("expected two loggers to be ambiguous, got %v", err)
	}

	// Unbind the default and the override resolves cleanly.
	removed, err := c.Unbind(def)
	if err != nil {
		t.Fatalf("unbind failed: %v", err)
	}
	if !removed {
		t.Fatal("Unbind reported nothing removed")
	}
	if err := c.Resolve(&a); err != nil {
		t.Fatalf("resolve after unbind failed: %v", err)
	}
	if a.A() != "alt" {
		t.Fatalf("expected the override, got %q", a.A())
	}

	// The type is free again, so a further correction is possible.
	if err := c.Instance(&implA{id: 2}); err != nil {
		t.Fatalf("re-registration after unbind failed: %v", err)
	}
}

// TestUnbindReportsAbsence keeps Unbind honest about doing nothing.
func TestUnbindReportsAbsence(t *testing.T) {
	c := New()
	removed, err := c.Unbind(&implA{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Fatal("Unbind reported a removal for a type that was never registered")
	}
	if _, err := c.Unbind(nil); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget for nil, got %v", err)
	}
}

// TestUnbindAcrossLifetimes checks all three maps are searched.
func TestUnbindAcrossLifetimes(t *testing.T) {
	cases := []struct {
		name     string
		register func(*defaultContainer) error
	}{
		{"singleton", func(c *defaultContainer) error { return c.Singleton(func() *implA { return &implA{} }) }},
		{"scoped", func(c *defaultContainer) error { return c.Scoped(func() *implA { return &implA{} }) }},
		{"transient", func(c *defaultContainer) error { return c.Transient(func() *implA { return &implA{} }) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			if err := tc.register(c); err != nil {
				t.Fatalf("registration failed: %v", err)
			}
			removed, err := c.Unbind(&implA{})
			if err != nil {
				t.Fatalf("unbind failed: %v", err)
			}
			if !removed {
				t.Fatalf("%s registration was not removed", tc.name)
			}
			var a *implA
			if err := c.Resolve(&a); !errors.Is(err, ErrNotRegistered) {
				t.Fatalf("expected ErrNotRegistered after unbind, got %v", err)
			}
		})
	}
}
