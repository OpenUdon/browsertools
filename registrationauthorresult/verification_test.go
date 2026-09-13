package registrationauthorresult

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationreview"
	"github.com/OpenUdon/uws/browserregistration"
)

func TestVerificationResultBindsExplicitReviewAndProviderBudgets(t *testing.T) {
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	for _, provider := range []string{"turnstile", "recaptcha_v2", "hcaptcha"} {
		for _, mode := range []string{"before_approval", "approved_submit"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				descriptor := &browserregistration.HumanVerification{Provider: provider, Activation: mode, WidgetBinding: "single_in_submit_form", SubmissionURL: "https://app.example.test/register", Dependencies: browserregistration.VerificationDependencies{Policy: provider + ".v1", MaxRequests: 256, MaxResponseBytes: 32 << 20, TimeoutMS: 120000}}
				backend := &registrationfixture.Browser{Verification: descriptor}
				completion, err := registrationfixture.AuthorVerification(t.Context(), backend, "https://app.example.test", at, descriptor)
				if err != nil {
					t.Fatal(err)
				}
				if !backend.Session.Closed || !backend.Session.VerificationApproved || completion.Protocol != registrationauthorsession.ProtocolV4 {
					t.Fatal("missing review or teardown")
				}
				completion.Network.ProviderRequests, completion.Network.ProviderPOSTRequests, completion.Network.ProviderResponseBytes = 3, 1, 128
				result, err := Build(BuildRequest{Completion: completion, CreatedAt: at})
				if err != nil {
					t.Fatal(err)
				}
				if result.Schema != SchemaV4 || result.Candidate.Review.Version != registrationreview.VersionV3 {
					t.Fatal("wrong version lineage")
				}
				data, err := MarshalDeterministic(result)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = Decode(data, at); err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(data, []byte("PROVIDER_POST")) {
					t.Fatal("non-HTTP method in result")
				}
				for name, mutate := range map[string]func(*Envelope){
					"legacy":            func(r *Envelope) { r.Schema = SchemaV3 },
					"missing authority": func(r *Envelope) { r.VerificationAuthority = nil },
					"policy":            func(r *Envelope) { r.VerificationAuthority.Dependencies.Policy = "unreviewed.v1" },
					"increased budget":  func(r *Envelope) { r.VerificationAuthority.Dependencies.MaxRequests++ },
					"wrong widget": func(r *Envelope) {
						for i := range r.History {
							for j := range r.History[i].Candidates {
								if r.History[i].Candidates[j].Verification != nil {
									r.History[i].Candidates[j].Verification.SubmissionURL = "https://app.example.test/other"
								}
							}
						}
					},
					"method accounting":    func(r *Envelope) { r.Network.ProviderPOSTRequests = 4 },
					"response budget":      func(r *Envelope) { r.Network.ProviderResponseBytes = 33 << 20 },
					"application mutation": func(r *Envelope) { r.Network.MutationRequests = 1 },
				} {
					t.Run(name, func(t *testing.T) {
						var changed Envelope
						if err := json.Unmarshal(data, &changed); err != nil {
							t.Fatal(err)
						}
						mutate(&changed)
						if Verify(&changed, at) == nil {
							t.Fatal("tampered result accepted")
						}
					})
				}
				descriptor.Dependencies.MaxRequests = 1
				if Verify(result, at) != nil {
					t.Fatal("result aliases caller authority")
				}
			})
		}
	}
}
