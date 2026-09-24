package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenUdon/uws/schemas"
)

//go:embed schema/browser.1.5.json
var schemaFS embed.FS

const schemaResource = "schema/browser.1.5.json"

// Accepted browser-profile schema discriminators, oldest first. Drafting emits
// the oldest sufficient version; validation dispatches every accepted version
// to the pinned UWS schema for that exact discriminator.
const (
	SchemaV15 = "uws.browser.1.5"
	SchemaV16 = "uws.browser.1.6"
	SchemaV17 = "uws.browser.1.7"
	SchemaV18 = "uws.browser.1.8"
	SchemaV19 = "uws.browser.1.9"
)

// SupportedSchemas returns a fresh copy of the accepted schema discriminators
// in ascending version order.
func SupportedSchemas() []string {
	return []string{SchemaV15, SchemaV16, SchemaV17, SchemaV18, SchemaV19}
}

// SupportsSchema reports whether a schema discriminator is accepted.
func SupportsSchema(schema string) bool {
	for _, supported := range SupportedSchemas() {
		if schema == supported {
			return true
		}
	}
	return false
}

// SchemaBytesFor returns independent bytes of the pinned UWS JSON Schema for
// one accepted discriminator. An unsupported version fails explicitly rather
// than falling back to another version.
func SchemaBytesFor(schema string) ([]byte, error) {
	if !SupportsSchema(schema) {
		return nil, fmt.Errorf("unsupported browser profile discriminator %q", schema)
	}
	data, err := schemas.BrowserSourceProfileSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("read pinned UWS schema %q: %w", schema, err)
	}
	return data, nil
}

// Issue is a deterministic, path-tagged semantic validation diagnostic.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError groups one or more validation failures.
type ValidationError struct {
	Issues []Issue
	Cause  error
}

