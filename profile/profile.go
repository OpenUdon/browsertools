// Package profile loads, models, and validates UWS browser-profile documents.
//
// The browser.1.5 and additive browser.1.6 schemas are owned by github.com/OpenUdon/uws. This package
// embeds a parity-checked copy and provides a complete, engine-neutral Go view
// of the portable document. It deliberately contains no browser runtime,
// session, credential, or Playwright behavior.
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// JSONSchema is an inline JSON Schema used by action parameters and output
// validation declarations.
type JSONSchema map[string]any

// Profile is the complete typed view of a uws.browser.1.5 or 1.6 document.
type Profile struct {
	Schema          string             `json:"profile" yaml:"profile"`
	Info            Info               `json:"info" yaml:"info"`
	ObservationKind ObservationKind    `json:"observationKind" yaml:"observationKind"`
	Evidence        Evidence           `json:"evidence" yaml:"evidence"`
	Confidence      Confidence         `json:"confidence" yaml:"confidence"`
	ExpiresAfter    Duration           `json:"expiresAfter" yaml:"expiresAfter"`
	Verification    Verification       `json:"verification" yaml:"verification"`
	Actions         map[string]Action  `json:"actions" yaml:"actions"`
	Contexts        map[string]Context `json:"contexts,omitempty" yaml:"contexts,omitempty"`
}

type Context struct {
	Kind   string `json:"kind" yaml:"kind"`
	Parent string `json:"parent" yaml:"parent"`
	Origin string `json:"origin" yaml:"origin"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
}

// Info is the browser-profile info block.
type Info struct {
	Title              string  `json:"title" yaml:"title"`
	Provider           string  `json:"provider,omitempty" yaml:"provider,omitempty"`
	Origin             Origins `json:"origin" yaml:"origin"`
	LoginStateRequired bool    `json:"loginStateRequired,omitempty" yaml:"loginStateRequired,omitempty"`
}

// Origins is the normalized origin allowlist. A single origin is serialized
// using the schema's scalar form; multiple origins use the array form.
type Origins []string

// ObservationKind identifies how the profile was learned.
type ObservationKind string

const (
	ObservationAccessibilitySnapshot ObservationKind = "accessibility_snapshot"
	ObservationDOMText               ObservationKind = "dom_text"
	ObservationScreenshotOCR         ObservationKind = "screenshot_ocr"
	ObservationOther                 ObservationKind = "other"
)

// Confidence is the reviewed target-stability assessment.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// Evidence records how and when the profile was learned. LearnedAt is the
// profile-level generation timestamp; evidence.Record.ObservedAt is the clock
// for an individual observation.
type Evidence struct {
	LearnedAt string `json:"learnedAt" yaml:"learnedAt"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
}

// Verification records the most recent successful verification.
type Verification struct {
	LastVerifiedAt   string   `json:"lastVerifiedAt" yaml:"lastVerifiedAt"`
	SuccessfulRuns   int      `json:"successfulRuns" yaml:"successfulRuns"`
	UIStabilityScore *float64 `json:"uiStabilityScore,omitempty" yaml:"uiStabilityScore,omitempty"`
}

// Duration is an ISO-8601 duration. AddTo applies calendar components relative
// to a reference time instead of approximating months or years as fixed days.
type Duration string

