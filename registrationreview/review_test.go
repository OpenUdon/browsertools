package registrationreview

import (
	"os"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/registrationprofile"
)

func TestBuildVerifyExpiryAndTamper(t *testing.T) {
	data, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	value, err := registrationprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	bundle, err := Build(value, current)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable || len(bundle.Gaps) != 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
	if err := Verify(bundle, current); err != nil {
		t.Fatal(err)
	}
	tampered := *bundle
	tampered.Profile.Confidence = "medium"
	if err := Verify(&tampered, current); err == nil {
		t.Fatal("tampered bundle unexpectedly verified")
	}
	for name, mutate := range map[string]func(*Bundle){
		"assessment": func(value *Bundle) { value.AssessedAt = "2026-08-26T00:00:00Z" },
		"assessment before verification": func(value *Bundle) {
			value.AssessedAt = "2026-08-24T23:59:59Z"
		},
		"expiry":    func(value *Bundle) { value.ExpiresAt = "2026-09-23T00:00:00Z" },
		"promotion": func(value *Bundle) { value.Promotable = false },
		"gaps":      func(value *Bundle) { value.Gaps = []string{"profile_expired"} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := *bundle
			changed.Gaps = append([]string(nil), bundle.Gaps...)
			mutate(&changed)
			if err := Verify(&changed, current); err == nil {
				t.Fatal("metadata-tampered bundle unexpectedly verified")
			}
		})
	}
	expired, err := Build(value, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if expired.Promotable || len(expired.Gaps) != 1 {
		t.Fatalf("expired bundle = %#v", expired)
	}
	future := *value
	future.Verification.LastVerifiedAt = "2099-08-25T00:00:00Z"
	if _, err := Build(&future, current); err == nil {
		t.Fatal("future-dated verification unexpectedly produced a review bundle")
	}
}
