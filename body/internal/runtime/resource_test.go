package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCognitiveCostUsesCachedInputWithoutDoubleCountingReasoning(t *testing.T) {
	model := testConfig(4).CognitiveResource.Models["main"]
	usage := apiUsage{
		InputTokens:  100,
		OutputTokens: 20,
		InputTokensDetails: apiInputTokensDetails{
			CachedTokens: 40,
		},
		OutputTokensDetails: apiOutputTokensDetails{ReasoningTokens: 15},
	}
	if got := usageCost(model, usage); got != 368 {
		t.Fatalf("usage cost = %d microUSD, want 368", got)
	}
}

func TestRollingHourAndDayBothGateReservations(t *testing.T) {
	config := testConfig(4).CognitiveResource
	now := time.Now().UTC()
	state := State{Usage: []UsageRecord{
		{Time: now.Add(-30 * time.Minute).Format(time.RFC3339Nano), ActualMicrousd: config.RollingHourLimitMicrousd - 100_000},
	}}
	if canReserve(state, config, 100_001, now) {
		t.Fatal("hour limit accepted an oversized reservation")
	}
	state.Usage = []UsageRecord{
		{Time: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), ActualMicrousd: config.RollingDayLimitMicrousd - 50_000},
	}
	if canReserve(state, config, 50_001, now) {
		t.Fatal("day limit accepted an oversized reservation")
	}
	if !canReserve(state, config, 50_000, now) {
		t.Fatal("reservation exactly reaching the day limit was rejected")
	}
}

