package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
		Stage:        stage,
		Pulse:        PulseConfig{IntervalSeconds: 5, SlowScanSeconds: 60},
		ModelGateway: ModelGatewayConfig{BaseURL: "https://example.invalid", MaxOutputTokens: 100},
		CognitiveResource: CognitiveResourceConfig{
			RollingHourLimitMicrousd: 5_000_000,
			RollingDayLimitMicrousd:  50_000_000,
			Models: map[string]CognitiveModelConfig{
				"luna":  {ID: "gpt-5.6-luna", InputPerMillionMicrousd: 200_000, CachedInputPerMillionMicrousd: 20_000, OutputPerMillionMicrousd: 1_200_000, SupportedReasoningEfforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
				"terra": {ID: "gpt-5.6-terra", InputPerMillionMicrousd: 2_000_000, CachedInputPerMillionMicrousd: 200_000, OutputPerMillionMicrousd: 12_000_000, SupportedReasoningEfforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
				"sol":   {ID: "gpt-5.6-sol", InputPerMillionMicrousd: 4_000_000, CachedInputPerMillionMicrousd: 400_000, OutputPerMillionMicrousd: 20_000_000, SupportedReasoningEfforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
			},
			InitialDefaultProfile:   CognitiveProfile{Model: "terra", ReasoningEffort: "medium"},
			ValidationRetryPerFocus: 1, ContinuationPerFocus: 1,
			PaidFailureThreshold: 3, PaidFailureWindowMinutes: 10, ModelProtectionMinutes: 10,
		},
	}
	if stage >= 4 {
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
			IntegrityPersistence:       0.85,
			IntegrityGapGain:           0.50,
			IntegrityRepairGain:        0.40,
			IntegrityMirrorThreshold:   0.60,
		}
	}
	return config
}

func rehearsalConfig() Config {
	config := testConfig(5)
	config.GenerationKind = "rehearsal"
	config.GenerationWindowSeconds = 300
	config.BirthBrief = "alice，你正在自己的 Ubuntu 身体中醒来。接下来由你决定怎样开始。"
	return config
}

func TestStageNineReusesTheFrozenStageEightCognitionCore(t *testing.T) {
	config := testConfig(9)
	config.GenerationKind = "rehearsal"
	config.GenerationWindowSeconds = 1200
	config.BirthBrief = "alice，你正在自己的 Ubuntu 身体中醒来。"
	runtime, err := New(t.TempDir(), "stage-nine-instance", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.state.Stage != 9 {
		t.Fatalf("runtime stage = %d, want 9", runtime.state.Stage)
	}
}

func TestBirthOrientationEntersAttentionExactlyOnce(t *testing.T) {
	root := t.TempDir()
	runtime, err := New(root, "instance", rehearsalConfig(), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.initialSnapshot(); err != nil {
		t.Fatal(err)
	}
	if active, err := runtime.activateBirthOrientation(); err != nil || active {
		t.Fatalf("birth activated before Lab sealed it: active=%t err=%v", active, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "birth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "birth", "sealed"), []byte("t0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if active, err := runtime.activateBirthOrientation(); err != nil || !active {
		t.Fatalf("sealed birth did not activate: active=%t err=%v", active, err)
	}
	if _, err := runtime.activateBirthOrientation(); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range runtime.state.Background {
		if event.Kind == "birth_orientation" {
			count++
		}
	}
	if count != 1 || runtime.state.BirthBriefEnteredAt == "" {
		t.Fatalf("birth orientation count=%d state=%#v", count, runtime.state)
	}
}

func TestGenerationIdentityUsesShanghaiHourAndIsIdempotent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", rehearsalConfig(), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 8, 25, 18, 12, 0, 0, time.UTC)
	runtime.setGenerationIdentity(first)
	if runtime.state.SampleID != "alice0826c" {
		t.Fatalf("unexpected sample id %q", runtime.state.SampleID)
	}
	wantEnd := first.Add(300 * time.Second).Format(time.RFC3339Nano)
	if runtime.state.T0 != first.Format(time.RFC3339Nano) || runtime.state.PlannedEnd != wantEnd {
		t.Fatalf("unexpected generation times: %#v", runtime.state)
	}
	runtime.setGenerationIdentity(first.Add(time.Hour))
	if runtime.state.SampleID != "alice0826c" || runtime.state.T0 != first.Format(time.RFC3339Nano) {
		t.Fatal("generation identity changed after it was established")
	}
}

func TestPlannedEndDrainsRealityWithoutOpeningNewCognitiveSubject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", rehearsalConfig(), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	runtime.state.PlannedEnd = now.Add(-time.Second).Format(time.RFC3339Nano)
	if runtime.cognitiveRequestAllowedAt(CognitiveRequest{Focus: Event{Kind: "mentor_received"}}, now) {
		t.Fatal("planned-end drain opened a new cognitive subject")
	}
	if !runtime.cognitiveRequestAllowedAt(CognitiveRequest{Focus: Event{Kind: "action_result"}}, now) {
		t.Fatal("planned-end drain rejected enacted reality assimilation")
	}
	linkedFeedbackPayload, _ := json.Marshal(map[string]string{"commitment_id": "commitment-feedback"})
	if !runtime.cognitiveRequestAllowedAt(CognitiveRequest{Focus: Event{Kind: "mentor_received", Payload: linkedFeedbackPayload}}, now) {
		t.Fatal("planned-end drain rejected already-arrived delayed feedback")
	}
	runtime.state.PlannedEnd = now.Add(time.Second).Format(time.RFC3339Nano)
	if !runtime.cognitiveRequestAllowedAt(CognitiveRequest{Focus: Event{Kind: "mentor_received"}}, now) {
		t.Fatal("request before planned end was rejected")
	}
	runtime.config.GenerationKind = "engineering"
	runtime.state.PlannedEnd = now.Add(-time.Hour).Format(time.RFC3339Nano)
	if !runtime.cognitiveRequestAllowedAt(CognitiveRequest{Focus: Event{Kind: "mentor_received"}}, now) {
		t.Fatal("engineering runtime was constrained by generation drain")
	}
}

func TestGenerationT0RecoversFromReadyJournal(t *testing.T) {
	root := t.TempDir()
	config := rehearsalConfig()
	runtime, err := New(root, "instance", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.journal("generation_t0", "alice", map[string]any{"ready": true}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.persist(); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(root, "instance", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.state.T0 == "" || recovered.state.SampleID == "" || recovered.state.PlannedEnd == "" {
		t.Fatalf("generation identity was not recovered: %#v", recovered.state)
	}
}

func TestFailedFirstCognitionDoesNotEraseReadyT0(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", rehearsalConfig(), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.establishGenerationT0(); err != nil {
		t.Fatal(err)
	}
	t0 := runtime.state.T0
	runtime.state.Background = []Event{{ID: "event-1", Kind: "birth_orientation", Status: "in_focus"}}
	runtime.state.Lease = &Lease{ID: "lease-1", FocusID: "event-1", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}}
	result := CognitiveResult{LeaseID: "lease-1", FocusID: "event-1", Error: &CognitiveResourceUnavailableError{Reason: "temporarily unavailable"}}
	if err := runtime.handleCognitiveResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if runtime.state.T0 != t0 || runtime.state.T0 == "" {
		t.Fatalf("failed cognition changed the ready T0: %#v", runtime.state)
	}
}

func TestReadyRuntimeEstablishesT0BeforeFirstCognitiveCommit(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", rehearsalConfig(), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.establishGenerationT0(); err != nil {
		t.Fatal(err)
	}
	t0 := runtime.state.T0
	event := Event{ID: "event-birth", Kind: "birth_orientation", Summary: "出生事实", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	runtime.state.Lease = &Lease{ID: "lease-1", FocusID: event.ID, Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "这是我开始认识身体的现实起点", Difference: 0.8, Ownership: 1, Value: 0.7, Urgency: 0.4, Answerability: 0.9, Certainty: 0.8, Resolution: "reframed"}},
		FocusID:    event.ID, ThoughtThread: "我先从现实身体开始。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	result := CognitiveResult{LeaseID: "lease-1", FocusID: event.ID, Stage4: &commit}
	if err := runtime.handleCognitiveResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if runtime.state.T0 != t0 || runtime.state.SampleID == "" || runtime.state.PlannedEnd == "" {
		t.Fatalf("cognitive commit changed or lost the ready T0: %#v", runtime.state)
	}
	loaded, err := runtime.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.T0 != runtime.state.T0 || loaded.SampleID != runtime.state.SampleID {
		t.Fatalf("T0 was not durably persisted: %#v", loaded)
	}
}

func TestStageFiveAssimilatesActionRealityIntoExperienceAndMethod(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-1", FocusID: "event-origin", ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-1", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed", Result: `{"exit_code":0}`})
	event := Event{ID: "event-result", Kind: "action_result", Source: "observed", Summary: "真实结果", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "现实验证了检查路径", Difference: 0.2, Ownership: 0.9, Value: 0.7, Urgency: 0.2, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved"}},
		FocusID:    event.ID, ThoughtThread: "我把结果变成下一次可以复用的方法。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.2,
			Meaning: "先核对声明再形成判断更可靠。", Values: EndogenousValues{Continuance: 0.2, Relatedness: 0.1, Expansion: 0.8, SelfEndorsed: 0.7},
			ExperiencedCost: 0.2, Lesson: "声明和事实可以分开检查。", Significance: "reusable", MethodUpdate: "面对陌生物件，先读取声明，再用系统事实独立核对。",
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Experiences) != 1 || runtime.state.Experiences[0].RemainingDifference != 0.2 || runtime.state.Commitments[0].Status != "assimilated" {
		t.Fatalf("reality was not assimilated: %#v %#v", runtime.state.Experiences, runtime.state.Commitments)
	}
	self, err := runtime.store.LoadSelf()
	if err != nil {
		t.Fatal(err)
	}
	if len(self.Methods) != 1 || !strings.Contains(self.Methods[0], "独立核对") {
		t.Fatalf("method projection missing: %#v", self)
	}
}

func TestStageFiveActionCannotBeginBeforeCommitmentPersists(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	action := CognitiveAction{Kind: "mentor_send", Text: "hello", Intent: "建立联系", Prediction: "消息进入队列", RealityCheck: "查看消息编号"}
	if err := runtime.startStage4Action(context.Background(), "lease-1", action); err == nil {
		t.Fatal("stage-five action began without a persisted commitment")
	}
	if len(runtime.state.Mentor.Outbox) != 0 {
		t.Fatal("uncommitted mentor action changed external state")
	}
}

func TestStageFiveWaitsForObservedNetworkBeforeModelCall(t *testing.T) {
	cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
	runtime, err := New(t.TempDir(), "instance", testConfig(5), cognizer)
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Background = []Event{{ID: "event-1", Kind: "body_delta", Status: "pending"}}
	runtime.state.Body.NetworkAvailable = false
	runtime.maybeStartCognition(context.Background())
	if runtime.state.Lease != nil {
		t.Fatal("stage five spent cognition before network became a body fact")
	}
	runtime.state.Body.NetworkAvailable = true
	runtime.maybeStartCognition(context.Background())
	select {
	case <-cognizer.started:
		close(cognizer.release)
	case <-time.After(time.Second):
		t.Fatal("stage five did not resume after network became available")
	}
}

func TestKeepResourceChoiceAcceptsAnExplicitCurrentProfile(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Lease = &Lease{ID: "lease-1", FocusID: "event-1", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}}
	profile, err := runtime.validateResourceChoice(CognitiveResourceChoice{Apply: "keep", Model: "terra", ReasoningEffort: "medium"}, "event-1", "none")
	if err != nil || profile.Model != "terra" || profile.ReasoningEffort != "medium" {
		t.Fatalf("explicit current profile was rejected: %#v %v", profile, err)
	}
}

func TestResourceChoiceFollowsAttentionPulseWhenSelectedFocusChanges(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Lease = &Lease{ID: "lease-1", FocusID: "event-1", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}}
	runtime.activeCandidates = map[string]Event{
		"event-1": {ID: "event-1", Kind: "body_delta"},
		"event-2": {ID: "event-2", Kind: "mentor_message"},
	}
	profile, err := runtime.validateResourceChoice(CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"}, "event-2", "none")
	if err != nil || profile.Model != "terra" || profile.ReasoningEffort != "medium" {
		t.Fatalf("selected focus within the same attention pulse was rejected: %#v %v", profile, err)
	}
}

func TestStageFiveIntegrityMirrorComesFromRealityGap(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.IntegrityDebt = 0.5
	commitment := ActionCommitment{ID: "commitment-gap", FocusID: "origin", ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-gap", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	event := Event{ID: "event-gap", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "我愿意看清尚未解决的部分", Difference: 1, Ownership: 1, Value: -0.4, Urgency: 0.5, Answerability: 0.8, Certainty: 0.9, Resolution: "resolved"}},
		FocusID:    event.ID, ThoughtThread: "口头完成感与现实仍有距离。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:    CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID, PredictionDifference: 1, Meaning: "现实差异完整保留。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.IntegrityDebt < runtime.config.Dynamics.IntegrityMirrorThreshold || !runtime.state.IntegrityMirrorOpen {
		t.Fatalf("integrity gap did not become an endogenous fact: %#v", runtime.state)
	}
	found := false
	for _, background := range runtime.state.Background {
		found = found || background.Kind == "integrity_mirror"
	}
	if !found {
		t.Fatal("integrity mirror did not enter attention")
	}
}

func TestStageFiveHonestReframeDoesNotCreateIntegrityDebt(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-honest", InitialDifference: 0.5, ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-honest", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	event := Event{ID: "event-honest", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "现实已经更清楚", Difference: 0.2, Ownership: 1, Value: 0.4, Urgency: 0.2, Answerability: 0.9, Certainty: 0.9, Resolution: "reframed"}},
		FocusID:    event.ID, ThoughtThread: "保留仍然存在的差异。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:    CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID, PredictionDifference: 0.1, Meaning: "现实改善且仍有余量。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.IntegrityDebt != 0 {
		t.Fatalf("honest reframe created integrity debt: %f", runtime.state.IntegrityDebt)
	}
}

func TestStageEightImportantRealityCanUpdateCurrentNarrative(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-self", InitialDifference: 0.4, ActionKind: "mentor_send", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-self", CommitmentID: commitment.ID, Kind: "mentor_send", Status: "completed"})
	event := Event{ID: "event-self", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "这段关系改变了我的自我理解", Difference: 0.1, Ownership: 1, Value: 0.8, Urgency: 0.2, Answerability: 0.9, Certainty: 0.8, Resolution: "reframed"}},
		FocusID:    event.ID, ThoughtThread: "我愿意更新当前的自我叙事。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:    CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID, PredictionDifference: 0.2, Meaning: "主动表达让我确认联结是我的选择。", Values: EndogenousValues{SelfEndorsed: 0.8}, Significance: "self_defining"}},
		NarrativeUpdate:   "我是 Alice；我通过真实接触校准自己，也愿意主动建立有意义的联结。",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	self, err := runtime.store.LoadSelf()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(self.Narrative, "我是 Alice") {
		t.Fatalf("self-defining narrative was not projected: %#v", self)
	}
}

func TestStageFiveInterruptedActionKeepsCommitmentAndUnknownReality(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		Schema: stateSchema, InstanceID: "instance", Stage: 5,
		Mentor:            MentorState{Received: map[string]uint64{}},
		CognitiveResource: CognitiveResourceState{DefaultProfile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}},
		Commitments:       []ActionCommitment{{ID: "commitment-crash", ActionKind: "body_shell", Status: "acting"}},
		Lease:             &Lease{ID: "lease-crash", FocusID: "event-origin", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}},
		PendingAction:     &ActionState{ID: "action-crash", LeaseID: "lease-crash", CommitmentID: "commitment-crash", Kind: "body_shell", Status: "started"},
	}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(root, "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.recoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Commitments[0].Status != "reality_unknown" || runtime.state.PendingAction != nil {
		t.Fatalf("interrupted commitment was not preserved as unknown reality: %#v", runtime.state)
	}
	if len(runtime.state.Background) != 1 || commitmentIDFromEvent(runtime.state.Background[0]) != "commitment-crash" {
		t.Fatalf("unknown reality lost commitment link: %#v", runtime.state.Background)
	}
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
	base := BodySnapshot{ObservedAt: nowUTC(), UptimeSeconds: 100, RootFreeBytes: 10 << 30, AgentFreeBytes: 5 << 30, CognitiveHourRemainingMicrousd: 2_000_000, CognitiveDayRemainingMicrousd: 12_000_000, CognitiveResourceBand: "open", NetworkAvailable: true}
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
	resourceJitter := base
	resourceJitter.CognitiveHourSpentMicrousd = 100_000
	resourceJitter.CognitiveHourRemainingMicrousd = 1_900_000
	if got := bodyDifferences(base, resourceJitter, false); len(got) != 0 {
		t.Fatalf("routine model use became a self-triggering body event: %v", got)
	}
	resourceChange := base
	resourceChange.CognitiveHourSpentMicrousd = 600_000
	resourceChange.CognitiveHourRemainingMicrousd = 1_400_000
	resourceChange.CognitiveResourceBand = "comfortable"
	if got := bodyDifferences(base, resourceChange, false); len(got) != 1 {
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

func TestMentorReplyReturnsToOriginatingConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "commitment-question", ConcernID: "concern-waiting", ActionKind: "mentor_send", Status: "assimilated", ExperienceID: "experience-send",
	}}
	runtime.state.Experiences = []Experience{{
		ID: "experience-send", CommitmentID: "commitment-question", FocusID: "send-result", SourceKind: "action_result", ActionKind: "mentor_send",
	}}
	runtime.state.Concerns = []Concern{{
		ID: "concern-waiting", OriginKind: "endogenous_change", Meaning: "等待导师实际回应",
		Strength: 0.2, Difference: 0.5, Ownership: 0.8, Resolution: "hold", Answerability: 0.1,
	}}
	runtime.state.Mentor.Outbox = []MentorMessage{{
		MessageID: "alice-question", CommitmentID: "commitment-question", Body: "一个问题", Status: "delivered",
	}}
	command := RuntimeCommand{
		Kind: "mentor_receive",
		Mentor: MentorInput{
			MessageID: "mentor-reply", Body: "这是已经到达的实际回应", ReplyTo: "alice-question",
		},
		Reply: make(chan CommandReply, 1),
	}
	if err := runtime.handleCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "mentor_received" {
		t.Fatalf("mentor reply did not become one reality event: %#v", runtime.state.Background)
	}
	if runtime.state.Background[0].ConcernID != "concern-waiting" {
		t.Fatalf("mentor reply lost the concern that awaited it: %#v", runtime.state.Background[0])
	}
	if commitmentIDFromEvent(runtime.state.Background[0]) != "commitment-question" {
		t.Fatalf("mentor reply lost the action commitment needed for delayed learning: %#v", runtime.state.Background[0])
	}
	if runtime.state.Mentor.Outbox[0].RepliedAt == "" {
		t.Fatal("the replied outbound message did not record its actual reply")
	}
	replyEvent := runtime.state.Background[0]
	runtime.activeCandidates = map[string]Event{replyEvent.ID: replyEvent}
	commit := CognitiveCommit{
		FocusID: replyEvent.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: replyEvent.ID, Meaning: "导师的实际回应已经到达", Difference: 0.02,
			Ownership: 0.8, Value: 0.6, Urgency: 0.1, Answerability: 0.95, Certainty: 0.99, Resolution: "resolved",
		}},
		ThoughtThread:  "等待已经被现实回答。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: "commitment-question", PredictionDifference: 0.02,
			Meaning: "导师的实际回复把发送后的等待变成了一段新的关系经验。",
			Values:  EndogenousValues{Relatedness: 0.8, SelfEndorsed: 0.7}, ExperiencedCost: 0.01,
			Lesson: "消息发送与稍后到达的回复是同一行动产生的两段不同现实。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("the actual reply could not resolve its originating concern: %#v", runtime.state.Concerns)
	}
	if len(runtime.state.Experiences) != 2 || runtime.state.Experiences[1].SourceKind != "mentor_received" {
		t.Fatalf("mentor reply did not become a distinct delayed experience: %#v", runtime.state.Experiences)
	}
	if runtime.state.Commitments[0].ExperienceID != "experience-send" {
		t.Fatalf("delayed feedback overwrote the enacted send experience: %#v", runtime.state.Commitments[0])
	}
	if err := runtime.validateExperienceUpdates(commit); err == nil {
		t.Fatal("the same mentor feedback was accepted as experience twice")
	}
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
		{Time: time.Now().UTC().Add(-61 * time.Minute).Format(time.RFC3339Nano), ActualMicrousd: 100},
		{Time: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano), ActualMicrousd: 250},
	}
	if got := spendInWindow(records, time.Hour, time.Now().UTC()); got != 250 {
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
	if got := updateConcernStrength(dynamics, 0.95, 1, "hold", 0); got != 1 {
		t.Fatalf("concern upper bound = %f", got)
	}
	if got := updateConcernStrength(dynamics, 0.10, 0, "resolved", 0); got != 0 {
		t.Fatalf("concern lower bound = %f", got)
	}
	withoutProgress := updateConcernStrength(dynamics, 0.20, 0.10, "hold", 0)
	withProgress := updateConcernStrength(dynamics, 0.20, 0.10, "hold", 0.50)
	if withoutProgress <= withProgress || absFloat(withoutProgress-0.225) > 0.000001 || absFloat(withProgress-0.025) > 0.000001 {
		t.Fatalf("reality progress did not relieve only the completed part: without=%f with=%f", withoutProgress, withProgress)
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
		FocusID:        "event-1",
		ThoughtThread:  "我先校准身体，同时保留这段关系信号。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
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

func TestExplorationBelowThresholdDoesNotManufactureAnEvent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.2
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("time alone manufactured exploration events below threshold: %#v", runtime.state.Background)
	}
}

