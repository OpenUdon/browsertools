package registrationauthorresult

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

var (
	observedAt = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	createdAt  = time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC)
)

func TestBuildVerifyAndDeterministicDigest(t *testing.T) {
	completion := validCompletion(t)
	result, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != Schema || result.Provenance != (Provenance{Producer: producerName, ResultVersion: Schema, SessionVersion: registrationauthorsession.Protocol}) {
		t.Fatalf("identity = %q %#v", result.Schema, result.Provenance)
	}
	if result.CreatedAt != "2026-08-26T12:01:00Z" || result.ObservedAt != "2026-08-26T12:00:00Z" || result.ExpiresAt != "2026-09-24T00:00:00Z" {
		t.Fatalf("lifecycle = %q %q %q", result.CreatedAt, result.ObservedAt, result.ExpiresAt)
	}
	profileDigest, err := registrationprofile.Digest(&completion.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate.Schema != registrationProfileSchema || result.Candidate.SourceDigest != profileDigest || result.Candidate.Review.ProfileDigest != profileDigest {
		t.Fatalf("candidate = %#v", result.Candidate)
	}
	reviewBytes, err := json.Marshal(result.Candidate.Review)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate.ReviewDigest != testDigest(reviewBytes) {
		t.Fatalf("review digest = %q", result.Candidate.ReviewDigest)
	}
	if result.Flow.Name != "create_dedicated_test_user" || result.Flow.Submit.SequenceIndex != 3 || result.Flow.Submit.CandidateID != completion.ReviewedCandidates[0].ID || result.Flow.Submit.Executed {
		t.Fatalf("flow review = %#v", result.Flow)
	}
	if len(result.Flow.CredentialSlots) != 2 || result.Flow.CredentialSlots[0].Slot != "identifier" || result.Flow.CredentialSlots[1].Slot != "password" {
		t.Fatalf("credential slots = %#v", result.Flow.CredentialSlots)
	}
	if len(result.Flow.Checkpoints) != 1 || result.Flow.Checkpoints[0].Kind != "email_verification" || result.Flow.Checkpoints[0].SequenceIndex != 4 {
		t.Fatalf("checkpoints = %#v", result.Flow.Checkpoints)
	}
	if result.CallPolicy != (CallPolicy{
		ApprovalSymbol: approvalSymbol, DuplicatePrevention: duplicatePrevention, OnDuplicate: onDuplicate,
		AmbiguousOutcome: ambiguousOutcome, CleanupDisposition: cleanupDelete,
	}) {
		t.Fatalf("call policy = %#v", result.CallPolicy)
	}
	if !equalStrings(result.Network.Methods, []string{"GET", "HEAD"}) || result.Network.MutationRequests != 0 || result.Network.SubmitExecuted || result.Network.AccountAttempted || result.Network.SessionEstablished || result.Network.RuntimeSupported {
		t.Fatalf("network posture = %#v", result.Network)
	}
	if err := Verify(result, createdAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	first, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || first[len(first)-1] != '\n' {
		t.Fatal("result bytes are not deterministic and newline terminated")
	}
	firstDigest, err := Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != testDigest(first) {
		t.Fatalf("result digest = %q", firstDigest)
	}
	decoded, err := Decode(first, createdAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	decodedBytes, err := MarshalDeterministic(decoded)
	if err != nil || !bytes.Equal(first, decodedBytes) {
		t.Fatalf("round trip changed result: error=%v", err)
	}
	for _, forbidden := range []string{
		"credentialValue", "accountIdentifier", "verificationValue", "cookie", "storageState",
		"pageContent", "rawWorkerOutput", "privatePath", "artifactPath", "POST",
	} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("result contains forbidden field or claim %q", forbidden)
		}
	}
}

func TestBuildCanonicalizesEmptyDiagnosticsAsArray(t *testing.T) {
	completion := validCompletion(t)
	completion.Diagnostics = nil
	result, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"diagnostics":[]`)) || bytes.Contains(data, []byte(`"diagnostics":null`)) {
		t.Fatalf("empty diagnostics are not canonical: %s", data)
	}
	if _, err := Decode(data, createdAt); err != nil {
		t.Fatal(err)
	}
}

func TestV2BuildDecodeAndVerifyUseAdditiveIdentity(t *testing.T) {
	completion := validCompletion(t)
	completion.Protocol = registrationauthorsession.ProtocolV2
	result, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != SchemaV2 || result.Provenance != (Provenance{Producer: producerName, ResultVersion: SchemaV2, SessionVersion: registrationauthorsession.ProtocolV2}) {
		t.Fatalf("identity = %q %#v", result.Schema, result.Provenance)
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data, createdAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != SchemaV2 || decoded.Provenance.SessionVersion != registrationauthorsession.ProtocolV2 {
		t.Fatalf("decoded identity = %q %#v", decoded.Schema, decoded.Provenance)
	}
}

func TestV2BuildValidatesEveryRetainedProfileNavigation(t *testing.T) {
	completion := validCompletion(t)
	completion.Protocol = registrationauthorsession.ProtocolV2
	flow := completion.Profile.Flows[completion.Flow]
	flow.Sequence[0].Navigate = "https://app.example.test/register?action=startnew"
	completion.Profile.Flows[completion.Flow] = flow
	completion.ProfileBytes, _ = registrationprofile.MarshalJSON(&completion.Profile)
	if _, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}

	flow = completion.Profile.Flows[completion.Flow]
	flow.Sequence[0].Navigate = "https://other.example.test/register?action=startnew"
	completion.Profile.Flows[completion.Flow] = flow
	completion.ProfileBytes, _ = registrationprofile.MarshalJSON(&completion.Profile)
	if _, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt}); err == nil || strings.Contains(err.Error(), "other.example") {
		t.Fatalf("unsafe retained origin error = %v", err)
	}
}

func TestVerifyRejectsEveryBoundIdentityAndSafetyMutation(t *testing.T) {
	base, err := Build(BuildRequest{Completion: validCompletion(t), CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Envelope){
		"schema":            func(value *Envelope) { value.Schema = "browsertools.registration-authoring.v2" },
		"producer":          func(value *Envelope) { value.Provenance.Producer = "other" },
		"session":           func(value *Envelope) { value.Provenance.SessionVersion = "browsertools.author-session.v2" },
		"created":           func(value *Envelope) { value.CreatedAt = "2026-08-26T12:02:00Z" },
		"observed":          func(value *Envelope) { value.ObservedAt = "2026-08-26T12:02:00Z" },
		"expires":           func(value *Envelope) { value.ExpiresAt = "2026-09-23T00:00:00Z" },
		"origin":            func(value *Envelope) { value.Origins[0] = "https://other.example.test" },
		"candidate schema":  func(value *Envelope) { value.Candidate.Schema = "uws.browser.1.7" },
		"source digest":     func(value *Envelope) { value.Candidate.SourceDigest = strings.Repeat("0", 64) },
		"source":            func(value *Envelope) { value.Candidate.Source[1] = 'X' },
		"review digest":     func(value *Envelope) { value.Candidate.ReviewDigest = strings.Repeat("1", 64) },
		"review profile":    func(value *Envelope) { value.Candidate.Review.Profile.Confidence = "medium" },
		"candidate":         func(value *Envelope) { value.ReviewedCandidates[0].Label = "Other" },
		"flow":              func(value *Envelope) { value.Flow.Name = "other" },
		"submit candidate":  func(value *Envelope) { value.Flow.Submit.CandidateID = "candidate-ffffffffffffffff" },
		"submit execution":  func(value *Envelope) { value.Flow.Submit.Executed = true },
		"checkpoint":        func(value *Envelope) { value.Flow.Checkpoints[0].Kind = "captcha" },
		"success":           func(value *Envelope) { value.Flow.Success.Path = "/other" },
		"approval":          func(value *Envelope) { value.CallPolicy.ApprovalSymbol = "other" },
		"duplicate policy":  func(value *Envelope) { value.CallPolicy.OnDuplicate = "continue" },
		"cleanup":           func(value *Envelope) { value.CallPolicy.CleanupDisposition = "automatic" },
		"bounds":            func(value *Envelope) { value.Bounds.MaxRequests = 0 },
		"observations":      func(value *Envelope) { value.Observations = value.Bounds.MaxObservations + 1 },
		"methods":           func(value *Envelope) { value.Network.Methods = []string{"GET", "POST"} },
		"network count":     func(value *Envelope) { value.Network.Requests++ },
		"mutation requests": func(value *Envelope) { value.Network.MutationRequests = 1 },
		"submit claim":      func(value *Envelope) { value.Network.SubmitExecuted = true },
		"account claim":     func(value *Envelope) { value.Network.AccountAttempted = true },
		"session claim":     func(value *Envelope) { value.Network.SessionEstablished = true },
		"runtime claim":     func(value *Envelope) { value.Network.RuntimeSupported = true },
		"diagnostic":        func(value *Envelope) { value.Diagnostics[0] = "invalid diagnostic" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneEnvelope(t, base)
			mutate(changed)
			if err := Verify(changed, createdAt.Add(time.Hour)); err == nil {
				t.Fatal("tampered result unexpectedly verified")
			}
		})
	}
}

func TestBuildRejectsInvalidCompletionAndUnsupportedClaims(t *testing.T) {
	tests := map[string]func(*registrationauthorsession.Completion, *time.Time){
		"protocol": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.Protocol = "browsertools.author-session.v2"
		},
		"profile ID": func(value *registrationauthorsession.Completion, _ *time.Time) { value.ProfileID = "../../private" },
		"profile bytes": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.ProfileBytes = append(value.ProfileBytes, ' ')
		},
		"observation kind": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.Profile.ObservationKind = "dom_text"
			value.ProfileBytes, _ = registrationprofile.MarshalJSON(&value.Profile)
		},
		"origins": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.Origins = []string{"https://other.example.test"}
		},
		"flow": func(value *registrationauthorsession.Completion, _ *time.Time) { value.Flow = "missing" },
		"cleanup": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.CleanupDisposition = "automatic"
		},
		"candidate": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.ReviewedCandidates[0].Label = "Other"
		},
		"candidate role": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.ReviewedCandidates[0].Role = "selector"
		},
		"redacted candidate": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.ReviewedCandidates[0].Label = authorsession.RedactedLabel
		},
		"candidate generation": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.ReviewedCandidates[0].Generation = 2
		},
		"observations": func(value *registrationauthorsession.Completion, _ *time.Time) { value.Observations = 0 },
		"network":      func(value *registrationauthorsession.Completion, _ *time.Time) { value.Network.Requests = 3 },
		"diagnostic order": func(value *registrationauthorsession.Completion, _ *time.Time) {
			value.Diagnostics = []string{"z_code", "a_code"}
		},
		"created before observed": func(_ *registrationauthorsession.Completion, created *time.Time) {
			*created = observedAt.Add(-time.Second)
		},
		"created fractional": func(_ *registrationauthorsession.Completion, created *time.Time) {
			*created = createdAt.Add(time.Nanosecond)
		},
		"created non-UTC": func(_ *registrationauthorsession.Completion, created *time.Time) {
			*created = createdAt.In(time.FixedZone("offset", 3600))
		},
		"expired": func(_ *registrationauthorsession.Completion, created *time.Time) {
			*created = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			completion := validCompletion(t)
			created := createdAt
			mutate(completion, &created)
			if _, err := Build(BuildRequest{Completion: completion, CreatedAt: created}); err == nil {
				t.Fatal("invalid completion unexpectedly built")
			}
		})
	}

	t.Run("multiple submit candidates", func(t *testing.T) {
		completion := validCompletion(t)
		completion.ReviewedCandidates = append(completion.ReviewedCandidates,
			registrationauthorsession.ReviewedCandidate{ID: "candidate-fedcba9876543210", Generation: 1, Role: "button", Label: "Register", Matches: 1},
		)
		if _, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt}); err == nil {
			t.Fatal("ambiguous submit candidates unexpectedly built")
		}
	})

	t.Run("nonportable submit locator", func(t *testing.T) {
		completion := validCompletion(t)
		flow := completion.Profile.Flows[completion.Flow]
		flow.Sequence[3].Submit.Locator.Name = ""
		flow.Sequence[3].Submit.Locator.Text = "Register"
		completion.Profile.Flows[completion.Flow] = flow
		completion.ProfileBytes, _ = registrationprofile.MarshalJSON(&completion.Profile)
		if _, err := Build(BuildRequest{Completion: completion, CreatedAt: createdAt}); err == nil {
			t.Fatal("text-based submit locator unexpectedly built")
		}
	})
}

func TestDecodeRejectsUnclosedDuplicateUnknownDeepAndSensitiveInput(t *testing.T) {
	result, err := Build(BuildRequest{Completion: validCompletion(t), CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	compact := strings.TrimSpace(string(data))
	tests := map[string][]byte{
		"empty":            {},
		"trailing":         []byte(compact + `{}`),
		"duplicate":        []byte(strings.Replace(compact, `"schema":"`+Schema+`"`, `"schema":"`+Schema+`","schema":"`+Schema+`"`, 1)),
		"unknown":          []byte(strings.TrimSuffix(compact, "}") + `,"privatePath":"do-not-retain"}`),
		"nested unknown":   []byte(strings.Replace(compact, `"approvalSymbol":"`+approvalSymbol+`"`, `"approvalSymbol":"`+approvalSymbol+`","credentialValue":"do-not-retain"`, 1)),
		"deep":             []byte(`{"schema":"` + Schema + `","nested":` + strings.Repeat("[", maxResultDepth+1) + `0` + strings.Repeat("]", maxResultDepth+1) + `}`),
		"sensitive source": []byte(strings.Replace(compact, "Synthetic dedicated test registration", "operator@example.test", 1)),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(input, createdAt.Add(time.Hour)); err == nil {
				t.Fatal("invalid result unexpectedly decoded")
			} else if strings.Contains(err.Error(), "do-not-retain") || strings.Contains(err.Error(), "privatePath") || strings.Contains(err.Error(), "credentialValue") {
				t.Fatalf("invalid input detail leaked through decoder error: %v", err)
			}
		})
	}
	privateDuplicate := []byte(`{"operator@example.test":1,"operator@example.test":2}`)
	if _, err := Decode(privateDuplicate, createdAt); err == nil || strings.Contains(err.Error(), "operator@example.test") {
		t.Fatalf("duplicate-field error leaked private field name: %v", err)
	}
	oversized := bytes.Repeat([]byte{' '}, MaxResultBytes+1)
	if _, err := Decode(oversized, createdAt); err == nil {
		t.Fatal("oversized result unexpectedly decoded")
	}
	if _, err := Decode([]byte{'{', '"', 0xff, '"', '}'}, createdAt); err == nil {
		t.Fatal("non-UTF-8 result unexpectedly decoded")
	}
}

func TestVerifyRejectsFutureAssessmentAndExpiry(t *testing.T) {
	result, err := Build(BuildRequest{Completion: validCompletion(t), CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(result, createdAt.Add(-time.Second)); err == nil {
		t.Fatal("future-created review unexpectedly verified")
	}
	if err := Verify(result, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expired result unexpectedly verified")
	}
}

func TestWritePrivateExclusiveUsesOwnerOnlyNonOverwritingArtifact(t *testing.T) {
	result, err := Build(BuildRequest{Completion: validCompletion(t), CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	written, err := WritePrivateExclusive(root, result)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(written.Path) != root || !strings.HasPrefix(filepath.Base(written.Path), "registration-authoring-") {
		t.Fatalf("written result identity is invalid")
	}
	info, err := os.Stat(written.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("result mode = %v", info.Mode())
	}
	data, err := os.ReadFile(written.Path)
	if err != nil {
		t.Fatal(err)
	}
	if written.Digest != testDigest(data) {
		t.Fatalf("written digest = %q", written.Digest)
	}
	if _, err := Decode(data, createdAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePrivateExclusive(root, result); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second write error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(written.Path) {
		t.Fatalf("private root retained temporary artifacts: %#v", entries)
	}

	publicRoot := t.TempDir()
	if err := os.Chmod(publicRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePrivateExclusive(publicRoot, result); err == nil {
		t.Fatal("public result root unexpectedly accepted")
	}
	symlink := filepath.Join(t.TempDir(), "private-link")
	if err := os.Symlink(root, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePrivateExclusive(symlink, result); err == nil {
		t.Fatal("symlink result root unexpectedly accepted")
	}
}

func TestFinalizePrivateReconstructsWritesAndReopensExactly(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	finalized, err := FinalizePrivate(FinalizeRequest{
		Completion: validCompletion(t), CreatedAt: createdAt,
		AssessmentAt: createdAt.Add(time.Hour), PrivateRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if finalized == nil || finalized.Result == nil || finalized.Written.Path == "" || finalized.Written.Digest == "" {
		t.Fatalf("finalized=%#v", finalized)
	}
	data, err := os.ReadFile(finalized.Written.Path)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Written.Digest != testDigest(data) {
		t.Fatalf("written digest=%q", finalized.Written.Digest)
	}
	promotable, err := json.Marshal(finalized.Result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(promotable, []byte(root)) || bytes.Contains(promotable, []byte(filepath.Base(finalized.Written.Path))) {
		t.Fatal("private path entered the result envelope")
	}
	decoded, err := Decode(data, createdAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want, err := MarshalDeterministic(finalized.Result)
	if err != nil || !bytes.Equal(data, want) || !reflectDeepEnvelope(decoded, finalized.Result) {
		t.Fatalf("finalized reconstruction changed: %v", err)
	}
	if _, err := FinalizePrivate(FinalizeRequest{
		Completion: validCompletion(t), CreatedAt: createdAt,
		AssessmentAt: createdAt.Add(time.Hour), PrivateRoot: root,
	}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("repeat finalization error=%v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("private entries=%#v err=%v", entries, err)
	}
}

func TestFinalizePrivateFailsBeforeWriteForInvalidLifecycleOrContent(t *testing.T) {
	tests := map[string]func(*FinalizeRequest){
		"teardown accounting": func(value *FinalizeRequest) { value.Completion.Network.Requests++ },
		"expired assessment":  func(value *FinalizeRequest) { value.AssessmentAt = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC) },
		"future creation":     func(value *FinalizeRequest) { value.AssessmentAt = value.CreatedAt.Add(-time.Second) },
		"missing root":        func(value *FinalizeRequest) { value.PrivateRoot = "" },
		"sensitive title": func(value *FinalizeRequest) {
			value.Completion.Profile.Info.Title = "operator@example.test"
			value.Completion.ProfileBytes, _ = registrationprofile.MarshalJSON(&value.Completion.Profile)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			request := FinalizeRequest{
				Completion: validCompletion(t), CreatedAt: createdAt,
				AssessmentAt: createdAt.Add(time.Hour), PrivateRoot: root,
			}
			mutate(&request)
			if _, err := FinalizePrivate(request); err == nil {
				t.Fatal("invalid finalization unexpectedly succeeded")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed finalization retained artifacts: %#v err=%v", entries, err)
			}
		})
	}
}

func TestReadPrivateExactRejectsSymlinkModeAndTamper(t *testing.T) {
	result, err := Build(BuildRequest{Completion: validCompletion(t), CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	rootPath := t.TempDir()
	if err := os.Chmod(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := openPrivateRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := resultName(testDigest(data))
	if err := root.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPrivateExact(root, name); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("exact read failed: %v", err)
	}
	if err := root.Chmod(name, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateExact(root, name); err == nil {
		t.Fatal("public result mode unexpectedly accepted")
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink("target", name); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateExact(root, name); err == nil {
		t.Fatal("symlink result unexpectedly accepted")
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, append(append([]byte(nil), data...), 'x'), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered, err := readPrivateExact(root, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(tampered, createdAt.Add(time.Hour)); err == nil {
		t.Fatal("tampered private result unexpectedly decoded")
	}
}

func validCompletion(t *testing.T) *registrationauthorsession.Completion {
	t.Helper()
	data, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profileValue, err := registrationprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	data, err = registrationprofile.MarshalJSON(profileValue)
	if err != nil {
		t.Fatal(err)
	}
	return &registrationauthorsession.Completion{
		Protocol:  registrationauthorsession.Protocol,
		ProfileID: "synthetic_registration", Profile: *profileValue, ProfileBytes: data,
		ReviewedCandidates: []registrationauthorsession.ReviewedCandidate{{
			ID: "candidate-0123456789abcdef", Generation: 1, Role: "button", Label: "Register", Matches: 1,
		}},
		Flow: "create_dedicated_test_user", CleanupDisposition: cleanupDelete,
		Origins: []string{"https://app.example.test"}, ObservedAt: observedAt,
		Bounds: registrationauthorsession.Bounds{
			NavigationTimeoutMS: 20_000, TotalTimeoutMS: 300_000, MaxRequests: 256,
			MaxResponseBytes: 32 << 20, MaxObservations: 64, MaxCandidates: 128,
		},
		Observations: 1, Diagnostics: []string{"synthetic_fixture"},
		Network: registrationauthorsession.NetworkSummary{Requests: 2, GETRequests: 1, HEADRequests: 1},
	}
}

func cloneEnvelope(t *testing.T, value *Envelope) *Envelope {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result Envelope
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func testDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func reflectDeepEnvelope(left, right *Envelope) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}