func TestUnaffordablePreferredProfileFallsBackWithoutTakingAwayTheChoice(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.GenerationKind = "rehearsal"
	runtime.config.CognitiveResource.RollingHourLimitMicrousd = 100_000
	runtime.config.CognitiveResource.RollingDayLimitMicrousd = 100_000
	runtime.state.Usage = []UsageRecord{{Time: nowUTC(), ActualMicrousd: 95_000, Status: "completed", CostConfirmed: true}}
	runtime.state.Background = []Event{{ID: "resource-focus", Kind: "endogenous_change", Status: "in_focus"}}
	runtime.state.Lease = &Lease{
		ID: "lease-preferred", FocusID: "resource-focus", Profile: CognitiveProfile{Model: "main", ReasoningEffort: "medium"},
	}
	reservation := ModelReservation{
		Profile: runtime.state.Lease.Profile, InputTokenUpperBound: 6_000,
		ReservedMicrousd: reservationCost(runtime.config.CognitiveResource.Models["main"], 6_000, runtime.config.ModelGateway.MaxOutputTokens),
	}
	ack := make(chan NoticeAck, 1)
	if err := runtime.handleNotice(WorkerNotice{LeaseID: runtime.state.Lease.ID, Kind: "model_reserve", Payload: reservation, Ack: ack}); err != nil {
		t.Fatal(err)
	}
	result := <-ack
	if result.Accepted {
		t.Fatal("an unaffordable preferred profile was reserved")
	}
	next := runtime.state.CognitiveResource.NextProfile
	if next == nil || next.FocusID != "resource-focus" || next.Profile != (CognitiveProfile{Model: "fast", ReasoningEffort: "none"}) || next.Source != "resource_fallback" {
		t.Fatalf("the affordable metabolic fallback was not prepared: %#v", next)
	}
	if runtime.state.CognitiveResource.Limited != nil {
		t.Fatalf("an affordable fallback was incorrectly treated as total exhaustion: %#v", runtime.state.CognitiveResource.Limited)
	}
	if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{
		LeaseID: "lease-preferred", FocusID: "resource-focus",
		Error: &CognitiveResourceUnavailableError{RequiredMicrousd: reservation.ReservedMicrousd, Reason: result.Output},
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Background[0].Status != "pending" || runtime.state.CognitiveResource.NextProfile == nil {
		t.Fatalf("the same focus was not reopened for affordable cognition: state=%#v next=%#v", runtime.state.Background[0], runtime.state.CognitiveResource.NextProfile)
	}
	profile, source, _ := activeProfileDecision(runtime.state, runtime.config.CognitiveResource, "resource-focus")
	if profile.Model != "fast" || source != "resource_fallback" {
		t.Fatalf("the fallback fact was not exposed to the next cognition: profile=%#v source=%q", profile, source)
	}
}

func TestResourceLimitWaitReopensWhenRollingSpendExpires(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.Usage = []UsageRecord{{Time: now.Add(-61 * time.Minute).Format(time.RFC3339Nano), ActualMicrousd: 2_000_000}}
	runtime.state.Background = []Event{{ID: "event-1", Status: "resource_wait"}}
	runtime.state.CognitiveResource.Limited = &CognitiveResourceLimit{
		FocusID: "event-1", Profile: CognitiveProfile{Model: "main", ReasoningEffort: "medium"}, RequiredMicrousd: 100_000,
	}
	if err := runtime.releaseCognitiveResourceWaits(); err != nil {
		t.Fatal(err)
	}
	if runtime.state.CognitiveResource.Limited != nil || runtime.state.Background[0].Status != "pending" {
		t.Fatalf("resource wait was not reopened: %#v", runtime.state.CognitiveResource)
	}
}

func TestValidationFailureTemporarilyEscalatesOnlyTheCurrentFocus(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.GenerationKind = "formal"
	runtime.state.BirthBriefEnteredAt = nowUTC()
	runtime.state.PlannedEnd = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.ValueField.Activation.Exploration = 1
	runtime.state.CognitiveResource.DefaultProfile = CognitiveProfile{Model: "fast", ReasoningEffort: "low"}
	runtime.state.Concerns = []Concern{{
		ID: "exploration-focus", OriginKind: "endogenous_change", Meaning: "接触一个尚未确定意义的现实",
		Strength: 1, Activation: 1, Ownership: 1, Answerability: 1, Resolution: "hold",
	}}

	failedProfile := CognitiveProfile{Model: "fast", ReasoningEffort: "low"}
	for attempt := 1; attempt <= 2; attempt++ {
		leaseID := "lease-validation-" + string(rune('0'+attempt))
		runtime.state.Lease = &Lease{ID: leaseID, FocusID: "exploration-focus", Profile: failedProfile}
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			LeaseID: leaseID, Time: nowUTC(), RequestedModel: "fast", ReasoningEffort: "low",
			ActualMicrousd: 1000, CostConfirmed: true, Status: "completed",
		})
		if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{
			LeaseID: leaseID, FocusID: "exploration-focus", Error: errors.New("organ_action action requires a command"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	next := runtime.state.CognitiveResource.NextProfile
	if next == nil || next.FocusID != "exploration-focus" || next.Source != "validation_fallback" || next.Profile != (CognitiveProfile{Model: "main", ReasoningEffort: "medium"}) {
		t.Fatalf("repeated invalid cognition did not receive one stronger focus-scoped profile: %#v", next)
	}
	if got := runtime.state.CognitiveResource.DefaultProfile; got != failedProfile {
		t.Fatalf("temporary validation recovery rewrote Alice's default choice: %#v", got)
	}
}

func TestStrictExperimentalProfileDoesNotEscalateValidationFailure(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.CognitiveResource.DisableValidationFallback = true
	if recovered, err := runtime.planValidationRecovery("focus", CognitiveProfile{Model: "fast", ReasoningEffort: "none"}); err != nil || recovered {
		t.Fatalf("strict experimental profile received automatic validation fallback: recovered=%v err=%v", recovered, err)
	}
	if runtime.state.CognitiveResource.NextProfile != nil {
		t.Fatalf("strict experimental profile scheduled an unrequested next profile: %#v", runtime.state.CognitiveResource.NextProfile)
	}
}

func TestValidationExhaustionPreservesConcernButEndsRapidRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 1
	runtime.state.Concerns = []Concern{{
		ID: "held", OriginKind: "endogenous_change", Meaning: "仍属于我的问题",
		Strength: 1, Activation: 1, Ownership: 1, Answerability: 1, Resolution: "hold",
	}}
	if err := runtime.exhaustCognition("held"); err != nil {
		t.Fatal(err)
	}
	concern := runtime.state.Concerns[0]
	if concern.Meaning != "仍属于我的问题" || concern.Ownership != 1 || concern.Resolution != "hold" {
		t.Fatalf("mechanical exhaustion changed Alice's concern: %#v", concern)
	}
	if !cognitionValidationExhausted(concern, runtime.config.CognitiveResource) {
		t.Fatalf("exhausted concern remained eligible for paid retries: %#v", concern)
	}
	if got := runtime.currentExplorationConcernID(); got != "" {
		t.Fatalf("exhausted concern still monopolized the exploration drive: %q", got)
	}
	if request, ok := runtime.nextStage4Request(); ok && request.Focus.ID == concern.ID {
		t.Fatalf("exhausted concern re-entered attention without a new fact: %#v", request)
	}
}

func TestSuccessfulCognitionClearsValidationAttempts(t *testing.T) {
	state := State{Concerns: []Concern{{ID: "focus", CognitionAttempts: 3, LastCommitErr: "old failure"}}}
	markEvent(&state, "focus", "processed")
	if state.Concerns[0].CognitionAttempts != 0 || state.Concerns[0].LastCommitErr != "" {
		t.Fatalf("successful cognition retained obsolete validation failures: %#v", state.Concerns[0])
	}
}

func TestAliceCanChangeDefaultAndRequestOneSerialContinuation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.applyResourceChoice(
		CognitiveResourceChoice{Apply: "default", Model: "fast", ReasoningEffort: "low", Purpose: "日常感知更轻快"},
		CognitiveProfile{Model: "fast", ReasoningEffort: "low"},
		"event-1",
	); err != nil {
		t.Fatal(err)
	}
	if runtime.state.CognitiveResource.DefaultProfile.Model != "fast" {
		t.Fatalf("default profile did not change: %#v", runtime.state.CognitiveResource.DefaultProfile)
	}
	if err := runtime.applyResourceChoice(
		CognitiveResourceChoice{Apply: "next", Model: "high", ReasoningEffort: "high", Purpose: "把当前矛盾想清楚"},
		CognitiveProfile{Model: "high", ReasoningEffort: "high"},
		"event-1",
	); err != nil {
		t.Fatal(err)
	}
	next := runtime.state.CognitiveResource.NextProfile
	if next == nil || next.Profile.Model != "high" || next.FocusID == "" || len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "cognition_continuation" {
		t.Fatalf("serial continuation was not formed: next=%#v background=%#v", next, runtime.state.Background)
	}
	profile, source, purpose := activeProfileDecision(runtime.state, runtime.config.CognitiveResource, next.FocusID)
	if profile.Model != "high" || source != "next" || purpose != "把当前矛盾想清楚" {
		t.Fatalf("continuation choice lost its source or purpose: profile=%#v source=%q purpose=%q", profile, source, purpose)
	}
	runtime.state.Concerns = []Concern{{
		ID: "loud-concern", OriginKind: "endogenous_change", Strength: 1, Activation: 1,
		Ownership: 1, Answerability: 1, Resolution: "hold",
	}}
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != next.FocusID || request.Focus.Kind != "cognition_continuation" || len(request.Candidates) != 1 {
		t.Fatalf("serial continuation did not become the immediate next focus: %#v", request)
	}
}