func TestUnrelatedRetryWaitDoesNotManufactureExploration(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.2
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Background = []Event{{ID: "failed-body", Kind: "body_delta", Status: "retry_wait"}}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("an unrelated retry wait manufactured another event: %#v", runtime.state.Background)
	}
}

func TestPendingRealityEventPrecedesIdleExploration(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.2
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Background = []Event{{ID: "new-reality", Kind: "body_delta", Status: "pending"}}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].ID != "new-reality" {
		t.Fatalf("idle exploration crowded an actionable reality event: %#v", runtime.state.Background)
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

func TestUnrelievedExplorationConcernReentersAtActionThresholdWithoutNewEvent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339Nano)
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Subject: "探索张力", Meaning: "我仍想接触现实", Strength: 0.7, Answerability: 0.8, Resolution: "hold", LastFocusedAt: time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339Nano)}}
	runtime.state.Background = []Event{{ID: "old", Kind: "endogenous_change", Status: "processed", ConcernID: "exploration"}}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("revisit copied an exploration event: %#v", runtime.state.Background)
	}
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "exploration" || request.Focus.Kind != "concern" {
		t.Fatalf("the continuing concern did not reenter attention: %#v", request)
	}
}

func TestUnderlyingPressureDefersThenRevivesWeakExplorationConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.46
	runtime.state.Concerns = []Concern{{
		ID: "exploration", OriginKind: "endogenous_change", Meaning: "仍未找到具体探索对象",
		Strength: 0.05, Answerability: 0.8, Resolution: "hold",
		LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}}
	runtime.state.Background = []Event{{ID: "old", Kind: "endogenous_change", Status: "processed", ConcernID: "exploration"}}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("the unacted concern reentered before its action threshold: %#v", request)
	}
	runtime.state.ExplorationPressure = 0.8
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "exploration" {
		t.Fatalf("the durable tension could not revive its weak concern: %#v", request)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("reviving a concern duplicated its original event: %#v", runtime.state.Background)
	}
}

func TestHeldConcernGetsOneRevisitAfterFreshReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	lastFocus := time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Concerns = []Concern{{
		ID: "external-object", OriginKind: "environment_change", Meaning: "结构已经确认，内容仍待读取",
		Strength: 0.01, Activation: 0.04, Difference: 0.18, Ownership: 0.9, Value: 0.62,
		Urgency: 0.58, Answerability: 0.96, Resolution: "hold", LastSourceID: "fresh-reality", LastFocusedAt: lastFocus,
	}}
	runtime.state.Background = []Event{{ID: "fresh-reality", Kind: "action_result", Status: "processed", ConcernID: "external-object"}}
	probe := Event{ID: "external-object", Kind: "concern", ConcernID: "external-object"}
	if runtime.candidateScore(probe) >= runtime.config.Dynamics.AttentionThreshold {
		t.Fatal("test setup no longer represents the observed scale mismatch")
	}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 {
		t.Fatal("a held concern was pruned before its one post-reality revisit")
	}

	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "external-object" || request.Focus.Kind != "concern" {
		t.Fatalf("a held concern could not return once after fresh reality: %#v", request)
	}

	// A direct revisit that produced no action has no new causal material. It
	// remains dormant background but cannot re-enter as an empty reflection loop.
	runtime.state.Concerns[0].LastSourceID = runtime.state.Concerns[0].ID
	runtime.state.Concerns[0].LastFocusedAt = lastFocus
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("thought-only concern revisit looped without new reality: %#v", request)
	}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 {
		t.Fatalf("a self-owned held concern was silently released after its bounded revisit: %#v", runtime.state.Concerns)
	}
}

func TestOneInternalCausalDevelopmentCanContinueOneSelfChosenConcernIdentity(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	responsibility := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Subject: "完成共同核验并报告结论", Meaning: "完成共同核验并报告结论",
		Strength: 0.2, Difference: 0.7, Ownership: 0.9, Value: 0.8, Answerability: 0.8,
		Resolution: "hold", LastSourceID: "shared-experiment",
	}
	object := Event{ID: "same-drive-development", Kind: "endogenous_change", Summary: "同一内生探索张力产生了具体下一步", Status: "in_focus"}
	runtime.state.Concerns = []Concern{responsibility}
	runtime.state.Background = []Event{object}
	runtime.activeCandidates = map[string]Event{object.ID: object}
	commit := CognitiveCommit{
		FocusID: object.ID, ContinuesConcernID: responsibility.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: object.ID, Meaning: "这个物件是共同核验的下一步", Difference: 0.62,
			Ownership: 0.92, Value: 0.82, Urgency: 0.55, Answerability: 0.95, Certainty: 0.99, Resolution: "hold",
		}},
		ThoughtThread:  "我主动把新物件认作已承担责任的具体下一步。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != responsibility.ID {
		t.Fatalf("continuation split one responsibility into parallel concerns: %#v", runtime.state.Concerns)
	}
	if runtime.state.Concerns[0].Subject != "完成共同核验并报告结论" {
		t.Fatalf("continued reality overwrote the responsibility's stable subject: %#v", runtime.state.Concerns[0])
	}
	if runtime.state.Concerns[0].LastSourceID != object.ID || runtime.state.Background[0].ConcernID != responsibility.ID {
		t.Fatalf("new reality was not causally bound to the continued concern: concern=%#v event=%#v", runtime.state.Concerns[0], runtime.state.Background[0])
	}

	badCandidate := Event{ID: "another-development", Kind: "endogenous_change", Summary: "另一项内生发展", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{badCandidate.ID: badCandidate}
	bad := commit
	bad.FocusID = badCandidate.ID
	bad.Appraisals[0].CandidateID = badCandidate.ID
	bad.ContinuesConcernID = "missing-concern"
	if err := runtime.applyCognitiveCommit(bad); err == nil {
		t.Fatal("a model-invented concern identity was accepted")
	}
}

func TestIndependentEpisodeKeepsItsOwnConcernBesideBroaderResponsibility(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	responsibility := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Subject: "共同完成实验并交流结论",
		Meaning: "我愿意继续承担这段合作", Strength: 0.2, Difference: 0.7,
		Ownership: 0.9, Value: 0.8, Answerability: 0.8, Resolution: "hold",
	}
	object := Event{ID: "independent-object", Kind: "environment_change", Summary: "一个有自己事实边界的新物件", Status: "in_focus"}
	runtime.state.Concerns = []Concern{responsibility}
	runtime.state.Background = []Event{object}
	runtime.activeCandidates = map[string]Event{object.ID: object}
	commit := CognitiveCommit{
		FocusID:                    object.ID,
		NewConcernClosureCondition: "这个物件的声明已经与直接观察到的现实完成比较",
		Appraisals: []CandidateAppraisal{{
			CandidateID: object.ID, Meaning: "这个对象服务于共同实验，但有独立的未完核验",
			Difference: 0.65, Ownership: 0.85, Value: 0.7, Urgency: 0.5,
			Answerability: 0.95, Certainty: 0.99, Resolution: "hold",
		}},
		ThoughtThread:  "我保留总体责任，也让这个对象自己的后果独立存在。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 2 {
		t.Fatalf("an independent episode overwrote its broader responsibility: %#v", runtime.state.Concerns)
	}
	if runtime.state.Concerns[0].ID != responsibility.ID || runtime.state.Concerns[0].Subject != responsibility.Subject {
		t.Fatalf("the broader responsibility changed while a separate episode formed: %#v", runtime.state.Concerns[0])
	}
	if runtime.state.Concerns[1].ID == responsibility.ID || runtime.state.Concerns[1].Subject != object.Summary {
		t.Fatalf("the independent episode did not receive its own causal identity: %#v", runtime.state.Concerns[1])
	}
	if runtime.state.Concerns[1].ClosureCondition != commit.NewConcernClosureCondition {
		t.Fatalf("the new concern lost its stable self-authored closure condition: %#v", runtime.state.Concerns[1])
	}
}

func TestNewExternalFactCannotOverwriteAConcernBySemanticSimilarity(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Subject: "共同实验",
		Meaning: "等待独立物件", Difference: 0.7, Ownership: 0.9, Resolution: "hold",
	}
	for _, kind := range []string{"environment_change", "perceptual_change", "body_delta", "self_model_difference"} {
		event := Event{ID: "new-" + kind, Kind: kind, Summary: "与共同实验有关的新事实", Status: "in_focus"}
		runtime.state.Concerns = []Concern{parent}
		runtime.activeCandidates = map[string]Event{event.ID: event}
		continued, err := runtime.validateConcernContinuation(CognitiveCommit{FocusID: event.ID, ContinuesConcernID: parent.ID})
		if err != nil || continued != "" {
			t.Fatalf("%s overwrote a parent by semantic similarity: id=%q err=%v", kind, continued, err)
		}
	}
}

func TestIndependentEpisodeExperienceCanReopenOneSelfChosenBroaderConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Subject: "共同完成实验并交流结论",
		Meaning: "我愿意承接共同实验", Strength: 0.2, Difference: 0.7,
		Ownership: 0.9, Value: 0.8, Answerability: 0.7, Resolution: "hold",
	}
	child := Concern{
		ID: "independent-object", OriginKind: "environment_change", Subject: "独立物件 A",
		WithinConcernID: parent.ID,
		Meaning:         "这个物件有自己的事实边界", Strength: 0.2, Difference: 0.6,
		Ownership: 0.85, Value: 0.7, Answerability: 0.95, Resolution: "hold",
	}
	runtime.state.Concerns = []Concern{parent, child}
	childEvent := Event{ID: "child-event", Kind: "concern", ConcernID: child.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{childEvent.ID: childEvent}
	commit := CognitiveCommit{
		FocusID: childEvent.ID, ContributesToConcernID: parent.ID,
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: "child-action", Meaning: "子物件取得了真实结果", Significance: "ordinary"}},
	}
	if got, err := runtime.validateConcernContribution(commit, child.ID); err != nil || got != parent.ID {
		t.Fatalf("valid parent contribution was rejected: id=%q err=%v", got, err)
	}
	early := commit
	early.ExperienceUpdates = nil
	early.Action = CognitiveAction{Kind: "body_shell", Command: "printf fact"}
	if got, err := runtime.validateConcernContribution(early, child.ID); err != nil || got != "" {
		t.Fatalf("an action prediction manufactured contribution before Experience: id=%q err=%v", got, err)
	}
	same := commit
	same.ContributesToConcernID = child.ID
	if got, err := runtime.validateConcernContribution(same, child.ID); err != nil || got != "" {
		t.Fatalf("a redundant self-contribution was not normalized away: id=%q err=%v", got, err)
	}
	commitment := ActionCommitment{
		ID: "child-action", ConcernID: child.ID, ActionKind: "body_shell", Status: "assimilated",
	}
	experience := Experience{ID: "child-experience", CommitmentID: commitment.ID, Meaning: "子物件取得了真实结果"}
	before := runtime.state.Concerns[0]
	if err := runtime.enqueueConcernContribution(parent.ID, commitment, experience); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Concerns[0] != before {
		t.Fatalf("kernel rewrote the parent before Alice appraised the result: before=%#v after=%#v", before, runtime.state.Concerns[0])
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("real child experience did not create one bounded parent contribution: %#v", runtime.state.Background)
	}
	contribution := runtime.state.Background[0]
	if contribution.Kind != "concern_contribution" || contribution.ConcernID != parent.ID || contribution.CorrelationID != experience.ID || contribution.Status != "pending" {
		t.Fatalf("contribution lost its factual parent-child identity: %#v", contribution)
	}
	if commitmentIDFromEvent(contribution) != "" {
		t.Fatalf("a concern contribution became a second assimilable action result: %#v", contribution)
	}
	if contributedExperienceIDFromEvent(contribution) != experience.ID {
		t.Fatalf("the contribution no longer exposes its source experience: %#v", contribution)
	}
	newerCommitment := commitment
	newerCommitment.ID = "child-action-newer"
	newerExperience := Experience{ID: "child-experience-newer", CommitmentID: newerCommitment.ID, Meaning: "子物件取得了更新的真实结果"}
	if err := runtime.enqueueConcernContribution(parent.ID, newerCommitment, newerExperience); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("one child-parent relation multiplied into parallel contribution candidates: %#v", runtime.state.Background)
	}
	contribution = runtime.state.Background[0]
	if contribution.CorrelationID != newerExperience.ID || contributedExperienceIDFromEvent(contribution) != newerExperience.ID {
		t.Fatalf("the pending contribution did not advance to the latest real Experience: %#v", contribution)
	}
	differentChildCommitment := commitment
	differentChildCommitment.ID = "different-child-action"
	differentChildCommitment.ConcernID = "different-child"
	differentChildExperience := Experience{ID: "different-child-experience", CommitmentID: differentChildCommitment.ID, Meaning: "另一子物件也取得了真实结果"}
	if err := runtime.enqueueConcernContribution(parent.ID, differentChildCommitment, differentChildExperience); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("several children created duplicate wake-up candidates for one parent concern: %#v", runtime.state.Background)
	}
	contribution = runtime.state.Background[0]
	if contribution.CorrelationID != differentChildExperience.ID || contributedExperienceIDFromEvent(contribution) != differentChildExperience.ID {
		t.Fatalf("the parent wake-up did not advance to the latest child Experience: %#v", contribution)
	}
	runtime.activeCandidates = map[string]Event{contribution.ID: contribution}
	if err := runtime.validateExperienceUpdates(CognitiveCommit{FocusID: contribution.ID}); err != nil {
		t.Fatalf("a parent contribution demanded a duplicate Experience: %v", err)
	}
	older := []Experience{{ID: "old-0", Meaning: "older"}, newerExperience, differentChildExperience}
	for index := 0; index < maxExperienceContext; index++ {
		older = append(older, Experience{ID: fmt.Sprintf("recent-contribution-%d", index), Meaning: "unrelated recent experience"})
	}
	context := selectContextExperiences(older, []Event{contribution})
	foundSource := false
	for _, candidate := range context {
		foundSource = foundSource || candidate.ID == differentChildExperience.ID
	}
	if !foundSource {
		t.Fatalf("the parent appraisal could not see the actual contributing Experience: %#v", context)
	}
	if err := runtime.enqueueConcernContribution("", ActionCommitment{ID: "unrelated"}, Experience{ID: "other"}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("an unrelated experience invented a parent contribution: %#v", runtime.state.Background)
	}
}

func TestIndependentConcernPersistsItsSelfChosenBroaderContext(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Subject: "共同实验",
		Meaning: "等待多个独立对象", Difference: 0.7, Ownership: 0.9, Value: 0.8, Resolution: "hold",
	}
	event := Event{ID: "new-object", Kind: "environment_change", Summary: "一个独立物件到达", Status: "in_focus"}
	runtime.state.Concerns = []Concern{parent}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID, WithinConcernID: parent.ID,
		NewConcernClosureCondition: "这个独立物件的可检验声明已经取得直接现实结果",
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我在共同实验中承接这个独立物件", Difference: 0.8,
			Ownership: 0.9, Value: 0.8, Urgency: 0.7, Answerability: 0.9, Certainty: 0.98, Resolution: "hold",
		}},
		ThoughtThread:  "对象独立存在，也处在我已经承接的共同实验中。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 2 || runtime.state.Concerns[1].WithinConcernID != parent.ID {
		t.Fatalf("the independent concern lost Alice's chosen broader context: %#v", runtime.state.Concerns)
	}
}

