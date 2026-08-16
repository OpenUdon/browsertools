package capture

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/profile"
)

func TestComparePortabilityUsesFreshExactProbePlans(t *testing.T) {
	observation := validObservation()
	observation.ProbeResults = []ProbeResult{
		{ID: "P001", Matches: 1},
		{ID: "P002", Reached: true},
		{ID: "P003", Matches: 1, ObservedType: "string"},
	}
	backends := map[Engine]*fakeAcquirer{}
	factory := func(engine Engine) Acquirer {
		backend := &fakeAcquirer{observation: observation}
		backends[engine] = backend
		return backend
	}
	report, err := ComparePortability(context.Background(), factory,
		[]Engine{EngineWebKit, EngineChromium, EngineFirefox}, validCheckRequest(validCheckProfile()))
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Version != PortabilityVersion || len(report.Engines) != 3 {
		t.Fatalf("report = %#v", report)
	}
	for index, engine := range []Engine{EngineChromium, EngineFirefox, EngineWebKit} {
		if report.Engines[index].Engine != engine || report.Engines[index].Status != PortabilityPassed {
			t.Fatalf("engine[%d] = %#v", index, report.Engines[index])
		}
		backend := backends[engine]
		if backend.calls != 1 || len(backend.request.Probes) != 3 {
			t.Fatalf("%s calls=%d probes=%#v", engine, backend.calls, backend.request.Probes)
		}
	}
	if len(report.ContractPressure) != 9 || report.ContractPressure[0].Capability != "screenshot" {
		t.Fatalf("pressure = %#v", report.ContractPressure)
	}
}

func TestComparePortabilityReportsFailuresWithoutBackendDetails(t *testing.T) {
	secret := "https://user:password@example.test/private?token=secret"
	factory := func(engine Engine) Acquirer {
		if engine == EngineChromium {
			return errorAcquirer{err: newEngineUnavailable(engine, errors.New(secret))}
		}
		observation := validObservation()
		observation.ProbeResults = []ProbeResult{
			{ID: "P001", Matches: 1}, {ID: "P002", Reached: true},
			{ID: "P003", Matches: 1, ObservedType: "string"},
		}
		return &fakeAcquirer{observation: observation}
	}
	report, err := ComparePortability(context.Background(), factory,
		[]Engine{EngineChromium, EngineFirefox}, validCheckRequest(validCheckProfile()))
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Engines[0].Status != PortabilityUnavailable || report.Engines[0].Diagnostic != "engine_unavailable" ||
		report.Engines[1].Diagnostic != "chromium_baseline_unavailable" {
		t.Fatalf("report = %#v", report)
	}
	if strings.Contains(strings.TrimSpace(report.Engines[0].Diagnostic), "password") || strings.Contains(strings.TrimSpace(report.Engines[0].Diagnostic), "https") {
		t.Fatalf("backend detail leaked: %#v", report.Engines[0])
	}
}

func TestComparePortabilityRejectsSuccessfulButDifferentArrayShape(t *testing.T) {
	prof := validCheckProfile()
	action := prof.Actions["read_status"]
	output := action.Outputs["status"]
	output.Type = profile.OutputArray
	action.Outputs["status"] = output
	prof.Actions["read_status"] = action
	factory := func(engine Engine) Acquirer {
		matches := 1
		if engine == EngineFirefox {
			matches = 2
		}
		observation := validObservation()
		observation.ProbeResults = []ProbeResult{
			{ID: "P001", Matches: 1}, {ID: "P002", Reached: true},
			{ID: "P003", Matches: matches, ObservedType: profile.OutputArray},
		}
		return &fakeAcquirer{observation: observation}
	}
	report, err := ComparePortability(context.Background(), factory,
		[]Engine{EngineChromium, EngineFirefox}, validCheckRequest(prof))
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Engines[0].Status != PortabilityPassed || report.Engines[1].Status != PortabilityFailed ||
		report.Engines[1].Diagnostic != "check_shape_mismatch" {
		t.Fatalf("report = %#v", report)
	}
}

func TestComparePortabilityRejectsEngineAndRequestExpansionBeforeAcquisition(t *testing.T) {
	calls := 0
	factory := func(Engine) Acquirer { calls++; return &fakeAcquirer{observation: validObservation()} }
	request := validCheckRequest(validCheckProfile())
	for _, engines := range [][]Engine{{EngineFirefox}, {EngineChromium, EngineChromium}, {EngineChromium, "edge"}} {
		if _, err := ComparePortability(context.Background(), factory, engines, request); err == nil {
			t.Fatalf("engines %#v accepted", engines)
		}
	}
	request.Capture.AllowedOrigins = append(request.Capture.AllowedOrigins, "https://assets.example.test")
	if _, err := ComparePortability(context.Background(), factory, []Engine{EngineChromium, EngineWebKit}, request); err == nil || calls != 0 {
		t.Fatalf("expanded request err=%v calls=%d", err, calls)
	}
}

func TestContractPressureIsStableAndCallerOwned(t *testing.T) {
	first := ContractPressure()
	second := ContractPressure()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("pressure is unstable")
	}
	first[0].Capability = "changed"
	if second[0].Capability != "screenshot" || ContractPressure()[0].Capability != "screenshot" {
		t.Fatalf("pressure shares mutable state")
	}
	seen := map[string]bool{}
	for _, item := range second {
		if seen[item.Capability] || item.Browser15 == "" || item.NextStep == "" {
			t.Fatalf("invalid pressure item %#v", item)
		}
		seen[item.Capability] = true
	}
}

type errorAcquirer struct{ err error }

func (a errorAcquirer) Acquire(context.Context, LiveRequest) (Observation, error) {
	return Observation{}, a.err
}