func TestNextCanReuseEitherPartOfTheCurrentProfile(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Lease = &Lease{
		ID: "lease-terra", FocusID: "event-1",
		Profile: CognitiveProfile{Model: "main", ReasoningEffort: "medium"},
	}
	runtime.activeCandidates = map[string]Event{"event-1": {ID: "event-1", Kind: "body_delta"}}
	choice := CognitiveResourceChoice{
		Apply: "next", Model: "current", ReasoningEffort: "current", Purpose: "用同样的档位吸收紧接现实",
	}
	profile, err := runtime.validateResourceChoice(choice, "event-1", "none")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model != "main" || profile.ReasoningEffort != "medium" {
		t.Fatalf("current profile reference resolved incorrectly: %#v", profile)
	}
	if err := runtime.applyResourceChoice(choice, profile, "event-1"); err != nil {
		t.Fatal(err)
	}
	if next := runtime.state.CognitiveResource.NextProfile; next == nil || next.Profile != profile || next.Source != "next" {
		t.Fatalf("current profile was not scheduled for one causal step: %#v", next)
	}

	partial := CognitiveResourceChoice{
		Apply: "default", Model: "current", ReasoningEffort: "low", Purpose: "日常思考更轻快",
	}
	profile, err = runtime.validateResourceChoice(partial, "event-1", "none")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model != "main" || profile.ReasoningEffort != "low" {
		t.Fatalf("partial current profile reference resolved incorrectly: %#v", profile)
	}
}