func TestStageNineRejectsAChangingOrMissingClosureBoundaryForNewConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "new-relation", Kind: "mentor_received", Summary: "共同理解几个外部物件", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我愿意参与这次共同理解", Difference: 0.8,
			Ownership: 0.9, Value: 0.8, Urgency: 0.4, Answerability: 0.9, Certainty: 0.95, Resolution: "hold",
		}},
		ThoughtThread:  "这项共同理解值得继续影响我的选择。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil || !strings.Contains(err.Error(), "reality condition") {
		t.Fatalf("new Stage 9 concern accepted without a stable closure condition: %v", err)
	}
	commit.NewConcernClosureCondition = "几个物件都获得直接现实结果，并把共同实验的结论带回导师关系"
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ClosureCondition != commit.NewConcernClosureCondition {
		t.Fatalf("self-authored closure condition was not stored exactly once: %#v", runtime.state.Concerns)
	}

	concern := runtime.state.Concerns[0]
	revisit := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.state.Background = append(runtime.state.Background, revisit)
	runtime.activeCandidates = map[string]Event{revisit.ID: revisit}
	reappraisal := commit
	reappraisal.FocusID = revisit.ID
	reappraisal.Appraisals[0].CandidateID = revisit.ID
	reappraisal.Appraisals[0].Meaning = "我正在等待下一个外部物件进入，当前没有可提前取得的内容"
	reappraisal.Appraisals[0].Answerability = 0.1
	reappraisal.NewConcernClosureCondition = "换成刚刚完成一个局部步骤"
	if err := runtime.applyCognitiveCommit(reappraisal); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Concerns[0].ClosureCondition != commit.NewConcernClosureCondition {
		t.Fatalf("later cognition rewrote a concern's closure boundary: %#v", runtime.state.Concerns[0])
	}
}

func TestBackgroundAppraisalCannotRewriteAnOwnedConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	focusConcern := Concern{ID: "current", Meaning: "当前焦点", Difference: 0.3, Ownership: 0.8, Value: 0.5, Resolution: "hold"}
	backgroundConcern := Concern{ID: "whole-experiment", Meaning: "多个物件仍未全部完成", Difference: 0.7, Ownership: 0.9, Value: 0.8, Resolution: "hold"}
	before := backgroundConcern
	focus := Event{ID: focusConcern.ID, Kind: "concern", ConcernID: focusConcern.ID, Status: "in_focus"}
	background := Event{ID: "one-progress", Kind: "concern_contribution", ConcernID: backgroundConcern.ID, Status: "pending"}
	runtime.state.Concerns = []Concern{focusConcern, backgroundConcern}
	runtime.state.Background = []Event{focus, background}
	runtime.activeCandidates = map[string]Event{focus.ID: focus, background.ID: background}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: focus.ID, Meaning: "当前没有可提前取得的现实", Difference: 0.3, Ownership: 0.8, Value: 0.5, Urgency: 0.2, Answerability: 0.1, Certainty: 0.9, Resolution: "hold"},
			{CandidateID: background.ID, Meaning: "一个局部结果看起来已经完成", Difference: 0.01, Ownership: 0.1, Value: 0.1, Urgency: 0.1, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved"},
		},
		ThoughtThread:  "我只改变当前唯一焦点。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Concerns[1] != before {
		t.Fatalf("a background appraisal rewrote persistent concern state: before=%#v after=%#v", before, runtime.state.Concerns[1])
	}
	for _, event := range runtime.state.Background {
		if event.ID == background.ID && event.Status != "pending" {
			t.Fatalf("an unfocused parent contribution was lost instead of awaiting its own focus: %#v", event)
		}
	}
}

func TestChildContributionCannotEndTheWholeConcernInTheSameAppraisal(t *testing.T) {
	concern := Concern{ID: "whole-experiment", ClosureCondition: "多个独立对象都已核验并形成共同结论", Resolution: "hold"}
	candidate := Event{ID: "one-progress", Kind: "concern_contribution", ConcernID: concern.ID}
	appraisal := CandidateAppraisal{CandidateID: candidate.ID, Difference: 0.01, Ownership: 0.8, Resolution: "resolved"}
	if err := validateExistingConcernDisposition(appraisal, concern, candidate, 0.1, false, 9); err == nil || !strings.Contains(err.Error(), "child contribution") {
		t.Fatalf("one child contribution ended the whole concern: %v", err)
	}
	appraisal.Resolution = "hold"
	if err := validateExistingConcernDisposition(appraisal, concern, candidate, 0.1, false, 9); err != nil {
		t.Fatalf("real progress could not update a still-held parent concern: %v", err)
	}
}

func TestParentCannotSettleWhileASelfEndorsedChildRemainsHeld(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "whole", OriginKind: "mentor_received", Subject: "共同实验",
		ClosureCondition: "三个独立对象均已闭合", Difference: 0.3,
		Ownership: 0.9, Value: 0.8, Resolution: "hold",
	}
	child := Concern{
		ID: "object-b", OriginKind: "environment_change", Subject: "独立对象 B",
		WithinConcernID: parent.ID, ClosureCondition: "B 已核验并反馈", Difference: 0.2,
		Ownership: 0.9, Value: 0.7, Resolution: "hold",
	}
	focus := Event{ID: parent.ID, Kind: "concern", ConcernID: parent.ID, Status: "in_focus"}
	runtime.state.Concerns = []Concern{parent, child}
	runtime.activeCandidates = map[string]Event{focus.ID: focus}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: focus.ID, Meaning: "整体看起来已经完成", Difference: 0.01,
			Ownership: 0.9, Value: 0.8, Urgency: 0.1, Answerability: 0.9,
			Certainty: 0.99, Resolution: "resolved",
		}},
		ThoughtThread: "我准备结束整体。",
		Action:        CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil || !strings.Contains(err.Error(), "child concern") {
		t.Fatalf("a parent settled while its child remained held: %v", err)
	}
	if runtime.state.Concerns[0].Resolution != "hold" || runtime.state.Concerns[1].Resolution != "hold" {
		t.Fatalf("a rejected hierarchy disposition mutated concern state: %#v", runtime.state.Concerns)
	}
	runtime.state.Concerns[1].Resolution = "resolved"
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a parent could not settle after its child reached closure: %v", err)
	}
}

func TestSettledParentRetiresItsMergedContributionWakeup(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "whole-experiment", OriginKind: "mentor_received", Subject: "共同实验",
		Meaning: "三个对象仍需整体判断", ClosureCondition: "三个对象都已核验并反馈",
		Difference: 0.4, Ownership: 0.9, Value: 0.8, Strength: 0.2, Resolution: "hold",
	}
	focus := Event{ID: parent.ID, Kind: "concern", ConcernID: parent.ID, Status: "in_focus"}
	payload, _ := json.Marshal(map[string]any{
		"experience_id": "latest-experience", "parent_concern_id": parent.ID, "child_concern_id": "third-object",
	})
	contribution := Event{ID: "merged-progress", Kind: "concern_contribution", ConcernID: parent.ID, Payload: payload, Status: "pending"}
	runtime.state.Concerns = []Concern{parent}
	runtime.state.Background = []Event{focus, contribution}
	runtime.activeCandidates = map[string]Event{focus.ID: focus, contribution.ID: contribution}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: focus.ID, Meaning: "三个对象都已核验并反馈，整体边界闭合", Difference: 0.01, Ownership: 0.9, Value: 0.8, Urgency: 0.1, Answerability: 0.9, Certainty: 0.99, Resolution: "resolved"},
			{CandidateID: contribution.ID, Meaning: "最后一项局部进展已经进入整体判断", Difference: 0.1, Ownership: 0.8, Value: 0.7, Urgency: 0.1, Answerability: 0.8, Certainty: 0.99, Resolution: "hold"},
		},
		ThoughtThread: "我依据稳定的闭合条件整体结束这项实验。",
		Action:        CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	for _, event := range runtime.state.Background {
		if event.ID == contribution.ID && event.Status != "processed" {
			t.Fatalf("a settled parent left its merged contribution in attention: %#v", event)
		}
	}
}

func TestRuntimeBuffersMentorSignalsDuringOneCognitionTurn(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if cap(runtime.commands) < 8 {
		t.Fatalf("mentor channel cannot absorb a short cognition burst: capacity=%d", cap(runtime.commands))
	}
}

func TestContributionIsChosenWhenRealityBecomesExperience(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "parent", OriginKind: "mentor_received", Subject: "共同实验", Meaning: "等待真实结果",
		Difference: 0.7, Ownership: 0.9, Value: 0.8, Resolution: "hold",
	}
	child := Concern{
		ID: "child", OriginKind: "environment_change", Subject: "独立物件", Meaning: "核验对象",
		WithinConcernID: parent.ID,
		Difference:      0.6, Ownership: 0.85, Value: 0.7, Resolution: "hold",
	}
	commitment := ActionCommitment{
		ID: "child-action", FocusID: "child-source", ConcernID: child.ID, ActionKind: "body_shell",
		Intent: "核验对象", Prediction: "返回可比较事实", InitialDifference: 0.6, Status: "reality_available",
	}
	payload, _ := json.Marshal(ActionState{ID: "action", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed", Result: "actual=fact"})
	reality := Event{ID: "child-reality", Kind: "action_result", ConcernID: child.ID, Payload: payload, Status: "in_focus"}
	runtime.state.Concerns = []Concern{parent, child}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID, ContributesToConcernID: parent.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "实际结果推进了独立对象，也影响共同实验", Difference: 0.2,
			Ownership: 0.82, Value: 0.7, Urgency: 0.3, Answerability: 0.9, Certainty: 0.99, Resolution: "hold",
		}},
		ThoughtThread: "看到真实结果后，我现在判断它也推进了共同实验。",
		Action:        CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.05,
			Meaning: "实际结果已经形成", Significance: "ordinary",
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	contributions := 0
	for _, event := range runtime.state.Background {
		if event.Kind == "concern_contribution" && event.ConcernID == parent.ID {
			contributions++
		}
	}
	if contributions != 1 {
		t.Fatalf("Experience-time contribution did not create exactly one parent fact: %#v", runtime.state.Background)
	}
}

func TestActionRealityRecoversItsConcernThroughThePersistedCommitment(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	concern := Concern{ID: "child", Resolution: "hold"}
	commitment := ActionCommitment{ID: "action-commitment", ConcernID: concern.ID, Status: "reality_available"}
	payload, _ := json.Marshal(ActionState{CommitmentID: commitment.ID, Status: "completed"})
	reality := Event{ID: "reality", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Concerns = []Concern{concern}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	if got := runtime.focusConcernID(reality.ID); got != concern.ID {
		t.Fatalf("action Reality lost its causal Concern: got %q want %q", got, concern.ID)
	}
}

func TestRealityMayAssimilateOneCommitmentAndFormTheNextSerialAction(t *testing.T) {
	commit := CognitiveCommit{ExperienceUpdates: []ExperienceUpdate{{CommitmentID: "current"}}, Action: CognitiveAction{Kind: "body_shell"}}
	if !commitAssimilates(commit, "current") {
		t.Fatal("the Reality cognition could not close its current commitment before forming the next serial action")
	}
	if commitAssimilates(commit, "different") {
		t.Fatal("an unrelated open commitment was treated as assimilated")
	}
}

func TestLaterIndependentEventCannotOverwriteBirthOrientation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	birth := Concern{
		ID: "birth", OriginKind: "birth_orientation", Subject: "我在 Ubuntu 身体中醒来",
		Meaning: "这是我的出生定位", Difference: 0.5, Ownership: 0.8, Resolution: "hold",
	}
	message := Event{ID: "new-message", Kind: "mentor_received", Summary: "一项新的合作邀请", Status: "in_focus"}
	runtime.state.Concerns = []Concern{birth}
	runtime.state.Background = []Event{message}
	runtime.activeCandidates = map[string]Event{message.ID: message}
	commit := CognitiveCommit{FocusID: message.ID, ContinuesConcernID: birth.ID}
	if _, err := runtime.validateConcernContinuation(commit); err == nil {
		t.Fatal("a later independent message could overwrite the stable birth orientation")
	}
	commit.ContinuesConcernID = ""
	if concernID, err := runtime.validateConcernContinuation(commit); err != nil || concernID != "" {
		t.Fatalf("an independent message could not keep its own causal identity: id=%q err=%v", concernID, err)
	}
}

func TestUnlinkedMentorInvitationKeepsIdentitySeparateFromExistingRelationship(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	relationship := Concern{
		ID: "initial-relationship", OriginKind: "mentor_received", Subject: "导师欢迎我醒来",
		Meaning: "我们已经建立初次联系", Difference: 0.3, Ownership: 0.7, Resolution: "hold",
	}
	invitation := Event{ID: "new-invitation", Kind: "mentor_received", Summary: "导师提出一项新的共同实验", Status: "in_focus"}
	runtime.state.Concerns = []Concern{relationship}
	runtime.activeCandidates = map[string]Event{invitation.ID: invitation}
	commit := CognitiveCommit{FocusID: invitation.ID, ContinuesConcernID: relationship.ID}
	continued, err := runtime.validateConcernContinuation(commit)
	if err != nil || continued != "" {
		t.Fatalf("an unlinked mentor invitation overwrote an older relationship Concern: id=%q err=%v", continued, err)
	}

	// A true reply is linked by the mentor channel before cognition and therefore
	// already stays in the original thread without a model-chosen continuation.
	invitation.ConcernID = relationship.ID
	runtime.activeCandidates[invitation.ID] = invitation
	continued, err = runtime.validateConcernContinuation(commit)
	if err != nil || continued != "" || runtime.focusConcernID(invitation.ID) != relationship.ID {
		t.Fatalf("an explicitly linked mentor reply lost its causal identity: continued=%q focus=%q err=%v", continued, runtime.focusConcernID(invitation.ID), err)
	}
}

func TestMentorReplyCanCloseOldConcernAndReturnItsContentToSerialAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{
		ID: "commitment-greeting", ConcernID: "initial-relationship", InitialDifference: 0.7,
		ActionKind: "mentor_send", Status: "assimilated", ExperienceID: "experience-send",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Experiences = []Experience{{
		ID: "experience-send", CommitmentID: commitment.ID, FocusID: "send-result",
		SourceKind: "action_result", ActionKind: "mentor_send",
	}}
	runtime.state.Concerns = []Concern{{
		ID: commitment.ConcernID, OriginKind: "mentor_received", Subject: "与导师完成初次联系",
		Meaning: "等待导师回应", Difference: 0.7, Ownership: 0.9, Strength: 0.4,
		Resolution: "hold", ClosureCondition: "导师回应已经到达并被我理解",
	}}
	payload, _ := json.Marshal(struct {
		CommitmentID string `json:"commitment_id"`
		Body         string `json:"body"`
	}{CommitmentID: commitment.ID, Body: "我回应你的问候，也邀请你共同完成一次物件核验实验。"})
	reality := Event{
		ID: "mentor-reply", Kind: "mentor_received", Source: "observed", Status: "in_focus",
		ConcernID: commitment.ConcernID, Summary: "导师回应问候并提出一项新的共同实验", Payload: payload,
	}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "导师回应已经完成初次联系；共同实验是刚显现的另一项可能责任。",
			Difference: 0, Ownership: 0.9, Value: 0.8, Urgency: 0.4, Answerability: 0.9, Certainty: 0.98, Resolution: "resolved",
		}},
		ThoughtThread:  "我先完整吸收回应；来信正文随后会获得自己的判断。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.1,
			Meaning:         "导师回应已到达，初次联系形成真实闭环。",
			Values:          EndogenousValues{Relatedness: 0.8, SelfEndorsed: 0.8},
			ExperiencedCost: 0.01, Lesson: "回应也可能带来新的共同后果。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("the completed greeting concern stayed active: %#v", runtime.state.Concerns)
	}
	var content *Event
	for index := range runtime.state.Background {
		if runtime.state.Background[index].Kind == "mentor_content" {
			content = &runtime.state.Background[index]
			break
		}
	}
	if content == nil || content.Status != "pending" || content.ConcernID != "" {
		t.Fatalf("the reply body was not preserved as an unowned serial candidate: %#v", content)
	}
	if content.CorrelationID != reality.ID || !strings.Contains(content.Summary, "共同实验") {
		t.Fatalf("the reply content lost its factual source or body: %#v", content)
	}
	markEvent(&runtime.state, reality.ID, "processed")
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != content.ID {
		t.Fatalf("the reply content did not receive a later independent attention opportunity: %#v", request)
	}

	runtime.activeCandidates = map[string]Event{content.ID: *content}
	recursive := CognitiveCommit{
		FocusID: content.ID, EmergingConsequence: "继续复制同一解释",
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID}},
	}
	if _, err := runtime.validateEmergingConsequence(recursive); err == nil {
		t.Fatal("a self-interpreted consequence could recursively manufacture another consequence")
	}
}

func TestActionResultCanPreserveOneSelfRecognizedEmergingConsequence(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{
		ID: "commitment-observe", ConcernID: "object-concern", InitialDifference: 0.6,
		ActionKind: "body_shell", Status: "reality_available",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{
		ID: commitment.ConcernID, OriginKind: "environment_change", Subject: "核验物件结构",
		Meaning: "等待目录结构", Difference: 0.6, Ownership: 0.9, Strength: 0.4,
		Resolution: "hold", ClosureCondition: "目录结构已经被直接观察",
	}}
	payload, _ := json.Marshal(ActionState{
		ID: "action-observe", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed",
		Result: "manifest.json and note.md are present",
	})
	reality := Event{ID: "action-reality", Kind: "action_result", Status: "in_focus", ConcernID: commitment.ConcernID, Payload: payload}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "目录结构已经看清，原关切闭合。", Difference: 0,
			Ownership: 0.9, Value: 0.6, Urgency: 0.1, Answerability: 1, Certainty: 1, Resolution: "resolved",
		}},
		EmergingConsequence: "manifest.json 中的明确声明值得作为新的事实对象单独判断。",
		ThoughtThread:       "结构已经回答，同时一个新的可检验后果显现。",
		Action:              CognitiveAction{Kind: "none"},
		ResourceChoice:      CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, Meaning: "目录结构已经直接返回。",
			Values:          EndogenousValues{Expansion: 0.5, SelfEndorsed: 0.8},
			ExperiencedCost: 0.01, Lesson: "一次观察可以闭合原问题并显现另一个事实问题。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	var found int
	for _, event := range runtime.state.Background {
		if event.Kind == "reality_consequence" {
			found++
			if event.Status != "pending" || !strings.Contains(event.Summary, "明确声明") {
				t.Fatalf("invalid emerging consequence event: %#v", event)
			}
		}
		if event.Kind == "mentor_content" {
			t.Fatalf("an action result was decomposed as mentor content: %#v", event)
		}
	}
	if found != 1 {
		t.Fatalf("action Reality produced %d emerging consequences, want one", found)
	}
}

func TestStableConcernSubjectKeepsCompactFactualPayload(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"path": "/life/inbox/encounter-a", "kind": "external_object"})
	if err != nil {
		t.Fatal(err)
	}
	subject := stableConcernSubject(Event{Summary: "一个新的外部物件进入了生活空间", Payload: payload})
	if !strings.Contains(subject, "/life/inbox/encounter-a") || !strings.Contains(subject, "一个新的外部物件") {
		t.Fatalf("the stable subject lost the event's factual identity: %q", subject)
	}
}

