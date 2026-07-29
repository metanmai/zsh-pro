package cli

import "reflect"

// isNilLike reports whether an interface is nil or wraps a nil value. Public
// constructors use it at composition boundaries so an injected typed nil takes
// the existing unavailable-dependency path instead of panicking on a method
// call later in a CLI verb.
func isNilLike(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
