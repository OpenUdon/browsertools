// Package profiledocument provides shared strict decoding and disclosure
// checks for portable profile documents.
package profiledocument

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenUdon/evidence/redact"
	"gopkg.in/yaml.v3"
)

var (
	emailValuePattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phoneValuePattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
)

// DecodeAndReject decodes exactly one JSON/YAML document, normalizes it to
// JSON-compatible values, and rejects secret- or PII-shaped keys and values.
func DecodeAndReject(data []byte, label string) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", label, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse %s: multiple YAML documents are not supported", label)
		}
		return nil, fmt.Errorf("parse %s trailing document: %w", label, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	if err := rejectSensitiveValues(normalized, "$", label); err != nil {
		return nil, err
	}
	return normalized, nil
}

func rejectSensitiveValues(value any, path, label string) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if redact.String(key) != key || emailValuePattern.MatchString(key) || phoneValuePattern.MatchString(key) {
				return fmt.Errorf("%s contains a secret- or PII-shaped key at %s", label, path)
			}
			if err := rejectSensitiveValues(typed[key], path+"."+key, label); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := rejectSensitiveValues(item, fmt.Sprintf("%s[%d]", path, i), label); err != nil {
				return err
			}
		}
	case string:
		if redact.String(typed) != typed || strings.Contains(typed, redact.Value) || strings.Contains(typed, "[REDACTED]") {
			return fmt.Errorf("%s contains a secret-shaped value at %s", label, path)
		}
		if emailValuePattern.MatchString(typed) || phoneValuePattern.MatchString(typed) {
			return fmt.Errorf("%s contains a PII-shaped value at %s", label, path)
		}
	}
	return nil
}