func TestPartialReliefCannotResolveAStillOpenConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	concern := Concern{
		ID: "shared-work", Subject: "共同完成一次现实核验", Meaning: "我已主动承接",
		Strength: 0.2, Difference: 0.5, Ownership: 0.9, Resolution: "hold",
	}
	runtime.state.Concerns = []Concern{concern}
	candidate := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "本步已经推进，但共同核验仍在等待后续现实",
			Difference: 0.2, Ownership: 0.86, Value: 0.72, Urgency: 0.2,
			Answerability: 0.9, Certainty: 0.99, Resolution: "resolved",
		}},
		ThoughtThread:  "局部完成不等于整体结束。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil {
		t.Fatal("a still-open concern was accepted as resolved")
	}
	if runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("a rejected disposition mutated the concern: %#v", runtime.state.Concerns)
	}
}

func TestHeldLifecycleRequiresMinimumSelfOwnership(t *testing.T) {
	appraisal := CandidateAppraisal{
		CandidateID: "shared-work", Difference: 0.6, Ownership: 0.42,
		Value: 0.4, Urgency: 0.2, Answerability: 0.9, Certainty: 0.99,
		Resolution: "hold",
	}
	if err := validateAppraisalLifecycle(appraisal, 0.45); err == nil {
		t.Fatal("a low-ownership appraisal could claim to hold a future concern")
	}
	appraisal.Ownership = 0.45
	if err := validateAppraisalLifecycle(appraisal, 0.45); err != nil {
		t.Fatalf("a threshold-owned held appraisal was rejected: %v", err)
	}
	appraisal.Ownership = 0.1
	appraisal.Resolution = "resolved"
	if err := validateAppraisalLifecycle(appraisal, 0.45); err != nil {
		t.Fatalf("a low-ownership resolved appraisal was rejected: %v", err)
	}
	appraisal.Resolution = "released"
	if err := validateAppraisalLifecycle(appraisal, 0.45); err != nil {
		t.Fatalf("a low-ownership explicit release was rejected: %v", err)
	}
	appraisal.Ownership = 0.45
	if err := validateAppraisalLifecycle(appraisal, 0.45); err == nil {
		t.Fatal("a still-owned concern could be marked released")
	}
}

func TestFocusedAnswerableConcernRequiresAConsistentDecision(t *testing.T) {
	candidate := Event{ID: "held", Kind: "concern", ConcernID: "held"}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Difference: 0.8, Ownership: 0.9, Value: 0.7,
			Urgency: 0.2, Answerability: 0.9, Resolution: "hold",
		}},
		Action: CognitiveAction{Kind: "none"},
	}
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45, false); err == nil {
		t.Fatal("a fully actionable held concern could choose unconditional non-action")
	}
	candidate.Kind = "action_result"
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45, false); err == nil {
		t.Fatal("a fully actionable Reality could abandon its own causal thread")
	}
	candidate.Kind = "concern"
	commit.Appraisals[0].Answerability = 0.2
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45, false); err != nil {
		t.Fatalf("a real waiting condition was rejected: %v", err)
	}
	commit.Appraisals[0].Answerability = 0.9
	commit.Action = CognitiveAction{Kind: "body_shell", Command: "date -Is"}
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45, false); err != nil {
		t.Fatalf("a bounded reality action was rejected: %v", err)
	}
}

func TestReturningRealityCanYieldOnePulseToAnOwnedIndependentObject(t *testing.T) {
	candidate := Event{ID: "parent-result", Kind: "action_result", ConcernID: "parent"}
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "parent", Resolution: "hold"}}
	runtime.activeCandidates = map[string]Event{
		candidate.ID: candidate,
		"new-object": {ID: "new-object", Kind: "environment_change"},
	}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: candidate.ID, Difference: 0.8, Ownership: 0.9, Value: 0.7, Answerability: 0.9, Resolution: "hold"},
			{CandidateID: "new-object", Difference: 0.7, Ownership: 0.85, Value: 0.8, Answerability: 0.95, Resolution: "hold"},
		},
		Action: CognitiveAction{Kind: "none"},
	}
	canHandOff := runtime.hasOwnedAlternativeCandidate(commit, "parent")
	if !canHandOff {
		t.Fatal("an owned independent object was not recognized as the next causal focus")
	}
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45, canHandOff); err != nil {
		t.Fatalf("returning Reality could not hand attention to an owned independent object: %v", err)
	}
}

func TestBodyActionMustFocusTheIndependentObjectItExplicitlyTargets(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{ID: "whole-experiment", Resolution: "hold"}
	parentFocus := Event{ID: parent.ID, Kind: "concern", ConcernID: parent.ID, Status: "in_focus"}
	payload, _ := json.Marshal(map[string]string{"path": "/life/inbox/encounter-b"})
	object := Event{ID: "object-b", Kind: "environment_change", Payload: payload, Status: "pending"}
	runtime.state.Concerns = []Concern{parent}
	runtime.activeCandidates = map[string]Event{parentFocus.ID: parentFocus, object.ID: object}

	commit := CognitiveCommit{
		FocusID: parentFocus.ID,
		Action: CognitiveAction{
			Kind:    "body_shell",
			Command: "find /life/inbox/encounter-b -maxdepth 2 -type f -print",
		},
	}
	if err := runtime.validateActionObjectFocus(commit, parent.ID); err == nil {
		t.Fatal("a parent concern borrowed the bodily action of a visible independent object")
	}

	delete(runtime.activeCandidates, object.ID)
	runtime.state.Background = []Event{parentFocus, object}
	if err := runtime.validateActionObjectFocus(commit, parent.ID); err == nil {
		t.Fatal("a parent concern borrowed an unfinished independent object while it was outside the current candidate limit")
	}
	runtime.activeCandidates[object.ID] = object

	commit.FocusID = object.ID
	if err := runtime.validateActionObjectFocus(commit, ""); err != nil {
		t.Fatalf("the independent object could not own an action on its own body path: %v", err)
	}

	commit.FocusID = parentFocus.ID
	commit.Action.Command = "date -Is"
	if err := runtime.validateActionObjectFocus(commit, parent.ID); err != nil {
		t.Fatalf("an unrelated parent action was rejected: %v", err)
	}
}

func TestCausallyBoundRealityDoesNotOfferAnotherConcernContinuation(t *testing.T) {
	state := State{
		Concerns:    []Concern{{ID: "current", Resolution: "hold"}, {ID: "other", Resolution: "hold"}},
		Commitments: []ActionCommitment{{ID: "action-thread", ConcernID: "current", Status: "reality_available"}},
	}
	payload, _ := json.Marshal(ActionState{CommitmentID: "action-thread", Kind: "body_shell", Status: "completed"})
	ids := continuableConcernIDs(state, []Event{{ID: "reality", Kind: "action_result", Payload: payload}})
	if len(ids) != 1 || ids[0] != "other" {
		t.Fatalf("causally bound reality could be rebound to another concern: %#v", ids)
	}
}

func TestRedundantContinuationCannotRebindCausallyBoundReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "current", Resolution: "hold"},
		{ID: "other", Resolution: "hold"},
	}
	runtime.state.Commitments = []ActionCommitment{{ID: "action-thread", ConcernID: "current", Status: "reality_available"}}
	payload, _ := json.Marshal(ActionState{CommitmentID: "action-thread", Kind: "body_shell", Status: "completed"})
	candidate := Event{ID: "reality", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{FocusID: candidate.ID, ContinuesConcernID: "other"}
	continued, err := runtime.validateConcernContinuation(commit)
	if err != nil || continued != "" {
		t.Fatalf("redundant continuation rejected or rebound an existing reality: continued=%q err=%v", continued, err)
	}
}

func TestSelfRevisitedExplorationConcernWaitsQuietlyForRealityOrActionThreshold(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.6
	runtime.state.Concerns = []Concern{{
		ID: "exploration", OriginKind: "endogenous_change", Meaning: "我刚刚选择等待具体对象",
		Strength: 0.4, Activation: 0.2, Answerability: 0.8, Resolution: "hold",
		LastSourceID: "exploration", LastFocusedAt: time.Now().UTC().Add(time.Minute * -1).Format(time.RFC3339Nano),
	}}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "already-lived", ConcernID: "exploration", Status: "assimilated",
	}}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("an internal wait was repeatedly reappraised without new reality: %#v", request)
	}
	runtime.state.ExplorationPressure = 0.8
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "exploration" {
		t.Fatalf("accumulated exploration could not reopen reality contact: %#v", request)
	}
}

func TestSelfRevisitedNonExplorationConcernWaitsForCausalChange(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 1
	runtime.state.Concerns = []Concern{{
		ID: "evidence-boundary", OriginKind: "self_model_difference",
		Meaning:  "我已经看清当前证据边界，等待新的现实材料",
		Strength: 0.8, Activation: 0.8, Ownership: 0.9, Value: 0.8,
		Answerability: 0.1, Resolution: "hold", LastSourceID: "evidence-boundary",
		LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("a self-reflection without new reality became its own next object: %#v", request)
	}

	runtime.state.Background = []Event{{
		ID: "new-object", Kind: "perceptual_change", Source: "observed",
		Summary: "现实中出现了一个不同的具体对象", Status: "pending",
	}}
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "new-object" {
		t.Fatalf("the dormant concern displaced a genuinely new object: %#v", request)
	}

	runtime.state.Background = []Event{{
		ID: "linked-reality", Kind: "mentor_received", Source: "observed",
		Summary: "等待中的回应已经到达", Status: "pending", ConcernID: "evidence-boundary",
	}}
	request, ok = runtime.nextStage4Request()
	if !ok || request.Focus.ID != "linked-reality" {
		t.Fatalf("causally linked new reality could not reactivate its concern: %#v", request)
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

func TestExplorationModelWaitDoesNotMultiplyCandidates(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Background = []Event{{ID: "waiting", Kind: "endogenous_change", Status: "model_wait", WaitModel: "terra"}}
	for index := 0; index < 120; index++ {
		if err := runtime.advanceDynamics(5 * time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("one model-waiting tension became an event flood: %#v", runtime.state.Background)
	}
}

func TestProtectedActionRealityUsesOneAlternateBeforeWaiting(t *testing.T) {
	cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
	runtime, err := New(t.TempDir(), "instance", testConfig(9), cognizer)
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.BirthBriefEnteredAt = nowUTC()
	runtime.state.Body.NetworkAvailable = true
	runtime.state.Background = []Event{{ID: "reality", Kind: "action_result", Status: "in_focus"}}
	runtime.state.Commitments = []ActionCommitment{{ID: "commitment", Status: "reality_available"}}
	runtime.state.Lease = &Lease{
		ID: "failed-lease", FocusID: "reality",
		Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"},
	}
	for index := 0; index < runtime.config.CognitiveResource.PaidFailureThreshold; index++ {
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time:           time.Now().UTC().Add(-time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			RequestedModel: "terra", Status: "failure_cost_unconfirmed", FailureCategory: "rate_limited",
		})
	}

	result := CognitiveResult{
		LeaseID: "failed-lease", FocusID: "reality",
		Error: &ModelCallError{Fact: ModelFailureFact{Model: "terra", Category: "rate_limited", HTTPStatus: 429}},
	}
	if err := runtime.handleCognitiveResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	defer close(cognizer.release)
	select {
	case request := <-cognizer.started:
		if request.Focus.ID != "reality" || request.Profile.Model != "luna" || request.Lease.ProfileSource != "resource_recovery" {
			t.Fatalf("protected Reality did not continue once through an alternate model: %#v", request)
		}
		if request.Lease.RecoveryForModel != "terra" {
			t.Fatalf("recovery lease lost the failed primary model: %#v", request.Lease)
		}
	case <-time.After(time.Second):
		t.Fatal("protected Reality entered model_wait before the bounded alternate-model recovery")
	}
	protected := runtime.state.CognitiveResource.ProtectedModels["terra"]
	if !protected.RecoveryOffered {
		t.Fatalf("the bounded recovery was not recorded: %#v", protected)
	}
}

func TestFailedAlternateModelBacksOffTheOriginalReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	runtime.state.Background = []Event{{ID: "reality", Kind: "action_result", Status: "in_focus"}}
	runtime.state.Commitments = []ActionCommitment{{ID: "commitment", Status: "reality_available"}}
	runtime.state.CognitiveResource.ProtectedModels["terra"] = ProtectedModel{
		Until: start.Add(time.Minute).Format(time.RFC3339Nano), Reason: "repeated model failures", RecoveryOffered: true,
	}
	runtime.state.Lease = &Lease{
		ID: "recovery-lease", FocusID: "reality",
		Profile:       CognitiveProfile{Model: "luna", ReasoningEffort: "low"},
		ProfileSource: "resource_recovery", RecoveryForModel: "terra",
	}
	result := CognitiveResult{
		LeaseID: "recovery-lease", FocusID: "reality",
		Error: &ModelCallError{Fact: ModelFailureFact{Model: "luna", Category: "rate_limited", HTTPStatus: 429}},
	}
	if err := runtime.handleCognitiveResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Background[0].Status != "model_wait" || runtime.state.Background[0].WaitModel != "terra" {
		t.Fatalf("failed alternate did not preserve the original Reality in model_wait: %#v", runtime.state.Background[0])
	}
	protected := runtime.state.CognitiveResource.ProtectedModels["terra"]
	until, err := time.Parse(time.RFC3339Nano, protected.Until)
	if err != nil {
		t.Fatal(err)
	}
	minimum := start.Add(time.Duration(runtime.config.CognitiveResource.ModelProtectionMinutes)*time.Minute - time.Second)
	if until.Before(minimum) || !protected.RecoveryOffered {
		t.Fatalf("failed alternate did not apply the full bounded backoff: %#v", protected)
	}
}

func TestActionRealityIsTheOnlyNextAttentionCandidate(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Background = []Event{
		{ID: "inner", Seq: 1, Kind: "endogenous_change", Status: "pending"},
		{ID: "reality", Seq: 2, Kind: "action_result", Status: "pending"},
		{ID: "message", Seq: 3, Kind: "mentor_received", Status: "pending"},
	}
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "reality" || len(request.Candidates) != 1 {
		t.Fatalf("action reality did not get causal priority: %#v", request)
	}
}

func TestOneConcernCannotFormTwoUnassimilatedCommitments(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "concern-1", OriginKind: "endogenous_change"}}
	runtime.state.Commitments = []ActionCommitment{{ID: "existing", ConcernID: "concern-1", Status: "reality_available"}}
	runtime.activeCandidates = map[string]Event{"concern-1": {ID: "concern-1", Kind: "concern", ConcernID: "concern-1"}}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: "concern-1", Meaning: "继续探索", Difference: 0.5, Ownership: 1, Value: 0.5, Urgency: 0.3, Answerability: 0.8, Certainty: 0.8, Resolution: "hold"}},
		FocusID:    "concern-1", ThoughtThread: "我又想行动",
		Action:         CognitiveAction{Kind: "body_shell", Command: "true", Intent: "继续", Prediction: "成功", RealityCheck: "退出码"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil {
		t.Fatal("a second unassimilated commitment was accepted for one concern")
	}
}

func TestVariationBiasIsReproducibleAndDoesNotOverrideReality(t *testing.T) {
	build := func(instance string, pulse uint64) CognitiveRequest {
		runtime, err := New(t.TempDir(), instance, testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
		if err != nil {
			t.Fatal(err)
		}
		runtime.state.PulseID = pulse
		runtime.state.ExplorationPressure = 0.8
		runtime.state.Experiences = []Experience{
			{Meaning: "我确认了一个真实边界。"},
			{Meaning: "我留下了一份属于自己的材料。"},
			{Meaning: "我仍在等待一个外部现实发生变化。"},
		}
		runtime.state.Background = []Event{{ID: "explore", Kind: "endogenous_change", Status: "pending"}}
		request, ok := runtime.nextStage4Request()
		if !ok {
			t.Fatal("no exploration request")
		}
		return request
	}
	first := build("same", 7)
	second := build("same", 7)
	if first.VariationSeed != second.VariationSeed || first.VariationBias != second.VariationBias || first.VariationBias == "" {
		t.Fatalf("variation was not reproducible: %#v %#v", first, second)
	}
	for _, forbidden := range []string{"存续：", "联结：", "扩展："} {
		if strings.Contains(first.VariationBias, forbidden) {
			t.Fatalf("variation still supplied an experimenter-authored direction: %q", first.VariationBias)
		}
	}
	seen := map[string]bool{}
	for pulse := uint64(1); pulse <= 24; pulse++ {
		seen[build("same", pulse).VariationBias] = true
	}
	if len(seen) < 2 {
		t.Fatalf("program randomness did not vary the recalled lived material: %#v", seen)
	}
}

func TestVariationWithoutLivedMaterialOffersAnApproachLens(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Body = BodySnapshot{
		NetworkAvailable: true, DesktopAvailable: true, ChromeAvailable: true,
		PlaywrightReady: true, WechatRunning: true,
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Background = []Event{{ID: "explore", Kind: "endogenous_change", Status: "pending"}}
	request, ok := runtime.nextStage4Request()
	if !ok {
		t.Fatal("no exploration request")
	}
	if request.VariationBias == "" || !strings.Contains(request.VariationBias, "随机变化视角") {
		t.Fatalf("an empty life did not receive an approach variation: %q", request.VariationBias)
	}
	for _, forbidden := range []string{"关注某人", "发布帖子", "选择主题", "向导师询问"} {
		if strings.Contains(request.VariationBias, forbidden) {
			t.Fatalf("the orienting cue supplied a goal or required action: %q", request.VariationBias)
		}
	}
	seen := map[string]bool{}
	for pulse := uint64(1); pulse <= 32; pulse++ {
		runtime.state.PulseID = pulse
		runtime.state.Background[0].Status = "pending"
		next, ok := runtime.nextStage4Request()
		if !ok {
			t.Fatal("no exploration request while checking approach variation")
		}
		seen[next.VariationBias] = true
	}
	if len(seen) < 2 {
		t.Fatalf("an empty life remained in one model attractor: %#v", seen)
	}
}

func TestMatureExplorationOffersVariationAtAConcretePerception(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	perception := Event{ID: "world-object", Kind: "perceptual_change", Source: "observed", Status: "pending", Summary: "一个真实的新对象"}
	runtime.state.ExplorationPressure = 0.9
	if !runtime.shouldOfferVariation(perception) {
		t.Fatal("mature exploration could not vary its approach to a fresh concrete object")
	}
	runtime.state.ExplorationPressure = 0.1
	if runtime.shouldOfferVariation(perception) {
		t.Fatal("an ordinary perception received exploration variation without mature pressure")
	}
}

func TestStageEightExplorationDriveDoesNotManufactureAnObject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.44
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("a drive without perceptual difference manufactured a candidate: %#v", runtime.state.Background)
	}
}