func TestKeepPreservesThePersistentDefaultAfterAOneUseProfile(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.CognitiveResource.DefaultProfile = CognitiveProfile{Model: "main", ReasoningEffort: "medium"}
	runtime.state.Lease = &Lease{
		ID: "lease-luna", FocusID: "reality", Profile: CognitiveProfile{Model: "fast", ReasoningEffort: "low"}, ProfileSource: "next",
	}
	choice := CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current", Purpose: "让当前轻量档位继续"}
	profile, err := runtime.validateResourceChoice(choice, "reality", "none")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.applyResourceChoice(choice, profile, "reality"); err != nil {
		t.Fatal(err)
	}
	if got := runtime.state.CognitiveResource.DefaultProfile; got.Model != "main" || got.ReasoningEffort != "medium" {
		t.Fatalf("a one-use profile silently replaced the persistent default: %#v", got)
	}
	if profile, source, _ := activeProfileDecision(runtime.state, runtime.config.CognitiveResource, "new-focus"); profile.Model != "main" || source != "default" {
		t.Fatalf("the next ordinary focus did not return to the persistent default: profile=%#v source=%q", profile, source)
	}
}

func TestOpenResourcesKeepTheCapabilityFirstPersistentBaseline(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Body.CognitiveResourceBand = "open"
	runtime.state.Lease = &Lease{ID: "lease", FocusID: "event", Profile: CognitiveProfile{Model: "main", ReasoningEffort: "medium"}}
	runtime.activeCandidates = map[string]Event{"event": {ID: "event", Kind: "environment_change"}}
	if _, err := runtime.validateResourceChoice(
		CognitiveResourceChoice{Apply: "default", Model: "fast", ReasoningEffort: "low", Purpose: "节省日常成本"},
		"event", "none",
	); err == nil {
		t.Fatal("open resources allowed an ungrounded persistent downgrade below the birth baseline")
	}
	if _, err := runtime.validateResourceChoice(
		CognitiveResourceChoice{Apply: "next", Model: "fast", ReasoningEffort: "low", Purpose: "一次边界清楚的轻量判断"},
		"event", "none",
	); err != nil {
		t.Fatalf("open resources prevented a bounded lower-cost choice: %v", err)
	}
}

func TestContinuationCanScheduleRealityAbsorptionOnlyWhenItActs(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Lease = &Lease{
		ID: "lease-continuation", FocusID: "continuation",
		Profile: CognitiveProfile{Model: "main", ReasoningEffort: "medium"},
	}
	runtime.activeCandidates = map[string]Event{
		"continuation": {ID: "continuation", Kind: "cognition_continuation", ConcernID: "concern-1"},
	}
	choice := CognitiveResourceChoice{
		Apply: "next", Model: "current", ReasoningEffort: "current", Purpose: "吸收即将返回的行动现实",
	}
	if _, err := runtime.validateResourceChoice(choice, "continuation", "none"); err == nil {
		t.Fatal("a thought-only continuation was allowed to recurse without new reality")
	}
	profile, err := runtime.validateResourceChoice(choice, "continuation", "organ_action")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model != "main" || profile.ReasoningEffort != "medium" {
		t.Fatalf("continuation action lost the current profile: %#v", profile)
	}
}