// Action is a named portable UI capability.
type Action struct {
	Description        string             `json:"description,omitempty" yaml:"description,omitempty"`
	Parameters         JSONSchema         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Sequence           []Step             `json:"sequence" yaml:"sequence"`
	Outputs            map[string]Output  `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	SideEffects        []SideEffect       `json:"sideEffects" yaml:"sideEffects"`
	ConfirmationPolicy ConfirmationPolicy `json:"confirmationPolicy" yaml:"confirmationPolicy"`
}

// SideEffect is a member of the browser.1.5 side-effect vocabulary.
type SideEffect string

const (
	SideEffectReadOnly        SideEffect = "read_only"
	SideEffectStateChange     SideEffect = "state_change"
	SideEffectSendsEmail      SideEffect = "sends_email"
	SideEffectCreatesRecord   SideEffect = "creates_record"
	SideEffectUpdatesRecord   SideEffect = "updates_record"
	SideEffectDeletesResource SideEffect = "deletes_resource"
)

// ConfirmationPolicy controls runtime confirmation for an action.
type ConfirmationPolicy struct {
	Required bool   `json:"required" yaml:"required"`
	Prompt   string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
}

// Role is an accessibility role accepted by browser.1.5.
type Role string

const (
	RoleButton     Role = "button"
	RoleLink       Role = "link"
	RoleTextbox    Role = "textbox"
	RoleCheckbox   Role = "checkbox"
	RoleRadio      Role = "radio"
	RoleDialog     Role = "dialog"
	RoleStatus     Role = "status"
	RoleAlert      Role = "alert"
	RoleHeading    Role = "heading"
	RoleImg        Role = "img"
	RoleList       Role = "list"
	RoleListItem   Role = "listitem"
	RoleCombobox   Role = "combobox"
	RoleOption     Role = "option"
	RoleMenu       Role = "menu"
	RoleMenuItem   Role = "menuitem"
	RoleTab        Role = "tab"
	RoleTabPanel   Role = "tabpanel"
	RoleTable      Role = "table"
	RoleRow        Role = "row"
	RoleCell       Role = "cell"
	RoleRegion     Role = "region"
	RoleNavigation Role = "navigation"
	RoleArticle    Role = "article"
	RoleForm       Role = "form"
	RoleSearch     Role = "search"
	RoleSwitch     Role = "switch"
	RoleGroup      Role = "group"
)

// Locator is an accessibility-only target.
type Locator struct {
	Role  Role   `json:"role" yaml:"role"`
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	Text  string `json:"text,omitempty" yaml:"text,omitempty"`
	Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// StepKind identifies one member of the closed sequence macro vocabulary.
type StepKind string

const (
	StepNavigate     StepKind = "navigate"
	StepClick        StepKind = "click"
	StepTypeText     StepKind = "type_text"
	StepCheckRadio   StepKind = "check_radio"
	StepUncheck      StepKind = "uncheck"
	StepSelectOption StepKind = "select_option"
	StepWaitFor      StepKind = "wait_for"
)

// FallbackReason explains why a CSS output fallback was necessary.
type FallbackReason string

const (
	FallbackNoA11yRegion     FallbackReason = "no_a11y_region"
	FallbackNoStructuredData FallbackReason = "no_structured_data"
	FallbackAmbiguousA11y    FallbackReason = "ambiguous_a11y"
	FallbackOther            FallbackReason = "other"
)

// Step is a strict tagged union. Exactly one payload matching Kind is present.
type Step struct {
	Kind            StepKind          `json:"-" yaml:"-"`
	Navigate        string            `json:"-" yaml:"-"`
	NavigateContext string            `json:"-" yaml:"-"`
	NavigateObject  bool              `json:"-" yaml:"-"`
	Click           *LocatorStep      `json:"-" yaml:"-"`
	TypeText        *TypeTextStep     `json:"-" yaml:"-"`
	CheckRadio      *LocatorStep      `json:"-" yaml:"-"`
	Uncheck         *LocatorStep      `json:"-" yaml:"-"`
	SelectOption    *SelectOptionStep `json:"-" yaml:"-"`
	WaitFor         *WaitForCondition `json:"-" yaml:"-"`
}

// LocatorStep is shared by click, check_radio, and uncheck.
type LocatorStep struct {
	Locator      Locator           `json:"locator" yaml:"locator"`
	WaitFor      *WaitForCondition `json:"wait_for,omitempty" yaml:"wait_for,omitempty"`
	Context      string            `json:"context,omitempty" yaml:"context,omitempty"`
	OpensContext string            `json:"opensContext,omitempty" yaml:"opensContext,omitempty"`
}

// TypeTextStep describes a type_text macro.
type TypeTextStep struct {
	Locator Locator           `json:"locator" yaml:"locator"`
	Value   string            `json:"value" yaml:"value"`
	WaitFor *WaitForCondition `json:"wait_for,omitempty" yaml:"wait_for,omitempty"`
	Context string            `json:"context,omitempty" yaml:"context,omitempty"`
}

// SelectOptionStep describes a select_option macro.
type SelectOptionStep struct {
	Locator Locator           `json:"locator" yaml:"locator"`
	Value   string            `json:"value" yaml:"value"`
	WaitFor *WaitForCondition `json:"wait_for,omitempty" yaml:"wait_for,omitempty"`
	Context string            `json:"context,omitempty" yaml:"context,omitempty"`
}

// NavigationWait is a supported navigation lifecycle event.
type NavigationWait string

const (
	NavigationLoad             NavigationWait = "load"
	NavigationDOMContentLoaded NavigationWait = "domcontentloaded"
	NavigationNetworkIdle      NavigationWait = "network_idle"
)

// WaitForCondition is either an accessibility locator or a navigation event.
type WaitForCondition struct {
	Locator    *Locator        `json:"-" yaml:"-"`
	Navigation *NavigationWait `json:"-" yaml:"-"`
	Context    string          `json:"-" yaml:"-"`
	Contextual bool            `json:"-" yaml:"-"`
}

// OutputType is the declared JSON type of an extracted value.
type OutputType string

const (
	OutputString  OutputType = "string"
	OutputInteger OutputType = "integer"
	OutputNumber  OutputType = "number"
	OutputBoolean OutputType = "boolean"
	OutputArray   OutputType = "array"
	OutputObject  OutputType = "object"
	OutputNull    OutputType = "null"
)

// OutputSource identifies a browser.1.5 extraction source.
type OutputSource string

const (
	OutputA11y      OutputSource = "a11y"
	OutputJSONLD    OutputSource = "jsonld"
	OutputMicrodata OutputSource = "microdata"
	OutputCSS       OutputSource = "css"
)

// Output is a typed output extraction declaration.
type Output struct {
	Type           OutputType     `json:"type" yaml:"type"`
	Source         OutputSource   `json:"source" yaml:"source"`
	Locator        *Locator       `json:"locator,omitempty" yaml:"locator,omitempty"`
	Selector       string         `json:"selector,omitempty" yaml:"selector,omitempty"`
	FallbackReason FallbackReason `json:"fallbackReason,omitempty" yaml:"fallbackReason,omitempty"`
	Validation     JSONSchema     `json:"validation,omitempty" yaml:"validation,omitempty"`
	Presence       *bool          `json:"presence,omitempty" yaml:"presence,omitempty"`
	Property       string         `json:"property,omitempty" yaml:"property,omitempty"`
	Attribute      string         `json:"attribute,omitempty" yaml:"attribute,omitempty"`
	Context        string         `json:"context,omitempty" yaml:"context,omitempty"`
}

// MarshalJSON preserves the distinction between an absent validation schema
// and an explicitly present empty schema. The latter is the JSON Schema true
// schema and must remain {} on the wire.
func (o Output) MarshalJSON() ([]byte, error) {
	type outputWire struct {
		Type           OutputType     `json:"type"`
		Source         OutputSource   `json:"source"`
		Locator        *Locator       `json:"locator,omitempty"`
		Selector       string         `json:"selector,omitempty"`
		FallbackReason FallbackReason `json:"fallbackReason,omitempty"`
		Validation     *JSONSchema    `json:"validation,omitempty"`
		Presence       *bool          `json:"presence,omitempty"`
		Property       string         `json:"property,omitempty"`
		Attribute      string         `json:"attribute,omitempty"`
		Context        string         `json:"context,omitempty"`
	}
	value := outputWire{
		Type: o.Type, Source: o.Source, Locator: o.Locator, Selector: o.Selector,
		FallbackReason: o.FallbackReason, Presence: o.Presence, Property: o.Property,
		Attribute: o.Attribute, Context: o.Context,
	}
	if o.Validation != nil {
		validation := o.Validation
		value.Validation = &validation
	}
	return json.Marshal(value)
}

// MarshalYAML uses the same presence-aware representation as MarshalJSON.
func (o Output) MarshalYAML() (any, error) {
	data, err := o.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

// ParseJSON validates and decodes a JSON browser profile.
func ParseJSON(data []byte) (*Profile, error) {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse JSON profile: %w", err)
	}
	if err := Validate(value); err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode typed JSON profile: %w", err)
	}
	return &p, nil
}

// ParseYAML validates and decodes a YAML browser profile.
func ParseYAML(data []byte) (*Profile, error) {
	var value any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse YAML profile: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse YAML profile: multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("parse YAML profile: trailing document: %w", err)
	}
	value = normalizeYAML(value)
	if err := Validate(value); err != nil {
		return nil, err
	}
	jsonData, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize YAML profile: %w", err)
	}
	var p Profile
	if err := json.Unmarshal(jsonData, &p); err != nil {
		return nil, fmt.Errorf("decode typed YAML profile: %w", err)
	}
	return &p, nil
}

// LoadFile validates and decodes a JSON or YAML browser profile.
func LoadFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var p *Profile
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		p, err = ParseJSON(data)
	case ".yaml", ".yml":
		p, err = ParseYAML(data)
	default:
		return nil, fmt.Errorf("unsupported profile extension %q for %s", filepath.Ext(path), path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Value returns the profile as a JSON-compatible generic value.
func (p Profile) Value() (map[string]any, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// MarshalYAML serializes a typed profile through its canonical JSON union
// representation so Step and WaitForCondition retain their wire shapes.
func MarshalYAML(p Profile) ([]byte, error) {
	value, err := p.Value()
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(value)
}

// ParseOrigin validates and canonicalizes an HTTP(S) origin.
func ParseOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("origin %q must use http or https", raw)
	}
	if u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("origin %q must contain only scheme, host, and optional port", raw)
	}
	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("origin %q has no hostname", raw)
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", fmt.Errorf("origin %q has invalid port: %w", raw, err)
		}
		host = net.JoinHostPort(hostname, port)
	}
	return strings.ToLower(u.Scheme) + "://" + host, nil
}

// OriginOfURL returns the canonical origin of an absolute HTTP(S) URL.
func OriginOfURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		if err == nil {
			err = fmt.Errorf("URL is not absolute")
		}
		return "", fmt.Errorf("parse URL %q: %w", raw, err)
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return ParseOrigin(u.String())
}

// ContainsURL reports whether an absolute URL is covered by the allowlist.
func (o Origins) ContainsURL(raw string) (bool, error) {
	target, err := OriginOfURL(raw)
	if err != nil {
		return false, err
	}
	for _, allowed := range o {
		canonical, err := ParseOrigin(allowed)
		if err != nil {
			return false, err
		}
		if canonical == target {
			return true, nil
		}
	}
	return false, nil
}

// AddTo applies d to reference using calendar-aware year/month/day arithmetic.
func (d Duration) AddTo(reference time.Time) (time.Time, error) {
	parts, err := parseDuration(string(d))
	if err != nil {
		return time.Time{}, err
	}
	result := reference.AddDate(parts.years, parts.months, parts.weeks*7+parts.days)
	result = result.Add(time.Duration(parts.hours)*time.Hour + time.Duration(parts.minutes)*time.Minute + time.Duration(parts.seconds)*time.Second)
	return result, nil
}

type durationParts struct{ years, months, weeks, days, hours, minutes, seconds int }

var durationPattern = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

func parseDuration(raw string) (durationParts, error) {
	var out durationParts
	matches := durationPattern.FindStringSubmatch(raw)
	if matches == nil {
		return out, fmt.Errorf("invalid ISO-8601 duration %q", raw)
	}
	values := []*int{&out.years, &out.months, &out.weeks, &out.days, &out.hours, &out.minutes, &out.seconds}
	parsed := false
	for i, target := range values {
		if matches[i+1] == "" {
			continue
		}
		n, err := strconv.ParseInt(matches[i+1], 10, 32)
		if err != nil {
			return out, fmt.Errorf("invalid ISO-8601 duration %q: %w", raw, err)
		}
		*target = int(n)
		parsed = true
	}
	if !parsed {
		return out, fmt.Errorf("invalid ISO-8601 duration %q", raw)
	}
	return out, nil
}

// UnmarshalJSON accepts the schema's string-or-array origin union.
func (o *Origins) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		canonical, err := ParseOrigin(one)
		if err != nil {
			return err
		}
		*o = Origins{canonical}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	for i := range many {
		canonical, err := ParseOrigin(many[i])
		if err != nil {
			return err
		}
		many[i] = canonical
	}
	*o = Origins(many)
	return nil
}

// MarshalJSON preserves the compact scalar form for one origin.
func (o Origins) MarshalJSON() ([]byte, error) {
	if len(o) == 1 {
		return json.Marshal(o[0])
	}
	return json.Marshal([]string(o))
}

// MarshalYAML preserves the compact scalar form for one origin.
func (o Origins) MarshalYAML() (any, error) {
	if len(o) == 1 {
		return o[0], nil
	}
	return []string(o), nil
}

// UnmarshalYAML accepts the schema's string-or-array origin union.
func (o *Origins) UnmarshalYAML(node *yaml.Node) error {
	var value any
	if err := node.Decode(&value); err != nil {
		return err
	}
	data, err := json.Marshal(normalizeYAML(value))
	if err != nil {
		return err
	}
	return o.UnmarshalJSON(data)
}

// UnmarshalJSON decodes one member of the closed step union.
func (s *Step) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 1 {
		return fmt.Errorf("sequence step must have exactly one macro key")
	}
	*s = Step{}
	for key, payload := range raw {
		s.Kind = StepKind(key)
		switch s.Kind {
		case StepNavigate:
			if err := json.Unmarshal(payload, &s.Navigate); err == nil {
				return nil
			}
			var navigate struct {
				URL     string `json:"url"`
				Context string `json:"context,omitempty"`
			}
			if err := decodeStrictJSON(payload, &navigate); err != nil {
				return err
			}
			s.Navigate, s.NavigateContext, s.NavigateObject = navigate.URL, navigate.Context, true
			return nil
		case StepClick:
			s.Click = &LocatorStep{}
			return decodeStrictJSON(payload, s.Click)
		case StepTypeText:
			s.TypeText = &TypeTextStep{}
			return decodeStrictJSON(payload, s.TypeText)
		case StepCheckRadio:
			s.CheckRadio = &LocatorStep{}
			return decodeStrictJSON(payload, s.CheckRadio)
		case StepUncheck:
			s.Uncheck = &LocatorStep{}
			return decodeStrictJSON(payload, s.Uncheck)
		case StepSelectOption:
			s.SelectOption = &SelectOptionStep{}
			return decodeStrictJSON(payload, s.SelectOption)
		case StepWaitFor:
			s.WaitFor = &WaitForCondition{}
			return decodeStrictJSON(payload, s.WaitFor)
		default:
			return fmt.Errorf("unknown browser.1.5 macro %q", key)
		}
	}
	return nil
}

// MarshalJSON encodes the step's single-key wire form.
func (s Step) MarshalJSON() ([]byte, error) {
	key, payload, err := s.payload()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{key: payload})
}

// MarshalYAML encodes the step's single-key wire form.
func (s Step) MarshalYAML() (any, error) {
	key, payload, err := s.payload()
	if err != nil {
		return nil, err
	}
	return map[string]any{key: payload}, nil
}

// UnmarshalYAML decodes one member of the closed step union.
func (s *Step) UnmarshalYAML(node *yaml.Node) error {
	var value any
	if err := node.Decode(&value); err != nil {
		return err
	}
	data, err := json.Marshal(normalizeYAML(value))
	if err != nil {
		return err
	}
	return s.UnmarshalJSON(data)
}

func (s Step) payload() (string, any, error) {
	switch s.Kind {
	case StepNavigate:
		if s.NavigateObject {
			value := map[string]any{"url": s.Navigate}
			if s.NavigateContext != "" {
				value["context"] = s.NavigateContext
			}
			return string(s.Kind), value, nil
		}
		return string(s.Kind), s.Navigate, nil
	case StepClick:
		if s.Click != nil {
			return string(s.Kind), s.Click, nil
		}
	case StepTypeText:
		if s.TypeText != nil {
			return string(s.Kind), s.TypeText, nil
		}
	case StepCheckRadio:
		if s.CheckRadio != nil {
			return string(s.Kind), s.CheckRadio, nil
		}
	case StepUncheck:
		if s.Uncheck != nil {
			return string(s.Kind), s.Uncheck, nil
		}
	case StepSelectOption:
		if s.SelectOption != nil {
			return string(s.Kind), s.SelectOption, nil
		}
	case StepWaitFor:
		if s.WaitFor != nil {
			return string(s.Kind), s.WaitFor, nil
		}
	}
	return "", nil, fmt.Errorf("step %q has no matching payload", s.Kind)
}

// UnmarshalJSON decodes a locator or navigation wait condition.
func (w *WaitForCondition) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	*w = WaitForCondition{}
	if raw, ok := probe["navigation"]; ok {
		if len(probe) != 1 {
			return fmt.Errorf("navigation wait must contain only navigation")
		}
		var nav NavigationWait
		if err := json.Unmarshal(raw, &nav); err != nil {
			return err
		}
		w.Navigation = &nav
		return nil
	}
	if raw, ok := probe["locator"]; ok {
		if len(probe) != 2 || probe["context"] == nil {
			return fmt.Errorf("contextual locator wait must contain locator and context")
		}
		var loc Locator
		if err := decodeStrictJSON(raw, &loc); err != nil {
			return err
		}
		if err := json.Unmarshal(probe["context"], &w.Context); err != nil {
			return err
		}
		w.Locator = &loc
		w.Contextual = true
		return nil
	}
	var loc Locator
	if err := decodeStrictJSON(data, &loc); err != nil {
		return err
	}
	w.Locator = &loc
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
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

// MarshalJSON encodes the wait condition's union form.
func (w WaitForCondition) MarshalJSON() ([]byte, error) {
	if w.Navigation != nil && w.Locator == nil {
		return json.Marshal(map[string]any{"navigation": *w.Navigation})
	}
	if w.Locator != nil && w.Navigation == nil {
		if w.Contextual {
			return json.Marshal(map[string]any{"locator": w.Locator, "context": w.Context})
		}
		return json.Marshal(w.Locator)
	}
	return nil, fmt.Errorf("wait_for must contain exactly one locator or navigation event")
}

// MarshalYAML encodes the wait condition's union form.
func (w WaitForCondition) MarshalYAML() (any, error) {
	if w.Navigation != nil && w.Locator == nil {
		return map[string]any{"navigation": *w.Navigation}, nil
	}
	if w.Locator != nil && w.Navigation == nil {
		if w.Contextual {
			return map[string]any{"locator": w.Locator, "context": w.Context}, nil
		}
		return w.Locator, nil
	}
	return nil, fmt.Errorf("wait_for must contain exactly one locator or navigation event")
}

// UnmarshalYAML decodes a locator or navigation wait condition.
func (w *WaitForCondition) UnmarshalYAML(node *yaml.Node) error {
	var value any
	if err := node.Decode(&value); err != nil {
		return err
	}
	data, err := json.Marshal(normalizeYAML(value))
	if err != nil {
		return err
	}
	return w.UnmarshalJSON(data)
}

// Locator returns the action locator used by this step, if any.
func (s Step) Locator() *Locator {
	switch s.Kind {
	case StepClick:
		if s.Click != nil {
			return &s.Click.Locator
		}
	case StepTypeText:
		if s.TypeText != nil {
			return &s.TypeText.Locator
		}
	case StepCheckRadio:
		if s.CheckRadio != nil {
			return &s.CheckRadio.Locator
		}
	case StepUncheck:
		if s.Uncheck != nil {
			return &s.Uncheck.Locator
		}
	case StepSelectOption:
		if s.SelectOption != nil {
			return &s.SelectOption.Locator
		}
	case StepWaitFor:
		if s.WaitFor != nil {
			return s.WaitFor.Locator
		}
	}
	return nil
}

// PostWait returns the wait attached to an actionable macro, if any.
func (s Step) PostWait() *WaitForCondition {
	switch s.Kind {
	case StepClick:
		if s.Click != nil {
			return s.Click.WaitFor
		}
	case StepTypeText:
		if s.TypeText != nil {
			return s.TypeText.WaitFor
		}
	case StepCheckRadio:
		if s.CheckRadio != nil {
			return s.CheckRadio.WaitFor
		}
	case StepUncheck:
		if s.Uncheck != nil {
			return s.Uncheck.WaitFor
		}
	case StepSelectOption:
		if s.SelectOption != nil {
			return s.SelectOption.WaitFor
		}
	}
	return nil
}

// SortedActionNames returns action names in deterministic order.
func (p Profile) SortedActionNames() []string {
	keys := make([]string, 0, len(p.Actions))
	for key := range p.Actions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
