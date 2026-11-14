package user

import (
	"encoding/json"
	"testing"
)

func TestOptionalStringUnmarshal(t *testing.T) {
	value := "hello"
	pointer := &value

	tests := []struct {
		name      string
		input     string
		wantSet   bool
		wantValue *string
	}{
		{name: "string", input: `"hello"`, wantSet: true, wantValue: pointer},
		{name: "null", input: `null`, wantSet: true, wantValue: nil},
		{name: "empty string", input: `""`, wantSet: true, wantValue: stringPointer("")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var optional OptionalString
			if err := json.Unmarshal([]byte(test.input), &optional); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if optional.Set != test.wantSet {
				t.Errorf("Set = %t, want %t", optional.Set, test.wantSet)
			}
			if (optional.Value == nil) != (test.wantValue == nil) {
				t.Errorf("Value = %v, want %v", optional.Value, test.wantValue)
			}
			if optional.Value != nil && test.wantValue != nil && *optional.Value != *test.wantValue {
				t.Errorf("Value = %q, want %q", *optional.Value, *test.wantValue)
			}
		})
	}
}

func TestOptionalStringOmitted(t *testing.T) {
	var request struct {
		Value OptionalString `json:"value"`
	}

	if err := json.Unmarshal([]byte(`{}`), &request); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if request.Value.Set {
		t.Error("omitted field was marked as set")
	}
}

func stringPointer(value string) *string {
	return &value
}
