package capture

import (
	"errors"
	"github.com/OpenUdon/browsertools/authordiagnostic"
	playwright "github.com/mxschmitt/playwright-go"
	"testing"
)

func TestAuthorBackendFailureClassDoesNotInspectErrorText(t *testing.T) {
	for _, tc := range []struct {
		stage  string
		err    error
		reason string
	}{
		{"launch", errors.New("TOKEN_CANARY"), "failed"},
		{"navigation", errors.New("TimeoutError TOKEN_CANARY"), "transport"},
		{"navigation", playwright.ErrTimeout, "timeout"},
		{"guard", errors.New("COOKIE_CANARY"), "failed"},
	} {
		got := authordiagnostic.Classify(authorBackendError(tc.stage, tc.err))
		if got.Stage != tc.stage || got.Reason != tc.reason || !got.Valid() {
			t.Fatal(got)
		}
	}
}

func TestAuthorObservationOriginFailureRetainsPolicyClass(t *testing.T) {
	_, _, err := authorURLFacts("https://other.invalid/path?token=TOKEN_CANARY", func(string) bool { return false })
	got := authordiagnostic.Classify(err)
	if got != (authordiagnostic.Class{Stage: "policy", Reason: "origin_escape"}) {
		t.Fatal(got)
	}
	var policy *policyError
	if !errors.As(err, &policy) || policy.Code != "origin_escape" {
		t.Fatal("lost original policy error")
	}
}