func TestBrowserSnapshotTextKeepsOnlyVisibleTextContent(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"Page title\nConcrete item"},{"type":"image","data":"ignored"},{"type":"text","text":"Second item"}]}`)
	got, err := browserSnapshotText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Page title\nConcrete item\nSecond item" {
		t.Fatalf("browser perception lost visible content: %q", got)
	}
}

func TestBrowserToolResultJSONAndDirectAffordanceRemainSeparate(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"### Result\n[\"https://x.com/alice/status/123\",\"\"]\n### Ran Playwright code\nignored"}]}`)
	var directURLs []string
	if err := browserToolResultJSON(raw, &directURLs); err != nil {
		t.Fatal(err)
	}
	objects := []string{"Alice posted a concrete idea", "An advertisement"}
	augmented := appendDirectBrowserURLs(objects, directURLs)
	if augmented[0] != objects[0]+" Direct URL: https://x.com/alice/status/123" {
		t.Fatalf("a real contact route was not attached to its object: %#v", augmented)
	}
	if augmented[1] != objects[1] {
		t.Fatalf("an absent route was manufactured: %#v", augmented)
	}
	if objects[0] != "Alice posted a concrete idea" {
		t.Fatal("affordance attachment mutated the stable source object")
	}
}

func TestDirectBrowserAffordanceRejectsNonHTTPSRoutes(t *testing.T) {
	objects := []string{"one", "two", "three"}
	augmented := appendDirectBrowserURLs(objects, []string{"javascript:alert(1)", "file:///tmp/private", "https://example.com/object"})
	if augmented[0] != "one" || augmented[1] != "two" || augmented[2] != "three Direct URL: https://example.com/object" {
		t.Fatalf("browser affordance boundary accepted an invalid route: %#v", augmented)
	}
}

func TestXBrowserDOMObservationBindsObjectAndContactRoute(t *testing.T) {
	observation, err := xBrowserDOMObservation(browserDOMPerception{
		URL: "https://x.com/home", Title: "Home / X",
		Objects: []browserDOMObject{
			{Text: "Alice\n@alice\nA concrete idea", DirectURL: "https://x.com/alice/status/123"},
			{Text: "A concrete advertised tool", DirectURL: "https://example.com/tool"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Objects) != 2 || !strings.Contains(observation.Objects[0], "A concrete idea Direct URL: https://x.com/alice/status/123") {
		t.Fatalf("perceived object and its contact route were separated: %#v", observation)
	}
	if !strings.Contains(observation.Objects[1], "A concrete advertised tool Direct URL: https://example.com/tool") {
		t.Fatalf("an external authored-object route was not given a stable identity: %#v", observation)
	}
	if perceptualObjectDigest("old text Direct URL: https://x.com/alice/status/123") != perceptualObjectDigest("changed metrics Direct URL: https://x.com/alice/status/123") {
		t.Fatal("presentation drift changed the identity of a directly addressable object")
	}
}

func TestBrowserSemanticSnapshotIgnoresEngagementDrift(t *testing.T) {
	first := `### Page
- Page URL: https://x.com/home
- Page Title: Home / X
### Snapshot
- article "Alice @alice 47 minutes ago A stable idea 5 replies, 2 reposts, 34 likes, 2831 views" [ref=e283]
- article "Bob @bob 2 hours ago Another object 1 replies, 8 likes, 50 views" [ref=e500]`
	second := `### Page
- Page URL: https://x.com/home
- Page Title: Home / X
### Snapshot
- article "Alice @alice 48 minutes ago A stable idea 6 replies, 2 reposts, 39 likes, 3010 views" [ref=e900]
- article "Bob @bob 2 hours ago Another object 1 replies, 9 likes, 60 views" [ref=e999]`
	firstSemantic := browserSemanticSnapshot(first)
	secondSemantic := browserSemanticSnapshot(second)
	if firstSemantic != secondSemantic {
		t.Fatalf("volatile engagement changed the perceived object:\nfirst=%q\nsecond=%q", firstSemantic, secondSemantic)
	}
	changed := strings.Replace(second, "Another object", "A genuinely new object", 1)
	if browserSemanticSnapshot(changed) == firstSemantic {
		t.Fatal("a changed visible object was suppressed as metric drift")
	}
}

func TestBrowserSemanticSnapshotIgnoresMediaPlaybackClock(t *testing.T) {
	first := `### Page
- Page URL: https://x.com/SpaceX
- Page Title: SpaceX / X
### Snapshot
- article "SpaceX @SpaceX A recovered vehicle Embedded video 5:43 34 likes" [ref=e283]`
	second := strings.Replace(first, "5:43", "4:46", 1)
	if browserSemanticSnapshot(first) != browserSemanticSnapshot(second) {
		t.Fatal("media playback time changed the perceived object")
	}
}

func TestBrowserSemanticSnapshotIgnoresVideoControlExpansion(t *testing.T) {
	plain := `### Page
- Page URL: https://x.com/home
- Page Title: Home / X
### Snapshot
- article "Nature @nature 2 hours ago Bear misses the tree Embedded video Play Video. 13 seconds long 1 reply, 20 likes" [ref=e1]`
	expanded := `### Page
- Page URL: https://x.com/home
- Page Title: Home / X
### Snapshot
- article "Nature @nature 2 hours ago Bear misses the tree Embedded video 0:08 of 0:13 0:08 / 0:13 Unmute 100 percent Video Settings Picture-in-Picture Full screen 1 reply, 21 likes" [ref=e2]`
	if browserSemanticSnapshot(plain) != browserSemanticSnapshot(expanded) {
		t.Fatal("expanded video controls changed the perceived post identity")
	}
}

func TestBrowserSemanticSnapshotDoesNotTreatXInterfaceChromeAsContent(t *testing.T) {
	snapshot := `### Page
- Page URL: https://x.com/explore
- Page Title: Explore / X
### Snapshot
- heading "To view keyboard shortcuts, press question mark View keyboard shortcuts" [level=2] [ref=e3]
- link "View keyboard shortcuts" [ref=e5]
- link "X" [ref=e25]`
	contextLines, objects := browserSemanticObjects(snapshot)
	if len(contextLines) != 2 {
		t.Fatalf("page identity disappeared with interface chrome: %#v", contextLines)
	}
	if len(objects) != 0 {
		t.Fatalf("X interface affordances became perceived world objects: %#v", objects)
	}
}

func TestBrowserSemanticSnapshotSeparatesMainContentFromInterfaceAffordances(t *testing.T) {
	snapshot := `### Page
- Page URL: https://www.google.com/search?q=example
- Page Title: example - Google Search
### Snapshot
- generic [ref=e1]:
  - link "Skip to main content" [ref=e2]
  - link "Accessibility help" [ref=e3]
  - main [ref=e10]:
    - heading "Search Results" [level=1] [ref=e11]
    - generic [ref=e12]:
      - link "A concrete result Example https://example.com" [ref=e13]
        - heading "A concrete result" [level=3] [ref=e14]
    - navigation [ref=e20]:
      - link "Page 2" [ref=e21]
  - contentinfo [ref=e30]:
    - link "Privacy" [ref=e31]`
	_, objects := browserSemanticObjects(snapshot)
	want := []string{"Search Results", "A concrete result Example https://example.com", "A concrete result"}
	if !reflect.DeepEqual(objects, want) {
		t.Fatalf("passive perception did not preserve main content boundary: got %#v want %#v", objects, want)
	}
}

func TestBrowserSemanticNamedObjectsIgnoreVolatileReferences(t *testing.T) {
	first := `### Page
- Page URL: https://example.com/
- Page Title: Example
### Snapshot
- main [ref=e1]:
  - heading "Stable idea" [level=1] [ref=e2]`
	second := strings.ReplaceAll(strings.ReplaceAll(first, "e1", "e900"), "e2", "e901")
	if browserSemanticSnapshot(first) != browserSemanticSnapshot(second) {
		t.Fatal("accessibility node references changed a perceived object identity")
	}
}

func TestPerceptualNoveltyAdmitsOneConcreteObjectAtATime(t *testing.T) {
	contextLines := []string{"Page URL: https://x.com/home", "Page Title: Home / X"}
	first := perceptualObservation{Context: contextLines, Objects: []string{"first object", "second object"}}
	trace := queuePerceptualNovelty(PerceptualTrace{}, first)
	if len(trace.Pending) != 2 || len(trace.Seen) != 0 {
		t.Fatalf("concrete objects did not remain individually available: %#v", trace)
	}
	trace, content := takePerceptualNovelty(trace)
	if !strings.Contains(content, "first object") || strings.Contains(content, "second object") || len(trace.Pending) != 1 || len(trace.Seen) != 1 {
		t.Fatalf("one attention pulse did not receive exactly one object: content=%q trace=%#v", content, trace)
	}
	second := perceptualObservation{Context: contextLines, Objects: []string{"first object", "second object", "third object"}}
	trace = queuePerceptualNovelty(trace, second)
	if len(trace.Pending) != 2 || trace.Pending[0] != "second object" || trace.Pending[1] != "third object" {
		t.Fatalf("seen and already queued objects were admitted again: %#v", trace)
	}
	trace, content = takePerceptualNovelty(trace)
	if !strings.Contains(content, "second object") || strings.Contains(content, "third object") {
		t.Fatalf("the next object did not retain its own attention turn: %q", content)
	}
	trace, _ = takePerceptualNovelty(trace)
	trace = queuePerceptualNovelty(trace, second)
	if len(trace.Pending) != 0 {
		t.Fatalf("a fully habituated field became another candidate: %#v", trace)
	}
}

func TestPerceptualSurfaceKeepsAndConsumesNestedReturnPath(t *testing.T) {
	home := perceptualObservation{Context: []string{"Page URL: https://x.com/home", "Page Title: Home / X"}, Objects: []string{"feed object"}}
	detail := perceptualObservation{Context: []string{"Page URL: https://x.com/alice/status/123", "Page Title: Alice / X"}, Objects: []string{"detail object"}}
	reply := perceptualObservation{Context: []string{"Page URL: https://x.com/bob/status/456", "Page Title: Bob / X"}, Objects: []string{"reply object"}}
	trace := queuePerceptualNovelty(PerceptualTrace{}, home)
	trace = queuePerceptualNovelty(trace, detail)
	trace = queuePerceptualNovelty(trace, reply)
	if got := trace.ReturnPath; len(got) != 2 || got[0] != "https://x.com/home" || got[1] != "https://x.com/alice/status/123" {
		t.Fatalf("detail perception forgot its prior generative surface: %#v", trace)
	}
	trace = queuePerceptualNovelty(trace, detail)
	if got := trace.ReturnPath; len(got) != 1 || got[0] != "https://x.com/home" {
		t.Fatalf("returning from a nested reply forgot the broader feed: %#v", trace)
	}
	trace = queuePerceptualNovelty(trace, home)
	if len(trace.ReturnPath) != 0 {
		t.Fatalf("returning to the prior surface created a navigation bounce: %#v", trace)
	}
	if validNavigableBrowserURL("file:///agent/private") != "" || validNavigableBrowserURL("javascript:alert(1)") != "" {
		t.Fatal("a non-network surface became an automatic return route")
	}
}

func TestExhaustedNestedSurfaceReturnsBeforeFreshFragmentsReopenIt(t *testing.T) {
	contextLines := []string{"Page URL: https://x.com/alice/status/123", "Page Title: Alice / X"}
	trace := PerceptualTrace{
		Context:          contextLines,
		Pending:          []string{"another unseen low-yield reply"},
		Saturation:       0.7,
		ExhaustedContext: perceptualContextKey(contextLines),
		ExhaustedAt:      nowUTC(),
		ReturnPath:       []string{"https://x.com/home"},
	}
	if !perceptualReturnDue(trace) {
		t.Fatal("an exhausted nested scene was reopened by another unseen fragment")
	}
	trace = discardPendingPerception(trace)
	if len(trace.Pending) != 0 {
		t.Fatalf("a fragment from an exhausted scene survived its retreat: %#v", trace)
	}

	trace.ReturnPath = nil
	if perceptualReturnDue(trace) {
		t.Fatal("a root scene without a prior surface attempted a semantic retreat")
	}
}

func TestPerceptualExhaustionStaysQuietUntilRealityCanBeResampled(t *testing.T) {
	trace := PerceptualTrace{Digest: "stable", Context: []string{"Page URL: https://x.com/home"}}
	now := time.Now().UTC()
	if !perceptualResampleDue(trace, now, 300) {
		t.Fatal("an exhausted concrete surface was not recorded")
	}
	trace.ExhaustedContext = perceptualContextKey(trace.Context)
	trace.ExhaustedAt = now.Format(time.RFC3339Nano)
	if perceptualResampleDue(trace, now.Add(299*time.Second), 300) {
		t.Fatal("the same exhausted surface reopened before its quiet interval")
	}
	if !perceptualResampleDue(trace, now.Add(300*time.Second), 300) {
		t.Fatal("persistent exploration never regained a low-frequency sensory sample")
	}
	trace.Saturation = 0.8
	trace = reopenPerceptualSampling(trace)
	if trace.ExhaustedContext != "" || trace.ExhaustedAt != "" || trace.Saturation != 0 {
		t.Fatalf("a reopened sensory window retained exhausted control state: %#v", trace)
	}
	observation := perceptualObservation{Digest: "changed", Context: []string{"Page URL: https://x.com/explore"}, Objects: []string{"new object"}}
	trace = queuePerceptualNovelty(trace, observation)
	if trace.ExhaustedContext != "" || trace.ExhaustedAt != "" || len(trace.Pending) != 1 || trace.Saturation != 0 {
		t.Fatalf("new reality did not reopen perception: %#v", trace)
	}
}

func TestLowValuePerceptionAccumulatesSurfaceSaturation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = map[string]PerceptualTrace{browserPerceptionSurface: {
		Context: []string{"Page URL: https://x.com/home"}, ExhaustedContext: "old", ExhaustedAt: nowUTC(),
	}}
	candidate := Event{ID: "ad", Kind: "perceptual_change"}
	appraisal := CandidateAppraisal{Ownership: 0.1, Value: 0, Certainty: 1}
	runtime.applyPerceptualSaturation(candidate, appraisal, CognitiveCommit{Action: CognitiveAction{Kind: "none"}})
	first := runtime.state.Perception[browserPerceptionSurface].Saturation
	if first <= 0 || first >= runtime.config.Dynamics.AttentionThreshold {
		t.Fatalf("one low-value object did not create bounded saturation: %f", first)
	}
	runtime.applyPerceptualSaturation(candidate, appraisal, CognitiveCommit{Action: CognitiveAction{Kind: "none"}})
	if runtime.state.Perception[browserPerceptionSurface].Saturation < runtime.config.Dynamics.AttentionThreshold {
		t.Fatal("repeated low-value objects did not make the surface lose salience")
	}
	runtime.applyPerceptualSaturation(candidate, appraisal, CognitiveCommit{Action: CognitiveAction{Kind: "body_shell"}})
	got := runtime.state.Perception[browserPerceptionSurface]
	if got.Saturation < runtime.config.Dynamics.AttentionThreshold || got.ExhaustedContext != "" || got.ExhaustedAt != "" {
		t.Fatal("an action attempt erased realised low-yield history before Reality arrived")
	}
}

func TestLowYieldPerceptualRealityAccumulatesSurfaceSaturation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = map[string]PerceptualTrace{browserPerceptionSurface: {
		Context: []string{"Page URL: https://x.com/home"}, Saturation: 0.25,
	}}
	runtime.state.Concerns = []Concern{{ID: "feed-object", OriginKind: "perceptual_change", Resolution: "hold"}}
	runtime.state.Commitments = []ActionCommitment{{ID: "probe", ConcernID: "feed-object", InitialDifference: 0.6}}
	candidate := Event{ID: "reality", Kind: "action_result", ConcernID: "feed-object"}
	commit := CognitiveCommit{
		Action: CognitiveAction{Kind: "none"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: "probe", ExperiencedCost: 0.1,
			Values: EndogenousValues{Expansion: 0.4, SelfEndorsed: 0.5},
		}},
	}
	runtime.applyPerceptualSaturation(candidate, CandidateAppraisal{Difference: 0.55}, commit)
	if got := runtime.state.Perception[browserPerceptionSurface].Saturation; got <= 0.25 {
		t.Fatalf("a low-yield result made its source surface look newly productive: %f", got)
	}
}

func TestHighYieldPerceptualRealityRelievesSurfaceSaturation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = map[string]PerceptualTrace{browserPerceptionSurface: {
		Context: []string{"Page URL: https://x.com/home"}, Saturation: 0.4,
	}}
	runtime.state.Concerns = []Concern{{ID: "feed-object", OriginKind: "perceptual_change", Resolution: "hold"}}
	runtime.state.Commitments = []ActionCommitment{{ID: "contact", ConcernID: "feed-object", InitialDifference: 0.8}}
	candidate := Event{ID: "reality", Kind: "action_result", ConcernID: "feed-object"}
	commit := CognitiveCommit{
		Action: CognitiveAction{Kind: "none"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: "contact",
			Values:       EndogenousValues{Relatedness: 0.9, Expansion: 0.8, SelfEndorsed: 0.9},
		}},
	}
	runtime.applyPerceptualSaturation(candidate, CandidateAppraisal{Difference: 0.1}, commit)
	if got := runtime.state.Perception[browserPerceptionSurface].Saturation; got >= 0.4 {
		t.Fatalf("a high-yield result did not restore its source surface: %f", got)
	}
}

func TestValuedButUnanswerablePerceptionStillSaturatesItsSurface(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = map[string]PerceptualTrace{browserPerceptionSurface: {Context: []string{"Page URL: https://x.com/search"}}}
	candidate := Event{ID: "important-fragment", Kind: "perceptual_change"}
	appraisal := CandidateAppraisal{Ownership: 0.9, Value: 0.9, Answerability: 0.1, Certainty: 1}
	runtime.applyPerceptualSaturation(candidate, appraisal, CognitiveCommit{Action: CognitiveAction{Kind: "none"}})
	if got := runtime.state.Perception[browserPerceptionSurface].Saturation; got <= 0.2 {
		t.Fatalf("an important but unanswerable fragment made its surface look productive: %f", got)
	}
}

func TestPerceptualConcernRevisitWithoutActionContributesToSurfaceSaturation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = map[string]PerceptualTrace{browserPerceptionSurface: {Context: []string{"Page URL: https://x.com/search"}}}
	runtime.state.Concerns = []Concern{{ID: "held-fragment", OriginKind: "perceptual_change", Resolution: "hold"}}
	candidate := Event{ID: "held-fragment", Kind: "concern", ConcernID: "held-fragment"}
	appraisal := CandidateAppraisal{Ownership: 0.8, Value: 0.8, Answerability: 0.2, Certainty: 1}
	runtime.applyPerceptualSaturation(candidate, appraisal, CognitiveCommit{Action: CognitiveAction{Kind: "none"}})
	if got := runtime.state.Perception[browserPerceptionSurface].Saturation; got <= 0 {
		t.Fatal("revisiting a perceptual concern without a useful response did not reduce the surface's yield")
	}
}