func TestStageTenAssistanceUsesExplicitlyContinuedConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "owned-concern", Strength: 0.4, Resolution: "hold"}}
	runtime.state.Lease = &Lease{
		ID: "lease", FocusID: "mentor-reply", Profile: CognitiveProfile{Model: "main", ReasoningEffort: "none"},
	}
	runtime.activeCandidates = map[string]Event{
		"mentor-reply": {ID: "mentor-reply", Kind: "mentor_received", ConcernID: "settled-old-concern"},
	}
	choice := CognitiveResourceChoice{
		Apply: "next", Model: "high", ReasoningEffort: "low", Purpose: "实现已认领且目标固定的身体动作",
	}
	profile, err := runtime.validateResourceChoice(choice, "mentor-reply", "none", "owned-concern")
	if err != nil {
		t.Fatalf("explicit concern continuation was ignored by resource validation: %v", err)
	}
	if err := runtime.applyResourceChoice(choice, profile, "mentor-reply", "owned-concern"); err != nil {
		t.Fatal(err)
	}
	continuation := runtime.state.Background[len(runtime.state.Background)-1]
	if continuation.Kind != "cognition_continuation" || continuation.ConcernID != "owned-concern" {
		t.Fatalf("assistance continuation split from the adopted concern: %#v", continuation)
	}
}

func TestSerialContinuationKeepsTheCurrentConcernIdentity(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "current-concern", Strength: 0.3, Resolution: "hold"}}
	runtime.state.Background = []Event{{ID: "event-1", Kind: "action_result", Status: "processed", ConcernID: "current-concern"}}
	if err := runtime.applyResourceChoice(
		CognitiveResourceChoice{Apply: "next", Model: "fast", ReasoningEffort: "low", Purpose: "继续理解同一段现实"},
		CognitiveProfile{Model: "fast", ReasoningEffort: "low"},
		"event-1",
	); err != nil {
		t.Fatal(err)
	}
	continuation := runtime.state.Background[len(runtime.state.Background)-1]
	if continuation.Kind != "cognition_continuation" || continuation.ConcernID != "current-concern" {
		t.Fatalf("continuation split from its causal concern: %#v", continuation)
	}
	runtime.activeCandidates = map[string]Event{continuation.ID: continuation}
	commit := CognitiveCommit{
		FocusID: continuation.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: continuation.ID, Meaning: "我继续理解同一段现实", Difference: 0.4,
			Ownership: 0.8, Value: 0.6, Urgency: 0.2, Answerability: 0.7, Certainty: 0.9, Resolution: "hold",
		}},
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != "current-concern" {
		t.Fatalf("continuation manufactured another Concern: %#v", runtime.state.Concerns)
	}
}

func TestActionRealityConsumesNextProfileInsteadOfAddingPostRealityThought(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	continuation := Event{
		ID: "continuation", Kind: "cognition_continuation", Status: "pending", ConcernID: "current-concern",
	}
	reality := Event{ID: "reality", Kind: "action_result", Status: "pending", ConcernID: "current-concern"}
	runtime.state.Background = []Event{continuation, reality}
	runtime.state.CognitiveResource.NextProfile = &NextCognitiveProfile{
		FocusID: continuation.ID, Purpose: "用轻量认知吸收结果",
		Profile: CognitiveProfile{Model: "fast", ReasoningEffort: "low"},
	}
	if err := runtime.bindNextProfileToReality("current-concern", reality.ID); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Background[0].Status != "processed" {
		t.Fatalf("superseded continuation remained pending: %#v", runtime.state.Background[0])
	}
	next := runtime.state.CognitiveResource.NextProfile
	if next == nil || next.FocusID != reality.ID {
		t.Fatalf("next profile was not bound to Reality: %#v", next)
	}
	profile, source, purpose := activeProfileDecision(runtime.state, runtime.config.CognitiveResource, reality.ID)
	if profile.Model != "fast" || source != "next" || purpose != "用轻量认知吸收结果" {
		t.Fatalf("Reality did not receive Alice's chosen profile: profile=%#v source=%q purpose=%q", profile, source, purpose)
	}
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != reality.ID || request.Focus.Kind != "action_result" {
		t.Fatalf("Reality lost causal priority after profile binding: %#v", request)
	}
}

func TestRepeatedPaidUnusableResponsesTemporarilyProtectModel(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time:           time.Now().UTC().Add(-time.Duration(index) * time.Minute).Format(time.RFC3339Nano),
			RequestedModel: "main", ActualMicrousd: 1, Status: "unusable", CostConfirmed: true,
		})
	}
	protected, err := runtime.protectModelAfterFailures("main")
	if err != nil {
		t.Fatal(err)
	}
	if !protected {
		t.Fatal("repeated paid unusable responses did not protect the model")
	}
	if active, _ := modelProtected(runtime.state, "main", time.Now().UTC()); !active {
		t.Fatal("model protection was not active")
	}
}

