package capture

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
)

const (
	DefaultRichRetention        = time.Hour
	MaxRichRetention            = 24 * time.Hour
	DefaultMaxRichArtifactBytes = int64(16 << 20)
	MaxRichArtifactBytes        = int64(20 << 20)
	MaxRichTotalArtifactBytes   = int64(19 << 20)
	MaxRichBundleBytes          = int64(20 << 20)
	RichBundleVersion           = "browsertools.private-rich-evidence.v1"
)

// PrivateArtifactKind is a closed rich-evidence opt-in. These artifacts never
// become portable profile fields or publication-eligible cache entries.
type PrivateArtifactKind string

const (
	PrivateArtifactScreenshot PrivateArtifactKind = "screenshot"
	PrivateArtifactTrace      PrivateArtifactKind = "trace"
	PrivateArtifactHAR        PrivateArtifactKind = "har"
)

type PrivateArtifact struct {
	Kind      PrivateArtifactKind
	MediaType string
	Bytes     []byte
}

type RichBackendRequest struct {
	Capture          LiveRequest
	Artifacts        []PrivateArtifactKind
	MaxArtifactBytes int64
}

type RichObservation struct {
	FinalURL  string
	Network   playwrightadapter.NetworkSummary
	Artifacts []PrivateArtifact
}

// RichAcquirer is separate from Acquirer so a generic E03 backend cannot
// accidentally begin retaining rich page material.
type RichAcquirer interface {
	AcquireRich(context.Context, RichBackendRequest) (RichObservation, error)
}

type RichRequest struct {
	Capture          LiveRequest
	Artifacts        []PrivateArtifactKind
	MaxArtifactBytes int64
}

type RichResult struct {
	Origin    string
	Artifacts []PrivateArtifact
}

type RichBundleArtifact struct {
	Kind      PrivateArtifactKind `json:"kind"`
	Name      string              `json:"name"`
	MediaType string              `json:"mediaType"`
	SizeBytes int64               `json:"sizeBytes"`
	Digest    string              `json:"digest"`
}

type RichBundleManifest struct {
	Version    string               `json:"version"`
	Engine     Engine               `json:"engine"`
	CapturedAt string               `json:"capturedAt"`
	Origin     string               `json:"origin"`
	Artifacts  []RichBundleArtifact `json:"artifacts"`
}

// ParsePrivateArtifactKind validates a CLI artifact name without fallback.
func ParsePrivateArtifactKind(raw string) (PrivateArtifactKind, error) {
	kind := PrivateArtifactKind(strings.TrimSpace(raw))
	switch kind {
	case PrivateArtifactScreenshot, PrivateArtifactTrace, PrivateArtifactHAR:
		return kind, nil
	default:
		return "", fmt.Errorf("artifact must be screenshot, trace, or har")
	}
}