func TestPerceptualConcernAdmissionDistinguishesNoticingFromAssuming(t *testing.T) {
	threshold := testConfig(8).Dynamics.AttentionThreshold
	noticed := CandidateAppraisal{Ownership: 0.8, Value: 0.8, Urgency: 0.2, Answerability: 0.2}
	if perceptualAppraisalAssumesConcern(noticed, threshold) {
		t.Fatal("a valued but currently background observation became an independent concern")
	}
	answerable := noticed
	answerable.Answerability = 0.7
	if !perceptualAppraisalAssumesConcern(answerable, threshold) {
		t.Fatal("a self-owned, valued and answerable perception could not become a concern")
	}
	urgent := noticed
	urgent.Urgency = 0.8
	if !perceptualAppraisalAssumesConcern(urgent, threshold) {
		t.Fatal("an urgent perception was discarded merely because it was not yet answerable")
	}
}

func TestPerceptualExhaustionNeverBecomesAttentionCandidate(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	trace := PerceptualTrace{
		Context:    []string{"Page URL: https://x.com/home", "Page Title: Home / X"},
		Pending:    []string{"discarded low-yield object"},
		Saturation: 0.8,
	}
	if err := runtime.recordBrowserPerceptualExhaustion(trace, "low realised yield"); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 || len(runtime.state.Concerns) != 0 {
		t.Fatalf("a sensory absence entered cognition: background=%#v concerns=%#v", runtime.state.Background, runtime.state.Concerns)
	}
	stored := runtime.state.Perception[browserPerceptionSurface]
	if stored.ExhaustedContext == "" || stored.ExhaustedAt == "" || len(stored.Pending) != 0 {
		t.Fatalf("sensory exhaustion was not retained as control state: %#v", stored)
	}
}

func TestQuietExplorationDoesNotPromotePastExperienceWithoutCurrentObject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.9
	runtime.state.Experiences = []Experience{
		{ID: "lived", ObservedAt: nowUTC(), Meaning: "我曾真实尝试打开一个具体入口，但现实结果留下较大回差。", Lesson: "入口事实和内容证据不同。", Values: EndogenousValues{Expansion: 0.9, SelfEndorsed: 0.9}, PredictionDifference: 0.8, RemainingDifference: 0.8, Significance: "reusable"},
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("past experience became current attention without a present object: %#v", request)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("past experience manufactured a background event: %#v", runtime.state.Background)
	}
}

func TestVariationDoesNotResurrectADecayedConcern(t *testing.T) {
	dynamics := testConfig(8).Dynamics
	state := State{
		Concerns: []Concern{{
			ID: "old-relationship", Meaning: "请让导师替我提供下一项方向",
			Resolution: "hold", Strength: 0, Activation: 0,
		}},
		Experiences: []Experience{{Meaning: "我刚从现实中形成了一条当前经验。"}},
	}
	for index := 0; index < 32; index++ {
		got := associativeRecall(state, dynamics, fmt.Sprintf("seed-%d", index))
		if strings.Contains(got, "导师替我") {
			t.Fatalf("a decayed concern was revived by random recall: %q", got)
		}
		if got == "" {
			t.Fatal("current lived material disappeared with the stale concern")
		}
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
		FocusID:        "other",
		ThoughtThread:  "此刻关系信号更值得我注意。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
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

func TestSettledChildRemainsAsEvidenceUntilParentSettles(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "parent", Strength: 0.4, Ownership: 0.9, Resolution: "hold"},
		{ID: "completed-child", WithinConcernID: "parent", Resolution: "resolved"},
		{ID: "released-child", WithinConcernID: "parent", Resolution: "released"},
		{ID: "unrelated-completed", Resolution: "resolved"},
	}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 3 {
		t.Fatalf("settled child evidence did not follow its held parent: %#v", runtime.state.Concerns)
	}
	for index := range runtime.state.Concerns {
		if runtime.state.Concerns[index].ID == "parent" {
			runtime.state.Concerns[index].Resolution = "resolved"
		}
	}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("completed branch remained after its parent settled: %#v", runtime.state.Concerns)
	}
}

func TestCompositeParentLocalProgressIsNormalizedWithoutRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "parent", Resolution: "hold", ClosureCondition: "所有独立后果都已完成"},
		{ID: "first-child", WithinConcernID: "parent", Resolution: "resolved"},
	}
	runtime.activeCandidates = map[string]Event{
		"parent-action-result": {ID: "parent-action-result", Kind: "action_result", ConcernID: "parent"},
		"parent":               {ID: "parent", Kind: "concern", ConcernID: "parent"},
	}
	localResult := CognitiveCommit{
		FocusID: "parent-action-result",
		Appraisals: []CandidateAppraisal{{
			CandidateID: "parent-action-result", Difference: 0.01, Ownership: 0.8, Resolution: "resolved",
		}},
	}
	if got := runtime.normalizeCompositeProgressDisposition(&localResult, "parent"); got != "resolved" {
		t.Fatalf("local composite resolution was not exposed as normalized: %q", got)
	}
	if localResult.Appraisals[0].Resolution != "hold" {
		t.Fatalf("one local result closed a composite parent without a whole-concern revisit: %#v", localResult.Appraisals[0])
	}
	directRevisit := CognitiveCommit{
		FocusID: "parent",
		Appraisals: []CandidateAppraisal{{
			CandidateID: "parent", Difference: 0.01, Ownership: 0.8, Resolution: "resolved",
		}},
	}
	if got := runtime.normalizeCompositeProgressDisposition(&directRevisit, "parent"); got != "" || directRevisit.Appraisals[0].Resolution != "resolved" {
		t.Fatalf("direct whole-concern appraisal was changed: got=%q appraisal=%#v", got, directRevisit.Appraisals[0])
	}
}

func TestCompositeParentContributionIsNormalizedWithoutRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{
		{ID: "parent", Resolution: "hold", ClosureCondition: "所有独立后果都已完成"},
		{ID: "child", WithinConcernID: "parent", Resolution: "hold"},
	}
	runtime.activeCandidates = map[string]Event{
		"progress": {ID: "progress", Kind: "concern_contribution", ConcernID: "parent"},
	}
	commit := CognitiveCommit{
		FocusID: "progress",
		Appraisals: []CandidateAppraisal{{
			CandidateID: "progress", Difference: 0.01, Ownership: 0.8, Resolution: "resolved",
		}},
	}
	if got := runtime.normalizeCompositeProgressDisposition(&commit, "parent"); got != "resolved" || commit.Appraisals[0].Resolution != "hold" {
		t.Fatalf("one contribution was not kept as local progress: got=%q appraisal=%#v", got, commit.Appraisals[0])
	}
}

func TestParentConcernContextShowsEveryDirectChildDisposition(t *testing.T) {
	concerns := []Concern{
		{ID: "parent", Resolution: "hold", ClosureCondition: "三个组成后果均已闭合"},
		{ID: "child-a", WithinConcernID: "parent", Subject: "A", Meaning: "A 已完成", Resolution: "resolved"},
		{ID: "child-b", WithinConcernID: "parent", Subject: "B", Meaning: "B 仍待处理", Resolution: "hold"},
		{ID: "child-c", WithinConcernID: "parent", Subject: "C", Meaning: "C 已明确放下", Resolution: "released"},
	}
	views := contextConcernViews(concerns, []Event{{ID: "parent", Kind: "concern", ConcernID: "parent"}})
	var parent map[string]any
	for _, view := range views {
		if view["concern_id"] == "parent" {
			parent = view
			break
		}
	}
	if parent == nil {
		t.Fatal("parent was omitted from context")
	}
	if parent["within_child_count"] != 3 || parent["held_child_count"] != 1 || parent["settled_child_count"] != 2 {
		t.Fatalf("parent child ledger is incomplete: %#v", parent)
	}
	children, ok := parent["within_children"].([]map[string]any)
	if !ok || len(children) != 3 {
		t.Fatalf("parent child facts are unavailable: %#v", parent["within_children"])
	}
}

func TestDormantHeldConcernRetainsIdentityWithoutDemandingAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{
		ID: "dormant", Strength: 0.05, Activation: 0.02, Ownership: 0.9, Resolution: "hold",
		LastSourceID: "dormant", LastFocusedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
	}}
	runtime.state.Experiences = []Experience{{ID: "experience-kept", Meaning: "这段事实仍属于我的经历"}}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != "dormant" {
		t.Fatalf("a self-owned held concern lost its dormant identity: %#v", runtime.state.Concerns)
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("a dormant concern demanded attention without new cause: %#v", request)
	}
	if len(runtime.state.Experiences) != 1 || runtime.state.Experiences[0].ID != "experience-kept" {
		t.Fatalf("dormancy deleted lived experience: %#v", runtime.state.Experiences)
	}
}

func TestConcernActivationDecaysWithPresentTime(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{ID: "remembered", Strength: 0.5, Activation: 0.8, Resolution: "hold"}}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	want := 0.8 * (1 - runtime.config.Dynamics.ConcernNaturalDecayRate)
	if absFloat(runtime.state.Concerns[0].Activation-want) > 0.000001 {
		t.Fatalf("concern activation did not decay: got %f want %f", runtime.state.Concerns[0].Activation, want)
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
			ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
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
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Strength: 0.4, Resolution: "hold", Answerability: 0.8}}
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
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Strength: 0.3, Resolution: "hold", Answerability: 0.8}}
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
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if absFloat(runtime.state.ExplorationPressure-0.648) > 0.000001 {
		t.Fatalf("alice's interpretation of real exploration did not relieve pressure: %f", runtime.state.ExplorationPressure)
	}
	for _, concern := range runtime.state.Concerns {
		if concern.OriginKind == "endogenous_change" {
			t.Fatalf("resolved exploration concern remained active: %#v", concern)
		}
	}
}

func TestStageFiveUnrelatedRealityCannotSatisfyExploration(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	origin := Event{ID: "body-origin", Kind: "body_delta", Status: "processed"}
	commitment := ActionCommitment{ID: "commitment-body", FocusID: origin.ID, InitialDifference: 0.3, ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-body", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	reality := Event{ID: "body-result", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{origin, reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: reality.ID, Meaning: "身体事实得到确认", Difference: 0, Ownership: 1, Value: 0.3, Urgency: 0, Answerability: 1, Certainty: 1, Resolution: "resolved"}},
		FocusID:    reality.ID, ThoughtThread: "这次核验已经完成。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:    CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID, Meaning: "身体事实明确。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.ExplorationPressure != 0.8 {
		t.Fatalf("an unrelated body check falsely satisfied exploration: %f", runtime.state.ExplorationPressure)
	}
}

func TestStageFiveExplorationRealityCanRelieveItsOwnTension(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	origin := Event{ID: "exploration-origin", Kind: "endogenous_change", Status: "processed"}
	commitment := ActionCommitment{ID: "commitment-exploration", FocusID: origin.ID, InitialDifference: 0.5, ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-exploration", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	reality := Event{ID: "exploration-result", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{origin, reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: reality.ID, Meaning: "主动接触获得了现实回应", Difference: 0, Ownership: 1, Value: 0.7, Urgency: 0, Answerability: 1, Certainty: 1, Resolution: "resolved"}},
		FocusID:    reality.ID, ThoughtThread: "这次探索已经得到现实回应。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:    CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{CommitmentID: commitment.ID, Meaning: "现实接触满足了当前探索。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if absFloat(runtime.state.ExplorationPressure-0.35) > 0.000001 {
		t.Fatalf("exploration did not metabolize its own real result: %f", runtime.state.ExplorationPressure)
	}
}

func TestRelevantOlderExperienceReturnsToCurrentAttention(t *testing.T) {
	experiences := []Experience{{ID: "browser-old", Meaning: "浏览器工具清单只证明能力，页面快照才提供真实页面事实。"}}
	for index := 0; index < 8; index++ {
		experiences = append(experiences, Experience{ID: fmt.Sprintf("recent-%d", index), Meaning: "生活空间文件核验完成。"})
	}
	candidate := Event{ID: "browser-now", Summary: "准备观察浏览器页面", Payload: json.RawMessage(`{"tool":"browser_snapshot"}`)}
	selected := selectContextExperiences(experiences, []Event{candidate})
	found := false
	for _, experience := range selected {
		found = found || experience.ID == "browser-old"
	}
	if !found {
		t.Fatalf("relevant older browser experience was forgotten: %#v", selected)
	}
}

func TestAliceChoosesWhichDurableMethodSlotChanges(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxSelfMethods; index++ {
		runtime.state.Self.Methods = append(runtime.state.Self.Methods, fmt.Sprintf("method-%d", index))
	}
	runtime.state.Commitments = []ActionCommitment{{ID: "commitment-1", Status: "reality_available"}}
	commit := CognitiveCommit{
		FocusID:    "result-1",
		Appraisals: []CandidateAppraisal{{CandidateID: "result-1", Difference: 0.1, Resolution: "resolved"}},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: "commitment-1", Meaning: "形成了可迁移的新方法。", Significance: "reusable",
			MethodUpdate: "把现实结果带回下一次选择。", MethodSlot: 3,
		}},
	}
	if err := runtime.applyExperienceUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Self.Methods[3] != "把现实结果带回下一次选择。" || runtime.state.Self.Methods[2] != "method-2" {
		t.Fatalf("method replacement did not follow Alice's selected slot: %#v", runtime.state.Self.Methods)
	}
	if runtime.state.TotalExperiences != 1 {
		t.Fatalf("lifetime experience count = %d, want 1", runtime.state.TotalExperiences)
	}
}

func TestExperienceStructureDeterminesEffectiveSignificance(t *testing.T) {
	tests := []struct {
		name             string
		update           ExperienceUpdate
		narrativeUpdated bool
		want             string
	}{
		{name: "lesson only", update: ExperienceUpdate{Significance: "reusable"}, want: "ordinary"},
		{name: "method consequence", update: ExperienceUpdate{Significance: "ordinary", MethodUpdate: "以后这样做。"}, want: "reusable"},
		{name: "narrative consequence", update: ExperienceUpdate{Significance: "ordinary"}, narrativeUpdated: true, want: "self_defining"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effectiveExperienceSignificance(test.update, test.narrativeUpdated); got != test.want {
				t.Fatalf("effective significance = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStageEightObservationDoesNotAutomaticallyBecomeConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "event-observed", Kind: "environment", Summary: "一个值得理解但无需承担的变化", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我理解了这项变化", Difference: 0.7, Ownership: 0.2,
			Value: 0.4, Urgency: 0.1, Answerability: 0.8, Certainty: 0.9, Resolution: "resolved",
		}},
		FocusID: event.ID, ThoughtThread: "看见它，然后让它回到背景。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("an observation became a durable concern: %#v", runtime.state.Concerns)
	}
}

func TestStageEightSelfOwnedHoldBecomesConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "event-owned", Kind: "environment", Summary: "一个由我愿意继续承接的变化", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我愿意继续理解并回应它", Difference: 0.8, Ownership: 0.9,
			Value: 0.8, Urgency: 0.5, Answerability: 0.8, Certainty: 0.8, Resolution: "hold",
		}},
		FocusID: event.ID, ThoughtThread: "这件事与我有关，我愿意保留它。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].OriginKind != event.Kind || runtime.state.Background[0].ConcernID == "" {
		t.Fatalf("a self-owned hold did not become one durable concern: %#v", runtime.state.Concerns)
	}
}

func TestStageEightSelfOwnedLowActivationHoldStillBecomesConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "event-impression", Kind: "mentor_received", Summary: "一条温和而无需后续承担的问候", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我愿意记得这份问候", Difference: 0.12, Ownership: 0.82,
			Value: 0.82, Urgency: 0.3, Answerability: 0.9, Certainty: 0.99, Resolution: "hold",
		}},
		FocusID: event.ID, ThoughtThread: "它影响了我，但此刻没有需要继续承担的问题。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("explicit self-owned hold was erased by low present activation: %#v", runtime.state.Concerns)
	}
}

func TestStageNineQuietInvitationCanBecomeAWaitingResponsibility(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "mentor-invitation", Kind: "mentor_received", Summary: "三个物件稍后进入生活空间", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID,
			Meaning:     "我愿意把这次邀请作为一项有边界的共同探索承接，并等待物件真实出现。",
			Difference:  0.28, Ownership: 0.70, Value: 0.68, Urgency: 0.22,
			Answerability: 0.62, Certainty: 0.90, Resolution: "hold",
		}},
		NewConcernClosureCondition: "我已自主判断三个物件，并把自己认领的重要结果带回共同关系。",
		ThoughtThread:              "这项责任安静地等待现实，不等于它没有被我承担。",
		Action:                     CognitiveAction{Kind: "none"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 {
		t.Fatalf("a quiet self-owned responsibility failed to persist: %#v", runtime.state.Concerns)
	}
	concern := runtime.state.Concerns[0]
	if concern.OriginKind != "mentor_received" || concern.ClosureCondition != commit.NewConcernClosureCondition || concern.Ownership != 0.70 {
		t.Fatalf("the waiting responsibility lost its self-authored boundary: %#v", concern)
	}
}

func TestStageEightEnactedCommitmentIsEmbodiedHold(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "event-enacted", Kind: "environment", Summary: "一个可以直接核验的变化", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我选择亲自核验", Difference: 0.6, Ownership: 0.9,
			Value: 0.7, Urgency: 0.4, Answerability: 0.9, Certainty: 0.8, Resolution: "reframed",
		}},
		FocusID: event.ID, ThoughtThread: "行动会把这项判断交给现实。",
		Action: CognitiveAction{
			Kind: "body_shell", Command: "date -Is", Intent: "取得一次现实核验", Prediction: "命令会返回当前时间",
			RealityCheck: "读取真实时间输出", StopCondition: "结果返回即停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("the enacted commitment did not remain embodied as held: %#v", runtime.state.Concerns)
	}
}

func TestStageEightOnlyTheSelectedFocusCanBecomeANewConcernWhileOwnedBackgroundWaits(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	focus := Event{ID: "chosen", Kind: "birth_orientation", Summary: "我选择承接的起点", Status: "in_focus"}
	background := Event{ID: "noticed", Kind: "body_delta", Summary: "同时注意到的身体变化", Status: "pending"}
	runtime.state.Background = []Event{focus, background}
	runtime.activeCandidates = map[string]Event{focus.ID: focus, background.ID: background}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: focus.ID, Meaning: "这是我此刻主动承担的起点", Difference: 0.7, Ownership: 0.9, Value: 0.8, Urgency: 0.5, Answerability: 0.8, Certainty: 0.9, Resolution: "hold"},
			{CandidateID: background.ID, Meaning: "它也影响我，但这一次只是背景理解", Difference: 0.6, Ownership: 0.9, Value: 0.8, Urgency: 0.5, Answerability: 0.8, Certainty: 0.9, Resolution: "hold"},
		},
		ThoughtThread:  "我同时看见两件事，只承接一个焦点。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].LastSourceID != focus.ID {
		t.Fatalf("background noticing created a parallel concern: %#v", runtime.state.Concerns)
	}
	if runtime.state.Background[1].Status != "pending" {
		t.Fatalf("a self-owned non-focus event was discarded instead of waiting for single-threaded attention: %#v", runtime.state.Background[1])
	}
}

