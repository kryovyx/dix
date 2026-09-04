// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix default scope implementation.
package dix

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// This file provides a default implementation of the Scope interface,
// managing scoped dependency resolution and resource cleanup.
//
// A scope is normally per-request and used from one goroutine, but a handler
// that fans out to goroutines which resolve would otherwise hit the same
// concurrent map read/write fatal error the container had, so scopedInstances
// is guarded too.

// defaultScope is the default implementation of the Scope interface.
type defaultScope struct {
	container *defaultContainer

	mu              sync.Mutex
	scopedInstances map[reflect.Type]any
	closed          bool
}

// get retrieves a value for the given type from the scope or container.
//
// Scoped registrations are instantiated here, memoised for the lifetime of the
// scope, and closed by Close. Everything else delegates to the container, with
// the scope passed as the Resolver so factories taking func(dix.Resolver) T see
// the scope rather than the root.
func (s *defaultScope) get(typ reflect.Type) (any, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: cannot resolve %s", ErrScopeClosed, typ)
	}
	if val, ok := s.scopedInstances[typ]; ok {
		s.mu.Unlock()
		return val, nil
	}
	s.mu.Unlock()

	cands := s.container.lookup(typ)
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
		return cd.sing.value(s), nil
	case lifetimeTransient:
		return cd.fn(s), nil
	}

	// Scoped: construct outside the lock — the factory is user code and may
	// resolve further dependencies through this same scope.
	val := cd.fn(s)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		// Lost a race with Close. Release the orphan rather than handing back
		// a value nothing will ever close.
		_ = closeValue(val)
		return nil, fmt.Errorf("%w: cannot resolve %s", ErrScopeClosed, typ)
	}
	if existing, ok := s.scopedInstances[cd.typ]; ok {
		s.mu.Unlock()
		// Another goroutine built it first. Keep the stored instance and
		// release ours, unless the factory handed back the very same value.
		if !sameInstance(existing, val) {
			_ = closeValue(val)
		}
		return existing, nil
	}
	s.scopedInstances[cd.typ] = val
	s.mu.Unlock()
	return val, nil
}

// sameInstance reports whether a and b are the same value, without panicking
// on types that are not comparable (a struct with a slice field, say).
func sameInstance(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

// Resolve injects dependencies into the given target.
func (s *defaultScope) Resolve(target any) error {
	elem, err := resolveTargetType(target)
	if err != nil {
		return err
	}
	val, err := s.get(elem)
	if err != nil {
		return err
	}
	return assign(target, val)
}

// ResolveAll injects all resolvable dependencies into the target.
//
// Scoped values instantiated here are tracked by the scope, so Close releases
// them. They used to be built and dropped on the floor, which leaked every
// closer ResolveAll ever touched.
func (s *defaultScope) ResolveAll(target any) error {
	val, err := resolveAllTarget(target)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrScopeClosed
	}
	already := make([]any, 0, len(s.scopedInstances))
	for _, v := range s.scopedInstances {
		already = append(already, v)
	}
	s.mu.Unlock()

	for _, v := range already {
		appendValue(val, v)
	}

	c := s.container
	c.mu.RLock()
	sings := make([]*singletonEntry, 0, len(c.singletons))
	for _, e := range c.singletons {
		sings = append(sings, e)
	}
	trans := make([]factory, 0, len(c.transient))
	for _, fn := range c.transient {
		trans = append(trans, fn)
	}
	scopedTypes := make([]reflect.Type, 0, len(c.scoped))
	for t := range c.scoped {
		scopedTypes = append(scopedTypes, t)
	}
	c.mu.RUnlock()

	for _, e := range sings {
		appendValue(val, e.value(s))
	}
	for _, fn := range trans {
		appendValue(val, fn(s))
	}
	// Route scoped types back through get so each one is memoised and closed.
	for _, t := range scopedTypes {
		v, err := s.get(t)
		if err != nil {
			return err
		}
		appendValue(val, v)
	}
	return nil
}

// Close releases all scoped resources.
//
// Every closer is closed even if an earlier one fails — returning on the first
// error left the remainder open, which turned one failing Close into a leak of
// all the others. Errors are joined. Close is idempotent.
func (s *defaultScope) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	vals := make([]any, 0, len(s.scopedInstances))
	for _, v := range s.scopedInstances {
		vals = append(vals, v)
	}
	s.scopedInstances = nil
	s.mu.Unlock()

	var errs []error
	for _, v := range vals {
		if err := closeValue(v); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// closeValue calls Close on v when it has one.
func closeValue(v any) error {
	if closer, ok := v.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
