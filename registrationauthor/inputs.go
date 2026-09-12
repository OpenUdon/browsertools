package registrationauthor

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationdraft"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

func buildV3(request BuildRequest, origins []string) (*Candidate, error) {
	bad := errors.New("registration 1.1 authoring evidence is invalid")
	profile, err := registrationdraft.Build(request.Spec)
	if err != nil || profile.Evidence.LearnedAt != request.AssessedAt.Format(time.RFC3339) || profile.Verification.LastVerifiedAt != request.AssessedAt.Format(time.RFC3339) || registrationprofile.ValidateAt(profile, request.AssessedAt) != nil || registrationprofile.ValidateRetainedNavigationV2(profile) != nil || !equalStrings(registrationprofile.Origins(profile), origins) {
		return nil, bad
	}
	if registrationauthorsession.ValidateV3Evidence(profile, request.Flow, request.History, request.Previews, request.StepCandidates, request.ReviewedCandidateIDs) != nil {
		return nil, bad
	}
	if !contains(request.ReviewedCandidateIDs, request.SubmitCandidateID) {
		return nil, bad
	}
	var submit registrationauthorsession.Candidate
	for _, observation := range request.History {
		for _, candidate := range observation.Candidates {
			if candidate.ID == request.SubmitCandidateID {
				submit = candidate
			}
		}
	}
	if bindSubmit(profile.Flows[request.Flow].Sequence, submit) != nil {
		return nil, bad
	}
	data, err := registrationprofile.MarshalJSON(profile)
	if err != nil {
		return nil, bad
	}
	last := request.History[len(request.History)-1]
	encoded, err := json.Marshal(last)
	if err != nil {
		return nil, bad
	}
	if json.Unmarshal(encoded, &last) != nil {
		return nil, bad
	}
	return &Candidate{profileID: request.ProfileID, profileBytes: data, observation: last, reviewedIDs: append([]string(nil), request.ReviewedCandidateIDs...), submitID: request.SubmitCandidateID, flow: request.Flow, controls: request.Controls, approvedOrigins: append([]string(nil), origins...), protocol: request.Protocol, stepCandidates: append([]string(nil), request.StepCandidates...)}, nil
}