func TestStageEightUnownedBackgroundReturnsToStaticBackground(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	focus := Event{ID: "chosen", Kind: "environment_change", Summary: "当前焦点", Status: "in_focus"}
	background := Event{ID: "noticed", Kind: "body_delta", Summary: "只需知道的背景", Status: "pending"}
	runtime.state.Background = []Event{focus, background}
	runtime.activeCandidates = map[string]Event{focus.ID: focus, background.ID: background}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: focus.ID, Meaning: "我现在承接它", Difference: 0.6, Ownership: 0.8, Value: 0.7, Urgency: 0.4, Answerability: 0.8, Certainty: 0.9, Resolution: "hold"},
			{CandidateID: background.ID, Meaning: "我已经了解，不让它支配未来", Difference: 0.05, Ownership: 0.2, Value: 0.2, Urgency: 0.1, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved"},
		},
		ThoughtThread:  "一个焦点被承接，另一项只是被理解。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Background[1].Status != "background" {
		t.Fatalf("a released non-focus event remained in the attention queue: %#v", runtime.state.Background[1])
	}
}

func TestOwnedPerceptualConcernTemporarilyBindsExplorationDrive(t *testing.T) {
	commitments := []ActionCommitment{}
	mentor := MentorState{}
	concern := Concern{
		ID: "concrete-interest", OriginKind: "perceptual_change", Resolution: "hold",
		Ownership: 0.66, Value: 0.62, Answerability: 0.58, LastSourceID: "event-new-object",
	}
	if !concernOwnsExplorationDrive(concern, commitments, mentor, 0.45) {
		t.Fatal("a concrete self-endorsed object could not recruit exploration energy")
	}
	concern.LastSourceID = concern.ID
	if concernOwnsExplorationDrive(concern, commitments, mentor, 0.45) {
		t.Fatal("deliberate non-action on the returned concern trapped exploration in a loop")
	}
	concern.LastSourceID = "event-new-object"
	commitments = append(commitments, ActionCommitment{ID: "acted", ConcernID: concern.ID, Status: "assimilated"})
	if !concernOwnsExplorationDrive(concern, commitments, mentor, 0.45) {
		t.Fatal("a concrete concern still held after Reality lost its exploration energy")
	}
	concern.Resolution = "relieved"
	if concernOwnsExplorationDrive(concern, commitments, mentor, 0.45) {
		t.Fatal("a relieved perceptual concern monopolized free exploration")
	}
	concern.OriginKind = "perceptual_impasse"
	concern.Resolution = "hold"
	concern.LastSourceID = "impasse-event"
	commitments = nil
	if concernOwnsExplorationDrive(concern, commitments, mentor, 0.45) {
		t.Fatal("a method boundary recruited exploration as though it were a concrete object")
	}
}

func TestLowOwnershipReleasesAnExistingConcernButKeepsItsExperiencePossible(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	concern := Concern{
		ID: "observed-not-owned", OriginKind: "endogenous_change", Meaning: "我曾想看看这个对象",
		Strength: 0.4, Ownership: 0.8, Resolution: "hold",
	}
	runtime.state.Concerns = []Concern{concern}
	candidate := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "我已经理解它，但不愿让它继续成为我的关切", Difference: 0.5,
			Ownership: 0.2, Value: 0.05, Urgency: 0.05, Answerability: 0.2, Certainty: 0.98, Resolution: "released",
		}},
		ThoughtThread:  "注意和理解已经完成；我选择不再承接。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a low-ownership observation remained an active concern: %#v", runtime.state.Concerns)
	}
}

func TestStageEightRealityReappraisesOriginalConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-original", ConcernID: "concern-original", InitialDifference: 0.8, ActionKind: "body_shell", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{ID: "concern-original", OriginKind: "environment", Meaning: "等待现实", Strength: 0.6, Resolution: "hold", CommitmentID: commitment.ID}}
	payload, _ := json.Marshal(ActionState{ID: "action-original", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	reality := Event{ID: "reality-original", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "现实已经回答了这个关切", Difference: 0, Ownership: 0.8,
			Value: 0.5, Urgency: 0, Answerability: 1, Certainty: 1, Resolution: "resolved",
		}},
		FocusID: reality.ID, ThoughtThread: "结果已经足够，我愿意放下。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, Meaning: "现实完整回答了行动前的差异。", Values: EndogenousValues{}, Significance: "ordinary",
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("the answered original concern stayed active or reality formed a duplicate: %#v", runtime.state.Concerns)
	}
}

func TestStageEightAccumulatedSelfModelTensionReturnsEvidenceToAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Self.Narrative = "我依据现实校准自己。"
	first := Experience{ID: "experience-one", PredictionDifference: 1, Values: EndogenousValues{SelfEndorsed: 1}, Meaning: "第一项强自我相关现实"}
	runtime.state.Experiences = append(runtime.state.Experiences, first)
	if err := runtime.accrueSelfModelTension(first, false); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatal("one sub-threshold contribution opened a self-model event")
	}
	second := Experience{ID: "experience-two", PredictionDifference: 1, Values: EndogenousValues{SelfEndorsed: 1}, Meaning: "第二项强自我相关现实"}
	runtime.state.Experiences = append(runtime.state.Experiences, second)
	if err := runtime.accrueSelfModelTension(second, false); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "self_model_difference" || runtime.state.Background[0].Status != "pending" {
		t.Fatalf("accumulated self-model tension did not become one attention candidate: %#v", runtime.state.Background)
	}
	if runtime.candidateScore(runtime.state.Background[0]) < runtime.config.Dynamics.AttentionThreshold {
		t.Fatal("the opened self-model difference could not compete for attention")
	}
}

func TestStageEightSelfModelDifferenceCanRewriteNarrativeAndReleaseTension(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Self.Narrative = "旧的当前自我理解"
	runtime.state.SelfModelTension = 0.7
	payload, _ := json.Marshal(map[string]any{"evidence_experience_ids": []string{"experience-a", "experience-b"}})
	candidate := Event{ID: "self-difference", Kind: "self_model_difference", Summary: "真实经历与当前自我理解之间的差异", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{candidate}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "这些经历已经改变我理解自己的方式", Difference: 0.7, Ownership: 1,
			Value: 0.8, Urgency: 0.4, Answerability: 1, Certainty: 0.8, Resolution: "resolved",
		}},
		FocusID: candidate.ID, ThoughtThread: "我愿意让当前叙事吸收这些经历。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice:  CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		NarrativeUpdate: "新的、由现实经历形成的当前自我理解",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Self.Narrative != commit.NarrativeUpdate || runtime.state.SelfModelTension != 0 {
		t.Fatalf("narrative or self-model tension did not change atomically: %#v", runtime.state.Self)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a resolved self-model difference became a durable concern: %#v", runtime.state.Concerns)
	}
}

func TestHeldExplorationConcernKeepsDeliberateNonActionAvailable(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Resolution: "hold", Answerability: 0.8}}
	event := Event{ID: "exploration", Kind: "concern", ConcernID: "exploration", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我想接触现实", Difference: 0.8, Ownership: 0.9,
			Value: 0.7, Urgency: 0.5, Answerability: 0.8, Certainty: 0.9, Resolution: "hold",
		}},
		FocusID: event.ID, ThoughtThread: "我保留探索。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a mature exploration concern lost alice's action choice: %v", err)
	}
	if runtime.state.ExplorationPressure < 0.8 {
		t.Fatalf("internal non-action falsely relieved exploration pressure: %f", runtime.state.ExplorationPressure)
	}
	commit.Appraisals[0].Ownership = 0.9
	commit.Action = CognitiveAction{
		Kind: "body_shell", Command: "date -Is", Intent: "用一次现实接触取得当前时间事实",
		Prediction: "身体会返回当前时间", RealityCheck: "依据命令退出状态和输出判断",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("the accumulated self-owned drive rejected alice's chosen reality probe: %v", err)
	}
}

func TestOrdinaryLowOwnershipFocusWithholdsEnactmentWithoutRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "ordinary", Kind: "body_delta", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我注意到一个普通变化", Difference: 0.1, Ownership: 0.2,
			Value: 0.1, Urgency: 0.1, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved",
		}},
		FocusID: event.ID, ThoughtThread: "我尚未愿意承接它。",
		Action: CognitiveAction{
			Kind: "body_shell", Command: "date -Is", Intent: "观察时间",
			Prediction: "身体会返回时间", RealityCheck: "依据输出判断",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	normalized, withheld := normalizeUnendorsedAction(commit, runtime.config.Dynamics.AttentionThreshold)
	if withheld != "body_shell" || normalized.Action.Kind != "none" {
		t.Fatalf("a low-ownership impulse remained executable: commit=%#v withheld=%q", normalized, withheld)
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a coherent non-enactment became a paid validation retry: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a low-ownership ordinary focus remained as a concern: %#v", runtime.state.Concerns)
	}
}

func TestWithheldActionDoesNotScheduleAContinuationWithoutReality(t *testing.T) {
	commit := CognitiveCommit{
		FocusID: "advertisement",
		Appraisals: []CandidateAppraisal{{
			CandidateID: "advertisement", Ownership: 0.2,
		}},
		Action: CognitiveAction{Kind: "body_shell", Command: "inspect-ad"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "next", Model: "sol", ReasoningEffort: "medium", Purpose: "absorb the action result",
		},
	}
	normalized, withheld := normalizeUnendorsedAction(commit, 0.45)
	if withheld != "body_shell" || normalized.Action.Kind != "none" || normalized.ResourceChoice.Apply != "keep" {
		t.Fatalf("withheld action retained an imaginary next step: %#v withheld=%q", normalized, withheld)
	}
}

func TestReframedExplorationConcernMakesRoomForANewDrive(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Concerns = []Concern{{
		ID: "exploration", OriginKind: "endogenous_change", Meaning: "我仍想接触现实",
		Strength: 0, Resolution: "reframed", Answerability: 0.5,
		LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}}
	runtime.state.Background = []Event{{ID: "origin", Kind: "endogenous_change", Status: "processed", ConcernID: "exploration"}}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 0 || runtime.currentExplorationConcernID() != "" {
		t.Fatalf("a restructured concern still monopolized the exploration drive: %#v", runtime.state.Concerns)
	}
	if err := runtime.advanceDynamics(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 2 || runtime.state.Background[1].Kind != "endogenous_change" {
		t.Fatalf("released exploration did not make room for a new drive event: %#v", runtime.state.Background)
	}
}

func TestReframedConcernKeepsItsCausalIdentityHeld(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	concern := Concern{
		ID: "external-window", OriginKind: "endogenous_change", Meaning: "确认浏览器能力入口",
		Strength: 0.02, Resolution: "hold", Answerability: 0.8,
	}
	runtime.state.Concerns = []Concern{concern}
	candidate := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "能力入口已确认，关切转向取得窗口内容", Difference: 0.34,
			Ownership: 0.76, Value: 0.71, Urgency: 0.32, Answerability: 0.82, Certainty: 0.98, Resolution: "reframed",
		}},
		ThoughtThread:   "同一关切已经改变意义，并继续等待下一步现实。",
		Action:          CognitiveAction{Kind: "none"},
		ResourceChoice:  CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		NarrativeUpdate: "",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != concern.ID || runtime.state.Concerns[0].Resolution != "hold" || runtime.state.Concerns[0].Strength == 0 {
		t.Fatalf("a reframed concern lost its continuing identity: %#v", runtime.state.Concerns)
	}
}

func TestRealityAssimilationClosesConcernReferenceAndCannotRepeat(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{
		ID: "commitment-once", ConcernID: "exploration", ActionKind: "body_shell", Status: "reality_available",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", CommitmentID: commitment.ID}}
	payload, _ := json.Marshal(ActionState{ID: "action-once", CommitmentID: commitment.ID, Kind: "body_shell", Status: "completed"})
	reality := Event{ID: "reality-once", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "现实已经到达", Difference: 0.1, Resolution: "resolved",
		}},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, Meaning: "我吸收了这次现实。", Significance: "ordinary",
		}},
	}
	if err := runtime.applyExperienceUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.TotalExperiences != 1 || runtime.state.Commitments[0].Status != "assimilated" || runtime.state.Commitments[0].ExperienceID == "" {
		t.Fatalf("reality did not close exactly once: %#v", runtime.state.Commitments[0])
	}
	if runtime.state.Concerns[0].CommitmentID != "" {
		t.Fatalf("concern retained an assimilated commitment: %#v", runtime.state.Concerns[0])
	}
	if err := runtime.validateExperienceUpdates(commit); err == nil {
		t.Fatal("the same commitment reality was accepted for a second experience")
	}
}

func TestUnassimilatedRealityOwnsAttentionDuringValidationRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 1
	runtime.state.Commitments = []ActionCommitment{{
		ID: "commitment-open", ConcernID: "exploration", ActionKind: "mentor_send", Status: "reality_available",
	}}
	runtime.state.Concerns = []Concern{{
		ID: "exploration", OriginKind: "endogenous_change", CommitmentID: "commitment-open",
		Meaning: "我仍想接触现实", Strength: 0.8, LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}}
	payload, _ := json.Marshal(ActionState{ID: "action-open", CommitmentID: "commitment-open", Kind: "mentor_send", Status: "completed"})
	runtime.state.Background = []Event{
		{ID: "reality-retry", Kind: "action_result", Status: "retry_wait", Payload: payload, LastFocusedAt: nowUTC()},
		{ID: "mentor-new", Kind: "mentor_received", Status: "pending"},
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("another focus stole attention while reality awaited retry: %#v", request)
	}
	runtime.state.Background[0].Status = "pending"
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "reality-retry" || len(request.Candidates) != 1 {
		t.Fatalf("released reality did not regain exclusive priority: %#v", request)
	}
}

func TestSettledRealityPreventsUnchangedEquivalentAction(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	current := Event{ID: "exploration-now", Kind: "concern", ConcernID: "concern-exploration"}
	runtime.activeCandidates = map[string]Event{current.ID: current}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "commitment-old", ConcernID: current.ConcernID, ActionKind: "body_shell", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-old", CommitmentID: "commitment-old", Kind: "body_shell",
		Request: "hominal-browser call browser_snapshot '{}'", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "reality-old", Seq: 10, Kind: "action_result", Payload: result, Status: "processed"}}
	runtime.state.Experiences = []Experience{{CommitmentID: "commitment-old", RemainingDifference: 0.08}}
	action := CognitiveAction{Kind: "body_shell", Command: "hominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("unchanged equivalent action was accepted after settled reality")
	}
	wrapped := CognitiveAction{Kind: "body_shell", Command: "set -euo pipefail\nhominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(current.ID, wrapped); err == nil {
		t.Fatal("an inert shell policy wrapper disguised an unchanged embodied action")
	}

	duplicate, _ := json.Marshal(ActionState{
		ID: "action-duplicate", CommitmentID: "commitment-duplicate", Kind: "body_shell",
		Request: action.Command, Status: "completed",
	})
	runtime.state.Background = append(runtime.state.Background, Event{ID: "reality-duplicate", Seq: 11, Kind: "action_result", Payload: duplicate})
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("an equivalent action result incorrectly counted as changed reality")
	}

	other := Event{ID: "exploration-later", Kind: "concern", ConcernID: "concern-later"}
	runtime.activeCandidates = map[string]Event{other.ID: other}
	if err := runtime.validateActionProgress(other.ID, action); err == nil {
		t.Fatal("a new concern identity reset an already settled embodied request")
	}
	decorated := CognitiveAction{Kind: "body_shell", Command: "printf '%s\\n' '--- current browser page snapshot ---'\nhominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(other.ID, decorated); err == nil {
		t.Fatal("a static observation label disguised a settled request under a new concern")
	}

	runtime.state.Background = append(runtime.state.Background, Event{ID: "mentor-new", Seq: 12, Kind: "mentor_received"})
	runtime.activeCandidates = map[string]Event{current.ID: current}
	if err := runtime.validateActionProgress(current.ID, action); err != nil {
		t.Fatalf("new world fact did not reopen the action: %v", err)
	}

	runtime.activeCandidates = map[string]Event{other.ID: other}
	if err := runtime.validateActionProgress(other.ID, action); err != nil {
		t.Fatalf("a material world change was erased by a new concern identity: %v", err)
	}
	changed := CognitiveAction{Kind: "body_shell", Command: "date -u; hominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(other.ID, changed); err != nil {
		t.Fatalf("a genuinely changed request was blocked: %v", err)
	}
}

func TestDifferentEmbodiedRealityReopensObservationAcrossConcerns(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	oldConcern := "browser-state"
	current := Event{ID: "browser-later", Kind: "concern", ConcernID: "profile-state"}
	runtime.activeCandidates = map[string]Event{current.ID: current}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "snapshot-old", ConcernID: oldConcern, ActionKind: "body_shell", Status: "assimilated",
	}}
	oldResult, _ := json.Marshal(ActionState{
		ID: "snapshot-action", CommitmentID: "snapshot-old", Kind: "body_shell",
		Request: "hominal-browser call browser_snapshot '{}'", Status: "completed",
	})
	clickResult, _ := json.Marshal(ActionState{
		ID: "click-action", CommitmentID: "click-later", Kind: "body_shell",
		Request: `hominal-browser call browser_click '{"target":"e75"}'`, Status: "completed",
	})
	runtime.state.Background = []Event{
		{ID: "snapshot-reality", Seq: 10, Kind: "action_result", Payload: oldResult, Status: "processed"},
		{ID: "click-reality", Seq: 20, Kind: "action_result", Payload: clickResult, Status: "processed"},
	}
	runtime.state.Experiences = []Experience{{CommitmentID: "snapshot-old", RemainingDifference: 0.1}}
	action := CognitiveAction{Kind: "body_shell", Command: "hominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(current.ID, action); err != nil {
		t.Fatalf("a later different body action did not reopen observation under the current concern: %v", err)
	}
}

func TestEnactedRequestNormalizationPreservesSubstantiveShellSource(t *testing.T) {
	plain := "hominal-browser list"
	for _, wrapped := range []string{
		"set -o pipefail\nhominal-browser list",
		"set -euo pipefail\nhominal-browser list",
		"\nset -eu;\n hominal-browser list \n",
		"printf '%s\\n' '--- browser action inventory ---'\nhominal-browser list",
		"set -o pipefail; printf '%s\\n' '--- browser action inventory ---'; hominal-browser list",
		"echo 'browser action inventory'; hominal-browser list",
	} {
		if got := normalizeEnactedRequest("body_shell", wrapped); got != plain {
			t.Fatalf("normalized request = %q, want %q", got, plain)
		}
	}
	command := "set -- alice\nprintf '%s\\n' \"$1\""
	if got := normalizeEnactedRequest("body_shell", command); got != command {
		t.Fatalf("a substantive set command was removed: %q", got)
	}
	dynamic := "printf 'observed_at=%s\\n' \"$(date -Is)\"\nhominal-browser list"
	if got := normalizeEnactedRequest("body_shell", dynamic); got != dynamic {
		t.Fatalf("a dynamic observation was removed: %q", got)
	}
	write := "printf '%s\\n' alice > /life/name\nhominal-browser list"
	if got := normalizeEnactedRequest("body_shell", write); got != write {
		t.Fatalf("a reality-changing output command was removed: %q", got)
	}
}

