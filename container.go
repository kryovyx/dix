// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix container abstractions.
package dix

// This file defines the Container interface, which is responsible for
// managing dependency lifetimes and producing scoped resolvers.

// Container manages dependency registration and resolution.
type Container interface {
	Resolver

	// Singleton registers a factory that is instantiated once
	// and shared across all scopes.
	Singleton(factory any) error

	// Scoped registers a factory that is instantiated once per scope.
	Scoped(factory any) error

	// Transient registers a factory that is instantiated on every resolve.
	Transient(factory any) error

	// Instance registers a pre-constructed value.
	Instance(v any) error

	// Unbind removes the registration for the exact type of v, if there is
	// one. It reports whether anything was removed.
	//
	// Registration is deliberately not sealed, so a value registered early
	// may need correcting later — replacing a default logger with the one the
	// application actually configured, say. Since a type may hold only one
	// registration (ErrAlreadyRegistered), correcting one means removing the
	// old registration first, and the old value may well be a different
	// concrete type than the new one:
	//
	//	_ = c.Unbind(oldLogger) // *slogLogger
	//	_ = c.Instance(newLogger) // *myLogger
	//
	// Unbind does not Close anything it removes; the caller owns that value.
	// Singletons already constructed and handed out are unaffected.
	Unbind(v any) (bool, error)

	// NewScope creates a child scope with scoped lifetime semantics.
	NewScope() Scope
}
