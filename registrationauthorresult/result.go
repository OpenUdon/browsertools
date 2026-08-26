// Package registrationauthorresult builds and verifies the private,
// deterministic result of one no-submit registration-profile authoring
// session. It records inert reviewed source and value-free provenance only.
package registrationauthorresult

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/internal/strictjson"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/browsertools/registrationreview"
	"github.com/OpenUdon/uws/browserregistration"
)

const (
	SchemaV1 = "browsertools.registration-authoring.v1"
	SchemaV2 = "browsertools.registration-authoring.v2"
	// Schema is the immutable legacy default.
	Schema = SchemaV1
	// MaxResultBytes bounds strict decoding and private persistence.
	MaxResultBytes = 2 << 20
	maxResultDepth = 32
)

const (
	producerName              = "browsertools"
	approvalSymbol            = "registration_approval"
	duplicatePrevention       = "operator_attestation"
	onDuplicate               = "fail"
	ambiguousOutcome          = "stop_without_retry"
	cleanupDelete             = "delete_separately"
	cleanupRetain             = "retain_dedicated_test_identity"
	registrationProfileSchema = "uws.browser-registration.1.0"
)

// Provenance identifies the exact producer contracts without naming a
// binary, worktree, private path, browser handle, or account.
type Provenance struct {
	Producer       string `json:"producer"`
	ResultVersion  string `json:"resultVersion"`
	SessionVersion string `json:"sessionVersion"`
}

// Candidate binds the canonical BRP source and existing independent review.
type Candidate struct {
	ProfileID    string                    `json:"profileId"`
	Schema       string                    `json:"schema"`
	SourceDigest string                    `json:"sourceDigest"`
	Source       json.RawMessage           `json:"source"`
	ReviewDigest string                    `json:"reviewDigest"`
	Review       registrationreview.Bundle `json:"review"`
}

// CredentialSlot is a symbolic inventory entry. It never carries a binding
// or runtime value.
type CredentialSlot struct {
	Slot string `json:"slot"`
	Kind string `json:"kind"`
}

// SubmitReview binds the one inert submit description to an exact reviewed
// current-generation reduced candidate and records that it was not executed.
type SubmitReview struct {
	SequenceIndex int                         `json:"sequenceIndex"`
	Locator       browserregistration.Locator `json:"locator"`
	CandidateID   string                      `json:"candidateId"`
	Generation    int                         `json:"generation"`
	Executed      bool                        `json:"executed"`
}

// CheckpointReview is one value-free human-checkpoint description.
type CheckpointReview struct {
	SequenceIndex int                          `json:"sequenceIndex"`
	Kind          string                       `json:"kind"`
	Locator       *browserregistration.Locator `json:"locator,omitempty"`
}

// FlowReview records the selected inert registration alternative.
type FlowReview struct {
	Name                 string                               `json:"name"`
	CredentialSlots      []CredentialSlot                     `json:"credentialSlots"`
	Effects              []string                             `json:"effects"`
	ConfirmationRequired bool                                 `json:"confirmationRequired"`
	Submit               SubmitReview                         `json:"submit"`
	Checkpoints          []CheckpointReview                   `json:"checkpoints"`
	Success              browserregistration.SuccessCondition `json:"success"`
}

// CallPolicy records only fixed or explicitly selected value-free call
// controls. It is not approval and cannot invoke a runtime.
type CallPolicy struct {
	ApprovalSymbol      string `json:"approvalSymbol"`
	DuplicatePrevention string `json:"duplicatePrevention"`
	OnDuplicate         string `json:"onDuplicate"`
	AmbiguousOutcome    string `json:"ambiguousOutcome"`
	CleanupDisposition  string `json:"cleanupDisposition"`
}

// NetworkPosture proves the authority and accounting asserted by the closed
// session backend. No mutation method exists in that backend interface.
type NetworkPosture struct {
	Methods            []string `json:"methods"`
	Requests           int      `json:"requests"`
	GETRequests        int      `json:"getRequests"`
	HEADRequests       int      `json:"headRequests"`
	MutationRequests   int      `json:"mutationRequests"`
	SubmitExecuted     bool     `json:"submitExecuted"`
	AccountAttempted   bool     `json:"accountAttempted"`
	SessionEstablished bool     `json:"sessionEstablished"`
	RuntimeSupported   bool     `json:"runtimeSupported"`
}

