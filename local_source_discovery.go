// Package browsertools exposes cross-package browser source discovery. Profile
// construction, registry distribution, and runtime execution remain in their
// dedicated subpackages.
package browsertools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/profile"
	eartifact "github.com/OpenUdon/evidence/artifact"
	"github.com/OpenUdon/evidence/digest"
	"gopkg.in/yaml.v3"
)

const (
	DefaultLocalMaxVisited    = 10_000
	DefaultLocalMaxCandidates = 100
	DefaultLocalMaxBytes      = int64(20 << 20)
)

// LocalSourceKind identifies a validated browser authoring input.
type LocalSourceKind string

const (
	LocalSourceProfile               LocalSourceKind = "browser_profile"
	LocalSourceAuthenticationProfile LocalSourceKind = "browser_authentication_profile"
	LocalSourceBundle                LocalSourceKind = "capability_bundle"
)

// LocalSourceDiscoveryOptions requires caller-selected roots and an explicit
// assessment time. Directory-name conventions affect score only after content
// validation succeeds.
type LocalSourceDiscoveryOptions struct {
	Roots         []string
	At            time.Time
	MaxVisited    int
	MaxCandidates int
	MaxBytes      int64
}

// LocalSourceCandidate is one exact validated file.
type LocalSourceCandidate struct {
	Kind        LocalSourceKind           `json:"kind"`
	ID          string                    `json:"id,omitempty"`
	Release     string                    `json:"release,omitempty"`
	Title       string                    `json:"title"`
	Origins     []string                  `json:"origins"`
	Actions     []string                  `json:"actions"`
	ActionCount int                       `json:"action_count"`
	Flows       []string                  `json:"flows,omitempty"`
	FlowCount   int                       `json:"flow_count,omitempty"`
	Score       int                       `json:"score"`
	Path        string                    `json:"path"`
	SizeBytes   int64                     `json:"size_bytes"`
	Digest      string                    `json:"digest"`
	Status      eartifact.LifecycleStatus `json:"status"`
	Provenance  string                    `json:"provenance"`
}

// LocalSourceDiagnostic is a safe path-level discovery finding.
type LocalSourceDiagnostic struct {
	Path        string `json:"path"`
	Code        string `json:"code"`
	Detail      string `json:"detail"`
	DuplicateOf string `json:"duplicate_of,omitempty"`
}

// LocalSourceDiscoveryReport exposes all visible discovery outcomes.
type LocalSourceDiscoveryReport struct {
	Roots      []string                `json:"roots"`
	Visited    int                     `json:"visited"`
	Candidates []LocalSourceCandidate  `json:"candidates"`
	Rejected   []LocalSourceDiagnostic `json:"rejected"`
	Ambiguous  []LocalSourceDiagnostic `json:"ambiguous"`
	Truncated  []LocalSourceDiagnostic `json:"truncated"`
}

// DiscoverLocalSources scans only explicit files/directories, never follows a
// symlink, and never contacts a network. Bounds produce visible truncation
// diagnostics and narrowing guidance.
func DiscoverLocalSources(ctx context.Context, options LocalSourceDiscoveryOptions) (LocalSourceDiscoveryReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.At.IsZero() {
		return LocalSourceDiscoveryReport{}, fmt.Errorf("browser source discovery time is required")
	}
	roots, err := normalizeDiscoveryRoots(options.Roots)
	if err != nil {
		return LocalSourceDiscoveryReport{}, err
	}
	report := LocalSourceDiscoveryReport{Roots: roots, Candidates: []LocalSourceCandidate{}, Rejected: []LocalSourceDiagnostic{}, Ambiguous: []LocalSourceDiagnostic{}, Truncated: []LocalSourceDiagnostic{}}
	state := discoveryState{
		ctx: ctx, options: effectiveDiscoveryOptions(options), report: &report,
		byDigest: map[string]int{}, explicitFiles: map[string]bool{},
	}
	for _, root := range roots {
		if err := rejectDiscoverySymlinkComponents(root); err != nil {
			return report, err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return report, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return report, fmt.Errorf("browser source root must not be a symlink: %s", root)
		}
		if info.Mode().IsRegular() {
			state.explicitFiles[root] = true
			if err := state.visitFile(root, info); err != nil {
				if errors.Is(err, errDiscoveryBound) {
					break
				}
				return report, err
			}
		} else if info.IsDir() {
			if err := state.walkRoot(root); err != nil {
				if errors.Is(err, errDiscoveryBound) {
					break
				}
				return report, err
			}
		} else {
			return report, fmt.Errorf("browser source root must be a regular file or directory: %s", root)
		}
	}
	state.sortReport()
	return report, nil
}

