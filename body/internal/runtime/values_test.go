package runtime

import (
	"encoding/json"
	"fmt"
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

func TestStageTwentyCriticalBudgetSlowsOnlyEndogenousAttention(t *testing.T) {
	config := testConfig(20)
	config.CognitiveCore = "continuous-v1"
	config.Dynamics.AttentionMaximumIdleSeconds = 10
	config.Dynamics.AttentionRevisitSeconds = 10
	runtime, err := New(t.TempDir(), "resource-paced-attention", config, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.Body.CognitiveResourceBand = "critical"
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	runtime.state.ValueField.Satiation = LifeValueVector{}
	runtime.state.LastAttentionAt = now.Add(-30 * time.Second).Format(time.RFC3339Nano)
	if emitted, err := runtime.maybeEmitLifeValueSignal(); err != nil || emitted {
		t.Fatalf("critical budget reopened an idle value after only 30 seconds: emitted=%v err=%v", emitted, err)
	}
	runtime.state.LastAttentionAt = now.Add(-121 * time.Second).Format(time.RFC3339Nano)
	if emitted, err := runtime.maybeEmitLifeValueSignal(); err != nil || !emitted {
		t.Fatalf("critical budget permanently silenced embodied value: emitted=%v err=%v", emitted, err)
	}

	// New Reality remains causal and immediate regardless of the low-resource
	// cadence. The pacing applies only to timers that would reopen old material.
	runtime.state.Background = []Event{{ID: "fresh-reality", Kind: "mentor_received", Status: "pending", Summary: "一条刚到达的导师消息"}}
	runtime.state.LastAttentionAt = now.Format(time.RFC3339Nano)
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "fresh-reality" {
		t.Fatalf("fresh Reality was delayed by resource pacing: %#v", request)
	}
}

func TestWeakSatisfiedValueStaysInEmbodiedDynamics(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-below-consciousness", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Body = readyWebBody()
	runtime.state.ValueField.Activation = LifeValueVector{Vitality: 0.70, Relatedness: 0.72}
	runtime.state.ValueField.Satiation = LifeValueVector{Vitality: 0.55, Relatedness: 0.54}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("weak post-satiation attraction repeatedly recruited consciousness: %#v", runtime.state.Background)
	}
	if runtime.state.ValueField.Activation.Vitality <= 0 || runtime.state.ValueField.Activation.Relatedness <= 0 {
		t.Fatal("subconscious gating erased the live value field")
	}
}

func TestSubthresholdValueSignalDoesNotHabituateAnUnseenAffordance(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-unseen-affordance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	// Stay inside the bounded continuity fallback: this test isolates whether a
	// signal that did not reach awareness can create habituation.
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	runtime.state.DifferenceField["endogenous/value_signal/relatedness/mentor_channel"] = DifferenceTrace{
		Key: "endogenous/value_signal/relatedness/mentor_channel", Observations: 1,
		ExpectedChangeRate: 1, AttentionValue: 0,
	}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("subthreshold value signal entered consciousness: %#v", runtime.state.Background)
	}
	if _, exists := runtime.state.ValueAffordances["mentor_channel"]; exists {
		t.Fatal("an affordance was habituated before Alice consciously encountered it")
	}
	trace := runtime.state.DifferenceField["endogenous/value_signal/relatedness/mentor_channel"]
	if trace.Accumulated <= 0 {
		t.Fatal("unseen value pressure was lost instead of accumulating toward attention")
	}
}

