package runtime

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func newDifferenceTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime, err := New(t.TempDir(), "difference-instance", testConfig(10), &blockingCognizer{
		started: make(chan CognitiveRequest, 1), release: make(chan struct{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func perceptualSignalPayload(t *testing.T, objectID string) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"organ_id": "browser", "surface_id": "chrome.current_page",
		"object_id": objectID, "content": "fact " + objectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestDifferenceFieldAdmitsNovelFactThenCompressesExpectedNoise(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	if err := runtime.addEvent("perceptual_change", "observed", "first", "first", perceptualSignalPayload(t, "first"), true); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("a first unfamiliar fact did not reach attention: %#v", runtime.state.Background)
	}
	first := runtime.state.Background[0]
	runtime.learnDifferenceFromAppraisal(first, CandidateAppraisal{
		Ownership: 0.05, Value: 0, Answerability: 0, Certainty: 1,
	}, false)
	trace := runtime.state.DifferenceField[first.DifferenceKey]
	trace.ExpectedChangeRate = 1
	trace.AttentionValue = 0
	trace.Accumulated = 0
	runtime.state.DifferenceField[first.DifferenceKey] = trace

	before := len(runtime.state.Background)
	if err := runtime.addEvent("perceptual_change", "observed", "second", "second", perceptualSignalPayload(t, "second"), true); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != before {
		t.Fatal("an expected, learned-low-value surface woke the main cognition again")
	}
	trace = runtime.state.DifferenceField[first.DifferenceKey]
	if trace.Observations != 2 || trace.LastPredictionGap != 0 {
		t.Fatalf("the expected change was not absorbed by the predictor: %#v", trace)
	}
}

func TestWeakUnresolvedDifferencesAccumulateUntilAttention(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	key := "observed/perceptual_change/browser/chrome.current_page"
	runtime.state.DifferenceField[key] = DifferenceTrace{
		Key: key, Observations: 1, LastDigest: "old", ExpectedChangeRate: 0.80,
		AttentionValue: 0.50,
	}
	admittedAt := 0
	for index := 1; index <= 8; index++ {
		before := len(runtime.state.Background)
		payload := perceptualSignalPayload(t, string(rune('a'+index)))
		if err := runtime.addEvent("perceptual_change", "observed", "changing", "", payload, true); err != nil {
			t.Fatal(err)
		}
		if len(runtime.state.Background) > before {
			admittedAt = index
			break
		}
	}
	if admittedAt < 2 {
		t.Fatalf("weak changes did not accumulate before ignition: admitted_at=%d", admittedAt)
	}
}

func TestLearnedLowChangingSignalIsSampledAgainWithoutWakingEveryTime(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	key := "observed/perceptual_change/browser/chrome.current_page"
	runtime.state.DifferenceField[key] = DifferenceTrace{
		Key: key, Observations: 50, LastDigest: "old", ExpectedChangeRate: 1,
		AttentionValue: 0,
	}
	admittedAt := 0
	for index := 1; index <= 20; index++ {
		before := len(runtime.state.Background)
		payload := perceptualSignalPayload(t, string(rune('a'+index)))
		if err := runtime.addEvent("perceptual_change", "observed", "changing", "", payload, true); err != nil {
			t.Fatal(err)
		}
		if len(runtime.state.Background) > before {
			admittedAt = index
			break
		}
	}
	if admittedAt <= 1 || admittedAt == 0 {
		t.Fatalf("epistemic sampling neither compressed nor reopened the learned-low source: admitted_at=%d", admittedAt)
	}
}

func TestCausalRealityCannotBeLearnedOutOfAttention(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	key := "observed/action_result/browser/click"
	payload, _ := json.Marshal(ActionState{ID: "action-1", OrganID: "browser", Operation: "click", Status: "completed"})
	digest := differenceDigest(Event{Kind: "action_result", Source: "observed", Summary: "done", Payload: payload})
	runtime.state.DifferenceField[key] = DifferenceTrace{
		Key: key, Observations: 20, LastDigest: digest, ExpectedChangeRate: 0,
		AttentionValue: 0,
	}
	if err := runtime.addEvent("action_result", "observed", "done", "action-1", payload, true); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].AttentionPressure < runtime.config.Dynamics.AttentionThreshold {
		t.Fatalf("a causally awaited Reality was suppressed: %#v", runtime.state.Background)
	}
}

func TestAliceAppraisalAndExperienceRetuneSignalValue(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	key := "observed/perceptual_change/browser/chrome.current_page"
	runtime.state.DifferenceField[key] = DifferenceTrace{Key: key, AttentionValue: 0.50}
	candidate := Event{ID: "fact", DifferenceKey: key}
	runtime.state.Background = []Event{candidate}
	runtime.learnDifferenceFromAppraisal(candidate, CandidateAppraisal{
		Ownership: 0.05, Value: 0, Answerability: 0, Certainty: 1,
	}, false)
	low := runtime.state.DifferenceField[key].AttentionValue
	if low >= 0.50 {
		t.Fatalf("Alice's release did not lower future attention access: %f", low)
	}
	commitment := ActionCommitment{FocusID: "fact"}
	experience := Experience{
		PredictionDifference: 0.9, ExperiencedCost: 0.1, Significance: "self_defining",
		Values: LifeValues{Agency: 0.8, SelfEndorsed: 0.9},
	}
	runtime.learnDifferenceFromExperience(commitment, experience)
	if got := runtime.state.DifferenceField[key].AttentionValue; got <= low {
		t.Fatalf("a consequential experience did not restore signal value: before=%f after=%f", low, got)
	}
}

func TestDifferenceAccumulationDecaysWithoutManufacturingEvent(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	key := "observed/perceptual_change/browser/chrome.current_page"
	runtime.state.DifferenceField[key] = DifferenceTrace{Key: key, Accumulated: 0.8, LastObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	runtime.decayDifferenceField(1)
	if got := runtime.state.DifferenceField[key].Accumulated; got >= 0.8 || got <= 0 {
		t.Fatalf("difference pressure did not decay continuously: %f", got)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatal("decay manufactured a cognitive event")
	}
}

func TestDifferenceFieldSurvivesOrdinaryProcessRestart(t *testing.T) {
	root := t.TempDir()
	config := testConfig(10)
	first, err := New(root, "same-life", config, &blockingCognizer{
		started: make(chan CognitiveRequest, 1), release: make(chan struct{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "observed/perceptual_change/browser/chrome.current_page"
	first.state.DifferenceField[key] = DifferenceTrace{
		Key: key, Observations: 7, ExpectedChangeRate: 0.75,
		Accumulated: 0.22, AttentionValue: 0.63, LastObservedAt: nowUTC(),
	}
	if err := first.persist(); err != nil {
		t.Fatal(err)
	}
	second, err := New(root, "same-life", config, &blockingCognizer{
		started: make(chan CognitiveRequest, 1), release: make(chan struct{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, exists := second.state.DifferenceField[key]
	if !exists || got.Observations != 7 || got.AttentionValue != 0.63 || got.Accumulated != 0.22 {
		t.Fatalf("ordinary restart lost pre-conscious learning: %#v", got)
	}
}

func TestDifferenceFieldRemainsBoundedWithoutDiscardingHigherValueTrace(t *testing.T) {
	runtime := newDifferenceTestRuntime(t)
	protectedKey := "observed/action_result/browser/protected"
	runtime.state.DifferenceField[protectedKey] = DifferenceTrace{
		Key: protectedKey, AttentionValue: 1, Observations: 4, LastObservedAt: "2026-01-01T00:00:00Z",
	}
	for index := 0; index < maximumDifferenceTraces+8; index++ {
		operation := fmt.Sprintf("unbounded-%03d", index)
		payload, err := json.Marshal(ActionState{OrganID: "browser", Operation: operation, Status: "failed"})
		if err != nil {
			t.Fatal(err)
		}
		runtime.admitDifference(Event{
			Kind: "action_result", Source: "observed", Summary: "failed",
			ObservedAt: fmt.Sprintf("2026-01-02T00:%02d:00Z", index%60), Payload: payload,
		})
	}
	if got := len(runtime.state.DifferenceField); got != maximumDifferenceTraces {
		t.Fatalf("difference field exceeded its physical bound: %d", got)
	}
	if _, exists := runtime.state.DifferenceField[protectedKey]; !exists {
		t.Fatal("bounded retention discarded a higher-value trace before lower-value families")
	}
}
