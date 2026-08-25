package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type blockingCognizer struct {
	started chan CognitiveRequest
	release chan struct{}
}

func (b *blockingCognizer) Run(ctx context.Context, request CognitiveRequest, _ chan<- WorkerNotice) CognitiveResult {
	b.started <- request
	select {
	case <-b.release:
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Text: "done"}
	case <-ctx.Done():
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: ctx.Err()}
	}
}

func testConfig(stage int) Config {
	config := Config{
		Stage: stage,
		Pulse: PulseConfig{IntervalSeconds: 5, SlowScanSeconds: 60},
		Model: ModelConfig{BaseURL: "https://example.invalid", Name: "test", MaxOutputTokens: 100},
		Quota: QuotaConfig{LimitTokens: 1_000_000, WindowMins: 60},
	}
	if stage == 4 {
		config.Dynamics = Dynamics{
			AffectReturnRate:           0.10,
			ConcernBaseDrive:           0.15,
			ConcernUrgencyWeight:       0.85,
			ConcernGrowthGain:          0.25,
			ConcernResolutionGain:      0.40,
			ConcernNaturalDecayRate:    0.02,
			AttentionAffectWeight:      0.30,
			AttentionExplorationWeight: 0.20,
			AttentionNoveltyWeight:     0.15,
			AttentionCostWeight:        0.25,
			AttentionThreshold:         0.45,
			AttentionCandidateLimit:    3,
			AttentionRevisitSeconds:    300,
			ExplorationIdleGrowth:      0.04,
			ExplorationUnknownGrowth:   0.10,
			ExplorationRelief:          0.45,
		}
	}
	return config
}

func TestStoreRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := State{Schema: stateSchema, InstanceID: "test", Stage: 3, Revision: 7, Mentor: MentorState{Received: map[string]uint64{"m1": 2}}}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Revision != 7 || loaded.Mentor.Received["m1"] != 2 {
		t.Fatalf("unexpected loaded state: %#v", loaded)
	}
	if matches, _ := filepath.Glob(filepath.Join(store.root, "state", "*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary state files remain: %v", matches)
	}
}

func TestStoreRecoversSequenceAlreadyFsyncedToJournal(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := State{Schema: stateSchema, InstanceID: "test", Stage: 3, EventSeq: 1, Mentor: MentorState{Received: map[string]uint64{}}}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(JournalRecord{Seq: 2, Time: nowUTC(), Kind: "crash_edge", InstanceID: "test"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EventSeq != 2 {
		t.Fatalf("recovered event sequence = %d, want 2", loaded.EventSeq)
	}
}

func TestStoreRestoresStageFourDynamics(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		Schema:              stateSchema,
		InstanceID:          "stage-four",
		Stage:               4,
		AffectiveState:      AffectiveState{Valence: 0.4, Activation: 0.7, Control: 0.6, Certainty: 0.8},
		ExplorationPressure: 0.55,
		Concerns:            []Concern{{ID: "concern-1", Meaning: "持续理解身体", Strength: 0.63}},
		Mentor:              MentorState{Received: map[string]uint64{}},
	}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ExplorationPressure != 0.55 || loaded.AffectiveState.Activation != 0.7 || len(loaded.Concerns) != 1 || loaded.Concerns[0].ID != "concern-1" {
		t.Fatalf("stage-four dynamics did not survive restart: %#v", loaded)
	}
}

func TestBodyDifferenceGate(t *testing.T) {
	base := BodySnapshot{ObservedAt: nowUTC(), UptimeSeconds: 100, RootFreeBytes: 10 << 30, AgentFreeBytes: 5 << 30, QuotaRemaining: 1_000_000, NetworkAvailable: true}
	same := base
	same.ObservedAt = nowUTC()
	same.RootFreeBytes -= diskEventThreshold / 2
	if got := bodyDifferences(base, same, false); len(got) != 0 {
		t.Fatalf("small jitter became an event: %v", got)
	}
	changed := same
	changed.RootFreeBytes = base.RootFreeBytes - diskEventThreshold
	changed.NetworkAvailable = false
	got := bodyDifferences(base, changed, false)
	if len(got) != 2 {
		t.Fatalf("expected two material differences, got %v", got)
	}
	quotaJitter := base
	quotaJitter.QuotaUsedTokens = 100_000
	quotaJitter.QuotaRemaining = 900_000
	if got := bodyDifferences(base, quotaJitter, false); len(got) != 0 {
		t.Fatalf("routine model use became a self-triggering body event: %v", got)
	}
	quotaChange := base
	quotaChange.QuotaUsedTokens = 260_000
	quotaChange.QuotaRemaining = 740_000
	if got := bodyDifferences(base, quotaChange, false); len(got) != 1 {
		t.Fatalf("quota resource-band crossing was not observed: %v", got)
	}
}

func TestOnlyOneCognitionLeaseStarts(t *testing.T) {
	cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 2), release: make(chan struct{})}
	runtime, err := New(t.TempDir(), "instance", testConfig(3), cognizer)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m2"} {
		payload, _ := json.Marshal(MentorInput{MessageID: id, Body: id})
		if err := runtime.addEvent("mentor_received", "observed", id, id, payload, true); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.maybeStartCognition(ctx)
	first := <-cognizer.started
	runtime.maybeStartCognition(ctx)
	select {
	case second := <-cognizer.started:
		t.Fatalf("second cognition started while lease active: %s", second.Lease.ID)
	case <-time.After(50 * time.Millisecond):
	}
	if runtime.state.Lease == nil || runtime.state.Lease.ID != first.Lease.ID {
		t.Fatal("active lease was not preserved")
	}
	close(cognizer.release)
}

func TestTransientCognitionFailureReturnsToAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(3), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Background = []Event{{ID: "event-1", Kind: "mentor_received", Status: "retry_wait", LastFocusedAt: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)}}
	runtime.releaseRetryableEvents()
	if runtime.state.Background[0].Status != "pending" {
		t.Fatalf("transiently failed event never returned: %#v", runtime.state.Background[0])
	}
	runtime.state.Background[0].Status = "retry_wait"
	runtime.state.Background[0].LastFocusedAt = nowUTC()
	runtime.releaseRetryableEvents()
	if runtime.state.Background[0].Status != "retry_wait" {
		t.Fatalf("event retried without a quiet interval: %#v", runtime.state.Background[0])
	}
}

func TestCommitValidationFeedbackSurvivesUntilSuccessfulRetry(t *testing.T) {
	state := State{Background: []Event{{ID: "event-1", Status: "in_focus"}}}
	markEventForRetry(&state, "event-1", "focus must be a current candidate")
	if state.Background[0].Status != "retry_wait" || state.Background[0].LastCommitErr == "" {
		t.Fatalf("commit validation feedback was lost: %#v", state.Background[0])
	}
	markEvent(&state, "event-1", "processed")
	if state.Background[0].LastCommitErr != "" {
		t.Fatalf("successful retry retained stale commit feedback: %#v", state.Background[0])
	}
}

