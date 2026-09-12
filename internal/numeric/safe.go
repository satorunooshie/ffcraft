// Package numeric contains numeric-domain checks shared by the input codecs.
package numeric

// SafeJSONIntegerMin and SafeJSONIntegerMax are the conservative integer
// bounds used when an integer is about to be represented by a JSON/protobuf
// Struct number (binary64).
const (
	SafeJSONIntegerMin int64 = -((1 << 53) - 1)
	SafeJSONIntegerMax int64 = (1 << 53) - 1
)

// IsSafeJSONInteger reports whether value can be represented in the v1 object
// numeric domain without losing integer intent.
func IsSafeJSONInteger(value int64) bool {
	return value >= SafeJSONIntegerMin && value <= SafeJSONIntegerMax
}
