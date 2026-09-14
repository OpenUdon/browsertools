package registrationauthorsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestTerminalFailureVocabularyAndTeardown(t *testing.T) {
	for _, protocol := range []string{ProtocolV1, ProtocolV4} {
		for _, tc := range []struct {
			name, command, want string
			closeFailure        bool
		}{
			{"observe", "observe", "browser_failure", false},
			{"unknown command", "unknown", "unknown_message", false},
			{"observe teardown", "observe", "teardown_failure", true},
			{"invalid command teardown", "unknown", "teardown_failure", true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				const canary = "private-browser-credential-canary"
				backend := &fakeSession{observeErr: errors.New(canary)}
				if tc.closeFailure {
					backend.closeErr = errors.New(canary)
				}
				var input, output bytes.Buffer
				enc := json.NewEncoder(&input)
				_ = enc.Encode(ClientMessage{Protocol: protocol, Type: "start", ProfileID: "terminal_fixture", URL: "https://app.example.test/register", Origins: []string{"https://app.example.test"}})
				_ = enc.Encode(ClientMessage{Protocol: protocol, Type: tc.command})
				completion, err := Serve(context.Background(), io.NopCloser(&input), &output, &fakeBrowser{session: backend}, ServeOptions{Protocol: protocol})
				if err == nil || completion != nil || backend.closeCount != 1 || strings.Contains(output.String()+err.Error(), canary) {
					t.Fatal("terminal failure did not close privately without a completion")
				}
				var terminal ServerMessage
				dec := json.NewDecoder(&output)
				for dec.More() {
					if err := dec.Decode(&terminal); err != nil {
						t.Fatal(err)
					}
				}
				if terminal.Diagnostic == nil || terminal.Diagnostic.Code != tc.want || !ValidTerminalDiagnostic(tc.want) || ValidDiagnostic(tc.want) {
					t.Fatalf("unexpected terminal classification: %#v", terminal)
				}
			})
		}
	}
	for _, code := range []string{"", "private error", "browser_failure\n", DiagnosticSyntheticFixture, DiagnosticAccessibilitySnapshotPartial, DiagnosticCrossOriginFrameOmitted, DiagnosticUnsupportedAccessibleControl} {
		if ValidTerminalDiagnostic(code) {
			t.Fatalf("non-terminal code accepted: %q", code)
		}
	}
}

func TestCancellationCannotHideFailedBrowserTeardown(t *testing.T) {
	backend := &fakeSession{closeErr: errors.New("private shutdown canary")}
	var output bytes.Buffer
	s := server{session: backend, output: &output, protocol: ProtocolV4}
	if err := s.cancel(); err == nil || !strings.Contains(err.Error(), "teardown_failure") || strings.Contains(err.Error(), "canary") || backend.closeCount != 1 {
		t.Fatal("cancellation hid failed teardown or leaked exception detail")
	}
}