func TestSatiatedLifeReopensAttentionWithoutLosingItsRhythm(t *testing.T) {
	config := testConfig(10)
	config.Dynamics.AttentionMaximumIdleSeconds = 10
	config.Dynamics.ValueIdleGrowth = 0.18
	config.Dynamics.ValueSatiationReturnRate = 0.15
	runtime, err := New(t.TempDir(), "value-rhythm", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Body = readyWebBody()
	runtime.state.Memories = append(runtime.state.Memories, Memory{
		ActionKind: "organ_action", Meaning: "我已经从一次真实接触中形成了可复用经验。",
		EnactedRequest: `{"organ_id":"browser","operation":"browser_snapshot","input":"{}"}`,
	})
	runtime.state.ValueField.Orientation = LifeValueVector{
		Continuance: 0.5812, Exploration: 0.6206, Agency: 0.5724,
		Vitality: 0.5518, Relatedness: 0.5809, Contribution: 0.5615,
	}
	// This is a deliberately well-satisfied state taken from the shape of a
	// completed lived encounter. Satisfaction should create a real pause, not
	// minutes of pre-conscious activity with no path back into the main stream.
	runtime.state.ValueField.Activation = LifeValueVector{
		Continuance: 0.731, Exploration: 0.748, Agency: 0.773,
		Vitality: 0.751, Relatedness: 0.697, Contribution: 0.761,
	}
	runtime.state.ValueField.Satiation = LifeValueVector{
		Continuance: 0.578, Exploration: 0.745, Agency: 0.838,
		Vitality: 0.708, Relatedness: 0.612, Contribution: 0.641,
	}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)

	for pulse := 0; pulse < 24 && len(runtime.state.Background) == 0; pulse++ {
		if err := runtime.advanceDynamics(5 * time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "value_signal" {
		t.Fatalf("a satisfied life did not regain a concrete attention direction within two minutes: %#v", runtime.state.Background)
	}
}

func TestIdleMemoryDoesNotReplayItselfWithoutPresentPressure(t *testing.T) {
	config := testConfig(10)
	config.Dynamics.AttentionMaximumIdleSeconds = 10
	runtime, err := New(t.TempDir(), "no-autonomous-recall", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Memories = append(runtime.state.Memories, Memory{
		ID: "lived-one", Meaning: "我曾经从一次具体行动中看见预测和现实的差异。", Lesson: "先核验结果。",
	})
	runtime.state.Mentor.Outbox = []MentorMessage{{MessageID: "already-in-flight", Status: "queued"}}
	runtime.state.ValueField.Activation = LifeValueVector{}
	runtime.state.ValueField.Satiation = LifeValueVector{
		Continuance: 1, Exploration: 1, Agency: 1, Vitality: 1, Relatedness: 1, Contribution: 1,
	}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	for _, event := range runtime.state.Background {
		if event.Kind == "lived_recall" {
			t.Fatalf("past memory replayed itself without a present bodily, environmental, relational or value cause: %#v", event)
		}
	}
}

func TestRelationalValueSignalCarriesCrossModalLivedMaterial(t *testing.T) {
	runtime, err := New(t.TempDir(), "situated-relation", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.95}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.Memories = append(runtime.state.Memories,
		Memory{ID: "talk-about-talk", ActionKind: "mentor_send", Meaning: "我曾经确认导师通道可以往返。"},
		Memory{ID: "world-memory", ActionKind: "organ_action", EnactedRequest: `{"organ_id":"browser"}`, Meaning: "我从公开世界中遇到了一个具体而陌生的事实。"},
	)
	if emitted, err := runtime.maybeEmitLifeValueSignal(); err != nil || !emitted {
		t.Fatalf("relational value signal was not emitted: emitted=%v err=%v", emitted, err)
	}
	var payload lifeValueSignalPayload
	if json.Unmarshal(runtime.state.Background[0].Payload, &payload) != nil {
		t.Fatal("value signal payload was not readable")
	}
	if payload.AffordanceKey != "mentor_channel" || payload.ContextMemoryID != "world-memory" {
		t.Fatalf("relationship channel recursively selected channel history instead of lived content: %#v", payload)
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
	if len(related) != 2 {
		t.Fatalf("a running app without a sensing organ became an actionable relational surface: %#v", related)
	}
	for _, affordance := range related {
		if affordance.Key == "public_web" || affordance.Key == "terminal_workspace" || affordance.Key == "x_account" {
			t.Fatalf("one-way information or a file surface was mislabeled as reciprocal relation: %#v", affordance)
		}
	}
	if related[1].Key != "x_social" {
		t.Fatalf("Alice's own publishing identity was not separated from the X social network: %#v", related)
	}
	runtime.state.Mentor.Outbox = []MentorMessage{{MessageID: "alice-unread", Status: "queued"}}
	related = runtime.lifeValueAffordances("relatedness")
	if len(related) != 2 || related[0].Key != "mentor_channel" || related[1].Key != "x_social" {
		t.Fatalf("an unread message removed an otherwise usable relationship surface: %#v", related)
	}
	runtime.state.Mentor.Outbox = nil
	for _, affordance := range runtime.lifeValueAffordances("vitality") {
		if affordance.Key == "mentor_channel" {
			t.Fatalf("generic vitality pressure independently reopened an already satisfied relationship: %#v", affordance)
		}
	}
	runtime.state.Body.Organs["desktop"] = OrganSnapshot{
		Name: "Desktop", Capabilities: []string{"observe", "perform", "desktop_ui"}, Status: "ready", Accepting: true,
	}
	related = runtime.lifeValueAffordances("relatedness")
	if len(related) != 3 || related[2].Key != "wechat" {
		t.Fatalf("a running client with a ready desktop organ did not become a real relational doorway: %#v", related)
	}
	contribution := runtime.lifeValueAffordances("contribution")
	if len(contribution) != 0 {
		t.Fatalf("an empty publishing effector became a contribution object by itself: %#v", contribution)
	}
	for _, direction := range []string{"continuance", "agency"} {
		if affordances := runtime.lifeValueAffordances(direction); len(affordances) != 0 {
			t.Fatalf("%s pressure turned a generic tool into a life object: %#v", direction, affordances)
		}
	}
	runtime.state.Memories = append(runtime.state.Memories, Memory{
		ActionKind: "organ_action", Meaning: "我从一个真实网页对象中形成了自己的理解。",
		EnactedRequest: `{"organ_id":"browser","operation":"browser_snapshot","input":"{}"}`,
	})
	for _, direction := range []string{"agency", "contribution"} {
		affordances := runtime.lifeValueAffordances(direction)
		if len(affordances) != 1 || affordances[0].Key != "lived_material" {
			t.Fatalf("%s could not meet Alice's actual lived material: %#v", direction, affordances)
		}
	}
}

func TestDeclinedLifeValueSurfaceHabituatesProgressively(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-surface-habituation", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.ValueAffordances["x_account"] = ValueAffordanceTrace{
		LastPresentedAt: now.Add(-55 * time.Second).Format(time.RFC3339Nano),
		LastSettledAt:   now.Add(-45 * time.Second).Format(time.RFC3339Nano),
		DismissedStreak: 1,
		EncounterStreak: 1,
	}
	if !runtime.lifeValueAffordanceHabituated("x_account", now) {
		t.Fatal("a declined stable surface returned at the ordinary acted interval")
	}
	if runtime.lifeValueAffordanceHabituated("mentor_channel", now) {
		t.Fatal("declining X incorrectly suppressed a different relational surface")
	}
	trace := runtime.state.ValueAffordances["x_account"]
	trace.DismissedStreak = 0
	runtime.state.ValueAffordances["x_account"] = trace
	if !runtime.lifeValueAffordanceHabituated("x_account", now) {
		t.Fatal("recent repeated use lost its proportional satiation")
	}
	if runtime.lifeValueAffordanceHabituated("x_account", now.Add(46*time.Second)) {
		t.Fatal("a used surface remained unavailable after its bounded encounter rhythm")
	}
}

func TestUsedLifeValueSurfaceCoolsFromSettlementAndCarriesEngagementSatiation(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-surface-settlement", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.ValueAffordances["wechat"] = ValueAffordanceTrace{
		LastPresentedAt:       now.Add(-7 * time.Minute).Format(time.RFC3339Nano),
		LastSettledAt:         now.Add(-time.Minute).Format(time.RFC3339Nano),
		LastEngagementSeconds: 6 * 60,
	}
	if !runtime.lifeValueAffordanceHabituated("wechat", now) {
		t.Fatal("a long encounter became fresh immediately after it settled")
	}
	if runtime.lifeValueAffordanceHabituated("wechat", now.Add(13*time.Minute)) {
		t.Fatal("engagement satiation made a real doorway permanently unavailable")
	}
}

func TestSuccessfulShortEncounterDoesNotExtinguishAGenerativeDoorway(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-surface-renewal", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 10
	now := time.Now().UTC()
	runtime.state.ValueAffordances["public_web"] = ValueAffordanceTrace{
		LastPresentedAt:       now.Add(-40 * time.Second).Format(time.RFC3339Nano),
		LastSettledAt:         now.Add(-20 * time.Second).Format(time.RFC3339Nano),
		LastEngagementSeconds: 20,
	}
	if !runtime.lifeValueAffordanceHabituated("public_web", now) {
		t.Fatal("a just-settled encounter had no proportional satiation")
	}
	if runtime.lifeValueAffordanceHabituated("public_web", now.Add(41*time.Second)) {
		t.Fatal("one successful object extinguished the whole generative doorway")
	}
}

func TestRepeatedEncountersHabituateTheSharedDoorway(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-contact-only", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 10
	now := time.Now().UTC()
	runtime.state.ValueAffordances["public_web"] = ValueAffordanceTrace{
		LastSettledAt: now.Add(-time.Minute).Format(time.RFC3339Nano), EncounterStreak: 2,
		LastEngagementSeconds: 20,
	}
	if !runtime.lifeValueAffordanceHabituated("public_web", now) {
		t.Fatal("repeated orientation-only contact returned before its adaptive satiation elapsed")
	}
	if runtime.lifeValueAffordanceHabituated("public_web", now.Add(31*time.Second)) {
		t.Fatal("encounter habituation permanently removed a generative doorway")
	}
}

func TestSelfReviewUsesPersonalLearningWithoutJudgingTheWholeResource(t *testing.T) {
	for _, sample := range []struct {
		name, judgment string
		value          float64
	}{
		{"productive_reading", "生态恢复的连续阅读帮助我理解物种与栖息地的关系。", 0.7},
		{"repeated_reload", "同一静态页面反复刷新没有新内容时，我应改变问题或接触方式。", -0.7},
	} {
		t.Run(sample.name, func(t *testing.T) {
			r, err := New(t.TempDir(), sample.name, testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			for i := 0; i < 2; i++ {
				focus, commitment, memory := fmt.Sprintf("focus-%d", i), fmt.Sprintf("commitment-%d", i), fmt.Sprintf("memory-%d", i)
				payload, _ := json.Marshal(lifeValueSignalPayload{Direction: "exploration", AffordanceKey: "public_web"})
				r.state.Background = append(r.state.Background, Event{ID: focus, Kind: "value_signal", Payload: payload, Status: "processed"})
				r.state.Commitments = append(r.state.Commitments, ActionCommitment{ID: commitment, FocusID: focus, ActionKind: "organ_action", Status: "assimilated", MemoryID: memory})
				r.state.Memories = append(r.state.Memories, Memory{ID: memory, CommitmentID: commitment, ActionKind: "organ_action", SourceKind: "action_result", Origin: "observed", Meaning: sample.judgment,
					EnactedRequest: `{"organ_id":"browser","operation":"browser_snapshot","input":"{}"}`, ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)})
			}
			before := ValueAffordanceTrace{EncounterStreak: 2, LastPresentedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), LastSettledAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
			r.state.ValueAffordances["public_web"] = before
			payload, _ := json.Marshal(map[string]any{"difference_kind": "attention_yield_balance", "evidence_memory_ids": []string{"memory-0", "memory-1"}})
			event := Event{ID: "review", Kind: "self_model_difference", Payload: payload, Status: "in_focus"}
			r.state.Background = append(r.state.Background, event)
			r.activeCandidates = map[string]Event{event.ID: event}
			commit := CognitiveCommit{
				FocusID: event.ID, ThoughtThread: sample.judgment,
				Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: sample.judgment, Difference: 0.4, Ownership: 0.9, Value: sample.value, Values: LifeValueVector{Exploration: sample.value}, Urgency: 0.2, Answerability: 0.7, Certainty: 0.8, Resolution: "hold"}},
				Action:     CognitiveAction{Kind: "none"}, ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
				MemoryUpdates:     []MemoryUpdate{{Content: sample.judgment, Origin: "reflected", SourceRefs: []string{event.ID}}},
				ExperienceUpdates: []ExperienceUpdate{{Judgment: sample.judgment, Context: sample.name, Evidence: []string{"new:0"}}},
			}
			if err := r.applyCognitiveCommit(commit); err != nil {
				t.Fatal(err)
			}
			if got := r.state.ValueAffordances["public_web"]; got != before {
				t.Fatalf("owning a review imposed a separate resource penalty: before=%#v after=%#v", before, got)
			}
			if r.state.SelfModelTension <= 0 || r.state.AffectiveState.Valence*sample.value <= 0 {
				t.Fatal("removing hidden regulation also erased the personally appraised tension or affect")
			}
			recalled := r.learning.recall(sample.judgment, "paired-review")
			found := false
			for _, experience := range recalled.Experiences {
				if experience.Judgment == sample.judgment && len(experience.Evidence) == 1 {
					found = true
				}
			}
			if !found {
				t.Fatal("the personally chosen revision did not remain available to subsequent cognition")
			}
		})
	}
}

