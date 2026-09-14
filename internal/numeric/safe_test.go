package numeric

import "testing"

func TestIsSafeJSONIntegerBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  bool
	}{
		{"minimum", SafeJSONIntegerMin, true},
		{"maximum", SafeJSONIntegerMax, true},
		{"below minimum", SafeJSONIntegerMin - 1, false},
		{"above maximum", SafeJSONIntegerMax + 1, false},
		{"zero", 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsSafeJSONInteger(test.value); got != test.want {
				t.Fatalf("IsSafeJSONInteger(%d) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
