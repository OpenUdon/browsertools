package registrationauthorresult

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationreview"
)

func TestV3ResultBindsActualProducerHistory(t *testing.T) {
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	completion, err := registrationfixture.Author(t.Context(), &registrationfixture.Browser{}, "https://app.example.test", at)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Build(BuildRequest{Completion: completion, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != SchemaV3 || result.Candidate.Review.Version != registrationreview.VersionV2 {
		t.Fatal("wrong v3 lineage")
	}
	// The reviewed source is a portable export, so it must not carry private
	// discovery inventory.
	if bytes.Contains(result.Candidate.Source, []byte(`"discovery"`)) {
		t.Fatal("v3 candidate source carries discovery metadata")
	}
	data, err := MarshalDeterministic(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data, at); err != nil {
		t.Fatal(err)
	}
	completion.History[0].Candidates[0].Control.Kind = "changed"
	if err := Verify(result, at); err != nil {
		t.Fatal("result aliases caller evidence")
	}
	for name, mutate := range map[string]func(*Envelope){
		"legacy schema":     func(r *Envelope) { r.Schema = SchemaV2 },
		"missing history":   func(r *Envelope) { r.History = nil },
		"wrong step":        func(r *Envelope) { r.StepCandidates[6] = r.StepCandidates[4] },
		"changed candidate": func(r *Envelope) { r.ReviewedCandidates[0].Label = "Unreviewed field" },
		"changed review":    func(r *Envelope) { r.Candidate.Review.Version = registrationreview.Version },
	} {
		t.Run(name, func(t *testing.T) {
			var copy Envelope
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			mutate(&copy)
			if Verify(&copy, at) == nil {
				t.Fatal("tampered v3 result accepted")
			}
		})
	}
}
