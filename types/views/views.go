// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package views provides read-only accessors for commonly used
// value types.
package views

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"go4.org/mem"
)

func unmarshalSliceFromJSON[T any](b []byte, x *[]T) error {
	if *x != nil {
		return errors.New("already initialized")
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, x)
}

// ByteSlice is a read-only accessor for types that are backed by a []byte.
type ByteSlice[T ~[]byte] struct {
	// ж is the underlying mutable value, named with a hard-to-type
	// character that looks pointy like a pointer.
	// It is named distinctively to make you think of how dangerous it is to escape
	// to callers. You must not let callers be able to mutate it.
	ж T
}

// ByteSliceOf returns a ByteSlice for the provided slice.
func ByteSliceOf[T ~[]byte](x T) ByteSlice[T] {
	return ByteSlice[T]{x}
}

// MapKey returns a unique key for a slice, based on its address and length.
func (v ByteSlice[T]) MapKey() SliceMapKey[byte] { return mapKey(v.ж) }

// Len returns the length of the slice.
func (v ByteSlice[T]) Len() int {
	return len(v.ж)
}

// IsNil reports whether the underlying slice is nil.
func (v ByteSlice[T]) IsNil() bool {
	return v.ж == nil
}

// Mem returns a read-only view of the underlying slice.
func (v ByteSlice[T]) Mem() mem.RO {
	return mem.B(v.ж)
}

// Equal reports whether the underlying slice is equal to b.
func (v ByteSlice[T]) Equal(b T) bool {
	return bytes.Equal(v.ж, b)
}

// EqualView reports whether the underlying slice is equal to b.
func (v ByteSlice[T]) EqualView(b ByteSlice[T]) bool {
	return bytes.Equal(v.ж, b.ж)
}

// AsSlice returns a copy of the underlying slice.
func (v ByteSlice[T]) AsSlice() T {
	return v.AppendTo(v.ж[:0:0])
}

// AppendTo appends the underlying slice values to dst.
func (v ByteSlice[T]) AppendTo(dst T) T {
	return append(dst, v.ж...)
}

// At returns the byte at index `i` of the slice.
func (v ByteSlice[T]) At(i int) byte { return v.ж[i] }

// SliceFrom returns v[i:].
func (v ByteSlice[T]) SliceFrom(i int) ByteSlice[T] { return ByteSlice[T]{v.ж[i:]} }

// SliceTo returns v[:i]
func (v ByteSlice[T]) SliceTo(i int) ByteSlice[T] { return ByteSlice[T]{v.ж[:i]} }

// Slice returns v[i:j]
func (v ByteSlice[T]) Slice(i, j int) ByteSlice[T] { return ByteSlice[T]{v.ж[i:j]} }

// MarshalJSON implements json.Marshaler.
func (v ByteSlice[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.ж) }

// UnmarshalJSON implements json.Unmarshaler.
func (v *ByteSlice[T]) UnmarshalJSON(b []byte) error {
	if v.ж != nil {
		return errors.New("already initialized")
	}
	return json.Unmarshal(b, &v.ж)
}

// StructView represents the corresponding StructView of a Viewable. The concrete types are
// typically generated by tailscale.com/cmd/viewer.
type StructView[T any] interface {
	// Valid reports whether the underlying Viewable is nil.
	Valid() bool
	// AsStruct returns a deep-copy of the underlying value.
	// It returns nil, if Valid() is false.
	AsStruct() T
}

// ViewCloner is any type that has had View and Clone funcs generated using
// tailscale.com/cmd/viewer.
type ViewCloner[T any, V StructView[T]] interface {
	// View returns a read-only view of Viewable.
	// If Viewable is nil, View().Valid() reports false.
	View() V
	// Clone returns a deep-clone of Viewable.
	// It returns nil, when Viewable is nil.
	Clone() T
}

// SliceOfViews returns a ViewSlice for x.
func SliceOfViews[T ViewCloner[T, V], V StructView[T]](x []T) SliceView[T, V] {
	return SliceView[T, V]{x}
}

// SliceView wraps []T to provide accessors which return an immutable view V of
// T. It is used to provide the equivalent of SliceOf([]V) without having to
// allocate []V from []T.
type SliceView[T ViewCloner[T, V], V StructView[T]] struct {
	// ж is the underlying mutable value, named with a hard-to-type
	// character that looks pointy like a pointer.
	// It is named distinctively to make you think of how dangerous it is to escape
	// to callers. You must not let callers be able to mutate it.
	ж []T
}

// MarshalJSON implements json.Marshaler.
func (v SliceView[T, V]) MarshalJSON() ([]byte, error) { return json.Marshal(v.ж) }

// UnmarshalJSON implements json.Unmarshaler.
func (v *SliceView[T, V]) UnmarshalJSON(b []byte) error { return unmarshalSliceFromJSON(b, &v.ж) }

// IsNil reports whether the underlying slice is nil.
func (v SliceView[T, V]) IsNil() bool { return v.ж == nil }

// Len returns the length of the slice.
func (v SliceView[T, V]) Len() int { return len(v.ж) }

