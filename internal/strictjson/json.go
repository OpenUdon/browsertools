// Package strictjson validates one bounded JSON value before typed decoding.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// Validate rejects empty, oversized, non-UTF-8, duplicate-name, deeply
// nested, and trailing JSON before a caller performs typed decoding.
func Validate(data []byte, maximumBytes, maximumDepth int) error {
	if len(data) == 0 {
		return errors.New("JSON value is empty")
	}
	if maximumBytes <= 0 || len(data) > maximumBytes {
		return fmt.Errorf("JSON value exceeds %d bytes", maximumBytes)
	}
	if maximumDepth <= 0 {
		return errors.New("JSON depth limit is invalid")
	}
	if !utf8.Valid(data) {
		return errors.New("JSON value must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanValue(decoder, 0, maximumDepth); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			_ = token
			return errors.New("JSON value contains trailing data")
		}
		return err
	}
	return nil
}

func scanValue(decoder *json.Decoder, depth, maximumDepth int) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= maximumDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", maximumDepth)
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("JSON object member name is not a string")
			}
			if _, duplicate := seen[name]; duplicate {
				return errors.New("JSON object contains a duplicate field")
			}
			seen[name] = struct{}{}
			if err := scanValue(decoder, depth+1, maximumDepth); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder, depth+1, maximumDepth); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
