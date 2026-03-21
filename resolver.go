// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix defines a minimal dependency injection contract.
package dix

// Resolver resolves dependencies into the provided target.
type Resolver interface {
	// Resolve injects dependencies into the given target.
	Resolve(target any) error

	// ResolveAll injects all resolvable dependencies into the target.
	ResolveAll(target any) error
}