// Envelope is one private, non-publishable registration-authoring result.
type Envelope struct {
	Schema             string                                        `json:"schema"`
	Provenance         Provenance                                    `json:"provenance"`
	CreatedAt          string                                        `json:"createdAt"`
	ObservedAt         string                                        `json:"observedAt"`
	ExpiresAt          string                                        `json:"expiresAt"`
	Origins            []string                                      `json:"origins"`
	Candidate          Candidate                                     `json:"candidate"`
	ReviewedCandidates []registrationauthorsession.ReviewedCandidate `json:"reviewedCandidates"`
	Flow               FlowReview                                    `json:"flow"`
	CallPolicy         CallPolicy                                    `json:"callPolicy"`
	Bounds             registrationauthorsession.Bounds              `json:"bounds"`
	Observations       int                                           `json:"observations"`
	Network            NetworkPosture                                `json:"network"`
	Diagnostics        []string                                      `json:"diagnostics"`
}

// BuildRequest contains one clean session completion and the deterministic
// result creation instant. Flow and cleanup were selected on the session's
// reviewed transition and cannot be replaced here.
type BuildRequest struct {
	Completion *registrationauthorsession.Completion
	CreatedAt  time.Time
}

// Written identifies a newly created private result in-process. Callers must
// not copy Path into protocol output, logs, packages, reports, or goal state.
type Written struct {
	Path   string
	Digest string
}

// FinalizeRequest contains the complete clean session handoff and the one
// explicit private persistence boundary. AssessmentAt is used for the
// independent post-build and post-write lifecycle checks.
type FinalizeRequest struct {
	Completion   *registrationauthorsession.Completion
	CreatedAt    time.Time
	AssessmentAt time.Time
	PrivateRoot  string
}

// Finalized returns the independently reconstructed result and its in-process
// private location. Path must never cross a protocol, log, package, or report.
type Finalized struct {
	Result  *Envelope
	Written Written
}

// Build constructs one deterministic, reviewed BRP result without browser or
// runtime access.
func Build(request BuildRequest) (*Envelope, error) {
	completion := request.Completion
	if completion == nil || completion.Protocol != registrationauthorsession.ProtocolV1 && completion.Protocol != registrationauthorsession.ProtocolV2 {
		return nil, errors.New("no-submit registration completion is required")
	}
	resultSchema := SchemaV1
	if completion.Protocol == registrationauthorsession.ProtocolV2 {
		resultSchema = SchemaV2
	}
	createdAt, err := canonicalTime(request.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("result creation time is invalid")
	}
	observedAt, err := canonicalTime(completion.ObservedAt)
	if err != nil || createdAt.Before(observedAt) {
		return nil, errors.New("result observation time is invalid")
	}
	if !identifierPattern.MatchString(completion.ProfileID) || !identifierPattern.MatchString(completion.Flow) {
		return nil, errors.New("result profile or flow identity is invalid")
	}
	if completion.CleanupDisposition != cleanupDelete && completion.CleanupDisposition != cleanupRetain {
		return nil, errors.New("result cleanup disposition is invalid")
	}
	canonicalProfile, err := registrationprofile.MarshalJSON(&completion.Profile)
	if err != nil || !bytes.Equal(canonicalProfile, completion.ProfileBytes) {
		return nil, errors.New("result profile source is not exact canonical UWS source")
	}
	if err := registrationprofile.ValidateAt(&completion.Profile, createdAt); err != nil {
		return nil, errors.New("result profile is not current")
	}
	if completion.Profile.Profile != registrationProfileSchema || completion.Profile.ObservationKind != "accessibility_snapshot" {
		return nil, errors.New("result candidate schema or observation kind is unsupported")
	}
	origins := registrationprofile.Origins(&completion.Profile)
	if !equalStrings(origins, completion.Origins) || len(origins) == 0 {
		return nil, errors.New("result origins do not match the profile")
	}
	if err := validateCompletionBounds(completion); err != nil {
		return nil, err
	}
	flow, ok := completion.Profile.Flows[completion.Flow]
	if !ok {
		return nil, errors.New("reviewed registration flow is missing")
	}
	flowReview, err := buildFlowReview(completion, flow)
	if err != nil {
		return nil, err
	}
	review, err := registrationreview.Build(&completion.Profile, createdAt)
	if err != nil || !review.Promotable || len(review.Gaps) != 0 {
		return nil, errors.New("registration review is not promotable")
	}
	reviewBytes, err := json.Marshal(review)
	if err != nil {
		return nil, errors.New("registration review cannot be encoded")
	}
	expiresAt, err := registrationprofile.ExpiresAt(&completion.Profile)
	if err != nil {
		return nil, errors.New("registration profile expiry is invalid")
	}
	diagnostics, err := canonicalDiagnostics(completion.Diagnostics)
	if err != nil {
		return nil, err
	}
	reviewedCandidates, err := canonicalReviewedCandidates(completion.ReviewedCandidates, completion.Observations, completion.Bounds.MaxCandidates)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Schema: resultSchema,
		Provenance: Provenance{
			Producer: producerName, ResultVersion: resultSchema,
			SessionVersion: completion.Protocol,
		},
		CreatedAt: createdAt.Format(time.RFC3339), ObservedAt: observedAt.Format(time.RFC3339),
		ExpiresAt: expiresAt.Format(time.RFC3339), Origins: append([]string(nil), origins...),
		Candidate: Candidate{
			ProfileID: completion.ProfileID, Schema: registrationProfileSchema,
			SourceDigest: digest(canonicalProfile), Source: append(json.RawMessage(nil), canonicalProfile...),
			ReviewDigest: digest(reviewBytes), Review: *review,
		},
		ReviewedCandidates: reviewedCandidates,
		Flow:               flowReview,
		CallPolicy: CallPolicy{
			ApprovalSymbol: approvalSymbol, DuplicatePrevention: duplicatePrevention,
			OnDuplicate: onDuplicate, AmbiguousOutcome: ambiguousOutcome,
			CleanupDisposition: completion.CleanupDisposition,
		},
		Bounds: completion.Bounds, Observations: completion.Observations,
		Network: NetworkPosture{
			Methods: []string{"GET", "HEAD"}, Requests: completion.Network.Requests,
			GETRequests: completion.Network.GETRequests, HEADRequests: completion.Network.HEADRequests,
			MutationRequests: 0, SubmitExecuted: false, AccountAttempted: false,
			SessionEstablished: false, RuntimeSupported: false,
		},
		Diagnostics: diagnostics,
	}, nil
}

