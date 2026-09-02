package runtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLifeValueFieldStartsFromGenesisOrientation(t *testing.T) {
	config := testConfig(10)
	config.Seed.ValueOrientation = LifeValueVector{
		Continuance: 0.58, Exploration: 0.62, Agency: 0.57,
		Vitality: 0.55, Relatedness: 0.58, Contribution: 0.56,
	}
	runtime, err := New(t.TempDir(), "value-seed", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.state.ValueField.Orientation != config.Seed.ValueOrientation {
		t.Fatalf("genesis orientation did not enter the persistent value field: %#v", runtime.state.ValueField)
	}
	if runtime.state.ValueField.Activation.Exploration <= 0 {
		t.Fatal("the initial exploration direction had no live activation")
	}
	for name, activation := range map[string]float64{
		"continuance":  runtime.state.ValueField.Activation.Continuance,
		"agency":       runtime.state.ValueField.Activation.Agency,
		"vitality":     runtime.state.ValueField.Activation.Vitality,
		"relatedness":  runtime.state.ValueField.Activation.Relatedness,
		"contribution": runtime.state.ValueField.Activation.Contribution,
	} {
		if activation <= 0 {
			t.Fatalf("the %s direction began as a dead configured value", name)
		}
	}
}

func TestUnsatisfiedNonExplorationValueCanEnterAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-signal", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	runtime.state.ValueField.Satiation = LifeValueVector{}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "value_signal" {
		t.Fatalf("an unsatisfied value did not become one interoceptive candidate: %#v", runtime.state.Background)
	}
	if direction := eventLifeValueDirection(runtime.state.Background[0]); direction != "relatedness" {
		t.Fatalf("the signal lost its causal value direction: %q", direction)
	}
	var payload lifeValueSignalPayload
	if json.Unmarshal(runtime.state.Background[0].Payload, &payload) != nil || payload.AffordanceKey == "" || payload.Surface == "" {
		t.Fatalf("the internal value was presented without a real environmental counterpart: %#v", payload)
	}
	if payload.AffordanceKey != "mentor_channel" {
		t.Fatalf("social relatedness was paired with a non-relational surface: %#v", payload)
	}
}

func TestValueAffordancesRespectEcologicalMeaning(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-affordances", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Body = readyWebBody()
	runtime.state.Body.DesktopAvailable = true
	runtime.state.Body.WechatRunning = true
	related := runtime.lifeValueAffordances("relatedness")
	if len(related) != 3 {
		t.Fatalf("expected mentor, X and WeChat as relational surfaces, got %#v", related)
	}
	for _, affordance := range related {
		if affordance.Key == "public_web" || affordance.Key == "terminal_workspace" || affordance.Key == "x_account" {
			t.Fatalf("one-way information or a file surface was mislabeled as reciprocal relation: %#v", affordance)
		}
	}
	if related[1].Key != "x_social" {
		t.Fatalf("Alice's own publishing identity was not separated from the X social network: %#v", related)
	}
	contribution := runtime.lifeValueAffordances("contribution")
	if len(contribution) != 1 || contribution[0].Key != "x_account" {
		t.Fatalf("contribution confused an effector with a concrete public surface: %#v", contribution)
	}
	for _, direction := range []string{"continuance", "agency"} {
		if affordances := runtime.lifeValueAffordances(direction); len(affordances) != 0 {
			t.Fatalf("%s pressure turned a generic tool into a life object: %#v", direction, affordances)
		}
	}
}

func TestDeclinedLifeValueSurfaceHabituatesProgressively(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-surface-habituation", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	encoded, _ := json.Marshal(lifeValueSignalPayload{Direction: "relatedness", AffordanceKey: "x_account"})
	runtime.state.Background = []Event{{
		ID: "x-signal", Kind: "value_signal", ObservedAt: now.Add(-6 * time.Minute).Format(time.RFC3339Nano), Payload: encoded,
	}}
	base := 5 * time.Minute
	if !runtime.lifeValueAffordanceHabituated("x_account", now, base) {
		t.Fatal("a declined stable surface returned at the ordinary acted interval")
	}
	if runtime.lifeValueAffordanceHabituated("mentor_channel", now, base) {
		t.Fatal("declining X incorrectly suppressed a different relational surface")
	}
	runtime.state.Commitments = []ActionCommitment{{FocusID: "x-signal"}}
	if runtime.lifeValueAffordanceHabituated("x_account", now, base) {
		t.Fatal("a surface Alice actually used did not return at the ordinary interval")
	}
}

func TestIdleValueGrowthKeepsAnInteriorBalance(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-balance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.ValueIdleGrowth = 0.18
	runtime.state.ValueField.Orientation = LifeValueVector{
		Continuance: 0.58, Exploration: 0.62, Agency: 0.57,
		Vitality: 0.55, Relatedness: 0.58, Contribution: 0.56,
	}
	runtime.state.ValueField.Activation = LifeValueVector{}
	for range 60 {
		runtime.decayLifeValueField(1)
		runtime.accumulateIdleLifeValues(1)
	}
	for index, activation := range lifeValueVectorValues(runtime.state.ValueField.Activation) {
		if activation <= 0.5 || activation >= 0.8 {
			t.Fatalf("dimension %d failed to settle inside a live unsaturated range: %#v", index, runtime.state.ValueField.Activation)
		}
	}
	if runtime.state.ValueField.Activation.Exploration <= runtime.state.ValueField.Activation.Vitality {
		t.Fatalf("orientation differences disappeared at equilibrium: %#v", runtime.state.ValueField.Activation)
	}
}

