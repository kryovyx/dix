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

	// NewScope creates a child scope with scoped lifetime semantics.
	NewScope() Scope
}