func TestRepeatedModelSpecificGatewayFailuresProtectOnceAndOfferRecovery(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time:           time.Now().UTC().Add(-time.Duration(2-index) * time.Second).Format(time.RFC3339Nano),
			RequestedModel: "main", Status: "failure_cost_unconfirmed", FailureCategory: "function_call_not_supported",
		})
	}
	protected, err := runtime.protectModelAfterFailures("main")
	if err != nil || !protected {
		t.Fatalf("model-specific gateway failures did not protect the model: %v", err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "cognitive_resource_change" {
		t.Fatalf("model protection did not create exactly one body fact: %#v", runtime.state.Background)
	}
	if _, err := runtime.protectModelAfterFailures("main"); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("one protected model created duplicate facts: %#v", runtime.state.Background)
	}
	profile, ok := runtime.recoveryProfile("main")
	if !ok || profile.Model != "high" || profile.ReasoningEffort != "low" {
		t.Fatalf("the bounded action-capable recovery cognition was unavailable: %#v", profile)
	}
}

func TestStageTwentyCoalescesRollingBandOscillationWithoutLosingBodyTruth(t *testing.T) {
	config := testConfig(20)
	config.CognitiveCore = "continuous-v1"
	config.CognitiveResource.RollingHourLimitMicrousd = 1000
	config.CognitiveResource.RollingDayLimitMicrousd = 10000
	runtime, err := New(t.TempDir(), "resource-oscillation", config, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.Body.CognitiveResourceBand = "open"
	runtime.state.Usage = []UsageRecord{{CallID: "first", Time: now.Format(time.RFC3339Nano), ActualMicrousd: 260}}
	if err := runtime.refreshResourceBody(now); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Body.CognitiveResourceBand != "comfortable" || len(runtime.state.Background) != 1 {
		t.Fatalf("first degradation was not a visible bodily fact: body=%#v background=%#v", runtime.state.Body, runtime.state.Background)
	}

	// A nearby spend crosses another boundary. Keep the exact state and journal
	// record, but do not manufacture another paid attention object.
	runtime.state.Usage[0].ActualMicrousd = 910
	if err := runtime.refreshResourceBody(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Body.CognitiveResourceBand != "critical" || len(runtime.state.Background) != 1 || runtime.state.EventSeq != 2 {
		t.Fatalf("nearby oscillation was not coalesced: body=%#v background=%#v seq=%d", runtime.state.Body, runtime.state.Background, runtime.state.EventSeq)
	}

	// Recovery is available in the body view and wakes any affordability wait by
	// its separate mechanism; it does not become another attention object.
	runtime.state.Usage = nil
	if err := runtime.refreshResourceBody(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Body.CognitiveResourceBand != "open" || len(runtime.state.Background) != 1 || runtime.state.EventSeq != 3 {
		t.Fatalf("resource recovery was not retained without a paid focus: body=%#v background=%#v seq=%d", runtime.state.Body, runtime.state.Background, runtime.state.EventSeq)
	}

	// After the refractory window, a new degradation may become conscious again.
	runtime.state.Body.CognitiveResourceBand = "scarce"
	runtime.state.Usage = []UsageRecord{{CallID: "later", Time: now.Add(6 * time.Minute).Format(time.RFC3339Nano), ActualMicrousd: 950}}
	if err := runtime.refreshResourceBody(now.Add(6 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 2 || runtime.state.EventSeq != 4 {
		t.Fatalf("later independent degradation stayed hidden: background=%#v seq=%d", runtime.state.Background, runtime.state.EventSeq)
	}
}

func TestSharedInfrastructureFailureDoesNotProtectOrSwitchModel(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time:           time.Now().UTC().Add(-time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			RequestedModel: "fast", Status: "failure_cost_unconfirmed", FailureCategory: "upstream_unavailable",
		})
	}
	protected, err := runtime.protectModelAfterFailures("fast")
	if err != nil {
		t.Fatal(err)
	}
	if protected {
		t.Fatal("a shared infrastructure interruption was attributed to Luna's cognitive ability")
	}
}
