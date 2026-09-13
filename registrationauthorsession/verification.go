package registrationauthorsession

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
)

// VerificationObservation is reduced metadata attached to the uniquely
// observed submit candidate. It grants no dependency or registration authority.
type VerificationObservation struct {
	Provider      string `json:"provider"`
	Activation    string `json:"activation"`
	SubmissionURL string `json:"submissionURL"`
	WidgetBinding string `json:"widgetBinding"`
	Coverage      string `json:"coverage"`
}

// VerificationSession can admit separately reviewed provider traffic. It adds
// no application mutation, input, token-reading or challenge-solving method.
type VerificationSession interface {
	ApproveVerification(context.Context, browserregistration.HumanVerification) error
}

func ValidateVerificationObservation(v *VerificationObservation, origins []string) error {
	if v == nil || !slices.Contains([]string{"turnstile", "recaptcha_v2", "hcaptcha"}, v.Provider) ||
		!slices.Contains([]string{"before_approval", "approved_submit"}, v.Activation) || v.WidgetBinding != "single_in_submit_form" || v.Coverage != "standard_single_widget" {
		return errors.New("unsupported verification observation")
	}
	_, origin, _, err := ValidateNavigationURL(ProtocolV4, v.SubmissionURL)
	if err != nil || !contains(origins, origin) {
		return errors.New("invalid verification destination")
	}
	return nil
}

func ValidateVerificationAuthority(v *browserregistration.HumanVerification, origins []string) error {
	if v == nil {
		return errors.New("verification authority is required")
	}
	if err := ValidateVerificationObservation(&VerificationObservation{Provider: v.Provider, Activation: v.Activation, SubmissionURL: v.SubmissionURL, WidgetBinding: v.WidgetBinding, Coverage: "standard_single_widget"}, origins); err != nil {
		return err
	}
	d := v.Dependencies
	if d.Policy != v.Provider+".v1" || d.MaxRequests < 1 || d.MaxRequests > 256 || d.MaxResponseBytes < 1 || d.MaxResponseBytes > 32<<20 || d.TimeoutMS < 1 || d.TimeoutMS > 120000 {
		return errors.New("invalid verification policy")
	}
	return nil
}

func (s *server) approveVerification(message ClientMessage) error {
	if s.protocol != ProtocolV4 || s.verificationAuthority != nil || ValidateVerificationAuthority(message.Verification, s.origins) != nil {
		return s.fail("invalid_review")
	}
	record, ok := s.candidates[message.CandidateID]
	if !ok || record.protocol.Matches != 1 || !matchesVerification(record.protocol.Verification, message.Verification) {
		return s.fail("invalid_review")
	}
	backend, ok := s.session.(VerificationSession)
	if !ok {
		return s.fail("invalid_review")
	}
	if err := s.withActiveContext(func(ctx context.Context) error { return backend.ApproveVerification(ctx, *message.Verification) }); err != nil {
		return s.failBrowser()
	}
	s.verificationAuthority = cloneVerification(message.Verification)
	s.reviewedProfile = nil
	return s.write(ServerMessage{Type: "state", Phase: s.phase})
}

func matchesVerification(o *VerificationObservation, v *browserregistration.HumanVerification) bool {
	return o != nil && v != nil && o.Provider == v.Provider && o.Activation == v.Activation && o.SubmissionURL == v.SubmissionURL && o.WidgetBinding == v.WidgetBinding && o.Coverage == "standard_single_widget"
}

func verificationEqual(a, b *browserregistration.HumanVerification) bool {
	return a != nil && b != nil && reflect.DeepEqual(a, b)
}
func cloneVerification(value *browserregistration.HumanVerification) *browserregistration.HumanVerification {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// ValidateVerificationEvidence binds the descriptor to the selected submit's
// actual observation. Provider/activation/destination edits invalidate review.
func ValidateVerificationEvidence(profile *registrationprofile.Profile, flowName string, history []Observation, steps []string) error {
	flow, ok := profile.Flows[flowName]
	if !ok || len(steps) != len(flow.Sequence) || ValidateVerificationAuthority(flow.HumanVerification, registrationprofile.Origins(profile)) != nil {
		return errors.New("invalid verification evidence")
	}
	for index, step := range flow.Sequence {
		if step.Submit == nil {
			continue
		}
		for _, observation := range history {
			for _, candidate := range observation.Candidates {
				if candidate.ID == steps[index] && candidate.Matches == 1 && matchesVerification(candidate.Verification, flow.HumanVerification) && ValidateVerificationObservation(candidate.Verification, registrationprofile.Origins(profile)) == nil {
					return nil
				}
			}
		}
	}
	return errors.New("verification widget is not bound to reviewed submit")
}

func ValidateNetworkSummaryForProtocol(summary NetworkSummary, maximum int, protocol string, authority *browserregistration.HumanVerification) error {
	if protocol != ProtocolV4 {
		if summary.ProviderRequests != 0 || summary.ProviderPOSTRequests != 0 || summary.ProviderResponseBytes != 0 || authority != nil {
			return errors.New("legacy session has verification authority")
		}
		return validateNetworkSummary(summary, maximum)
	}
	if summary.ProviderRequests < 0 || summary.ProviderPOSTRequests < 0 || summary.ProviderPOSTRequests > summary.ProviderRequests || summary.ProviderResponseBytes < 0 {
		return errors.New("invalid verification accounting")
	}
	if authority == nil {
		if summary.ProviderRequests != 0 || summary.ProviderResponseBytes != 0 {
			return errors.New("unapproved verification traffic")
		}
	} else if summary.ProviderRequests > authority.Dependencies.MaxRequests || summary.ProviderResponseBytes > int64(authority.Dependencies.MaxResponseBytes) {
		return errors.New("verification bounds exceeded")
	}
	return validateNetworkSummary(NetworkSummary{Requests: summary.Requests, GETRequests: summary.GETRequests, HEADRequests: summary.HEADRequests}, maximum)
}

// CopyObservation preserves the full versioned metadata without aliasing.
func CopyObservation(value Observation) Observation {
	data, _ := json.Marshal(value)
	var out Observation
	_ = json.Unmarshal(data, &out)
	return out
}