var errDiscoveryBound = errors.New("browser source discovery bound reached")

type discoveryState struct {
	ctx           context.Context
	options       LocalSourceDiscoveryOptions
	report        *LocalSourceDiscoveryReport
	byDigest      map[string]int
	explicitFiles map[string]bool
}

func (state *discoveryState) walkRoot(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			state.reject(path, "walk_error", walkErr.Error())
			return nil
		}
		if err := state.ctx.Err(); err != nil {
			return err
		}
		state.report.Visited++
		if state.report.Visited > state.options.MaxVisited {
			state.truncate(path, "visited_limit", fmt.Sprintf("visited-entry limit %d reached; narrow --source-root", state.options.MaxVisited))
			return errDiscoveryBound
		}
		info, err := entry.Info()
		if err != nil {
			state.reject(path, "stat_error", err.Error())
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			state.reject(path, "symlink", "symlinks are not scanned")
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			state.reject(path, "non_regular", "non-regular files are not scanned")
			return nil
		}
		if !supportedDiscoveryExtension(path) {
			return nil
		}
		return state.visitFile(path, info)
	})
}

func (state *discoveryState) visitFile(path string, info os.FileInfo) error {
	if err := state.ctx.Err(); err != nil {
		return err
	}
	if state.explicitFiles[path] {
		state.report.Visited++
		if state.report.Visited > state.options.MaxVisited {
			state.truncate(path, "visited_limit", fmt.Sprintf("visited-entry limit %d reached; narrow the explicit roots", state.options.MaxVisited))
			return errDiscoveryBound
		}
	}
	if !supportedDiscoveryExtension(path) {
		state.reject(path, "unsupported_extension", "browser sources must use .json, .yaml, or .yml")
		return nil
	}
	if info.Size() > state.options.MaxBytes {
		state.reject(path, "oversized", fmt.Sprintf("file exceeds %d bytes", state.options.MaxBytes))
		return nil
	}
	data, err := readDiscoveryFile(state.ctx, path, state.options.MaxBytes)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		state.reject(path, "read_error", err.Error())
		return nil
	}
	candidate, classification, err := classifyBrowserSource(path, data, state.options.At)
	if err != nil {
		state.reject(path, "invalid_"+classification, err.Error())
		return nil
	}
	if classification == "ambiguous" {
		state.ambiguous(path, "ambiguous_document", "JSON/YAML is valid but has no explicit browser profile or capability bundle discriminator")
		return nil
	}
	if classification == "unrelated" {
		if state.explicitFiles[path] {
			state.ambiguous(path, "ambiguous_document", "explicit JSON/YAML has no browser source discriminator")
		} else {
			state.reject(path, "not_browser_source", "document is not a browser profile or capability bundle")
		}
		return nil
	}
	if priorIndex, ok := state.byDigest[candidate.Digest]; ok {
		prior := state.report.Candidates[priorIndex]
		if candidate.Path < prior.Path {
			state.report.Candidates[priorIndex] = candidate
			state.report.Rejected = append(state.report.Rejected, LocalSourceDiagnostic{Path: prior.Path, Code: "duplicate", Detail: "identical content was already discovered", DuplicateOf: candidate.Path})
		} else {
			state.report.Rejected = append(state.report.Rejected, LocalSourceDiagnostic{Path: candidate.Path, Code: "duplicate", Detail: "identical content was already discovered", DuplicateOf: prior.Path})
		}
		return nil
	}
	if len(state.report.Candidates) >= state.options.MaxCandidates {
		state.truncate(path, "candidate_limit", fmt.Sprintf("accepted-candidate limit %d reached; narrow --source-root", state.options.MaxCandidates))
		return errDiscoveryBound
	}
	state.byDigest[candidate.Digest] = len(state.report.Candidates)
	state.report.Candidates = append(state.report.Candidates, candidate)
	return nil
}