func TestInterruptedActionBecomesUnknownRealityWithoutAutomaticRetry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		Schema:     stateSchema,
		InstanceID: "instance",
		Stage:      3,
		Mentor:     MentorState{Received: map[string]uint64{}},
		Lease:      &Lease{ID: "lease-old", FocusID: "event-old"},
		PendingAction: &ActionState{
			ID: "action-old", LeaseID: "lease-old", Kind: "body_shell", Status: "started",
		},
	}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(root, "instance", testConfig(3), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.recoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Lease != nil {
		t.Fatal("interrupted lease survived recovery")
	}
	if runtime.state.PendingAction != nil {
		t.Fatalf("unknown action blocked the action slot: %#v", runtime.state.PendingAction)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("stage three must not automatically focus unknown action reality: %#v", runtime.state.Background)
	}
	data, err := os.ReadFile(filepath.Join(root, "journal", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"kind":"action_unknown"`) || !strings.Contains(string(data), `"kind":"action_result"`) {
		t.Fatalf("unknown action reality was not journaled: %s", data)
	}
}

func TestRollingUsageWindow(t *testing.T) {
	records := []UsageRecord{
		{Time: time.Now().UTC().Add(-61 * time.Minute).Format(time.RFC3339Nano), TotalTokens: 100},
		{Time: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano), TotalTokens: 250},
	}
	if got := usageInWindow(records, 60); got != 250 {
		t.Fatalf("usage window = %d, want 250", got)
	}
}

func TestAppraisalActivationAndConcernBounds(t *testing.T) {
	dynamics := testConfig(4).Dynamics
	appraisal := CandidateAppraisal{Difference: 1, Ownership: 1, Value: -0.8, Urgency: 0.5}
	activation := appraisalActivation(dynamics, appraisal)
	if activation < 0.459 || activation > 0.461 {
		t.Fatalf("activation = %.4f, want 0.46", activation)
	}
	if got := updateConcernStrength(dynamics, 0.95, 1, "hold"); got != 1 {
		t.Fatalf("concern upper bound = %f", got)
	}
	if got := updateConcernStrength(dynamics, 0.10, 0, "resolved"); got != 0 {
		t.Fatalf("concern lower bound = %f", got)
	}
}

func TestCognitiveCommitPreservesUnselectedBackground(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{
		{ID: "event-1", Kind: "body_delta", Source: "observed", Summary: "body changed", Status: "pending"},
		{ID: "event-2", Kind: "mentor_received", Source: "observed", Summary: "mentor wrote", Status: "pending"},
	}
	runtime.state.Background = append(runtime.state.Background, events...)
	runtime.activeCandidates = map[string]Event{"event-1": events[0], "event-2": events[1]}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{
			{CandidateID: "event-1", Meaning: "身体值得校准", Difference: 0.8, Ownership: 0.9, Value: 0.7, Urgency: 0.4, Answerability: 0.8, Certainty: 0.8, Resolution: "hold"},
			{CandidateID: "event-2", Meaning: "这是一段可以稍后理解的关系信号", Difference: 0.5, Ownership: 0.6, Value: 0.4, Urgency: 0.2, Answerability: 0.9, Certainty: 0.7, Resolution: "hold"},
		},
		FocusID:       "event-1",
		ThoughtThread: "我先校准身体，同时保留这段关系信号。",
		Action:        CognitiveAction{Kind: "none"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Background[1].Status != "background" {
		t.Fatalf("unselected candidate disappeared instead of returning to background: %#v", runtime.state.Background[1])
	}
	if len(runtime.state.Concerns) != 2 {
		t.Fatalf("appraisals did not form two distinct concerns: %#v", runtime.state.Concerns)
	}
	if runtime.state.AffectiveState.Activation <= 0 {
		t.Fatal("AIP did not change the affective background")
	}
}

func TestAffectiveSalienceChangesCandidateOrder(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "quiet", Strength: 0.5, Activation: 0.1, Answerability: 0.5},
		{ID: "charged", Strength: 0.5, Activation: 0.9, Answerability: 0.5},
	}
	quiet := Event{ID: "quiet", Kind: "concern", ConcernID: "quiet"}
	charged := Event{ID: "charged", Kind: "concern", ConcernID: "charged"}
	if runtime.candidateScore(charged) <= runtime.candidateScore(quiet) {
		t.Fatal("object affect did not change attention priority")
	}
	before := runtime.candidateScore(quiet)
	runtime.state.AffectiveState.Activation = 0.8
	if runtime.candidateScore(quiet) <= before {
		t.Fatal("overall affective background did not enter later salience")
	}
}

func TestExplorationThresholdCreatesOneEndogenousEvent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.44
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "endogenous_change" {
		t.Fatalf("exploration crossing did not become one candidate: %#v", runtime.state.Background)
	}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("exploration above threshold retriggered every pulse: %#v", runtime.state.Background)
	}
}

func TestExplorationIdleGrowthPausesDuringActiveCognition(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.2
	runtime.state.Lease = &Lease{ID: "active"}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if runtime.state.ExplorationPressure != 0.2 {
		t.Fatalf("active cognition was counted as idle time: %f", runtime.state.ExplorationPressure)
	}
}

func TestUnrelievedExplorationReturnsAfterRevisitInterval(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.7
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339Nano)
	runtime.state.Concerns = []Concern{{ID: "exploration", Subject: "探索张力"}}
	runtime.state.Background = []Event{{ID: "old", Kind: "endogenous_change", Status: "processed", ConcernID: "exploration"}}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 2 || runtime.state.Background[1].Kind != "endogenous_change" || runtime.state.Background[1].Status != "pending" {
		t.Fatalf("unrelieved exploration never returned to attention: %#v", runtime.state.Background)
	}
	if runtime.state.Background[1].ConcernID != "exploration" {
		t.Fatalf("one continuing exploration tension became a new concern: %#v", runtime.state.Background[1])
	}
}

func TestExplorationRetryWaitDoesNotMultiplyCandidates(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	runtime.state.Background = []Event{{ID: "retry", Kind: "endogenous_change", Status: "retry_wait", LastFocusedAt: nowUTC()}}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("one retrying exploration event multiplied: %#v", runtime.state.Background)
	}
}

func TestAffectiveReturnUsesNeutralControlAndCertainty(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.AffectiveState = AffectiveState{Valence: 1, Activation: 1, Control: 1, Certainty: 0}
	if err := runtime.advanceDynamics(5 * time.Minute); err != nil {
		t.Fatal(err)
	}
	state := runtime.state.AffectiveState
	if state.Valence != 0.5 || state.Activation != 0.5 || state.Control != 0.75 || state.Certainty != 0.25 {
		t.Fatalf("unexpected neutral return: %#v", state)
	}
}

func TestStageFourAllowsAliceToChooseNonTopCandidate(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	top := Event{ID: "top", Kind: "body_delta", Source: "observed", Summary: "top", Status: "pending"}
	other := Event{ID: "other", Kind: "mentor_received", Source: "observed", Summary: "other", Status: "pending"}
	runtime.activeCandidates = map[string]Event{"top": top, "other": other}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{
			{CandidateID: "top", Meaning: "身体变化可以稍后复看", Difference: 0.3, Ownership: 0.5, Value: 0.2, Urgency: 0.1, Answerability: 0.8, Certainty: 0.8, Resolution: "hold"},
			{CandidateID: "other", Meaning: "我选择先理解关系", Difference: 0.5, Ownership: 0.8, Value: 0.5, Urgency: 0.2, Answerability: 0.9, Certainty: 0.8, Resolution: "hold"},
		},
		FocusID:       "other",
		ThoughtThread: "此刻关系信号更值得我注意。",
		Action:        CognitiveAction{Kind: "none"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("kernel overruled alice's valid non-top focus: %v", err)
	}
}

func TestStageFourConcernContextIsBoundedAndKeepsCandidateConcern(t *testing.T) {
	concerns := make([]Concern, 0, 20)
	for index := 0; index < 20; index++ {
		concerns = append(concerns, Concern{ID: fmt.Sprintf("c-%02d", index), Strength: float64(index) / 100})
	}
	selected := selectContextConcerns(concerns, []Event{{ID: "event", ConcernID: "c-00"}})
	if len(selected) != defaultConcernContextLimit {
		t.Fatalf("concern context length = %d, want %d", len(selected), defaultConcernContextLimit)
	}
	if selected[0].ID != "c-00" {
		t.Fatalf("candidate-linked concern was omitted from bounded context: %#v", selected)
	}
}

func TestTerminalZeroStrengthConcernLeavesActiveSet(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "terminal", Strength: 0, Resolution: "resolved"},
		{ID: "residual", Strength: 0.2, Resolution: "hold"},
	}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != "residual" {
		t.Fatalf("inactive concern remained active: %#v", runtime.state.Concerns)
	}
}

func TestCompletedExperiencesDoNotAccumulateAsActiveConcerns(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 100; index++ {
		id := fmt.Sprintf("result-%03d", index)
		event := Event{ID: id, Kind: "action_result", Source: "observed", Summary: "现实结果", Status: "pending"}
		runtime.activeCandidates = map[string]Event{id: event}
		commit := CognitiveCommit{
			Appraisals: []CandidateAppraisal{{
				CandidateID: id, Meaning: "这次经验已经完成", Difference: 0.2, Ownership: 0.8,
				Value: 0.2, Urgency: 0.1, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved",
			}},
			FocusID: id, ThoughtThread: "经验完成。", Action: CognitiveAction{Kind: "none"},
		}
		if err := runtime.applyCognitiveCommit(commit); err != nil {
			t.Fatal(err)
		}
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("completed experiences inflated active concerns: %d", len(runtime.state.Concerns))
	}
}

func TestExplorationConcernIdentitySurvivesBackgroundPruning(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Strength: 0.4, Resolution: "hold"}}
	runtime.state.Background = nil
	if got := runtime.currentExplorationConcernID(); got != "exploration" {
		t.Fatalf("exploration concern identity was lost with old background events: %q", got)
	}
}

func TestStageFourBodyActionReturnsAsNewRealityEvent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Strength: 0.3, Resolution: "hold"}}
	if err := runtime.startStage4Action(ctx, "lease-1", CognitiveAction{Kind: "body_shell", Command: "printf stage-four-reality"}); err != nil {
		t.Fatal(err)
	}
	var result ActionResultNotice
	select {
	case result = <-runtime.actionResults:
	case <-time.After(5 * time.Second):
		t.Fatal("body action did not return")
	}
	if err := runtime.handleStage4ActionResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	if runtime.state.PendingAction != nil {
		t.Fatalf("completed action remained pending: %#v", runtime.state.PendingAction)
	}
	if runtime.state.ExplorationPressure != 0.8 {
		t.Fatalf("action completion granted relief before alice interpreted the result: %f", runtime.state.ExplorationPressure)
	}
	found := false
	for _, event := range runtime.state.Background {
		if event.Kind == "action_result" && event.Status == "in_focus" {
			found = true
		}
	}
	if !found {
		t.Fatalf("real action result did not re-enter the attention field: %#v", runtime.state.Background)
	}
	request := <-runtime.cognizer.(*blockingCognizer).started
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: request.Focus.ID, Meaning: "这次真实观察满足了当前探索", Difference: 0.6,
			Ownership: 0.9, Value: 0.7, Urgency: 0.2, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved",
		}},
		FocusID: request.Focus.ID, ThoughtThread: "现实回应已经足够。", Action: CognitiveAction{Kind: "none"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if absFloat(runtime.state.ExplorationPressure-0.36) > 0.000001 {
		t.Fatalf("alice's interpretation of real exploration did not relieve pressure: %f", runtime.state.ExplorationPressure)
	}
	for _, concern := range runtime.state.Concerns {
		if concern.OriginKind == "endogenous_change" {
			t.Fatalf("resolved exploration concern remained active: %#v", concern)
		}
	}
}
