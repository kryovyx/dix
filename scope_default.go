// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix default scope implementation.
package dix

import (
	"fmt"
	"reflect"
)

// This file provides a default implementation of the Scope interface,
// managing scoped dependency resolution and resource cleanup.

// defaultScope is the default implementation of the Scope interface.
type defaultScope struct {
	container       *defaultContainer
	scopedInstances map[reflect.Type]any
}

// get retrieves a value for the given type from the scope or container.
func (s *defaultScope) get(typ reflect.Type) (any, error) {
	if val, ok := s.scopedInstances[typ]; ok {
		return val, nil
	}
	if factory, ok := s.container.scoped[typ]; ok {
		val := factory(s)
		s.scopedInstances[typ] = val
		return val, nil
	}
	return s.container.get(typ)
}

// Resolve injects dependencies into the given target.
// Resolve injects dependencies into the given target.
func (s *defaultScope) Resolve(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	val, err := s.get(elem)
	if err != nil {
		return err
	}
	reflect.ValueOf(target).Elem().Set(reflect.ValueOf(val))
	return nil
}

// ResolveAll injects all resolvable dependencies into the target.
func (s *defaultScope) ResolveAll(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	if elem.Kind() != reflect.Slice || elem.Elem().Kind() != reflect.Interface {
		return fmt.Errorf("target must be a pointer to []interface{}")
	}
	val := reflect.ValueOf(target).Elem()
	// Append all resolvable dependencies from scope and container
	for _, v := range s.scopedInstances {
		val.Set(reflect.Append(val, reflect.ValueOf(v)))
	}
	// Also from container
	for _, v := range s.container.singletons {
		val.Set(reflect.Append(val, reflect.ValueOf(v)))
	}
	for _, factory := range s.container.transient {
		if factory != nil {
			v := factory(s)
			val.Set(reflect.Append(val, reflect.ValueOf(v)))
		}
	}
	for _, factory := range s.container.scoped {
		if factory != nil {
			v := factory(s)
			val.Set(reflect.Append(val, reflect.ValueOf(v)))
		}
	}
	return nil
}

// Close releases all scoped resources.
func (s *defaultScope) Close() error {
	for _, val := range s.scopedInstances {
		if closer, ok := val.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
