// Package optional provides JSON field wrappers that distinguish an omitted
// field from an explicit null.
package optional

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// String records whether a JSON string field was present and supports an
// explicit null value.
type String struct {
	Set   bool
	Value *string
}

// UnmarshalJSON records presence and supports explicit null.
func (o *String) UnmarshalJSON(data []byte) error {
	o.Set = true

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode optional string: %w", err)
	}

	o.Value = &value
	return nil
}
