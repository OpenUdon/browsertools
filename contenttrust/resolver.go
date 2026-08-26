// Package contenttrust supplies UWS content-trust contracts for validated
// browser profiles. It performs no browser execution and observes no runtime
// values.
package contenttrust

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenUdon/browsertools/profile"
	uwstrust "github.com/OpenUdon/uws/contenttrust"
	"github.com/OpenUdon/uws/uws1"
)

var placeholderPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_-]{1,128})\}\}`)

// Resolver describes browser-profile operation channels and outputs to the UWS
// advisory content-trust analyzer. Profiles are validated and cloned when the
// resolver is constructed, so later caller mutations cannot change a report.
type Resolver struct {
	profiles map[string]*profile.Profile
}

var _ uwstrust.Resolver = (*Resolver)(nil)

// NewResolver constructs a deterministic resolver from profiles keyed by UWS
// sourceDescription name.
func NewResolver(profiles map[string]*profile.Profile) (*Resolver, error) {
	resolver := &Resolver{profiles: make(map[string]*profile.Profile, len(profiles))}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("browser content-trust resolver: source description name is required")
		}
		cloned, err := profile.CloneValidated(profiles[name])
		if err != nil {
			return nil, fmt.Errorf("browser content-trust resolver: validate profile for source %q: %w", name, err)
		}
		resolver.profiles[name] = cloned
	}
	return resolver, nil
}

// ResolveOperation implements contenttrust.Resolver. Browser-derived outputs
// default to untrusted; the value capability is derived from the reviewed
// profile output shape.
func (r *Resolver) ResolveOperation(ctx context.Context, doc *uws1.Document, operation *uws1.Operation) (bool, uwstrust.OperationContract, error) {
	if err := ctx.Err(); err != nil {
		return false, uwstrust.OperationContract{}, err
	}
	if r == nil {
		return false, uwstrust.OperationContract{}, fmt.Errorf("browser content-trust resolver is nil")
	}
	if doc == nil || operation == nil {
		return false, uwstrust.OperationContract{}, fmt.Errorf("browser content-trust resolver requires a document and operation")
	}
	source := findSourceDescription(doc, operation.SourceDescription)
	if source == nil || source.Type != uws1.SourceDescriptionTypeBrowserProfile {
		return false, uwstrust.OperationContract{}, nil
	}
	browserProfile, ok := r.profiles[source.Name]
	if !ok {
		return true, uwstrust.OperationContract{}, fmt.Errorf("browser content-trust resolver has no profile for source %q", source.Name)
	}
	actionName, err := selectedActionName(operation)
	if err != nil {
		return true, uwstrust.OperationContract{}, err
	}
	action, ok := browserProfile.Actions[actionName]
	if !ok {
		return true, uwstrust.OperationContract{}, fmt.Errorf("browser content-trust resolver cannot resolve action %q", actionName)
	}

	inputs, err := inputChannels(operation, action)
	if err != nil {
		return true, uwstrust.OperationContract{}, err
	}
	outputs := make(map[string]uwstrust.ValueContract, len(operation.Outputs))
	for _, name := range sortedOperationOutputNames(operation.Outputs) {
		output, ok := selectedProfileOutput(action.Outputs, operation.Outputs[name])
		if ok {
			outputs[name] = uwstrust.ValueContract{Capability: outputCapability(output)}
			continue
		}
		if operation.Outputs[name] == "$response.body" {
			outputs[name] = uwstrust.ValueContract{Capability: uwstrust.CapabilityComposite}
		}
	}
	return true, uwstrust.OperationContract{
		Inputs:       inputs,
		Outputs:      outputs,
		DefaultTrust: uws1.ContentTrustUntrusted,
	}, nil
}

func findSourceDescription(doc *uws1.Document, name string) *uws1.SourceDescription {
	if name == "" {
		return nil
	}
	for _, source := range doc.SourceDescriptions {
		if source != nil && source.Name == name {
			return source
		}
	}
	return nil
}

func selectedActionName(operation *uws1.Operation) (string, error) {
	if operation.SourceOperationID != "" {
		return operation.SourceOperationID, nil
	}
	const prefix = "#/actions/"
	if !strings.HasPrefix(operation.SourceOperationRef, prefix) {
		return "", fmt.Errorf("browser content-trust resolver requires sourceOperationId or an #/actions reference")
	}
	token := strings.TrimPrefix(operation.SourceOperationRef, prefix)
	if token == "" || strings.Contains(token, "/") {
		return "", fmt.Errorf("browser content-trust resolver has an invalid action reference")
	}
	decoded, ok := decodePointerToken(token)
	if !ok || decoded == "" {
		return "", fmt.Errorf("browser content-trust resolver has an invalid action reference")
	}
	return decoded, nil
}

func decodePointerToken(token string) (string, bool) {
	var result strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] != '~' {
			result.WriteByte(token[i])
			continue
		}
		if i+1 >= len(token) {
			return "", false
		}
		switch token[i+1] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", false
		}
		i++
	}
	return result.String(), true
}

func inputChannels(operation *uws1.Operation, action profile.Action) ([]uwstrust.InputChannel, error) {
	body, hasBody := operation.Request["body"]
	if !hasBody {
		body = nil
	}
	properties, objectBody, err := objectPropertyNames(body)
	if err != nil {
		return nil, fmt.Errorf("browser content-trust resolver cannot inspect request body: %w", err)
	}
	usage := placeholderUsage(action)
	declared := actionParameterNames(action.Parameters)
	for name := range usage {
		if !declared[name] {
			return nil, fmt.Errorf("browser content-trust resolver found undeclared action parameter %q", name)
		}
	}

	channels := make([]uwstrust.InputChannel, 0, 1+len(usage))
	if hasBody {
		channels = append(channels, uwstrust.InputChannel{Path: "/request/body", Kind: uwstrust.ChannelData})
	}
	if objectBody {
		for name, kinds := range usage {
			if !properties[name] {
				continue
			}
			for kind := range kinds {
				channels = append(channels, uwstrust.InputChannel{
					Path: "/request/body/" + encodePointerToken(name),
					Kind: kind,
				})
			}
		}
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].Path != channels[j].Path {
			return channels[i].Path < channels[j].Path
		}
		return channels[i].Kind < channels[j].Kind
	})
	return channels, nil
}

func objectPropertyNames(value any) (map[string]bool, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, false, err
	}
	object, ok := normalized.(map[string]any)
	if !ok {
		return nil, false, nil
	}
	properties := make(map[string]bool, len(object))
	for name := range object {
		properties[name] = true
	}
	return properties, true, nil
}

func placeholderUsage(action profile.Action) map[string]map[uwstrust.ChannelKind]bool {
	usage := make(map[string]map[uwstrust.ChannelKind]bool)
	for _, step := range action.Sequence {
		switch step.Kind {
		case profile.StepNavigate:
			addPlaceholderUsage(usage, step.Navigate, uwstrust.ChannelAuthority)
		case profile.StepTypeText:
			if step.TypeText != nil {
				addPlaceholderUsage(usage, step.TypeText.Value, uwstrust.ChannelData)
			}
		case profile.StepSelectOption:
			if step.SelectOption != nil {
				addPlaceholderUsage(usage, step.SelectOption.Value, uwstrust.ChannelAuthority)
			}
		}
	}
	addPlaceholderUsage(usage, action.ConfirmationPolicy.Prompt, uwstrust.ChannelInstruction)
	return usage
}

func addPlaceholderUsage(target map[string]map[uwstrust.ChannelKind]bool, value string, kind uwstrust.ChannelKind) {
	for _, match := range placeholderPattern.FindAllStringSubmatch(value, -1) {
		name := match[1]
		if target[name] == nil {
			target[name] = make(map[uwstrust.ChannelKind]bool)
		}
		target[name][kind] = true
	}
}

func actionParameterNames(schema profile.JSONSchema) map[string]bool {
	names := make(map[string]bool)
	properties, _ := schema["properties"].(map[string]any)
	for name := range properties {
		names[name] = true
	}
	return names
}

func encodePointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

func outputCapability(output profile.Output) uwstrust.ValueCapability {
	if output.Presence != nil && *output.Presence {
		return uwstrust.CapabilityConstrainedScalar
	}
	switch output.Type {
	case profile.OutputString:
		if hasNonemptyStringEnum(output.Validation) {
			return uwstrust.CapabilityConstrainedScalar
		}
		return uwstrust.CapabilityFreeText
	case profile.OutputInteger, profile.OutputNumber, profile.OutputBoolean, profile.OutputNull:
		return uwstrust.CapabilityConstrainedScalar
	case profile.OutputArray, profile.OutputObject:
		return uwstrust.CapabilityComposite
	default:
		return uwstrust.CapabilityUnknown
	}
}

func selectedProfileOutput(outputs map[string]profile.Output, expression string) (profile.Output, bool) {
	const dotPrefix = "$response.body."
	if strings.HasPrefix(expression, dotPrefix) {
		name := strings.TrimPrefix(expression, dotPrefix)
		output, ok := outputs[name]
		return output, ok
	}
	const pointerPrefix = "$response.body#/"
	if strings.HasPrefix(expression, pointerPrefix) {
		token := strings.TrimPrefix(expression, pointerPrefix)
		if token == "" || strings.Contains(token, "/") {
			return profile.Output{}, false
		}
		name, ok := decodePointerToken(token)
		if !ok {
			return profile.Output{}, false
		}
		output, ok := outputs[name]
		return output, ok
	}
	return profile.Output{}, false
}

func hasNonemptyStringEnum(schema profile.JSONSchema) bool {
	values, ok := schema["enum"].([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func sortedOperationOutputNames(outputs map[string]string) []string {
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
