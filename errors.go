// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix sentinel errors.
package dix

import "errors"

// Sentinel errors returned by the default container and scope. They exist so
// callers can branch with errors.Is rather than matching on message text —
// which is what the extensions were reduced to doing elsewhere in the
// ecosystem (see the strings.Contains checks fixed by D39).
var (
	// ErrNotRegistered means nothing in the container satisfies the requested
	// type.
	ErrNotRegistered = errors.New("dix: no registration for type")

	// ErrScopedFromRoot means the requested type is registered as Scoped and
	// was resolved from the root container.
	//
	// The container deliberately does NOT instantiate it. A scoped value built
	// from the root would have no owning scope, so nothing would ever call its
	// Close — which is precisely the per-request leak that closing request
	// scopes exists to prevent (D8/O1). Resolve it from a Scope instead:
	//
	//	scope := container.NewScope()
	//	defer scope.Close()
	//	var svc MyService
	//	err := scope.Resolve(&svc)
	ErrScopedFromRoot = errors.New("dix: type is registered as Scoped; resolve it from a Scope")

	// ErrAmbiguousResolution means more than one registration satisfies the
	// requested interface.
	//
	// Returning one of them would make the winner depend on Go's randomised
	// map iteration order, so the same binary would resolve differently from
	// run to run (D6). The error names every candidate.
	ErrAmbiguousResolution = errors.New("dix: ambiguous resolution")

	// ErrAlreadyRegistered means the type already has a registration under a
	// different (or the same) lifetime.
	//
	// Registering one type twice would force the container to pick a lifetime
	// by precedence, which is the same nondeterminism ErrAmbiguousResolution
	// rejects — only hidden behind a lookup order instead of a map range.
	ErrAlreadyRegistered = errors.New("dix: type is already registered")

	// ErrInvalidFactory means the value passed to Singleton, Scoped or
	// Transient is not an accepted factory signature.
	ErrInvalidFactory = errors.New("dix: invalid factory")

	// ErrScopeClosed means a Scope was used after Close.
	ErrScopeClosed = errors.New("dix: scope is closed")

	// ErrInvalidTarget means the target passed to Resolve or ResolveAll has
	// the wrong shape (not a pointer, or not a pointer to a slice of
	// interfaces for ResolveAll).
	ErrInvalidTarget = errors.New("dix: invalid resolve target")
)
