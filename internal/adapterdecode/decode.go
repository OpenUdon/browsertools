// Package adapterdecode implements the shared, bounded, strict JSON boundary
// used by saved evidence adapters.
package adapterdecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSON decodes exactly one JSON value, rejecting oversized input, unknown
// struct fields, trailing values, and incomplete input.
func JSON(data []byte, maximum int64, target any) error {
	if maximum <= 0 {
		return fmt.Errorf("fixture byte limit must be positive")
	}
	if int64(len(data)) > maximum {
		return fmt.Errorf("fixture exceeds %d bytes", maximum)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}
