package browsertools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/OpenUdon/browsertools/internal/profiledocument"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/uws/binding"
	"github.com/OpenUdon/uws/schemas"
)

// ErrBrowserShapeTable contains no source values or parser excerpts.
var ErrBrowserShapeTable = errors.New("invalid or bounded browser shape-table source contract")

// BrowserShapeSourceInput binds an explicit source ID to exact JSON/YAML bytes.
// URL is inert provenance and is never fetched. Content is required; this API
// performs no file, network, browser, credential or session lookup.
type BrowserShapeSourceInput struct {
	ID      string
	URL     string
	Content []byte
}

// BrowserShapeOptions optionally tightens the fixed ceilings: 32 sources,
// 1 MiB each, 32 MiB aggregate, 10,000 operations, 64 document levels, and the
// UWS 8 MiB table / 256 KiB schema bounds. Zero limits select these defaults.
// Cancellation is cooperative; hard CPU/RSS limits require an isolated worker.
type BrowserShapeOptions struct {
	Sources       []BrowserShapeSourceInput
	MaxBytes      int
	MaxOperations int
}

// BuildBrowserShapeTable independently projects complete native declarations.
// SourceSHA256 covers exact input bytes. SelectedSHA256 covers the complete
// selected action/flow under the M51 lossless canonical encoding, never a typed
// subset. Schemas and extraction/condition annotations are retained. The table
// is inert metadata: validation, runtime support and current authority differ.
func BuildBrowserShapeTable(ctx context.Context, options BrowserShapeOptions) (binding.ShapeTable, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return binding.ShapeTable{}, err
	}
	if len(options.Sources) == 0 || len(options.Sources) > 32 || options.MaxBytes < 0 || options.MaxBytes > 1<<20 || options.MaxOperations < 0 || options.MaxOperations > binding.MaxOperations {
		return binding.ShapeTable{}, ErrBrowserShapeTable
	}
	maxBytes, maxOperations := options.MaxBytes, options.MaxOperations
	if maxBytes == 0 {
		maxBytes = 1 << 20
	}
	if maxOperations == 0 {
		maxOperations = binding.MaxOperations
	}
	table := binding.ShapeTable{Version: binding.TableVersion, Sources: []binding.Source{}, Operations: []binding.OperationShape{}}
	ids := map[string]bool{}
	total, tableBytes := 0, 0
	for _, input := range options.Sources {
		if err := ctx.Err(); err != nil {
			return binding.ShapeTable{}, err
		}
		if len(input.Content) == 0 || len(input.Content) > maxBytes || ids[input.ID] {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		total += len(input.Content)
		if total > 32<<20 {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		ids[input.ID] = true
		root, err := decodeBrowserShapeDocument(input.Content)
		if err != nil {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		version, _ := root["profile"].(string)
		kind, inventory := "", "flows"
		encoded, err := canonicalBrowserShapeJSON(root)
		if err != nil {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		switch {
		case profile.SupportsSchema(version):
			kind, inventory = "action", "actions"
			err = profile.Validate(root)
		case version == "uws.browser-authentication.1.0" || version == "uws.browser-authentication.1.1":
			kind = "authentication"
			err = schemas.ValidateBrowserAuthenticationProfile(encoded)
		case version == "uws.browser-registration.1.0" || version == "uws.browser-registration.1.1" || version == "uws.browser-registration.1.2":
			kind = "registration"
			err = schemas.ValidateBrowserRegistrationProfile(encoded)
		default:
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		if kind != "action" && err == nil {
			// Match existing Browsertools recipe disclosure checks, without using
			// their typed/float decoding for any digest or schema projection.
			_, err = profiledocument.DecodeAndReject(encoded, "browser shape source")
		}
		if err != nil {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		source := binding.Source{ID: input.ID, Kind: binding.KindBrowser, SHA256: browserShapeDigest(input.Content), URL: input.URL}
		if (binding.ShapeTable{Version: binding.TableVersion, Sources: []binding.Source{source}}).Validate() != nil {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		leaves, _ := root[inventory].(map[string]any)
		if len(leaves) == 0 || len(leaves) > maxOperations-len(table.Operations) {
			return binding.ShapeTable{}, ErrBrowserShapeTable
		}
		table.Sources = append(table.Sources, source)
		for _, key := range browserShapeKeys(leaves) {
			if err := ctx.Err(); err != nil {
				return binding.ShapeTable{}, err
			}
			op, err := browserOperationShape(root, leaves[key].(map[string]any), key, version, kind, source)
			if err != nil {
				return binding.ShapeTable{}, ErrBrowserShapeTable
			}
			wire, err := json.Marshal(op)
			tableBytes += len(wire)
			if err != nil || tableBytes > binding.MaxTableBytes {
				return binding.ShapeTable{}, ErrBrowserShapeTable
			}
			table.Operations = append(table.Operations, op)
		}
	}
	data, err := table.Marshal()
	if err != nil {
		return binding.ShapeTable{}, ErrBrowserShapeTable
	}
	if err := ctx.Err(); err != nil {
		return binding.ShapeTable{}, err
	}
	return binding.ParseTable(data)
}

// VerifyBrowserShapeTable reproduces the complete claimed table from the exact
// independently supplied source set. Structural validity and a matching source
// hash alone do not attest its operations. It grants no execution authority.
func VerifyBrowserShapeTable(ctx context.Context, options BrowserShapeOptions, claimed binding.ShapeTable) error {
	if err := browserShapeClaimBudget(claimed); err != nil {
		return err
	}
	expected, err := BuildBrowserShapeTable(ctx, options)
	if err != nil {
		return err
	}
	actualJSON, err := claimed.Marshal()
	if err != nil {
		return ErrBrowserShapeTable
	}
	expectedJSON, err := expected.Marshal()
	if err != nil {
		return ErrBrowserShapeTable
	}
	actual, err := decodeBrowserShapeDocument(actualJSON)
	if err != nil {
		return ErrBrowserShapeTable
	}
	want, err := decodeBrowserShapeDocument(expectedJSON)
	if err != nil {
		return ErrBrowserShapeTable
	}
	a, _ := canonicalBrowserShapeJSON(actual)
	b, _ := canonicalBrowserShapeJSON(want)
	if !bytes.Equal(a, b) {
		return ErrBrowserShapeTable
	}
	if ctx != nil {
		return ctx.Err()
	}
	return nil
}

func browserOperationShape(root, leaf map[string]any, key, version, kind string, source binding.Source) (binding.OperationShape, error) {
	selected, err := canonicalBrowserShapeJSON(leaf)
	if err != nil {
		return binding.OperationShape{}, err
	}
	b := &binding.BrowserOperationShape{ProfileVersion: version, CallKind: kind, SelectedSHA256: browserShapeDigest(selected), CredentialSlots: []binding.CredentialSlotShape{}}
	info := root["info"].(map[string]any)
	origins := map[string]bool{}
	for _, field := range []string{"origin", "applicationOrigins", "authenticationOrigins", "registrationOrigins"} {
		switch value := info[field].(type) {
		case string:
			origins[value] = true
		case []any:
			for _, value := range value {
				origins[value.(string)] = true
			}
		}
	}
	b.Origins = browserShapeKeys(origins)
	effects := "effects"
	if kind == "action" {
		effects = "sideEffects"
		b.AuthenticationRequired, _ = info["loginStateRequired"].(bool)
	}
	b.Effects, err = canonicalBrowserShapeJSON(leaf[effects])
	if err != nil {
		return binding.OperationShape{}, err
	}
	if policy, present := leaf["confirmationPolicy"]; present {
		b.ConfirmationPolicy, err = canonicalBrowserShapeJSON(policy)
		if err != nil {
			return binding.OperationShape{}, err
		}
	}
	if declarations, present := root["credentialSlots"].(map[string]any); present {
		for _, name := range browserShapeKeys(declarations) {
			declaration := declarations[name].(map[string]any)
			b.CredentialSlots = append(b.CredentialSlots, binding.CredentialSlotShape{Name: name, Kind: declaration["kind"].(string), Required: true})
		}
	}
	if declarations, present := root["inputSlots"].(map[string]any); present {
		b.RegistrationInputSlots = []binding.RegistrationInputSlotShape{}
		for _, name := range browserShapeKeys(declarations) {
			declaration := declarations[name].(map[string]any)
			raw, err := canonicalBrowserShapeJSON(declaration)
			if err != nil {
				return binding.OperationShape{}, err
			}
			slot := binding.RegistrationInputSlotShape{Name: name, Kind: declaration["type"].(string), Schema: raw}
			slot.Required, _ = declaration["required"].(bool)
			if condition, present := declaration["requiredWhen"]; present {
				slot.Condition, err = canonicalBrowserShapeJSON(condition)
				if err != nil {
					return binding.OperationShape{}, err
				}
			}
			b.RegistrationInputSlots = append(b.RegistrationInputSlots, slot)
		}
	}
	op := binding.OperationShape{Source: source, Selector: binding.Selector{Kind: "id", Value: key, Key: key}, Protocol: "browser", Complete: true, Browser: b}
	// Credentials and named sessions are separate native browser metadata.
	// An authenticated action cannot be described as anonymous generic security.
	if !b.AuthenticationRequired {
		op.Security = binding.Security{Known: true, Alternatives: []binding.SecurityAlternative{{Requirements: []binding.SecurityRequirement{}}}}
	}
	if kind == "action" {
		if parameters, present := leaf["parameters"]; present {
			raw, err := canonicalBrowserShapeJSON(parameters)
			if err != nil {
				return binding.OperationShape{}, err
			}
			op.Inputs = []binding.Input{{Location: "body", Name: "body", Schema: binding.Schema{Known: true, JSON: raw}}}
		}
		if outputs, present := leaf["outputs"].(map[string]any); present {
			for _, name := range browserShapeKeys(outputs) {
				raw, err := canonicalBrowserShapeJSON(outputs[name])
				if err != nil {
					return binding.OperationShape{}, err
				}
				op.Outputs = append(op.Outputs, binding.Output{Location: "body", Name: name, Schema: binding.Schema{Known: true, JSON: raw}})
			}
		}
	}
	return op, nil
}

func browserShapeKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func browserShapeDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Bound caller-owned raw fragments and slice inventories before serialization
// or native validation can copy them. Escaping overhead is finally bounded by
// the native Marshal contract; this preflight bounds intermediate allocations.
func browserShapeClaimBudget(table binding.ShapeTable) error {
	if len(table.Sources) > binding.MaxSources || len(table.Operations) > binding.MaxOperations {
		return ErrBrowserShapeTable
	}
	budget := 0
	addSize := func(size int) {
		if size > binding.MaxTableBytes-budget {
			budget = binding.MaxTableBytes + 1
		} else {
			budget += size
		}
	}
	add := func(values ...string) {
		for _, value := range values {
			addSize(len(value))
		}
	}
	for _, src := range table.Sources {
		add(src.ID, src.Kind, src.URL, src.SHA256)
	}
	for _, op := range table.Operations {
		if len(op.Inputs) > 2048 || len(op.Outputs) > 2048 || len(op.Servers) > 64 || len(op.Aliases) > 32 || len(op.Security.Alternatives) > 64 {
			return ErrBrowserShapeTable
		}
		add(op.Source.ID, op.Source.Kind, op.Source.SHA256, op.Source.URL, op.Selector.Kind, op.Selector.Key, op.Selector.Value, op.Protocol, op.Method, op.Path)
		for _, alias := range op.Aliases {
			add(alias.Kind, alias.Value, alias.Key)
		}
		add(op.Servers...)
		for _, in := range op.Inputs {
			add(in.Location, in.Name)
			addSize(len(in.Schema.JSON))
		}
		for _, out := range op.Outputs {
			add(out.Location, out.Name)
			addSize(len(out.Schema.JSON))
		}
		for _, alt := range op.Security.Alternatives {
			if len(alt.Requirements) > 64 {
				return ErrBrowserShapeTable
			}
			for _, req := range alt.Requirements {
				if len(req.Scopes) > 128 {
					return ErrBrowserShapeTable
				}
				add(req.Scheme, req.Type, req.Location, req.Name)
				add(req.Scopes...)
			}
		}
		if b := op.Browser; b != nil {
			if len(b.Origins) > 64 || len(b.CredentialSlots) > 128 || len(b.RegistrationInputSlots) > 128 {
				return ErrBrowserShapeTable
			}
			add(b.ProfileVersion, b.CallKind, b.SelectedSHA256)
			addSize(len(b.Effects))
			addSize(len(b.ConfirmationPolicy))
			add(b.Origins...)
			for _, slot := range b.CredentialSlots {
				add(slot.Name, slot.Kind)
			}
			for _, slot := range b.RegistrationInputSlots {
				add(slot.Name, slot.Kind)
				addSize(len(slot.Schema))
				addSize(len(slot.Condition))
			}
		}
		if budget > binding.MaxTableBytes {
			return ErrBrowserShapeTable
		}
	}
	if budget > binding.MaxTableBytes {
		return ErrBrowserShapeTable
	}
	return nil
}