func (e *ValidationError) Error() string {
	if len(e.Issues) > 0 {
		return fmt.Sprintf("browser profile validation failed at %s: %s", e.Issues[0].Path, e.Issues[0].Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("browser profile schema validation failed: %v", e.Cause)
	}
	return "browser profile validation failed"
}

// Unwrap exposes the underlying JSON Schema error, when present.
func (e *ValidationError) Unwrap() error { return e.Cause }

// SchemaBytes returns the embedded parity copy of the uws.browser.1.5 JSON
// Schema. Use SchemaBytesFor to read any accepted version from the pinned UWS
// module.
func SchemaBytes() ([]byte, error) { return schemaFS.ReadFile(schemaResource) }

// Validate checks a JSON-compatible browser-profile value against both the
// pinned UWS schema and Browsertools' engine-neutral semantic safety rules.
func Validate(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return &ValidationError{Issues: []Issue{{Code: "invalid_document", Path: "$", Message: "browser profile must be an object"}}}
	}
	discriminator, ok := root["profile"].(string)
	if !ok || strings.TrimSpace(discriminator) == "" {
		return &ValidationError{Issues: []Issue{{Code: "unsupported_profile", Path: "profile", Message: "browser profile discriminator is required"}}}
	}
	if !SupportsSchema(discriminator) {
		return &ValidationError{Issues: []Issue{{Code: "unsupported_profile", Path: "profile", Message: fmt.Sprintf("unsupported browser profile discriminator %q", discriminator)}}}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return &ValidationError{Cause: err}
	}
	if err := schemas.ValidateBrowserSourceProfile(data); err != nil {
		return &ValidationError{Cause: err}
	}
	issues := Check(value)
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// Check returns semantic safety issues not expressed by the JSON Schema. Call
// it after schema validation when the full diagnostic list is needed.
func Check(value any) []Issue {
	data, err := json.Marshal(value)
	if err != nil {
		return []Issue{{Code: "typed_decode", Path: "$", Message: err.Error()}}
	}
	var p Profile
	if err := decodeJSONWithNumbers(data, &p); err != nil {
		return []Issue{{Code: "typed_decode", Path: "$", Message: err.Error()}}
	}
	var issues []Issue
	for _, actionName := range p.SortedActionNames() {
		action := p.Actions[actionName]
		for i, step := range action.Sequence {
			if step.Kind != StepNavigate {
				continue
			}
			path := fmt.Sprintf("actions.%s.sequence[%d].navigate", actionName, i)
			if relative, parseErr := relativeNavigateTarget(step.Navigate); parseErr == nil && relative && len(p.Info.Origin) > 1 {
				issues = append(issues, Issue{
					Code: "ambiguous_relative_origin", Path: path,
					Message: "relative navigate target is ambiguous when info.origin contains more than one origin",
				})
				continue
			}
			target := step.Navigate
			if p.Schema == SchemaV19 {
				// UWS has already checked brace escapes and their placement.
				// For the local origin check, resolve them as URI octets.
				target = strings.ReplaceAll(strings.ReplaceAll(target, "{{{{", "%7B"), "}}}}", "%7D")
			}
			resolved, templateErr := resolveNavigateTemplate(target)
			if templateErr != nil {
				issues = append(issues, Issue{Code: "invalid_navigate_target", Path: path, Message: templateErr.Error()})
				continue
			}
			allowed, checkErr := literalTargetAllowed(resolved, p.Info.Origin)
			if checkErr != nil {
				issues = append(issues, Issue{Code: "invalid_navigate_target", Path: path, Message: checkErr.Error()})
			} else if !allowed {
				issues = append(issues, Issue{
					Code: "origin_rejected", Path: path,
					Message: fmt.Sprintf("navigate target %q is outside info.origin", step.Navigate),
				})
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Path < issues[j].Path
	})
	return issues
}

var navigateTemplatePattern = regexp.MustCompile(`\{\{[A-Za-z0-9_-]{1,128}\}\}`)

// resolveNavigateTemplate substitutes only well-formed path/query/fragment
// placeholders. A placeholder in a scheme or authority is unprovable and is
// rejected even if one sample substitution happens to match an allowed host.
func resolveNavigateTemplate(target string) (string, error) {
	matches := navigateTemplatePattern.FindAllStringIndex(target, -1)
	remainder := navigateTemplatePattern.ReplaceAllString(target, "")
	if strings.ContainsAny(remainder, "{}") {
		return "", fmt.Errorf("navigate target %q contains a malformed template", target)
	}
	authorityStart, authorityEnd := -1, -1
	if index := strings.Index(target, "://"); index >= 0 {
		authorityStart = index + 3
	} else if strings.HasPrefix(target, "//") {
		authorityStart = 2
	}
	if authorityStart >= 0 {
		authorityEnd = len(target)
		if offset := strings.IndexAny(target[authorityStart:], "/?#"); offset >= 0 {
			authorityEnd = authorityStart + offset
		}
	}
	for _, match := range matches {
		if authorityStart >= 0 && match[0] < authorityEnd {
			return "", fmt.Errorf("navigate target %q has a dynamic or unprovable authority", target)
		}
		if authorityStart > 0 && match[0] < authorityStart {
			return "", fmt.Errorf("navigate target %q has a dynamic or unprovable scheme", target)
		}
	}
	return navigateTemplatePattern.ReplaceAllString(target, "browsertools-placeholder"), nil
}

func literalTargetAllowed(target string, origins Origins) (bool, error) {
	if len(origins) == 0 {
		return false, fmt.Errorf("info.origin is empty")
	}
	ref, err := url.Parse(target)
	if err != nil {
		return false, fmt.Errorf("parse navigate target %q: %w", target, err)
	}
	if ref.IsAbs() {
		return origins.ContainsURL(ref.String())
	}
	if len(origins) > 1 {
		return false, fmt.Errorf("relative navigate target is ambiguous when info.origin contains more than one origin")
	}
	base, err := url.Parse(origins[0])
	if err != nil {
		return false, err
	}
	return origins.ContainsURL(base.ResolveReference(ref).String())
}

func relativeNavigateTarget(target string) (bool, error) {
	ref, err := url.Parse(target)
	if err != nil {
		return false, fmt.Errorf("parse navigate target %q: %w", target, err)
	}
	return !ref.IsAbs(), nil
}

// normalizeYAML recursively converts YAML-decoder maps into JSON-compatible
// string-keyed maps.
func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = normalizeYAML(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[fmt.Sprint(key)] = normalizeYAML(val)
		}
		return out
	case []any:
		for i := range typed {
			typed[i] = normalizeYAML(typed[i])
		}
		return typed
	default:
		return typed
	}
}
