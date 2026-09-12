package registrationdraft

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/browsertools/registrationreview"
)

func TestTypedInputsSelectAdditiveProfileAndReview(t *testing.T) {
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	profile := registrationfixture.Profile("https://app.example.test", at)
	data, err := registrationprofile.MarshalJSON(profile)
	if err != nil {
		t.Fatal(err)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	built, err := Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	if built.Profile != "uws.browser-registration.1.1" {
		t.Fatal("typed recipe did not select 1.1")
	}
	review, err := registrationreview.Build(built, at)
	if err != nil {
		t.Fatal(err)
	}
	if review.Version != registrationreview.VersionV2 {
		t.Fatal("new review mislabeled as legacy")
	}
	if err := registrationreview.Verify(review, at); err != nil {
		t.Fatal(err)
	}
	review.Version = registrationreview.Version
	if registrationreview.Verify(review, at) == nil {
		t.Fatal("legacy review accepted 1.1")
	}
}