func TestValueAffordanceFollowsConcernUntilItsActualSettlement(t *testing.T) {
	runtime, err := New(t.TempDir(), "value-surface-concern", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-6 * time.Minute)
	runtime.recordValueAffordancePresentation("wechat", start)
	payload, _ := json.Marshal(lifeValueSignalPayload{Direction: "relatedness", AffordanceKey: "wechat"})
	event := Event{Kind: "value_signal", Payload: payload}
	concern := Concern{ID: "wechat-concern", Resolution: "hold"}
	runtime.updateValueAffordanceDisposition(event, &concern, true, start.Format(time.RFC3339Nano))
	if runtime.state.ValueAffordances["wechat"].ActiveConcernID != concern.ID {
		t.Fatal("an adopted doorway lost the Concern carrying its encounter")
	}
	concern.Resolution = "resolved"
	runtime.state.Commitments = []ActionCommitment{{ConcernID: concern.ID, Status: "assimilated"}}
	settled := time.Now().UTC()
	runtime.updateValueAffordanceDisposition(Event{Kind: "action_result"}, &concern, false, settled.Format(time.RFC3339Nano))
	trace := runtime.state.ValueAffordances["wechat"]
	if trace.ActiveConcernID != "" || trace.LastSettledAt == "" || trace.LastEngagementSeconds < 5*60 {
		t.Fatalf("the doorway did not begin satiation at actual settlement: %#v", trace)
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
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9, Exploration: 0.8}
	runtime.state.ValueField.Satiation = LifeValueVector{Relatedness: 0.8}
	runtime.state.Body = readyWebBody()
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("expected one value signal, got %#v", runtime.state.Background)
	}
	if direction := eventLifeValueDirection(runtime.state.Background[0]); direction != "exploration" {
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
	runtime.satiateLifeValues(Memory{
		PredictionDifference: 0.1,
		Values:               LifeValues{Relatedness: 0.8, SelfEndorsed: 0.9},
		ExperiencedCost:      0.1,
	})
	satiated := runtime.state.ValueField.Satiation.Relatedness
	if satiated <= 0 {
		t.Fatal("a positively endorsed real memory did not create temporary satiation")
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