// At returns a View of the element at index `i` of the slice.
func (v SliceView[T, V]) At(i int) V { return v.ж[i].View() }

// SliceFrom returns v[i:].
func (v SliceView[T, V]) SliceFrom(i int) SliceView[T, V] { return SliceView[T, V]{v.ж[i:]} }

// SliceTo returns v[:i]
func (v SliceView[T, V]) SliceTo(i int) SliceView[T, V] { return SliceView[T, V]{v.ж[:i]} }

// Slice returns v[i:j]
func (v SliceView[T, V]) Slice(i, j int) SliceView[T, V] { return SliceView[T, V]{v.ж[i:j]} }

// SliceMapKey represents a comparable unique key for a slice, based on its
// address and length. It can be used to key maps by slices but should only be
// used when the underlying slice is immutable.
//
// Empty and nil slices have different keys.
type SliceMapKey[T any] struct {
	// t is the address of the first element, or nil if the slice is nil or
	// empty.
	t *T
	// n is the length of the slice, or -1 if the slice is nil.
	n int
}

// MapKey returns a unique key for a slice, based on its address and length.
func (v SliceView[T, V]) MapKey() SliceMapKey[T] { return mapKey(v.ж) }

// AppendTo appends the underlying slice values to dst.
func (v SliceView[T, V]) AppendTo(dst []V) []V {
	for _, x := range v.ж {
		dst = append(dst, x.View())
	}
	return dst
}

// AsSlice returns a copy of underlying slice.
func (v SliceView[T, V]) AsSlice() []V {
	return v.AppendTo(nil)
}

// Slice is a read-only accessor for a slice.
type Slice[T any] struct {
	// ж is the underlying mutable value, named with a hard-to-type
	// character that looks pointy like a pointer.
	// It is named distinctively to make you think of how dangerous it is to escape
	// to callers. You must not let callers be able to mutate it.
	ж []T
}

// MapKey returns a unique key for a slice, based on its address and length.
func (v Slice[T]) MapKey() SliceMapKey[T] { return mapKey(v.ж) }

// mapKey returns a unique key for a slice, based on its address and length.
func mapKey[T any](x []T) SliceMapKey[T] {
	if x == nil {
		return SliceMapKey[T]{nil, -1}
	}
	if len(x) == 0 {
		return SliceMapKey[T]{nil, 0}
	}
	return SliceMapKey[T]{&x[0], len(x)}
}

// SliceOf returns a Slice for the provided slice for immutable values.
// It is the caller's responsibility to make sure V is immutable.
func SliceOf[T any](x []T) Slice[T] {
	return Slice[T]{x}
}

// MarshalJSON implements json.Marshaler.
func (v Slice[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.ж)
}

// UnmarshalJSON implements json.Unmarshaler.
func (v *Slice[T]) UnmarshalJSON(b []byte) error {
	return unmarshalSliceFromJSON(b, &v.ж)
}

// IsNil reports whether the underlying slice is nil.
func (v Slice[T]) IsNil() bool { return v.ж == nil }

// Len returns the length of the slice.
func (v Slice[T]) Len() int { return len(v.ж) }

// At returns the element at index `i` of the slice.
func (v Slice[T]) At(i int) T { return v.ж[i] }

// SliceFrom returns v[i:].
func (v Slice[T]) SliceFrom(i int) Slice[T] { return Slice[T]{v.ж[i:]} }

// SliceTo returns v[:i]
func (v Slice[T]) SliceTo(i int) Slice[T] { return Slice[T]{v.ж[:i]} }

// Slice returns v[i:j]
func (v Slice[T]) Slice(i, j int) Slice[T] { return Slice[T]{v.ж[i:j]} }

// AppendTo appends the underlying slice values to dst.
func (v Slice[T]) AppendTo(dst []T) []T {
	return append(dst, v.ж...)
}

// AsSlice returns a copy of underlying slice.
func (v Slice[T]) AsSlice() []T {
	return v.AppendTo(v.ж[:0:0])
}

// IndexFunc returns the first index of an element in v satisfying f(e),
// or -1 if none do.
//
// As it runs in O(n) time, use with care.
func (v Slice[T]) IndexFunc(f func(T) bool) int {
	for i := 0; i < v.Len(); i++ {
		if f(v.At(i)) {
			return i
		}
	}
	return -1
}

// ContainsFunc reports whether any element in v satisfies f(e).
//
// As it runs in O(n) time, use with care.
func (v Slice[T]) ContainsFunc(f func(T) bool) bool {
	for _, x := range v.ж {
		if f(x) {
			return true
		}
	}
	return false
}

// SliceContains reports whether v contains element e.
//
// As it runs in O(n) time, use with care.
func SliceContains[T comparable](v Slice[T], e T) bool {
	for _, x := range v.ж {
		if x == e {
			return true
		}
	}
	return false
}

// SliceContainsFunc reports whether f reports true for any element in v.
func SliceContainsFunc[T any](v Slice[T], f func(T) bool) bool {
	for _, x := range v.ж {
		if f(x) {
			return true
		}
	}
	return false
}