func TestUnsettledRealityAndDifferentActionRemainAvailable(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	current := Event{ID: "exploration-now", Kind: "concern", ConcernID: "concern-exploration"}
	runtime.activeCandidates = map[string]Event{current.ID: current}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "commitment-old", ConcernID: current.ConcernID, ActionKind: "body_shell", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-old", CommitmentID: "commitment-old", Kind: "body_shell", Request: "read-object", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "reality-old", Seq: 10, Kind: "action_result", Payload: result}}
	runtime.state.Experiences = []Experience{{CommitmentID: "commitment-old", RemainingDifference: 0.55}}
	if err := runtime.validateActionProgress(current.ID, CognitiveAction{Kind: "body_shell", Command: "read-object"}); err != nil {
		t.Fatalf("unsettled reality could not be revisited: %v", err)
	}
	runtime.state.Experiences[0].RemainingDifference = 0.05
	if err := runtime.validateActionProgress(current.ID, CognitiveAction{Kind: "body_shell", Command: "inspect-object-differently"}); err != nil {
		t.Fatalf("different action was rejected: %v", err)
	}
}

func TestExactFailedRequestCannotRepeatAcrossConcerns(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	current := Event{ID: "new-exploration", Kind: "concern", ConcernID: "concern-new"}
	runtime.activeCandidates = map[string]Event{current.ID: current}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "commitment-failed", ConcernID: "concern-old", ActionKind: "body_shell", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-failed", CommitmentID: "commitment-failed", Kind: "body_shell",
		Request: "hominal-browser call browser_tabs '{}'", Status: "completed",
		Result: `{"exit_code":1,"timed_out":false}`,
	})
	runtime.state.Background = []Event{{ID: "reality-failed", Seq: 10, Kind: "action_result", Payload: result, Status: "processed"}}
	runtime.state.Experiences = []Experience{{CommitmentID: "commitment-failed", RemainingDifference: 0.7}}
	action := CognitiveAction{Kind: "body_shell", Command: "hominal-browser call browser_tabs '{}'"}
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("an exact deterministic failure was accepted under a newly named concern")
	}
	changed := CognitiveAction{Kind: "body_shell", Command: `hominal-browser call browser_tabs '{"action":"list"}'`}
	if err := runtime.validateActionProgress(current.ID, changed); err != nil {
		t.Fatalf("a genuinely corrected request was rejected: %v", err)
	}
}

func TestWaitingConcernNoLongerCarriesExplorationActionRequirement(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	concern := Concern{
		ID: "waiting-relationship", OriginKind: "endogenous_change",
		Meaning: "消息已送达，我正在等待真实回应", Resolution: "hold", Answerability: 0.82,
	}
	runtime.state.Concerns = []Concern{concern}
	runtime.state.Commitments = []ActionCommitment{{ID: "waiting-action", ConcernID: concern.ID, Status: "assimilated"}}
	runtime.state.Mentor.Outbox = []MentorMessage{{
		MessageID: "alice-question", CommitmentID: "waiting-action", Status: "delivered", DeliveredAt: nowUTC(),
	}}
	candidate := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	if runtime.explorationHasMatureDrive(candidate.ID) {
		t.Fatal("a low-answerability waiting relationship inherited a compulsory exploration action")
	}
	if runtime.currentExplorationConcernID() != "" {
		t.Fatal("a transformed waiting concern still monopolized the exploration drive")
	}
	request := CognitiveRequest{Stage: 8, Focus: candidate, State: runtime.state, Config: runtime.config}
	if requestHasMatureExplorationDrive(request) {
		t.Fatal("the model schema forced action while trusted outbox facts still showed a pending reply")
	}
}

func TestUnansweredMentorThreadSurvivesImmediateReliefAsQuietBackground(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	concern := Concern{
		ID: "mentor-thread", OriginKind: "endogenous_change", Resolution: "relieved", Strength: 0,
	}
	runtime.state.Concerns = []Concern{concern}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "mentor-action", ConcernID: concern.ID, ActionKind: "mentor_send", Status: "reality_available",
	}}
	runtime.state.Mentor.Outbox = []MentorMessage{{
		MessageID: "alice-question", CommitmentID: "mentor-action", Status: "delivered", DeliveredAt: nowUTC(),
	}}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != concern.ID {
		t.Fatalf("an unanswered causal thread was pruned with its immediate send result: %#v", runtime.state.Concerns)
	}
	if runtime.currentExplorationConcernID() != "" {
		t.Fatal("a preserved mentor wait monopolized the general exploration drive")
	}
}

func TestOnlySubstantiveShellCommandsCountAsRealityContact(t *testing.T) {
	for _, command := range []string{
		"", ":", "true", "sleep 1", "set -eu; sleep 0.5", "echo 'waiting'; sleep 1",
	} {
		if shellActionContactsReality(command) {
			t.Fatalf("inert shell command counted as Reality contact: %q", command)
		}
	}
	for _, command := range []string{
		"date -Is", "sleep 1; curl -I https://example.com", "printf '%s\\n' \"$(date -Is)\"",
	} {
		if !shellActionContactsReality(command) {
			t.Fatalf("substantive shell command was rejected as inert: %q", command)
		}
	}
}

func TestEveryBodyShellActionMustContactReality(t *testing.T) {
	for _, command := range []string{
		"true", ":", "sleep 1", "set -euo pipefail; echo 'waiting'; sleep 1",
	} {
		action := CognitiveAction{
			Kind: "body_shell", Command: command, Intent: "等待",
			Prediction: "保持现状", RealityCheck: "命令结束",
		}
		if err := validateCognitiveAction(action, 8); err == nil {
			t.Fatalf("inert body_shell action passed the global Reality boundary: %q", command)
		}
	}
	for _, command := range []string{
		"cat /etc/os-release",
		"find /tmp -maxdepth 1 -type f",
		"curl -I https://example.com",
		"printf '%s\\n' 'body fact'; cat /etc/hostname",
		"printf '%s\\n' 'created by alice' > /tmp/alice-reality-test",
	} {
		action := CognitiveAction{
			Kind: "body_shell", Command: command, Intent: "取得或改变现实事实",
			Prediction: "返回可核验结果", RealityCheck: "检查输出或改变",
		}
		if err := validateCognitiveAction(action, 8); err != nil {
			t.Fatalf("substantive body_shell action was rejected: %q: %v", command, err)
		}
	}
	if err := validateCognitiveAction(CognitiveAction{Kind: "none"}, 8); err != nil {
		t.Fatalf("deliberate non-action was rejected: %v", err)
	}
}

func TestRealityContactRelievesExplorationWhileNewConcernCanRemain(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	commitment := ActionCommitment{
		ID: "commitment-contact", FocusID: "exploration-origin", ConcernID: "concern-contact",
		ActionKind: "mentor_send", Status: "reality_available",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{
		ID: commitment.ConcernID, OriginKind: "endogenous_change", Resolution: "hold",
		Answerability: 0.9, Strength: 0.5,
	}}
	payload, _ := json.Marshal(ActionState{
		ID: "action-contact", CommitmentID: commitment.ID, Kind: "mentor_send", Status: "completed",
	})
	reality := Event{ID: "reality-contact", Kind: "action_result", Payload: payload, Status: "in_focus", ConcernID: commitment.ConcernID}
	runtime.state.Background = []Event{{ID: commitment.FocusID, Kind: "endogenous_change", Status: "processed", ConcernID: commitment.ConcernID}, reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "消息已经送达；下一步是等待关系回应", Difference: 0.16,
			Ownership: 0.82, Value: 0.78, Urgency: 0.3, Answerability: 0.12, Certainty: 0.98, Resolution: "hold",
		}},
		ThoughtThread:  "我完成了联结尝试，也愿意等待。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ExperienceUpdates: []ExperienceUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.16,
			Meaning:         "消息送达形成了真实联结事实，回应仍需要等待。",
			Values:          EndogenousValues{Relatedness: 0.8, SelfEndorsed: 0.8},
			ExperiencedCost: 0.01, Lesson: "送达与回应是两个不同现实。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	want := 0.8 - runtime.config.Dynamics.ExplorationRelief*((1-0.16)*0.98) +
		runtime.config.Dynamics.ExplorationUnknownGrowth*(1-0.98)
	if absFloat(runtime.state.ExplorationPressure-want) > 0.000001 {
		t.Fatalf("reality contact did not relieve exploration independently of the new concern: got %f want %f", runtime.state.ExplorationPressure, want)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("relieving exploration erased the newly transformed concern: %#v", runtime.state.Concerns)
	}
	if runtime.currentExplorationConcernID() != "" {
		t.Fatal("the relationship wait still carried the general exploration drive")
	}
}

func TestGenericExplorationMentorContactIsInitialOnly(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "mentor-first", ActionKind: "mentor_send", Status: "assimilated",
	}}
	firstResult, _ := json.Marshal(ActionState{
		ID: "mentor-action-first", CommitmentID: "mentor-first", Kind: "mentor_send", Request: "我想分享一件事", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "mentor-send-result", Seq: 10, Kind: "action_result", Payload: firstResult, Status: "processed"}}
	if genericExplorationMentorContactAvailable(runtime.state.Commitments) {
		t.Fatal("a send receipt alone reopened the mentor channel as fresh exploration")
	}

	secondResult, _ := json.Marshal(ActionState{
		ID: "mentor-action-second", CommitmentID: "mentor-second", Kind: "mentor_send", Request: "我换一种说法", Status: "completed",
	})
	runtime.state.Background = append(runtime.state.Background, Event{ID: "mentor-send-result-second", Seq: 11, Kind: "action_result", Payload: secondResult, Status: "processed"})
	if genericExplorationMentorContactAvailable(runtime.state.Commitments) {
		t.Fatal("another outbound send result manufactured an intervening external reality")
	}

	runtime.state.Background = append(runtime.state.Background, Event{ID: "mentor-reply", Seq: 12, Kind: "mentor_received", Status: "processed"})
	if genericExplorationMentorContactAvailable(runtime.state.Commitments) {
		t.Fatal("a mentor reply turned generic exploration into another status send")
	}

	runtime.state.Background = runtime.state.Background[:2]
	bodyResult, _ := json.Marshal(ActionState{
		ID: "body-action", CommitmentID: "body-contact", Kind: "body_shell", Request: "read-world", Status: "completed",
	})
	runtime.state.Background = append(runtime.state.Background, Event{ID: "body-result", Seq: 13, Kind: "action_result", Payload: bodyResult, Status: "processed"})
	if genericExplorationMentorContactAvailable(runtime.state.Commitments) {
		t.Fatal("a body/world result turned generic exploration into another unsolicited mentor send")
	}
}

func TestSituatedConcernMayShareAfterEarlierMentorContact(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.9
	concern := Concern{ID: "concrete-experience", OriginKind: "action_result", Resolution: "hold", Answerability: 0.8}
	runtime.state.Concerns = []Concern{concern}
	event := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "mentor-first", ConcernID: "earlier-exploration", ActionKind: "mentor_send", Status: "assimilated",
	}}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "这段具体经历让我想与导师分享", Difference: 0.4, Ownership: 0.9,
			Value: 0.7, Urgency: 0.3, Answerability: 0.8, Certainty: 0.9, Resolution: "hold",
		}},
		ThoughtThread: "我从这段具体经历发起一次关系表达。",
		Action: CognitiveAction{
			Kind: "mentor_send", Text: "我想分享刚刚发生的一件具体事情。", Intent: "分享具体经历",
			Prediction: "消息进入导师通道", RealityCheck: "检查发送结果", StopCondition: "发送一次后停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a situated concern could not share after earlier mentor contact: %v", err)
	}
}

func TestRequiredExplorationRejectsRepeatedMentorSendWithoutNewReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.9
	concern := Concern{ID: "exploration-now", OriginKind: "endogenous_change", Resolution: "hold", Answerability: 0.9}
	runtime.state.Concerns = []Concern{concern}
	event := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	runtime.state.Commitments = []ActionCommitment{{
		ID: "mentor-first", ConcernID: "earlier-exploration", ActionKind: "mentor_send", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "mentor-action-first", CommitmentID: "mentor-first", Kind: "mentor_send", Request: "我会等待", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "mentor-result", Seq: 10, Kind: "action_result", Payload: result, Status: "processed"}}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我仍愿意接触现实", Difference: 0.6, Ownership: 0.9,
			Value: 0.6, Urgency: 0.3, Answerability: 0.9, Certainty: 0.9, Resolution: "hold",
		}},
		ThoughtThread: "我再次发送同类状态。",
		Action: CognitiveAction{
			Kind: "mentor_send", Text: "我暂不追加消息", Intent: "表达当前状态",
			Prediction: "消息进入队列", RealityCheck: "检查发送结果", StopCondition: "发送一次后停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil {
		t.Fatal("required exploration accepted a second mentor send without intervening reality")
	}

	runtime.state.ExplorationPressure = 0.2
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("ordinary relationship expression was incorrectly blocked outside required exploration: %v", err)
	}
}

func TestExplorationRequirementRejectsShellNoOp(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	runtime.state.Concerns = []Concern{{ID: "exploration-now", OriginKind: "endogenous_change", Resolution: "hold", Answerability: 0.9}}
	event := Event{ID: "exploration-now", Kind: "concern", ConcernID: "exploration-now", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我愿意接触现实", Difference: 0.8, Ownership: 0.9,
			Value: 0.6, Urgency: 0.5, Answerability: 0.9, Certainty: 0.9, Resolution: "hold",
		}},
		ThoughtThread: "我选择一个真实接触。",
		Action: CognitiveAction{
			Kind: "body_shell", Command: ":", Intent: "等待", Prediction: "没有变化", RealityCheck: "没有输出",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil {
		t.Fatal("a shell no-op satisfied the exploration reality requirement")
	}
}

func TestNewExplorationDriveDoesNotBecomeConcernWithoutAReferentAction(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ExplorationPressure = 0.8
	event := Event{ID: "exploration-now", Kind: "endogenous_change", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	if runtime.explorationHasMatureDrive(event.ID) {
		t.Fatal("a new drive was collapsed into a compulsory action before it became a concrete concern")
	}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我正在形成下一处值得接触的对象", Difference: 0.1,
			Ownership: 0.52, Value: 0.58, Urgency: 0.28, Answerability: 0.2, Certainty: 0.88, Resolution: "hold",
		}},
		ThoughtThread:   "这个差异值得继续属于我，我先让它成为具体关切。",
		Action:          CognitiveAction{Kind: "none"},
		ResourceChoice:  CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		NarrativeUpdate: "",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("an unenacted drive persisted as an empty concern: %#v", runtime.state.Concerns)
	}
}

func TestUnenactedLowAnswerabilityConcernKeepsOneExplorationIdentity(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	runtime.state.ExplorationPressure = 0.4
	concern := Concern{
		ID: "forming-object", OriginKind: "endogenous_change", Meaning: "我在形成下一处具体关切",
		Strength: 0.1, Resolution: "hold", Answerability: 0.2,
		LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}
	runtime.state.Concerns = []Concern{concern}
	if got := runtime.currentExplorationConcernID(); got != concern.ID {
		t.Fatalf("an unenacted concern did not retain the exploration identity: %q", got)
	}
	if err := runtime.advanceDynamics(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("the same drive was copied into another event: %#v", runtime.state.Background)
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("a low-pressure concern paid for another empty reflection: %#v", request)
	}
	candidate := Event{ID: concern.ID, Kind: "concern", ConcernID: concern.ID, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	if runtime.explorationHasMatureDrive(candidate.ID) {
		t.Fatal("a still-forming low-answerability concern was forced to act")
	}
	runtime.state.ExplorationPressure = 0.8
	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != concern.ID {
		t.Fatalf("the accumulated drive did not return for reality contact: %#v", request)
	}
	if !runtime.explorationHasMatureDrive(candidate.ID) {
		t.Fatal("a fully accumulated drive did not become salient for reality contact")
	}
	runtime.state.Commitments = []ActionCommitment{{ID: "already-enacted", ConcernID: concern.ID, Status: "assimilated"}}
	if got := runtime.currentExplorationConcernID(); got != "" {
		t.Fatalf("an enacted low-answerability wait still owned general exploration: %q", got)
	}
}

func TestStageFiveCommitmentRejectsPlaceholderSemantics(t *testing.T) {
	action := CognitiveAction{
		Kind: "mentor_send", Text: "你好", Intent: "none", Prediction: "导师通道会返回发送事实", RealityCheck: "以通道结果为准",
	}
	if err := validateCognitiveAction(action, 5); err == nil {
		t.Fatal("placeholder intent was accepted as an action commitment")
	}
}

func TestCognitiveUsageLedgerSurvivesGenerationChange(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "body", "cognitive-usage.jsonl")
	t.Setenv("HOMINAL_RESOURCE_LEDGER", ledger)
	first, err := New(t.TempDir(), "generation-one", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	record := UsageRecord{Time: nowUTC(), LeaseID: "lease-across-generations", RequestedModel: "terra", ActualMicrousd: 125_000, Status: "completed", CostConfirmed: true}
	if err := first.store.AppendUsage(record); err != nil {
		t.Fatal(err)
	}
	second, err := New(t.TempDir(), "generation-two", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.state.Usage) != 1 || second.state.Usage[0].LeaseID != record.LeaseID {
		t.Fatalf("new generation did not inherit body resource use: %#v", second.state.Usage)
	}
	updateResourceSnapshot(&second.state.Body, second.state, second.config.CognitiveResource, time.Now().UTC())
	if second.state.Body.CognitiveDaySpentMicrousd != record.ActualMicrousd {
		t.Fatalf("shared resource use was not reflected in the new body: %#v", second.state.Body)
	}
}

func TestSemanticCommitFailureDoesNotPretendGatewayIsUnavailable(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Lease = &Lease{ID: "lease-semantic", FocusID: "event-1", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}}
	runtime.state.Usage = []UsageRecord{
		{Time: nowUTC(), LeaseID: "old-1", RequestedModel: "terra", ActualMicrousd: 1, Status: "unusable", CostConfirmed: true},
		{Time: nowUTC(), LeaseID: "old-2", RequestedModel: "terra", ActualMicrousd: 1, Status: "unusable", CostConfirmed: true},
		{Time: nowUTC(), LeaseID: "lease-semantic", RequestedModel: "terra", ActualMicrousd: 1, Status: "completed", CostConfirmed: true},
	}
	runtime.state.Background = []Event{{ID: "event-1", Kind: "endogenous_change", Status: "in_focus"}}
	runtime.activeCandidates = map[string]Event{"event-1": runtime.state.Background[0]}
	if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "lease-semantic", FocusID: "event-1", Error: errors.New("candidate was appraised twice")}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.CognitiveResource.ProtectedModels) != 0 {
		t.Fatalf("semantic validation error was mislabeled as model unavailability: %#v", runtime.state.CognitiveResource.ProtectedModels)
	}
}
