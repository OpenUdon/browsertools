// Package secretwalk provides one configurable recursive boundary for
// secret-shaped strings and sensitive object keys.
package secretwalk

import "github.com/OpenUdon/evidence/redact"

// Config controls optional key classification and permitted reference values.
type Config struct {
	CheckKeys    bool
	SensitiveKey func(string) bool
	IsReference  func(string) bool
}

// Contains reports whether value contains a redacted string or a sensitive
// key whose value is not an explicitly permitted reference.
func Contains(value any, config Config) bool {
	switch typed := value.(type) {
	case string:
		return redact.String(typed) != typed
	case []any:
		for _, item := range typed {
			if Contains(item, config) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if config.CheckKeys && config.SensitiveKey != nil && config.SensitiveKey(key) {
				text, stringValue := item.(string)
				if !stringValue || config.IsReference == nil || !config.IsReference(text) {
					return true
				}
			}
			if Contains(item, config) {
				return true
			}
		}
	}
	return false
}
