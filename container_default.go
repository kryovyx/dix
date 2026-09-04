// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix default container implementation.
package dix

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// This file provides a default implementation of the Container interface,
// managing dependency registration and resolution with different lifetimes.
//
// Concurrency (D5): every read of the registration maps takes the RWMutex.
// Before that guard existed, container.get ranged over singletons on every
// interface resolve while Instance and Singleton wrote the same map, which is
// an unrecoverable "fatal error: concurrent map read and map write" — not a
// recoverable panic, and not something -race merely warns about. Late
// registration stays legal; the container is not sealed.
//
// Factories are always invoked with the lock released. They are user code and
// may resolve further dependencies, so calling them under RLock would risk a
// deadlock the moment a writer queued up behind the re-entrant read.

// resolverType is the reflect.Type of the Resolver interface, used to detect
// the func(Resolver) T factory form.
var resolverType = reflect.TypeOf((*Resolver)(nil)).Elem()

// factory constructs a value, given the Resolver its dependencies should come
// from. Factories registered as plain func() T ignore the argument.
type factory func(r Resolver) any

// lifetime names the three registration kinds, for error messages.
type lifetime uint8

const (
	lifetimeSingleton lifetime = iota
	lifetimeScoped
	lifetimeTransient
)

func (l lifetime) String() string {
	switch l {
	case lifetimeSingleton:
		return "Singleton"
	case lifetimeScoped:
		return "Scoped"
	case lifetimeTransient:
		return "Transient"
	}
	return "unknown"
}

// singletonEntry holds a single-instance registration.
//
// Singletons are lazy (D7): the factory runs on first resolve, not at
// registration. Registering eagerly meant a factory could not depend on
// anything registered after it, which made registration order load-bearing for
// no reason. sync.Once gives exactly-once construction under concurrency.
//
// A value registered via Instance has a nil fn and is returned as-is.
type singletonEntry struct {
	once sync.Once
	fn   factory
	val  any
}

// value returns the instance, constructing it on first call.
//
// fn is written once at registration and never again: clearing it inside the
// Once to release the closure would race with the fn != nil test out here,
// which is a plain data race even though the construction itself is guarded.
func (e *singletonEntry) value(r Resolver) any {
	if e.fn != nil {
		e.once.Do(func() { e.val = e.fn(r) })
	}
	return e.val
}

// defaultContainer is the default implementation of the Container interface.
type defaultContainer struct {
	mu         sync.RWMutex
	singletons map[reflect.Type]*singletonEntry
	scoped     map[reflect.Type]factory
	transient  map[reflect.Type]factory
}

// New creates a new default container.
func New() *defaultContainer {
	return &defaultContainer{
		singletons: make(map[reflect.Type]*singletonEntry),
		scoped:     make(map[reflect.Type]factory),
		transient:  make(map[reflect.Type]factory),
	}
}

// ---------------------------------------------------------------------------
// Factory adaptation
// ---------------------------------------------------------------------------

// makeFactory validates a registration argument and adapts it to a factory.
//
// Two signatures are accepted (D7/O1):
//
//	func() T             — no dependencies
//	func(dix.Resolver) T — dependencies resolved from the caller's Resolver
//
// Anything else is rejected here, at registration, rather than panicking later
// inside reflect.Call on the first resolve.
func makeFactory(f any) (reflect.Type, factory, error) {
	typ := reflect.TypeOf(f)
	if typ == nil || typ.Kind() != reflect.Func {
		return nil, nil, fmt.Errorf("%w: expected a function, got %T", ErrInvalidFactory, f)
	}
	if typ.NumOut() != 1 {
		return nil, nil, fmt.Errorf("%w: factory must return exactly one value, got %d", ErrInvalidFactory, typ.NumOut())
	}
	out := typ.Out(0)
	fv := reflect.ValueOf(f)

	switch {
	case typ.NumIn() == 0:
		return out, func(Resolver) any { return fv.Call(nil)[0].Interface() }, nil

	case typ.NumIn() == 1 && typ.In(0) == resolverType:
		return out, func(r Resolver) any {
			// Build the argument as a zero Resolver and set it, so a nil
			// Resolver stays a valid (nil) interface value instead of
			// becoming an invalid reflect.Value.
			arg := reflect.New(resolverType).Elem()
			if r != nil {
				arg.Set(reflect.ValueOf(r))
			}
			return fv.Call([]reflect.Value{arg})[0].Interface()
		}, nil

	default:
		return nil, nil, fmt.Errorf(
			"%w: signature %s not supported — use func() %s or func(dix.Resolver) %s",
			ErrInvalidFactory, typ, out, out)
	}
}

