// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 Kryovyx

// Package dix reflection-based resolver implementation.
package dix

import (
	"fmt"
	"reflect"
)

// This file provides a reflection-based implementation of the Resolver interface,
// using Go's reflection capabilities to inject dependencies into target objects.

// reflectResolver is a reflection-based implementation of the Resolver interface.
type reflectResolver struct {
	values map[reflect.Type]any
}

// NewReflectResolver creates a new reflection-based resolver.
func NewReflectResolver() *reflectResolver {
	return &reflectResolver{values: make(map[reflect.Type]any)}
}

// Register associates a value with a type for resolution.
func (r *reflectResolver) Register(typ reflect.Type, value any) {
	r.values[typ] = value
}

// Resolve injects dependencies into the given target.
func (r *reflectResolver) Resolve(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	if val, ok := r.values[elem]; ok {
		reflect.ValueOf(target).Elem().Set(reflect.ValueOf(val))
		return nil
	}
	return fmt.Errorf("no value registered for type %s", elem)
}

// ResolveAll injects all resolvable dependencies into the target.
func (r *reflectResolver) ResolveAll(target any) error {
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	elem := typ.Elem()
	if elem.Kind() != reflect.Slice || elem.Elem().Kind() != reflect.Interface {
		return fmt.Errorf("target must be a pointer to []interface{}")
	}
	val := reflect.ValueOf(target).Elem()
	for _, v := range r.values {
		val.Set(reflect.Append(val, reflect.ValueOf(v)))
	}
	return nil
}
