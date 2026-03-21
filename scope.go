// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix scope abstraction.
package dix

// A Scope represents a bounded lifetime for scoped dependencies.
// Scopes must be explicitly closed to release resources.

// Scope represents a scoped dependency resolver.
type Scope interface {
	Resolver

	// Close releases all scoped resources.
	Close() error
}