// claim reserves typ for a lifetime, refusing a type that is already
// registered. Callers must hold the write lock.
func (c *defaultContainer) claim(typ reflect.Type) error {
	if _, ok := c.singletons[typ]; ok {
		return fmt.Errorf("%w: %s (as %s)", ErrAlreadyRegistered, typ, lifetimeSingleton)
	}
	if _, ok := c.scoped[typ]; ok {
		return fmt.Errorf("%w: %s (as %s)", ErrAlreadyRegistered, typ, lifetimeScoped)
	}
	if _, ok := c.transient[typ]; ok {
		return fmt.Errorf("%w: %s (as %s)", ErrAlreadyRegistered, typ, lifetimeTransient)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// Singleton registers a factory that is instantiated once, on first resolve,
// and shared across all scopes.
func (c *defaultContainer) Singleton(f any) error {
	out, fn, err := makeFactory(f)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.claim(out); err != nil {
		return err
	}
	c.singletons[out] = &singletonEntry{fn: fn}
	return nil
}

// Scoped registers a factory that is instantiated once per scope.
//
// Scoped types cannot be resolved from the root container; doing so returns
// ErrScopedFromRoot. See that error's documentation for why.
func (c *defaultContainer) Scoped(f any) error {
	out, fn, err := makeFactory(f)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.claim(out); err != nil {
		return err
	}
	c.scoped[out] = fn
	return nil
}

// Transient registers a factory that is instantiated on every resolve.
func (c *defaultContainer) Transient(f any) error {
	out, fn, err := makeFactory(f)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.claim(out); err != nil {
		return err
	}
	c.transient[out] = fn
	return nil
}

// Instance registers a pre-constructed value.
func (c *defaultContainer) Instance(v any) error {
	typ := reflect.TypeOf(v)
	if typ == nil {
		return fmt.Errorf("%w: cannot register nil as an instance", ErrInvalidFactory)
	}
	if typ.Kind() == reflect.Func {
		return fmt.Errorf("%w: cannot register a function as an instance — use Singleton, Scoped or Transient", ErrInvalidFactory)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.claim(typ); err != nil {
		return err
	}
	c.singletons[typ] = &singletonEntry{val: v}
	return nil
}

// Unbind removes the registration for the exact type of v, if any.
//
// This is the supported way to correct an earlier registration: because a type
// may hold only one registration, and because the replacement is often a
// different concrete type than the original, "just register the new one" would
// leave both in place — and with two candidates an interface resolve becomes
// ErrAmbiguousResolution rather than the intended override.
func (c *defaultContainer) Unbind(v any) (bool, error) {
	typ := reflect.TypeOf(v)
	if typ == nil {
		return false, fmt.Errorf("%w: cannot unbind nil", ErrInvalidTarget)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.singletons[typ]; ok {
		delete(c.singletons, typ)
		return true, nil
	}
	if _, ok := c.scoped[typ]; ok {
		delete(c.scoped, typ)
		return true, nil
	}
	if _, ok := c.transient[typ]; ok {
		delete(c.transient, typ)
		return true, nil
	}
	return false, nil
}

// NewScope creates a child scope with scoped lifetime semantics.
func (c *defaultContainer) NewScope() Scope {
	return &defaultScope{
		container:       c,
		scopedInstances: make(map[reflect.Type]any),
	}
}

// ---------------------------------------------------------------------------
// Resolution
// ---------------------------------------------------------------------------

// candidate is one registration that satisfies a requested type.
type candidate struct {
	typ      reflect.Type
	lifetime lifetime
	sing     *singletonEntry // set when lifetime is Singleton
	fn       factory         // set when lifetime is Scoped or Transient
}

// lookup returns every registration satisfying typ.
//
// An exact type match is unique by construction, because registration refuses
// a type that is already claimed. Only an interface request can produce more
// than one candidate, and when it does the caller must treat that as an error
// rather than picking one — see ErrAmbiguousResolution.
func (c *defaultContainer) lookup(typ reflect.Type) []candidate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if e, ok := c.singletons[typ]; ok {
		return []candidate{{typ: typ, lifetime: lifetimeSingleton, sing: e}}
	}
	if fn, ok := c.transient[typ]; ok {
		return []candidate{{typ: typ, lifetime: lifetimeTransient, fn: fn}}
	}
	if fn, ok := c.scoped[typ]; ok {
		return []candidate{{typ: typ, lifetime: lifetimeScoped, fn: fn}}
	}

	if typ.Kind() != reflect.Interface {
		return nil
	}

	// Interface request: scan every registration and collect all matches, so
	// ambiguity surfaces as an error instead of resolving differently on each
	// process start (D6). The router resolves event.Bus and log.Logger this
	// way, so a coin flip there picks a different logger per boot.
	var out []candidate
	for t, e := range c.singletons {
		if t.Implements(typ) {
			out = append(out, candidate{typ: t, lifetime: lifetimeSingleton, sing: e})
		}
	}
	for t, fn := range c.transient {
		if t.Implements(typ) {
			out = append(out, candidate{typ: t, lifetime: lifetimeTransient, fn: fn})
		}
	}
	for t, fn := range c.scoped {
		if t.Implements(typ) {
			out = append(out, candidate{typ: t, lifetime: lifetimeScoped, fn: fn})
		}
	}
	return out
}

// ambiguityError names every candidate, sorted so the message is stable.
func ambiguityError(typ reflect.Type, cands []candidate) error {
	names := make([]string, 0, len(cands))
	for _, cd := range cands {
		names = append(names, fmt.Sprintf("%s (%s)", cd.typ, cd.lifetime))
	}
	sort.Strings(names)
	return fmt.Errorf("%w: %d registrations satisfy %s: %s — register only one, or resolve the concrete type",
		ErrAmbiguousResolution, len(cands), typ, strings.Join(names, ", "))
}

// get retrieves a value for the given type from the container.
//
// Scoped types are never instantiated here; see ErrScopedFromRoot.
func (c *defaultContainer) get(typ reflect.Type) (any, error) {
	cands := c.lookup(typ)
	switch len(cands) {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, typ)
	case 1:
	default:
		return nil, ambiguityError(typ, cands)
	}

	cd := cands[0]
	switch cd.lifetime {
	case lifetimeSingleton:
		return cd.sing.value(c), nil
	case lifetimeTransient:
		return cd.fn(c), nil
	default: // lifetimeScoped
		return nil, fmt.Errorf("%w: %s", ErrScopedFromRoot, cd.typ)
	}
}

// Resolve injects dependencies into the given target.
func (c *defaultContainer) Resolve(target any) error {
	elem, err := resolveTargetType(target)
	if err != nil {
		return err
	}
	val, err := c.get(elem)
	if err != nil {
		return err
	}
	return assign(target, val)
}

// ResolveAll injects all resolvable dependencies into the target.
//
// Scoped registrations are skipped: instantiating them here would produce
// values with no owning scope, which is the leak ErrScopedFromRoot documents.
// Call ResolveAll on a Scope to include them.
func (c *defaultContainer) ResolveAll(target any) error {
	val, err := resolveAllTarget(target)
	if err != nil {
		return err
	}

	c.mu.RLock()
	sings := make([]*singletonEntry, 0, len(c.singletons))
	for _, e := range c.singletons {
		sings = append(sings, e)
	}
	trans := make([]factory, 0, len(c.transient))
	for _, fn := range c.transient {
		trans = append(trans, fn)
	}
	c.mu.RUnlock()

	for _, e := range sings {
		appendValue(val, e.value(c))
	}
	for _, fn := range trans {
		appendValue(val, fn(c))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared target handling
// ---------------------------------------------------------------------------

// resolveTargetType validates a Resolve target and returns the type to look up.
func resolveTargetType(target any) (reflect.Type, error) {
	typ := reflect.TypeOf(target)
	if typ == nil || typ.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%w: target must be a pointer, got %T", ErrInvalidTarget, target)
	}
	return typ.Elem(), nil
}

// resolveAllTarget validates a ResolveAll target and returns the slice to
// append into.
func resolveAllTarget(target any) (reflect.Value, error) {
	typ := reflect.TypeOf(target)
	if typ == nil || typ.Kind() != reflect.Ptr {
		return reflect.Value{}, fmt.Errorf("%w: target must be a pointer, got %T", ErrInvalidTarget, target)
	}
	elem := typ.Elem()
	if elem.Kind() != reflect.Slice || elem.Elem().Kind() != reflect.Interface {
		return reflect.Value{}, fmt.Errorf("%w: target must be a pointer to a slice of interfaces, got *%s", ErrInvalidTarget, elem)
	}
	return reflect.ValueOf(target).Elem(), nil
}

// assign writes val into the pointer target.
func assign(target any, val any) error {
	dst := reflect.ValueOf(target).Elem()
	if val == nil {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	v := reflect.ValueOf(val)
	if !v.Type().AssignableTo(dst.Type()) {
		return fmt.Errorf("%w: resolved %s is not assignable to %s", ErrInvalidTarget, v.Type(), dst.Type())
	}
	dst.Set(v)
	return nil
}

// appendValue appends v to the slice, tolerating a nil value.
func appendValue(slice reflect.Value, v any) {
	if v == nil {
		return
	}
	slice.Set(reflect.Append(slice, reflect.ValueOf(v)))
}
