// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix default container implementation.
package dix

import (
	"fmt"
	"reflect"
)

// This file provides a default implementation of the Container interface,
// managing dependency registration and resolution with different lifetimes.

// defaultContainer is the default implementation of the Container interface.
type defaultContainer struct {
	singletons map[reflect.Type]any
	scoped     map[reflect.Type]func(r Resolver) any
	transient  map[reflect.Type]func(r Resolver) any
}

// New creates a new default container.
func New() *defaultContainer {
	return &defaultContainer{
		singletons: make(map[reflect.Type]any),
		scoped:     make(map[reflect.Type]func(r Resolver) any),
		transient:  make(map[reflect.Type]func(r Resolver) any),
	}
}

// Singleton registers a factory that is instantiated once and shared across all scopes.
func (c *defaultContainer) Singleton(factory any) error {
	typ := reflect.TypeOf(factory)
	if typ.Kind() != reflect.Func || typ.NumOut() != 1 {
		return fmt.Errorf("factory must be a function returning one value")
	}
	outTyp := typ.Out(0)
	val := reflect.ValueOf(factory).Call(nil)[0].Interface()
	c.singletons[outTyp] = val
	return nil
}

// Scoped registers a factory that is instantiated once per scope.
func (c *defaultContainer) Scoped(factory any) error {
	typ := reflect.TypeOf(factory)
	if typ.Kind() != reflect.Func || typ.NumOut() != 1 {
		return fmt.Errorf("factory must be a function returning one value")
	}
	c.scoped[typ.Out(0)] = func(r Resolver) any {
		return reflect.ValueOf(factory).Call(nil)[0].Interface()
	}
	return nil
}

// Transient registers a factory that is instantiated on every resolve.
func (c *defaultContainer) Transient(factory any) error {
	typ := reflect.TypeOf(factory)
	if typ.Kind() != reflect.Func || typ.NumOut() != 1 {
		return fmt.Errorf("factory must be a function returning one value")
	}
	c.transient[typ.Out(0)] = func(r Resolver) any {
		return reflect.ValueOf(factory).Call(nil)[0].Interface()
	}
	return nil
}

// Instance registers a pre-constructed value.
func (c *defaultContainer) Instance(v any) error {
	typ := reflect.TypeOf(v)
	if typ == nil {
		return fmt.Errorf("cannot register nil as instance")
	}
	if typ.Kind() == reflect.Func {
		return fmt.Errorf("cannot register function as instance, use Singleton, Scoped, or Transient for factories")
	}
	c.singletons[typ] = v
	return nil
}

// NewScope creates a child scope with scoped lifetime semantics.
func (c *defaultContainer) NewScope() Scope {
	return &defaultScope{
		container:       c,
		scopedInstances: make(map[reflect.Type]any),
	}
}

// get retrieves a value for the given type from the container.
func (c *defaultContainer) get(typ reflect.Type) (any, error) {
	// First try direct lookup
	if val, ok := c.singletons[typ]; ok {
		return val, nil
	}

	// If typ is an interface, check if any stored concrete types implement it
	if typ.Kind() == reflect.Interface {
		for storedType, val := range c.singletons {
			if storedType.Implements(typ) {
				return val, nil
			}
		}
	}

	if factory, ok := c.transient[typ]; ok {
		return factory(c), nil
	}
	return nil, fmt.Errorf("no registration for %s", typ)
}

// Resolve injects dependencies into the given target.
func (c *defaultContainer) Resolve(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	val, err := c.get(elem)
	if err != nil {
		return err
	}
	reflect.ValueOf(target).Elem().Set(reflect.ValueOf(val))
	return nil
}

// ResolveAll injects all resolvable dependencies into the target.
func (c *defaultContainer) ResolveAll(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	if elem.Kind() != reflect.Slice || elem.Elem().Kind() != reflect.Interface {
		return fmt.Errorf("target must be a pointer to []interface{}")
	}
	val := reflect.ValueOf(target).Elem()
	// Append all registered dependencies
	for _, v := range c.singletons {
		val.Set(reflect.Append(val, reflect.ValueOf(v)))
	}
	for _, factory := range c.transient {
		if factory != nil {
			v := factory(c)
			val.Set(reflect.Append(val, reflect.ValueOf(v)))
		}
	}
	for _, factory := range c.scoped {
		if factory != nil {
			v := factory(c)
			val.Set(reflect.Append(val, reflect.ValueOf(v)))
		}
	}
	return nil
}