func classifyBrowserSource(path string, data []byte, at time.Time) (LocalSourceCandidate, string, error) {
	document, err := decodeDiscoveryDocument(data)
	if err != nil {
		return LocalSourceCandidate{}, "document", err
	}
	object, ok := document.(map[string]any)
	if !ok {
		return LocalSourceCandidate{}, "ambiguous", nil
	}
	bundleDiscriminator, _ := object["version"].(string)
	profileDiscriminator, _ := object["profile"].(string)
	if bundleDiscriminator == bundle.Version && strings.HasPrefix(profileDiscriminator, "uws.browser.") {
		return LocalSourceCandidate{}, "ambiguous", nil
	}
	if bundleDiscriminator == bundle.Version {
		if strings.ToLower(filepath.Ext(path)) != ".json" {
			return LocalSourceCandidate{}, "bundle", fmt.Errorf("capability bundles must use JSON")
		}
		value, err := bundle.Parse(data)
		if err != nil {
			return LocalSourceCandidate{}, "bundle", err
		}
		if err := bundle.Verify(value, at); err != nil {
			return LocalSourceCandidate{}, "bundle", err
		}
		actions := value.Payload.Profile.SortedActionNames()
		return LocalSourceCandidate{
			Kind: LocalSourceBundle, ID: value.Payload.Identity.ID, Release: value.Payload.Identity.Release,
			Title: value.Payload.Identity.Title, Origins: append([]string(nil), value.Payload.Identity.Origins...),
			Actions: actions, ActionCount: len(actions), Score: discoveryScore(LocalSourceBundle, path, value.Payload.Profile.Confidence, len(actions)),
			Path: path, SizeBytes: int64(len(data)), Digest: digest.SHA256String(data),
			Status: eartifact.EffectiveStatus(value.Assessment, at), Provenance: value.Payload.Provenance.Source,
		}, "bundle", nil
	}
	if profileDiscriminator == "uws.browser-authentication.1.0" {
		value, err := authprofile.Parse(data)
		if err != nil {
			return LocalSourceCandidate{}, "authentication_profile", err
		}
		flows := authprofile.SortedFlowNames(value)
		status := eartifact.LifecycleActive
		expires, expiryErr := authprofile.ExpiresAt(value)
		if expiryErr != nil {
			return LocalSourceCandidate{}, "authentication_profile", expiryErr
		}
		if !at.Before(expires) {
			status = eartifact.LifecycleStale
		}
		return LocalSourceCandidate{
			Kind: LocalSourceAuthenticationProfile, Title: value.Info.Title,
			Origins: authprofile.Origins(value), Actions: []string{}, Flows: flows, FlowCount: len(flows),
			Score: discoveryScore(LocalSourceAuthenticationProfile, path, profile.Confidence(value.Confidence), len(flows)),
			Path:  path, SizeBytes: int64(len(data)), Digest: digest.SHA256String(data), Status: status, Provenance: value.Evidence.Source,
		}, "authentication_profile", nil
	}
	if strings.HasPrefix(profileDiscriminator, "uws.browser-authentication.") {
		return LocalSourceCandidate{}, "authentication_profile", fmt.Errorf("unsupported browser authentication profile version %q", profileDiscriminator)
	}
	if strings.HasPrefix(profileDiscriminator, "uws.browser.") {
		var value *profile.Profile
		if strings.ToLower(filepath.Ext(path)) == ".json" {
			value, err = profile.ParseJSON(data)
		} else {
			value, err = profile.ParseYAML(data)
		}
		if err != nil {
			return LocalSourceCandidate{}, "profile", err
		}
		actions := value.SortedActionNames()
		status := eartifact.LifecycleActive
		if expiry := discoveryProfileExpiry(value); !expiry.IsZero() && !at.Before(expiry) {
			status = eartifact.LifecycleStale
		}
		return LocalSourceCandidate{
			Kind: LocalSourceProfile, Title: value.Info.Title, Origins: append([]string(nil), value.Info.Origin...),
			Actions: actions, ActionCount: len(actions), Score: discoveryScore(LocalSourceProfile, path, value.Confidence, len(actions)),
			Path: path, SizeBytes: int64(len(data)), Digest: digest.SHA256String(data), Status: status, Provenance: value.Evidence.Source,
		}, "profile", nil
	}
	if bundleDiscriminator != "" && strings.HasPrefix(bundleDiscriminator, "browsertools.capability-bundle") {
		return LocalSourceCandidate{}, "bundle", fmt.Errorf("unsupported capability bundle version %q", bundleDiscriminator)
	}
	if profileDiscriminator != "" || (object["actions"] != nil && object["info"] != nil) {
		return LocalSourceCandidate{}, "ambiguous", nil
	}
	return LocalSourceCandidate{}, "unrelated", nil
}