// SliceEqual is like the standard library's slices.Equal, but for two views.
func SliceEqual[T comparable](a, b Slice[T]) bool {
	return slices.Equal(a.ж, b.ж)
}

// SliceEqualAnyOrder reports whether a and b contain the same elements, regardless of order.
// The underlying slices for a and b can be nil.
func SliceEqualAnyOrder[T comparable](a, b Slice[T]) bool {
	if a.Len() != b.Len() {
		return false
	}

	var diffStart int // beginning index where a and b differ
	for n := a.Len(); diffStart < n; diffStart++ {
		if a.At(diffStart) != b.At(diffStart) {
			break
		}
	}
	if diffStart == a.Len() {
		return true
	}

	// count the occurrences of remaining values and compare
	valueCount := make(map[T]int)
	for i, n := diffStart, a.Len(); i < n; i++ {
		valueCount[a.At(i)]++
		valueCount[b.At(i)]--
	}
	for _, count := range valueCount {
		if count != 0 {
			return false
		}
	}
	return true
}

// MapOf returns a view over m. It is the caller's responsibility to make sure K
// and V is immutable, if this is being used to provide a read-only view over m.
func MapOf[K comparable, V comparable](m map[K]V) Map[K, V] {
	return Map[K, V]{m}
}

// Map is a view over a map whose values are immutable.
type Map[K comparable, V any] struct {
	// ж is the underlying mutable value, named with a hard-to-type
	// character that looks pointy like a pointer.
	// It is named distinctively to make you think of how dangerous it is to escape
	// to callers. You must not let callers be able to mutate it.
	ж map[K]V
}

// Has reports whether k has an entry in the map.
func (m Map[K, V]) Has(k K) bool {
	_, ok := m.ж[k]
	return ok
}

// IsNil reports whether the underlying map is nil.
func (m Map[K, V]) IsNil() bool {
	return m.ж == nil
}

// Len returns the number of elements in the map.
func (m Map[K, V]) Len() int { return len(m.ж) }

// Get returns the element with key k.
func (m Map[K, V]) Get(k K) V {
	return m.ж[k]
}

// GetOk returns the element with key k and a bool representing whether the key
// is in map.
func (m Map[K, V]) GetOk(k K) (V, bool) {
	v, ok := m.ж[k]
	return v, ok
}

// MarshalJSON implements json.Marshaler.
func (m Map[K, V]) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.ж)
}

// UnmarshalJSON implements json.Unmarshaler.
// It should only be called on an uninitialized Map.
func (m *Map[K, V]) UnmarshalJSON(b []byte) error {
	if m.ж != nil {
		return errors.New("already initialized")
	}
	return json.Unmarshal(b, &m.ж)
}

// AsMap returns a shallow-clone of the underlying map.
// If V is a pointer type, it is the caller's responsibility to make sure
// the values are immutable.
func (m *Map[K, V]) AsMap() map[K]V {
	if m == nil {
		return nil
	}
	return maps.Clone(m.ж)
}

// MapRangeFn is the func called from a Map.Range call.
// Implementations should return false to stop range.
type MapRangeFn[K comparable, V any] func(k K, v V) (cont bool)

// Range calls f for every k,v pair in the underlying map.
// It stops iteration immediately if f returns false.
func (m Map[K, V]) Range(f MapRangeFn[K, V]) {
	for k, v := range m.ж {
		if !f(k, v) {
			return
		}
	}
}

// MapFnOf returns a MapFn for m.
func MapFnOf[K comparable, T any, V any](m map[K]T, f func(T) V) MapFn[K, T, V] {
	return MapFn[K, T, V]{
		ж:     m,
		wrapv: f,
	}
}

// MapFn is like Map but with a func to convert values from T to V.
// It is used to provide map of slices and views.
type MapFn[K comparable, T any, V any] struct {
	// ж is the underlying mutable value, named with a hard-to-type
	// character that looks pointy like a pointer.
	// It is named distinctively to make you think of how dangerous it is to escape
	// to callers. You must not let callers be able to mutate it.
	ж     map[K]T
	wrapv func(T) V
}

// Has reports whether k has an entry in the map.
func (m MapFn[K, T, V]) Has(k K) bool {
	_, ok := m.ж[k]
	return ok
}

// Get returns the element with key k.
func (m MapFn[K, T, V]) Get(k K) V {
	return m.wrapv(m.ж[k])
}

// IsNil reports whether the underlying map is nil.
func (m MapFn[K, T, V]) IsNil() bool {
	return m.ж == nil
}

// Len returns the number of elements in the map.
func (m MapFn[K, T, V]) Len() int { return len(m.ж) }

// GetOk returns the element with key k and a bool representing whether the key
// is in map.
func (m MapFn[K, T, V]) GetOk(k K) (V, bool) {
	v, ok := m.ж[k]
	return m.wrapv(v), ok
}

// Range calls f for every k,v pair in the underlying map.
// It stops iteration immediately if f returns false.
func (m MapFn[K, T, V]) Range(f MapRangeFn[K, V]) {
	for k, v := range m.ж {
		if !f(k, m.wrapv(v)) {
			return
		}
	}
}