// Verify reconstructs the complete deterministic result from its exact source
// and rejects drift, tampering, stale review, or unsupported claims.
func Verify(value *Envelope, at time.Time) error {
	if value == nil || at.IsZero() {
		return errors.New("registration authoring result and assessment time are required")
	}
	createdAt, err := parseCanonicalTime(value.CreatedAt)
	if err != nil {
		return err
	}
	observedAt, err := parseCanonicalTime(value.ObservedAt)
	if err != nil {
		return err
	}
	profileValue, err := registrationprofile.Parse(value.Candidate.Source)
	if err != nil {
		return errors.New("registration authoring source is invalid")
	}
	protocol := registrationauthorsession.ProtocolV1
	if value.Schema == SchemaV2 && value.Provenance.ResultVersion == SchemaV2 && value.Provenance.SessionVersion == registrationauthorsession.ProtocolV2 {
		protocol = registrationauthorsession.ProtocolV2
	}
	completion := &registrationauthorsession.Completion{
		Protocol:  protocol,
		ProfileID: value.Candidate.ProfileID, Profile: *profileValue,
		ProfileBytes:       append([]byte(nil), value.Candidate.Source...),
		ReviewedCandidates: append([]registrationauthorsession.ReviewedCandidate(nil), value.ReviewedCandidates...),
		Flow:               value.Flow.Name, CleanupDisposition: value.CallPolicy.CleanupDisposition,
		Origins: append([]string(nil), value.Origins...), ObservedAt: observedAt,
		Bounds: value.Bounds, Observations: value.Observations,
		Diagnostics: append([]string(nil), value.Diagnostics...),
		Network: registrationauthorsession.NetworkSummary{
			Requests: value.Network.Requests, GETRequests: value.Network.GETRequests,
			HEADRequests: value.Network.HEADRequests,
		},
	}
	expected, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt})
	if err != nil {
		return err
	}
	want, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	got, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return errors.New("registration authoring result does not match its source and review")
	}
	if err := registrationreview.Verify(&value.Candidate.Review, at); err != nil {
		return errors.New("registration authoring review is stale or invalid")
	}
	return nil
}