func decodeDiscoveryDocument(data []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple documents are not supported")
		}
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func readDiscoveryFile(ctx context.Context, path string, max int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("file changed while opening")
	}
	data, err := io.ReadAll(&discoveryContextReader{ctx: ctx, reader: io.LimitReader(file, max+1)})
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("file exceeds %d bytes", max)
	}
	return data, nil
}

func normalizeDiscoveryRoots(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one explicit browser source root is required")
	}
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("browser source roots must be non-empty")
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, err
		}
		set[filepath.Clean(absolute)] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

func rejectDiscoverySymlinkComponents(path string) error {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(remainder, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("browser source path contains symlink: %s", current)
		}
	}
	return nil
}

func effectiveDiscoveryOptions(value LocalSourceDiscoveryOptions) LocalSourceDiscoveryOptions {
	if value.MaxVisited <= 0 || value.MaxVisited > DefaultLocalMaxVisited {
		value.MaxVisited = DefaultLocalMaxVisited
	}
	if value.MaxCandidates <= 0 || value.MaxCandidates > DefaultLocalMaxCandidates {
		value.MaxCandidates = DefaultLocalMaxCandidates
	}
	if value.MaxBytes <= 0 || value.MaxBytes > DefaultLocalMaxBytes {
		value.MaxBytes = DefaultLocalMaxBytes
	}
	return value
}

func supportedDiscoveryExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func discoveryScore(kind LocalSourceKind, path string, confidence profile.Confidence, actions int) int {
	score := 60
	if kind == LocalSourceBundle {
		score = 100
	} else if kind == LocalSourceAuthenticationProfile {
		score = 70
	}
	switch confidence {
	case profile.ConfidenceHigh:
		score += 20
	case profile.ConfidenceMedium:
		score += 10
	}
	if actions > 10 {
		actions = 10
	}
	score += actions
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, hint := range []string{"/browser-profiles/", "/browser-authentication/", "/capability-bundles/", "/browsertools/"} {
		if strings.Contains(lower, hint) {
			score += 5
			break
		}
	}
	return score
}

func discoveryProfileExpiry(value *profile.Profile) time.Time {
	if value == nil {
		return time.Time{}
	}
	verified, err := time.Parse(time.RFC3339, value.Verification.LastVerifiedAt)
	if err != nil {
		return time.Time{}
	}
	expires, err := value.ExpiresAfter.AddTo(verified)
	if err != nil {
		return time.Time{}
	}
	return expires.UTC().Round(0)
}

func (state *discoveryState) reject(path, code, detail string) {
	state.report.Rejected = append(state.report.Rejected, LocalSourceDiagnostic{Path: path, Code: code, Detail: detail})
}

func (state *discoveryState) ambiguous(path, code, detail string) {
	state.report.Ambiguous = append(state.report.Ambiguous, LocalSourceDiagnostic{Path: path, Code: code, Detail: detail})
}

func (state *discoveryState) truncate(path, code, detail string) {
	state.report.Truncated = append(state.report.Truncated, LocalSourceDiagnostic{Path: path, Code: code, Detail: detail})
}

func (state *discoveryState) sortReport() {
	slices.SortFunc(state.report.Candidates, func(a, b LocalSourceCandidate) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.Path, b.Path)
	})
	compareDiagnostic := func(a, b LocalSourceDiagnostic) int {
		if result := strings.Compare(a.Path, b.Path); result != 0 {
			return result
		}
		return strings.Compare(a.Code, b.Code)
	}
	slices.SortFunc(state.report.Rejected, compareDiagnostic)
	slices.SortFunc(state.report.Ambiguous, compareDiagnostic)
	slices.SortFunc(state.report.Truncated, compareDiagnostic)
}

type discoveryContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *discoveryContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
