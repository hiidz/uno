// Package jsonwire holds the conversions Uno applies to a value on its way
// out to JSON, where Go's zero values and the wire's don't line up.
package jsonwire

// OrEmpty returns s unchanged unless it's nil, in which case it returns a
// non-nil empty slice. A nil slice and a zero-length slice are the same
// slice to Go, but not to encoding/json: nil marshals as "null", not "[]".
// Used at the spots that produce a slice by map lookup, by decoding a
// response, or by an early return rather than by appending — a range/append
// naturally ends up non-nil even at zero length, so callers that already
// build a slice that way don't need this.
func OrEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// OrEmptyMap is OrEmpty for a map, which marshals as "null" when nil for the
// same reason.
func OrEmptyMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return map[K]V{}
	}
	return m
}