// MarshalDeterministic verifies the envelope at its creation instant and
// returns compact, newline-terminated JSON.
func MarshalDeterministic(value *Envelope) ([]byte, error) {
	if value == nil {
		return nil, errors.New("registration authoring result is required")
	}
	createdAt, err := parseCanonicalTime(value.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := Verify(value, createdAt); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > MaxResultBytes {
		return nil, errors.New("registration authoring result exceeds its byte limit")
	}
	return append(data, '\n'), nil
}

// Digest returns the exact digest of deterministic newline-terminated result
// bytes used by OpenUdon transaction provenance.
func Digest(value *Envelope) (string, error) {
	data, err := MarshalDeterministic(value)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// Decode strictly decodes and verifies one complete result at the supplied
// lifecycle assessment instant.
func Decode(data []byte, at time.Time) (*Envelope, error) {
	if err := strictjson.Validate(data, MaxResultBytes, maxResultDepth); err != nil {
		return nil, errors.New("registration authoring result JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Envelope
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("registration authoring result fields are invalid")
	}
	if err := Verify(&result, at); err != nil {
		return nil, err
	}
	return &result, nil
}

// WritePrivateExclusive creates one owner-only result without replacing an
// existing digest-named artifact.
func WritePrivateExclusive(root string, value *Envelope) (*Written, error) {
	privateRoot, err := openPrivateRoot(root)
	if err != nil {
		return nil, err
	}
	defer privateRoot.Close()
	data, err := MarshalDeterministic(value)
	if err != nil {
		return nil, err
	}
	resultDigest := digest(data)
	name := resultName(resultDigest)
	path := filepath.Join(root, name)
	if err := writeExclusive(privateRoot, name, data); err != nil {
		return nil, err
	}
	if err := validateOpenedPrivateRoot(root, privateRoot); err != nil {
		_ = privateRoot.Remove(name)
		return nil, err
	}
	return &Written{Path: path, Digest: resultDigest}, nil
}

// FinalizePrivate performs the complete post-teardown lifecycle: construct,
// strict-decode independently, create once under one anchored private root,
// reopen through that root, and verify exact bytes, digest, and freshness.
func FinalizePrivate(request FinalizeRequest) (_ *Finalized, resultErr error) {
	if request.PrivateRoot == "" || request.AssessmentAt.IsZero() {
		return nil, errors.New("private registration finalization inputs are required")
	}
	result, err := Build(BuildRequest{Completion: request.Completion, CreatedAt: request.CreatedAt})
	if err != nil {
		return nil, err
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		return nil, err
	}
	decoded, err := Decode(data, request.AssessmentAt)
	if err != nil {
		return nil, err
	}
	decodedBytes, err := MarshalDeterministic(decoded)
	if err != nil || !bytes.Equal(data, decodedBytes) {
		return nil, errors.New("registration authoring reconstruction changed canonical bytes")
	}
	resultDigest := digest(data)
	name := resultName(resultDigest)
	privateRoot, err := openPrivateRoot(request.PrivateRoot)
	if err != nil {
		return nil, err
	}
	defer privateRoot.Close()
	created := false
	defer func() {
		if resultErr != nil && created {
			_ = privateRoot.Remove(name)
		}
	}()
	if err := writeExclusive(privateRoot, name, data); err != nil {
		return nil, err
	}
	created = true
	readBack, err := readPrivateExact(privateRoot, name)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(readBack, data) || digest(readBack) != resultDigest {
		return nil, errors.New("private registration result changed after creation")
	}
	verified, err := Decode(readBack, request.AssessmentAt)
	if err != nil {
		return nil, err
	}
	if err := validateOpenedPrivateRoot(request.PrivateRoot, privateRoot); err != nil {
		return nil, err
	}
	return &Finalized{
		Result: verified,
		Written: Written{
			Path: filepath.Join(request.PrivateRoot, name), Digest: resultDigest,
		},
	}, nil
}

func buildFlowReview(completion *registrationauthorsession.Completion, flow browserregistration.Flow) (FlowReview, error) {
	if !flow.ConfirmationPolicy.Required || !containsString(flow.Effects, "creates_account") {
		return FlowReview{}, errors.New("reviewed flow lacks required account-creation confirmation")
	}
	credentialSlots := make([]CredentialSlot, 0, len(completion.Profile.CredentialSlots))
	for slot, declaration := range completion.Profile.CredentialSlots {
		credentialSlots = append(credentialSlots, CredentialSlot{Slot: slot, Kind: declaration.Kind})
	}
	sort.Slice(credentialSlots, func(i, j int) bool { return credentialSlots[i].Slot < credentialSlots[j].Slot })
	checkpoints := []CheckpointReview{}
	submitIndex := -1
	var submit browserregistration.SubmitStep
	for index, step := range flow.Sequence {
		if step.Submit != nil {
			if submitIndex >= 0 {
				return FlowReview{}, errors.New("reviewed flow contains multiple submit descriptions")
			}
			submitIndex, submit = index, *step.Submit
		}
		if step.HumanCheckpoint != nil {
			checkpoint := CheckpointReview{SequenceIndex: index, Kind: step.HumanCheckpoint.Kind}
			if step.HumanCheckpoint.Locator != nil {
				locator := *step.HumanCheckpoint.Locator
				checkpoint.Locator = &locator
			}
			checkpoints = append(checkpoints, checkpoint)
		}
	}
	if submitIndex < 0 || submit.Locator.Text != "" || submit.Locator.Value != "" ||
		submit.Locator.Role == "" || !promotableLabel(submit.Locator.Name) {
		return FlowReview{}, errors.New("reviewed submit description is not accessibility-name portable")
	}
	matching := []registrationauthorsession.ReviewedCandidate{}
	for _, candidate := range completion.ReviewedCandidates {
		if candidate.Role != submit.Locator.Role || candidate.Matches != 1 {
			continue
		}
		if submit.Locator.Name == "" || candidate.Label == submit.Locator.Name {
			matching = append(matching, candidate)
		}
	}
	if len(matching) != 1 {
		return FlowReview{}, errors.New("reviewed submit does not bind one exact current candidate")
	}
	effects := append([]string(nil), flow.Effects...)
	sort.Strings(effects)
	return FlowReview{
		Name: completion.Flow, CredentialSlots: credentialSlots, Effects: effects,
		ConfirmationRequired: true,
		Submit: SubmitReview{
			SequenceIndex: submitIndex, Locator: submit.Locator,
			CandidateID: matching[0].ID, Generation: matching[0].Generation, Executed: false,
		},
		Checkpoints: checkpoints, Success: flow.Success,
	}, nil
}

func canonicalReviewedCandidates(values []registrationauthorsession.ReviewedCandidate, observations, maximum int) ([]registrationauthorsession.ReviewedCandidate, error) {
	if len(values) == 0 || len(values) > maximum || maximum <= 0 {
		return nil, errors.New("reviewed candidate inventory is invalid")
	}
	result := append([]registrationauthorsession.ReviewedCandidate(nil), values...)
	if !sort.SliceIsSorted(result, func(i, j int) bool { return result[i].ID < result[j].ID }) {
		return nil, errors.New("reviewed candidates are not in canonical order")
	}
	seen := map[string]struct{}{}
	for _, candidate := range result {
		if !candidatePattern.MatchString(candidate.ID) || candidate.Generation != observations ||
			candidate.Matches != 1 || !portableRoles[candidate.Role] || !promotableLabel(candidate.Label) {
			return nil, errors.New("reviewed candidate is invalid")
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return nil, errors.New("reviewed candidates are duplicated")
		}
		seen[candidate.ID] = struct{}{}
	}
	return result, nil
}

func validateCompletionBounds(completion *registrationauthorsession.Completion) error {
	bounds := completion.Bounds
	if bounds.NavigationTimeoutMS <= 0 || bounds.NavigationTimeoutMS > time.Minute.Milliseconds() ||
		bounds.TotalTimeoutMS < bounds.NavigationTimeoutMS || bounds.TotalTimeoutMS > (30*time.Minute).Milliseconds() ||
		bounds.MaxRequests <= 0 || bounds.MaxRequests > 4096 ||
		bounds.MaxResponseBytes <= 0 || bounds.MaxResponseBytes > 128<<20 ||
		bounds.MaxObservations <= 0 || bounds.MaxObservations > 256 ||
		bounds.MaxCandidates <= 0 || bounds.MaxCandidates > 512 ||
		completion.Observations <= 0 || completion.Observations > bounds.MaxObservations ||
		completion.Network.Requests < 0 || completion.Network.GETRequests < 0 || completion.Network.HEADRequests < 0 ||
		completion.Network.Requests != completion.Network.GETRequests+completion.Network.HEADRequests || completion.Network.Requests > bounds.MaxRequests {
		return errors.New("registration completion bounds or network summary is invalid")
	}
	return nil
}

func canonicalDiagnostics(values []string) ([]string, error) {
	if len(values) > registrationauthorsession.MaxUniqueDiagnostics || !sort.StringsAreSorted(values) {
		return nil, errors.New("registration diagnostics are not canonical")
	}
	result := make([]string, len(values))
	copy(result, values)
	for index, value := range result {
		if !registrationauthorsession.ValidDiagnostic(value) || index > 0 && result[index-1] == value {
			return nil, errors.New("registration diagnostic is invalid")
		}
	}
	return result, nil
}

func canonicalTime(value time.Time) (time.Time, error) {
	if value.IsZero() || value.Location() != time.UTC || value.Nanosecond() != 0 {
		return time.Time{}, errors.New("time must be whole-second UTC")
	}
	return value, nil
}

func parseCanonicalTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.Format(time.RFC3339) != value || parsed.Location() != time.UTC || parsed.Nanosecond() != 0 {
		return time.Time{}, errors.New("registration authoring time is not canonical UTC")
	}
	return parsed, nil
}

func openPrivateRoot(root string) (*os.Root, error) {
	if !filepath.IsAbs(root) || strings.TrimSpace(root) != root {
		return nil, errors.New("private result root must be an absolute path")
	}
	before, err := os.Lstat(root)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 || before.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private result root must be an existing non-symlink owner-only directory")
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, errors.New("open private result root")
	}
	anchored, anchorErr := opened.Stat(".")
	after, pathErr := os.Lstat(root)
	if anchorErr != nil || pathErr != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 ||
		after.Mode().Perm()&0o077 != 0 || !os.SameFile(before, anchored) || !os.SameFile(anchored, after) {
		_ = opened.Close()
		return nil, errors.New("private result root changed during validation")
	}
	return opened, nil
}

func validateOpenedPrivateRoot(path string, opened *os.Root) error {
	if opened == nil {
		return errors.New("private result root is unavailable")
	}
	anchored, anchorErr := opened.Stat(".")
	current, pathErr := os.Lstat(path)
	if anchorErr != nil || pathErr != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 ||
		current.Mode().Perm()&0o077 != 0 || !os.SameFile(anchored, current) {
		return errors.New("private result root changed during finalization")
	}
	return nil
}

func readPrivateExact(root *os.Root, name string) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		before.Mode().Perm() != 0o600 || before.Size() <= 0 || before.Size() > MaxResultBytes {
		return nil, errors.New("private registration result identity is invalid")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, errors.New("open private registration result")
	}
	defer file.Close()
	opened, statErr := file.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 || !os.SameFile(before, opened) {
		return nil, errors.New("private registration result changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxResultBytes+1))
	if err != nil || len(data) == 0 || len(data) > MaxResultBytes {
		return nil, errors.New("read private registration result")
	}
	after, pathErr := root.Lstat(name)
	if pathErr != nil || !after.Mode().IsRegular() || after.Mode().Perm() != 0o600 ||
		!os.SameFile(opened, after) || after.Size() != int64(len(data)) {
		return nil, errors.New("private registration result changed during read")
	}
	return data, nil
}

func writeExclusive(root *os.Root, name string, data []byte) (resultErr error) {
	temporaryName := ".browsertools-registration-author-" + strings.ToLower(rand.Text()) + ".tmp"
	temporary, err := root.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create private result temporary file")
	}
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = root.Remove(temporaryName)
	}()
	written, err := io.Copy(temporary, bytes.NewReader(data))
	if err != nil || written != int64(len(data)) {
		return errors.New("write private result temporary file")
	}
	if err := temporary.Sync(); err != nil {
		return errors.New("sync private result temporary file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close private result temporary file")
	}
	closed = true
	if err := root.Link(temporaryName, name); errors.Is(err, os.ErrExist) {
		return errors.New("refusing to overwrite private registration result")
	} else if err != nil {
		return errors.New("publish private registration result")
	}
	return nil
}

func resultName(resultDigest string) string {
	return "registration-authoring-" + strings.TrimPrefix(resultDigest, "sha256:")[:16] + ".json"
}

func promotableLabel(value string) bool {
	return value != "" && value != authorsession.RedactedLabel && value != authorsession.UntrustedLabel &&
		len(value) <= 256 && authorsession.ReduceAccessibilityLabel(value).Value == value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	candidatePattern  = regexp.MustCompile(`^candidate-[a-f0-9]{16}$`)
	portableRoles     = map[string]bool{
		"button": true, "link": true, "textbox": true, "checkbox": true, "radio": true,
		"dialog": true, "status": true, "alert": true, "heading": true, "img": true,
		"list": true, "listitem": true, "combobox": true, "option": true, "menu": true,
		"menuitem": true, "tab": true, "tabpanel": true, "table": true, "row": true,
		"cell": true, "region": true, "navigation": true, "article": true, "form": true,
		"search": true, "switch": true, "group": true,
	}
)