func TestRecentSatiationChangesWhichValueCanSignal(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-satiation-signal", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9, Contribution: 0.8}
	runtime.state.ValueField.Satiation = LifeValueVector{Relatedness: 0.8}
	runtime.state.Body = readyWebBody()
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("expected one value signal, got %#v", runtime.state.Background)
	}
	if direction := eventLifeValueDirection(runtime.state.Background[0]); direction != "contribution" {
		t.Fatalf("recent relational satisfaction did not redirect current pressure: %q", direction)
	}
}

func TestNearThresholdValuesEnterOneSharedCompetition(t *testing.T) {
	values := []namedLifeValue{
		{Name: "continuance", Pressure: 0.451},
		{Name: "relatedness", Pressure: 0.447},
		{Name: "contribution", Pressure: 0.443},
		{Name: "vitality", Pressure: 0.36},
	}
	competitive := competitiveLifeValues(values, 0.45)
	if len(competitive) != 3 {
		t.Fatalf("near-threshold directions were excluded before shared competition: %#v", competitive)
	}
	for _, value := range competitive {
		if value.Name == "vitality" {
			t.Fatalf("a clearly weaker direction entered the competition: %#v", competitive)
		}
	}
	if got := competitiveLifeValues(values[1:], 0.45); len(got) != 0 {
		t.Fatalf("attention opened before any direction reached the common threshold: %#v", got)
	}
}

func TestPassiveExplorationRequiresDistinctDominance(t *testing.T) {
	field := LifeValueField{Activation: LifeValueVector{Exploration: 0.8, Relatedness: 0.77}}
	if explorationDominatesValuePressure(field) {
		t.Fatal("a near-equal relational pressure was silenced by passive exploration")
	}
	field.Activation.Relatedness = 0.6
	if !explorationDominatesValuePressure(field) {
		t.Fatal("clearly dominant exploration could not reach its reality sensor")
	}
}

func TestLifeValueMeaningChangesAttentionWithoutChoosingAnAction(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-attention", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Orientation = LifeValueVector{Relatedness: 0.9, Agency: 0.1}
	runtime.state.Concerns = []Concern{
		{ID: "relation", Strength: 0.4, Activation: 0.3, Answerability: 0.7, Values: LifeValueVector{Relatedness: 1}},
		{ID: "ability", Strength: 0.4, Activation: 0.3, Answerability: 0.7, Values: LifeValueVector{Agency: 1}},
	}
	relation := Event{ID: "relation", Kind: "concern", ConcernID: "relation"}
	ability := Event{ID: "ability", Kind: "concern", ConcernID: "ability"}
	if runtime.candidateScore(relation) <= runtime.candidateScore(ability) {
		t.Fatal("Alice's current value orientation did not affect attention competition")
	}
}

func TestLifeValueActivationSatiationAndReturnAreDistinct(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-motion", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.activateLifeValues(CandidateAppraisal{
		Difference: 0.8, Ownership: 0.9,
		Values: LifeValueVector{Relatedness: 1},
	})
	activated := runtime.state.ValueField.Activation.Relatedness
	if activated <= 0 {
		t.Fatal("a self-owned relationship meaning did not activate its value direction")
	}
	runtime.satiateLifeValues(Experience{
		PredictionDifference: 0.1,
		Values:               LifeValues{Relatedness: 0.8, SelfEndorsed: 0.9},
		ExperiencedCost:      0.1,
	})
	satiated := runtime.state.ValueField.Satiation.Relatedness
	if satiated <= 0 {
		t.Fatal("a positively endorsed real experience did not create temporary satiation")
	}
	runtime.decayLifeValueField(time.Minute.Minutes())
	if runtime.state.ValueField.Activation.Relatedness >= activated || runtime.state.ValueField.Satiation.Relatedness >= satiated {
		t.Fatalf("fast value state did not return over time: %#v", runtime.state.ValueField)
	}
}

func TestLifeValueOrientationChangesSlowlyAndSurvivesStorage(t *testing.T) {
	root := t.TempDir()
	runtime, err := New(root, "value-persistence", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Orientation = LifeValueVector{Relatedness: 0.5}
	runtime.applyValueOrientationUpdate(LifeValueVector{Relatedness: 1})
	if got := runtime.state.ValueField.Orientation.Relatedness; got != 0.53 {
		t.Fatalf("orientation update was not slow and bounded: %f", got)
	}
	if err := runtime.store.Save(&runtime.state); err != nil {
		t.Fatal(err)
	}
	loaded, err := runtime.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ValueField.Orientation.Relatedness != 0.53 {
		t.Fatalf("value orientation did not survive restart storage: %#v", loaded.ValueField)
	}
}