// AcquireRich performs one bounded, non-interactive, read-only capture and
// validates all raw artifacts after the ephemeral context has closed.
func AcquireRich(ctx context.Context, acquirer RichAcquirer, request RichRequest) (RichResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if acquirer == nil {
		return RichResult{}, fmt.Errorf("rich capture acquirer is required")
	}
	if len(request.Capture.Probes) != 0 || strings.TrimSpace(request.Capture.ActionHint) != "" {
		return RichResult{}, fmt.Errorf("rich capture does not accept probes or action hints")
	}
	normalized, origin, err := normalizeLiveRequest(request.Capture)
	if err != nil {
		return RichResult{}, fmt.Errorf("rich capture: %w", err)
	}
	kinds, err := normalizePrivateArtifactKinds(request.Artifacts)
	if err != nil {
		return RichResult{}, err
	}
	if request.MaxArtifactBytes == 0 {
		request.MaxArtifactBytes = DefaultMaxRichArtifactBytes
	}
	if request.MaxArtifactBytes < 1 || request.MaxArtifactBytes > MaxRichArtifactBytes {
		return RichResult{}, fmt.Errorf("rich capture max artifact bytes must be between 1 and %d", MaxRichArtifactBytes)
	}
	backendRequest := RichBackendRequest{
		Capture: normalized, Artifacts: kinds, MaxArtifactBytes: request.MaxArtifactBytes,
	}
	ctx, cancel := context.WithTimeout(ctx, normalized.TotalTimeout)
	defer cancel()
	observation, err := acquirer.AcquireRich(ctx, backendRequest)
	if err != nil {
		return RichResult{}, fmt.Errorf("rich capture browser: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return RichResult{}, err
	}
	if err := validateRichObservation(backendRequest, observation); err != nil {
		return RichResult{}, err
	}
	artifacts := make([]PrivateArtifact, len(observation.Artifacts))
	for index, artifact := range observation.Artifacts {
		artifacts[index] = PrivateArtifact{
			Kind: artifact.Kind, MediaType: artifact.MediaType,
			Bytes: append([]byte(nil), artifact.Bytes...),
		}
	}
	return RichResult{Origin: origin, Artifacts: artifacts}, nil
}

func normalizePrivateArtifactKinds(values []PrivateArtifactKind) ([]PrivateArtifactKind, error) {
	if len(values) == 0 || len(values) > 3 {
		return nil, fmt.Errorf("rich capture requires one to three explicit artifacts")
	}
	seen := map[PrivateArtifactKind]struct{}{}
	for _, raw := range values {
		kind, err := ParsePrivateArtifactKind(string(raw))
		if err != nil {
			return nil, fmt.Errorf("rich capture: %w", err)
		}
		if _, duplicate := seen[kind]; duplicate {
			return nil, fmt.Errorf("rich capture artifact %q is duplicated", kind)
		}
		seen[kind] = struct{}{}
	}
	ordered := make([]PrivateArtifactKind, 0, len(seen))
	for _, kind := range []PrivateArtifactKind{PrivateArtifactScreenshot, PrivateArtifactTrace, PrivateArtifactHAR} {
		if _, ok := seen[kind]; ok {
			ordered = append(ordered, kind)
		}
	}
	return ordered, nil
}

func validateRichObservation(request RichBackendRequest, observation RichObservation) error {
	if strings.TrimSpace(observation.FinalURL) == "" {
		return fmt.Errorf("rich capture returned no final URL")
	}
	if _, err := validateCaptureURL(observation.FinalURL); err != nil || !originAllowed(observation.FinalURL, request.Capture.AllowedOrigins) {
		return fmt.Errorf("rich capture final URL violates the exact-origin policy")
	}
	if observation.Network.Requests < 1 || observation.Network.Responses < 0 || observation.Network.Responses > observation.Network.Requests ||
		observation.Network.ResponseBytes < 0 || observation.Network.Requests > request.Capture.MaxRequests ||
		observation.Network.ResponseBytes > request.Capture.MaxResponseBytes || observation.Network.BlockedRequests < 0 ||
		observation.Network.BlockedRequests > observation.Network.Requests || observation.Network.BlockedWebSockets < 0 ||
		observation.Network.BlockedPopups < 0 || observation.Network.BlockedDownloads < 0 ||
		observation.Network.BlockedDialogs < 0 || observation.Network.BlockedFileChoosers < 0 {
		return fmt.Errorf("rich capture backend returned invalid or out-of-bounds network counts")
	}
	if len(observation.Artifacts) != len(request.Artifacts) {
		return fmt.Errorf("rich capture backend returned an incomplete artifact set")
	}
	total := int64(0)
	for index, expected := range request.Artifacts {
		artifact := observation.Artifacts[index]
		if artifact.Kind != expected {
			return fmt.Errorf("rich capture backend returned artifacts out of canonical order")
		}
		if artifact.MediaType != richArtifactMediaType(expected) {
			return fmt.Errorf("rich capture backend returned an invalid media type for %s", expected)
		}
		if len(artifact.Bytes) == 0 || int64(len(artifact.Bytes)) > request.MaxArtifactBytes {
			return fmt.Errorf("rich capture %s artifact is empty or exceeds the byte limit", expected)
		}
		if int64(len(artifact.Bytes)) > MaxRichTotalArtifactBytes-total {
			return fmt.Errorf("rich capture artifacts exceed the total byte limit")
		}
		total += int64(len(artifact.Bytes))
	}
	return nil
}

// MarshalRichBundle packages one already validated capture as a deterministic
// private ZIP. A single cache entry makes storage and exact-ID deletion
// transactional at the artifact-set boundary.
func MarshalRichBundle(result RichResult, engine Engine, capturedAt time.Time) ([]byte, RichBundleManifest, error) {
	if _, err := ParseEngine(string(engine)); err != nil {
		return nil, RichBundleManifest{}, err
	}
	if capturedAt.IsZero() {
		return nil, RichBundleManifest{}, fmt.Errorf("rich bundle capture time is required")
	}
	if _, err := validateCaptureOrigin(result.Origin); err != nil {
		return nil, RichBundleManifest{}, fmt.Errorf("rich bundle origin: %w", err)
	}
	kinds := make([]PrivateArtifactKind, len(result.Artifacts))
	for index, artifact := range result.Artifacts {
		kinds[index] = artifact.Kind
	}
	normalizedKinds, err := normalizePrivateArtifactKinds(kinds)
	if err != nil || !slices.Equal(kinds, normalizedKinds) {
		return nil, RichBundleManifest{}, fmt.Errorf("rich bundle artifacts must be a canonical explicit set")
	}
	manifest := RichBundleManifest{
		Version: RichBundleVersion, Engine: engine,
		CapturedAt: capturedAt.UTC().Format(time.RFC3339Nano), Origin: result.Origin,
		Artifacts: make([]RichBundleArtifact, 0, len(result.Artifacts)),
	}
	total := int64(0)
	for _, artifact := range result.Artifacts {
		if artifact.MediaType != richArtifactMediaType(artifact.Kind) || len(artifact.Bytes) == 0 {
			return nil, RichBundleManifest{}, fmt.Errorf("rich bundle contains an invalid %s artifact", artifact.Kind)
		}
		if int64(len(artifact.Bytes)) > MaxRichTotalArtifactBytes-total {
			return nil, RichBundleManifest{}, fmt.Errorf("rich bundle artifacts exceed the total byte limit")
		}
		total += int64(len(artifact.Bytes))
		digest := sha256.Sum256(artifact.Bytes)
		manifest.Artifacts = append(manifest.Artifacts, RichBundleArtifact{
			Kind: artifact.Kind, Name: richArtifactName(artifact.Kind), MediaType: artifact.MediaType,
			SizeBytes: int64(len(artifact.Bytes)), Digest: "sha256:" + hex.EncodeToString(digest[:]),
		})
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, RichBundleManifest{}, err
	}
	manifestJSON = append(manifestJSON, '\n')
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	write := func(name string, payload []byte) error {
		header := &zip.FileHeader{Name: name, Method: zip.Store, Modified: capturedAt.UTC()}
		header.SetMode(0o600)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = writer.Write(payload)
		return err
	}
	if err := write("manifest.json", manifestJSON); err != nil {
		_ = archive.Close()
		return nil, RichBundleManifest{}, err
	}
	for _, artifact := range result.Artifacts {
		if err := write(richArtifactName(artifact.Kind), artifact.Bytes); err != nil {
			_ = archive.Close()
			return nil, RichBundleManifest{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, RichBundleManifest{}, err
	}
	if int64(buffer.Len()) > MaxRichBundleBytes {
		return nil, RichBundleManifest{}, fmt.Errorf("rich bundle exceeds the byte limit")
	}
	return append([]byte(nil), buffer.Bytes()...), manifest, nil
}

func richArtifactMediaType(kind PrivateArtifactKind) string {
	switch kind {
	case PrivateArtifactScreenshot:
		return "image/png"
	case PrivateArtifactTrace:
		return "application/zip"
	case PrivateArtifactHAR:
		return "application/json"
	default:
		return ""
	}
}

func richArtifactName(kind PrivateArtifactKind) string {
	switch kind {
	case PrivateArtifactScreenshot:
		return "screenshot.png"
	case PrivateArtifactTrace:
		return "trace.zip"
	case PrivateArtifactHAR:
		return "network.har"
	default:
		return ""
	}
}

func hasPrivateArtifact(values []PrivateArtifactKind, kind PrivateArtifactKind) bool {
	return slices.Contains(values, kind)
}
