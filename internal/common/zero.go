package common

import "reflect"

func IsNilOrZero(i any) bool {
	return IsNil(i) || IsZero(i)
}

// IsNil reports whether i is nil. It safely handles typed nil values held in interfaces.
func IsNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// IsZero reports whether v is the zero value for its type.
// Note: For slices and maps, only a nil value is zero; empty but non-nil values are not zero.
func IsZero[T any](v T) bool {
	return reflect.ValueOf(v).IsZero()
}

// NilOrZero reports whether pointer p is nil or points to a zero value of its element type.
func NilOrZero[T any](p *T) bool {
	if p == nil {
		return true
	}
	return IsZero(*p)
}
