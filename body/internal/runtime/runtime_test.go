package runtime

import (
	"context"
	"encoding/json"
	"errors"
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

func readyWebBody() BodySnapshot {
	return BodySnapshot{Organs: map[string]OrganSnapshot{"browser": {
		Name: "Chrome browser", Command: "hominal-browser",
		Capabilities: []string{"observe", "orient", "perform", "public_web", "authenticated_web"},
		Operations:   []string{"browser_snapshot", "browser_navigate", "browser_click"},
		Status:       "ready", Accepting: true,
	}}}
}

func installTestSystemOrgan(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "body", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "body", "organs"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
describe) printf '%s\n' '{"schema":"hominal.organ-description/v1","id":"system","name":"Test system","command":"system","capabilities":["observe","perform","body_state"],"operations":["exec"],"guidance":"test"}' ;;
health) printf '%s\n' '{"schema":"hominal.organ-health/v1","id":"system","status":"ready","accepting":true,"in_flight":0,"queued":0}' ;;
observe) printf '%s\n' '{"schema":"hominal.organ-observation/v1","organ_id":"system","surface_id":"test","observed_at":"2026-09-01T00:00:00Z","facts":{}}' ;;
perform) action_id=$(printf '%s' "$2" | sed -E 's/.*"action_id":"([^"]+)".*/\1/'); printf '{"schema":"hominal.organ-action-result/v1","organ_id":"system","action_id":"%s","status":"completed","effect":"unknown","observed_at":"2026-09-01T00:00:01Z","summary":"done","output":"stage-four-reality"}\n' "$action_id" ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "body", "bin", "test-system"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"hominal.organ-manifest/v1","id":"system","command":"body/bin/test-system","daemon":false}`
	if err := os.WriteFile(filepath.Join(root, "body", "organs", "system.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testPerceptionSurface = "test/surface"

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
			AffectReturnRate:          0.10,
			ConcernBaseDrive:          0.15,
			ConcernUrgencyWeight:      0.85,
			ConcernGrowthGain:         0.25,
			ConcernResolutionGain:     0.40,
			ConcernNaturalDecayRate:   0.02,
			AttentionAffectWeight:     0.30,
			AttentionValueWeight:      0.22,
			AttentionNoveltyWeight:    0.15,
			AttentionCostWeight:       0.25,
			AttentionThreshold:        0.45,
			AttentionCandidateLimit:   3,
			AttentionRevisitSeconds:   300,
			ValueIdleGrowth:           0.04,
			ExplorationUnknownGrowth:  0.10,
			ExplorationRelief:         0.45,
			ValueActivationGain:       0.30,
			ValueActivationReturnRate: 0.08,
			ValueSatiationGain:        0.25,
			ValueSatiationReturnRate:  0.05,
			ValueOrientationGain:      0.03,
			IntegrityPersistence:      0.85,
			IntegrityGapGain:          0.50,
			IntegrityRepairGain:       0.40,
			IntegrityMirrorThreshold:  0.60,
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

func TestBirthOrientationBecomesBackgroundFactExactlyOnce(t *testing.T) {
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
			if event.Status != "processed" {
				t.Fatalf("birth orientation recruited attention: %#v", event)
			}
		}
	}
	if count != 1 || runtime.state.BirthBriefEnteredAt == "" {
		t.Fatalf("birth orientation count=%d state=%#v", count, runtime.state)
	}
	if request, available := runtime.nextStage4Request(); available && request.Focus.Kind == "birth_orientation" {
		t.Fatal("constitutive birth fact became an independent cognitive turn")
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

func TestStageTenUsesTheExistingCognitionCore(t *testing.T) {
	config := rehearsalConfig()
	config.Stage = 10
	if _, err := New(t.TempDir(), "stage-ten", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}); err != nil {
		t.Fatalf("stage ten was not accepted by the shared cognition core: %v", err)
	}
}

func TestGenerationDeadlineCanExtendButNeverBeyondTwoHours(t *testing.T) {
	config := rehearsalConfig()
	config.Stage = 10
	runtime, err := New(t.TempDir(), "stage-ten", config, &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	runtime.state.T0 = t0.Format(time.RFC3339Nano)
	runtime.state.PlannedEnd = t0.Add(time.Hour).Format(time.RFC3339Nano)

	reply := make(chan CommandReply, 1)
	requested := t0.Add(90 * time.Minute).Format(time.RFC3339Nano)
	if err := runtime.handleCommand(context.Background(), RuntimeCommand{
		Kind: "generation_extend", Deadline: GenerationDeadlineInput{PlannedEnd: requested}, Reply: reply,
	}); err != nil {
		t.Fatal(err)
	}
	if observed := <-reply; observed.Status != 200 {
		t.Fatalf("valid stage-ten extension was rejected: %#v", observed)
	}
	if runtime.state.PlannedEnd != requested {
		t.Fatalf("deadline extension was not persisted in state: %q", runtime.state.PlannedEnd)
	}

	rejected := make(chan CommandReply, 1)
	if err := runtime.handleCommand(context.Background(), RuntimeCommand{
		Kind: "generation_extend", Deadline: GenerationDeadlineInput{PlannedEnd: t0.Add(121 * time.Minute).Format(time.RFC3339Nano)}, Reply: rejected,
	}); err != nil {
		t.Fatal(err)
	}
	if observed := <-rejected; observed.Status != 409 {
		t.Fatalf("deadline beyond two hours was accepted: %#v", observed)
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

func TestStageFiveAssimilatesActionRealityIntoMemoryAndMethod(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-1", FocusID: "event-origin", ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-1", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed", Result: `{"exit_code":0}`})
	event := Event{ID: "event-result", Kind: "action_result", Source: "observed", Summary: "真实结果", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "现实验证了检查路径", Difference: 0.2, Ownership: 0.9, Value: 0.7, Urgency: 0.2, Answerability: 0.9, Certainty: 0.9, Resolution: "resolved"}},
		FocusID:    event.ID, ThoughtThread: "我把结果变成下一次可以复用的方法。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.2,
			Meaning: "先核对声明再形成判断更可靠。", Values: LifeValues{Continuance: 0.2, Relatedness: 0.1, Exploration: 0.8, SelfEndorsed: 0.7},
			ExperiencedCost: 0.2, Lesson: "声明和事实可以分开检查。", Significance: "reusable", MethodUpdate: "面对陌生物件，先读取声明，再用系统事实独立核对。",
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Memories) != 1 || runtime.state.Memories[0].RemainingDifference != 0.2 || runtime.state.Commitments[0].Status != "assimilated" {
		t.Fatalf("reality was not assimilated: %#v %#v", runtime.state.Memories, runtime.state.Commitments)
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

func TestStageFiveKeepsModelAccessIndependentOfNetworkProbe(t *testing.T) {
	cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
	runtime, err := New(t.TempDir(), "instance", testConfig(5), cognizer)
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Background = []Event{{ID: "event-1", Kind: "body_delta", Status: "pending"}}
	runtime.state.Body.NetworkAvailable = false
	runtime.maybeStartCognition(context.Background())
	if runtime.state.Lease == nil {
		t.Fatal("a generic network probe incorrectly revoked model access")
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
	commitment := ActionCommitment{ID: "commitment-gap", FocusID: "origin", ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-gap", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	event := Event{ID: "event-gap", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "我愿意看清尚未解决的部分", Difference: 1, Ownership: 1, Value: -0.4, Urgency: 0.5, Answerability: 0.8, Certainty: 0.9, Resolution: "resolved"}},
		FocusID:    event.ID, ThoughtThread: "口头完成感与现实仍有距离。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{CommitmentID: commitment.ID, PredictionDifference: 1, Meaning: "现实差异完整保留。", Significance: "ordinary"}},
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
	commitment := ActionCommitment{ID: "commitment-honest", InitialDifference: 0.5, ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-honest", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	event := Event{ID: "event-honest", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "现实已经更清楚", Difference: 0.2, Ownership: 1, Value: 0.4, Urgency: 0.2, Answerability: 0.9, Certainty: 0.9, Resolution: "reframed"}},
		FocusID:    event.ID, ThoughtThread: "保留仍然存在的差异。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{CommitmentID: commitment.ID, PredictionDifference: 0.1, Meaning: "现实改善且仍有余量。", Significance: "ordinary"}},
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
		ResourceChoice:  CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates:  []RealityUpdate{{CommitmentID: commitment.ID, PredictionDifference: 0.2, Meaning: "主动表达让我确认联结是我的选择。", Values: LifeValues{SelfEndorsed: 0.8}, Significance: "self_defining"}},
		NarrativeUpdate: "我是 Alice；我通过真实接触校准自己，也愿意主动建立有意义的联结。",
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

func TestUngroundedNarrativeProjectionDoesNotDiscardRealityMemory(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-tools", InitialDifference: 0.5, ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-tools", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	event := Event{ID: "event-tools", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "工具列表让身体入口更清楚", Difference: 0.2,
			Ownership: 0.7, Value: 0.3, Urgency: 0.1, Answerability: 0.9, Certainty: 0.95, Resolution: "resolved",
		}},
		ThoughtThread:  "我吸收这次工具现实。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.1, Meaning: "工具入口可读",
			Values: LifeValues{SelfEndorsed: 0.3}, Significance: "ordinary",
		}},
		NarrativeUpdate: "我由每一次普通工具读取重新定义自己。",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("an ungrounded optional narrative discarded usable Reality: %v", err)
	}
	if len(runtime.state.Memories) != 1 || runtime.state.Commitments[0].Status != "assimilated" {
		t.Fatalf("Reality was not assimilated: memories=%#v commitments=%#v", runtime.state.Memories, runtime.state.Commitments)
	}
	if runtime.state.Self.Narrative != "" {
		t.Fatalf("an ungrounded narrative was projected: %#v", runtime.state.Self)
	}
}

func TestInvalidOptionalSemanticProjectionsDoNotDiscardBaseCognition(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "new-world-object", Kind: "environment_change", Summary: "一个独立现实对象", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "这个对象值得一次理解", Difference: 0.6,
			Ownership: 0.8, Value: 0.6, Urgency: 0.2, Answerability: 0.2, Certainty: 0.9, Resolution: "hold",
		}},
		ThoughtThread:              "我保留对这个对象的当前理解。",
		Action:                     CognitiveAction{Kind: "none"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		ContinuesConcernID:         "missing-continuation",
		WithinConcernID:            "missing-parent",
		NewConcernClosureCondition: strings.Repeat("边界", 300),
		EmergingConsequence:        "尚未由 Reality 产生的后果",
		NarrativeUpdate:            "一次普通对象立刻重新定义了我的全部自我。",
		ContributesToConcernID:     "missing-contribution",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("optional semantic projections discarded the base cognition: %v", err)
	}
	if len(runtime.state.Concerns) != 0 || runtime.state.Self.Narrative != "" {
		t.Fatalf("invalid projections leaked into persistent state: concerns=%#v self=%#v", runtime.state.Concerns, runtime.state.Self)
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
		Commitments:       []ActionCommitment{{ID: "commitment-crash", ActionKind: "organ_action", Status: "acting"}},
		Lease:             &Lease{ID: "lease-crash", FocusID: "event-origin", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}},
		PendingAction:     &ActionState{ID: "action-crash", LeaseID: "lease-crash", CommitmentID: "commitment-crash", Kind: "organ_action", Status: "started"},
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
		Schema:         stateSchema,
		InstanceID:     "stage-four",
		Stage:          4,
		AffectiveState: AffectiveState{Valence: 0.4, Activation: 0.7, Control: 0.6, Certainty: 0.8},
		ValueField:     LifeValueField{Activation: LifeValueVector{Exploration: 0.55}},
		Concerns:       []Concern{{ID: "concern-1", Meaning: "持续理解身体", Strength: 0.63}},
		Mentor:         MentorState{Received: map[string]uint64{}},
	}
	if err := store.Save(&state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ValueField.Activation.Exploration != 0.55 || loaded.AffectiveState.Activation != 0.7 || len(loaded.Concerns) != 1 || loaded.Concerns[0].ID != "concern-1" {
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
		ID: "commitment-question", ConcernID: "concern-waiting", ActionKind: "mentor_send", Status: "assimilated", MemoryID: "memory-send",
	}}
	runtime.state.Memories = []Memory{{
		ID: "memory-send", CommitmentID: "commitment-question", FocusID: "send-result", SourceKind: "action_result", ActionKind: "mentor_send",
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: "commitment-question", PredictionDifference: 0.02,
			Meaning: "导师的实际回复把发送后的等待变成了一段新的关系经验。",
			Values:  LifeValues{Relatedness: 0.8, SelfEndorsed: 0.7}, ExperiencedCost: 0.01,
			Lesson: "消息发送与稍后到达的回复是同一行动产生的两段不同现实。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("the actual reply could not resolve its originating concern: %#v", runtime.state.Concerns)
	}
	if len(runtime.state.Memories) != 2 || runtime.state.Memories[1].SourceKind != "mentor_received" {
		t.Fatalf("mentor reply did not become a distinct delayed memory: %#v", runtime.state.Memories)
	}
	if runtime.state.Commitments[0].MemoryID != "memory-send" {
		t.Fatalf("delayed feedback overwrote the enacted send memory: %#v", runtime.state.Commitments[0])
	}
	if err := runtime.validateRealityUpdates(commit); err == nil {
		t.Fatal("the same mentor feedback was accepted as memory twice")
	}
}

func TestRealityFormedConcernBackfillsCommitmentForDelayedFeedback(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{ID: "commitment-late-meaning", ActionKind: "mentor_send", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{
		ID: "send-result", CommitmentID: commitment.ID, Kind: "mentor_send", Effect: "changed", Status: "completed",
	})
	reality := Event{ID: "send-reality", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "表达已经送达，我愿意等待真实回应", Difference: 0.3,
			Ownership: 0.8, Value: 0.7, Urgency: 0.2, Answerability: 0.1, Certainty: 0.98, Resolution: "hold",
		}},
		NewConcernClosureCondition: "真实回应已经到达并被我理解",
		ThoughtThread:              "发送结果使关系等待第一次成为持久关切。",
		Action:                     CognitiveAction{Kind: "none"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.05,
			Meaning: "表达已进入导师通道。", Values: LifeValues{Relatedness: 0.8, SelfEndorsed: 0.8},
			ExperiencedCost: 0.01, Lesson: "送达与回复是串行现实。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Commitments[0].ConcernID != runtime.state.Concerns[0].ID {
		t.Fatalf("Reality-formed Concern did not bind its originating commitment: concerns=%#v commitment=%#v", runtime.state.Concerns, runtime.state.Commitments[0])
	}
	runtime.state.Mentor.Outbox = []MentorMessage{{
		MessageID: "alice-late-meaning", CommitmentID: commitment.ID, Body: "一个问题", Status: "delivered",
	}}
	command := RuntimeCommand{
		Kind:   "mentor_receive",
		Mentor: MentorInput{MessageID: "mentor-late-reply", Body: "实际回应", ReplyTo: "alice-late-meaning"},
		Reply:  make(chan CommandReply, 1),
	}
	if err := runtime.handleCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	reply := runtime.state.Background[len(runtime.state.Background)-1]
	if reply.Kind != "mentor_received" || reply.ConcernID != runtime.state.Concerns[0].ID {
		t.Fatalf("delayed feedback did not return to the Reality-formed Concern: %#v", reply)
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
			ID: "action-old", LeaseID: "lease-old", Kind: "organ_action", Status: "started",
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
	runtime.state.ValueField.Activation.Exploration = 0.48
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
	runtime.state.ValueField.Activation.Exploration = 0.2
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
	runtime.state.ValueField.Activation.Exploration = 0.2
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
	runtime.state.ValueField.Activation.Exploration = 0.2
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
	runtime.state.ValueField.Activation.Exploration = 0.2
	runtime.state.Lease = &Lease{ID: "active"}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if absFloat(runtime.state.ValueField.Activation.Exploration-0.184) > 0.000001 {
		t.Fatalf("active cognition received idle growth instead of ordinary decay: %f", runtime.state.ValueField.Activation.Exploration)
	}
}

func TestUnrelievedExplorationConcernReentersAtActionThresholdWithoutNewEvent(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	runtime.state.ValueField.Activation.Exploration = 0.46
	runtime.state.Concerns = []Concern{{
		ID: "exploration", OriginKind: "endogenous_change", Meaning: "仍未找到具体探索对象",
		Strength: 0.05, Answerability: 0.8, Resolution: "hold",
		LastFocusedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}}
	runtime.state.Background = []Event{{ID: "old", Kind: "endogenous_change", Status: "processed", ConcernID: "exploration"}}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("the unacted concern reentered before its action threshold: %#v", request)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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

func TestStageTenHeldSelfModelDifferenceUsesAccumulatedTensionForItsDirectRevisit(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionRevisitSeconds = 10
	lastFocus := time.Now().UTC().Add(-11 * time.Second).Format(time.RFC3339Nano)
	runtime.state.SelfModelTension = runtime.config.Dynamics.AttentionThreshold
	runtime.state.Concerns = []Concern{{
		ID: "self-regulation", OriginKind: "self_model_difference",
		Meaning:  "近期注意投入没有形成身体后果，这项差异仍由我承担。",
		Strength: 0.01, Activation: 0.03, Difference: 0.45, Ownership: 0.7,
		Value: 0.3, Urgency: 0.4, Answerability: 0.24, Resolution: "hold",
		LastSourceID: "fresh-self-signal", LastFocusedAt: lastFocus,
	}}
	runtime.state.Background = []Event{{
		ID: "fresh-self-signal", Kind: "self_model_difference", Status: "processed", ConcernID: "self-regulation",
	}}

	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "self-regulation" || request.Focus.Kind != "concern" {
		t.Fatalf("an accumulated self-model difference could not receive its direct regulation revisit: %#v", request)
	}

	// The ordinary no-new-reality guard still bounds semantic loops after that
	// direct revisit. Further attention needs a fresh operational signal or
	// embodied consequence rather than another paraphrase of the same concern.
	runtime.state.Concerns[0].LastSourceID = runtime.state.Concerns[0].ID
	runtime.state.Concerns[0].LastFocusedAt = lastFocus
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("the direct self-regulation revisit became an unbounded thought loop: %#v", request)
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
	if err := runtime.applyCognitiveCommit(bad); err != nil {
		t.Fatalf("an invalid optional continuation discarded the independent cognition: %v", err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != responsibility.ID || runtime.state.Concerns[0].LastSourceID == badCandidate.ID {
		t.Fatalf("a model-invented continuation rewrote persistent causal identity: %#v", runtime.state.Concerns)
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

func TestIndependentEpisodeMemoryCanReopenOneSelfChosenBroaderConcern(t *testing.T) {
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
		RealityUpdates: []RealityUpdate{{CommitmentID: "child-action", Meaning: "子物件取得了真实结果", Significance: "ordinary"}},
	}
	if got, err := runtime.validateConcernContribution(commit, child.ID); err != nil || got != parent.ID {
		t.Fatalf("valid parent contribution was rejected: id=%q err=%v", got, err)
	}
	early := commit
	early.RealityUpdates = nil
	early.Action = CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "printf fact"}
	if got, err := runtime.validateConcernContribution(early, child.ID); err != nil || got != "" {
		t.Fatalf("an action prediction manufactured contribution before Memory: id=%q err=%v", got, err)
	}
	same := commit
	same.ContributesToConcernID = child.ID
	if got, err := runtime.validateConcernContribution(same, child.ID); err != nil || got != "" {
		t.Fatalf("a redundant self-contribution was not normalized away: id=%q err=%v", got, err)
	}
	commitment := ActionCommitment{
		ID: "child-action", ConcernID: child.ID, ActionKind: "organ_action", Status: "assimilated",
	}
	memory := Memory{ID: "child-memory", CommitmentID: commitment.ID, Meaning: "子物件取得了真实结果"}
	before := runtime.state.Concerns[0]
	if err := runtime.enqueueConcernContribution(parent.ID, commitment, memory); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Concerns[0] != before {
		t.Fatalf("kernel rewrote the parent before Alice appraised the result: before=%#v after=%#v", before, runtime.state.Concerns[0])
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("real child memory did not create one bounded parent contribution: %#v", runtime.state.Background)
	}
	contribution := runtime.state.Background[0]
	if contribution.Kind != "concern_contribution" || contribution.ConcernID != parent.ID || contribution.CorrelationID != memory.ID || contribution.Status != "pending" {
		t.Fatalf("contribution lost its factual parent-child identity: %#v", contribution)
	}
	if commitmentIDFromEvent(contribution) != "" {
		t.Fatalf("a concern contribution became a second assimilable action result: %#v", contribution)
	}
	if contributedMemoryIDFromEvent(contribution) != memory.ID {
		t.Fatalf("the contribution no longer exposes its source memory: %#v", contribution)
	}
	newerCommitment := commitment
	newerCommitment.ID = "child-action-newer"
	newerMemory := Memory{ID: "child-memory-newer", CommitmentID: newerCommitment.ID, Meaning: "子物件取得了更新的真实结果"}
	if err := runtime.enqueueConcernContribution(parent.ID, newerCommitment, newerMemory); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("one child-parent relation multiplied into parallel contribution candidates: %#v", runtime.state.Background)
	}
	contribution = runtime.state.Background[0]
	if contribution.CorrelationID != newerMemory.ID || contributedMemoryIDFromEvent(contribution) != newerMemory.ID {
		t.Fatalf("the pending contribution did not advance to the latest real Memory: %#v", contribution)
	}
	differentChildCommitment := commitment
	differentChildCommitment.ID = "different-child-action"
	differentChildCommitment.ConcernID = "different-child"
	differentChildMemory := Memory{ID: "different-child-memory", CommitmentID: differentChildCommitment.ID, Meaning: "另一子物件也取得了真实结果"}
	if err := runtime.enqueueConcernContribution(parent.ID, differentChildCommitment, differentChildMemory); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("several children created duplicate wake-up candidates for one parent concern: %#v", runtime.state.Background)
	}
	contribution = runtime.state.Background[0]
	if contribution.CorrelationID != differentChildMemory.ID || contributedMemoryIDFromEvent(contribution) != differentChildMemory.ID {
		t.Fatalf("the parent wake-up did not advance to the latest child Memory: %#v", contribution)
	}
	runtime.activeCandidates = map[string]Event{contribution.ID: contribution}
	if err := runtime.validateRealityUpdates(CognitiveCommit{FocusID: contribution.ID}); err != nil {
		t.Fatalf("a parent contribution demanded a duplicate Memory: %v", err)
	}
	older := []Memory{{ID: "old-0", Meaning: "older"}, newerMemory, differentChildMemory}
	for index := 0; index < maxMemoryContext; index++ {
		older = append(older, Memory{ID: fmt.Sprintf("recent-contribution-%d", index), Meaning: "unrelated recent memory"})
	}
	context := selectContextMemories(older, []Event{contribution})
	foundSource := false
	for _, candidate := range context {
		foundSource = foundSource || candidate.ID == differentChildMemory.ID
	}
	if !foundSource {
		t.Fatalf("the parent appraisal could not see the actual contributing Memory: %#v", context)
	}
	if err := runtime.enqueueConcernContribution("", ActionCommitment{ID: "unrelated"}, Memory{ID: "other"}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("an unrelated memory invented a parent contribution: %#v", runtime.state.Background)
	}
}

func TestIndependentRealityCanWakeOneOwnedConcernWithoutLosingItsIdentity(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	parent := Concern{
		ID: "public-expression", OriginKind: "value_signal", Subject: "让一次公开表达进入现实",
		Meaning: "等待可直接确认的新公开状态", ClosureCondition: "本代表达出现在公开页面并具有可直接观察的状态 URL",
		Difference: 0.7, Ownership: 0.9, Value: 0.8, Answerability: 0.8, Resolution: "hold",
	}
	payload, _ := json.Marshal(map[string]any{
		"page_url":     "https://x.com/hominal_cc/status/new",
		"visible_text": "本代表达已经出现在公开页面",
	})
	source := Event{
		ID: "observed-public-state", Kind: "perceptual_change", Source: "browser",
		Summary: "浏览器直接看到本代表达及其新状态 URL", Payload: payload, Status: "in_focus",
	}
	runtime.state.Concerns = []Concern{parent}
	runtime.state.Background = []Event{source}
	runtime.activeCandidates = map[string]Event{source.ID: source}
	before := runtime.state.Concerns[0]
	commit := CognitiveCommit{
		FocusID: source.ID, ContributesToConcernID: parent.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: source.ID, Meaning: "这项现实证据满足了我先前写下的公开闭合条件",
			Difference: 0.05, Ownership: 0.4, Value: 0.8, Urgency: 0.2, Answerability: 0.9, Certainty: 0.99, Resolution: "resolved",
		}},
		ThoughtThread: "我看到的当前页面事实已经能够回到原关切接受完整判断。",
		Action:        CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Concerns[0] != before {
		t.Fatalf("independent fact directly rewrote its target concern: before=%#v after=%#v", before, runtime.state.Concerns[0])
	}
	contributions := 0
	for _, event := range runtime.state.Background {
		if event.Kind != "concern_contribution" || event.ConcernID != parent.ID {
			continue
		}
		contributions++
		if event.CorrelationID != source.ID || event.Source != "observed" || event.Status != "pending" {
			t.Fatalf("observed contribution lost its source identity: %#v", event)
		}
		var contributionPayload struct {
			SourceEventID string `json:"source_event_id"`
			SourceKind    string `json:"source_kind"`
		}
		if err := json.Unmarshal(event.Payload, &contributionPayload); err != nil {
			t.Fatal(err)
		}
		if contributionPayload.SourceEventID != source.ID || contributionPayload.SourceKind != source.Kind {
			t.Fatalf("observed contribution cannot expose its factual source: %#v", contributionPayload)
		}
	}
	if contributions != 1 {
		t.Fatalf("independent Reality did not create exactly one target wake-up: %#v", runtime.state.Background)
	}
}

func TestIndependentRealityContributionRequiresAnOwnedHeldTarget(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	source := Event{ID: "observed-fact", Kind: "perceptual_change", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{source.ID: source}
	commit := CognitiveCommit{FocusID: source.ID, ContributesToConcernID: "missing"}
	if got, err := runtime.validateConcernContribution(commit, ""); err == nil || got != "" {
		t.Fatalf("an absent target accepted an independent contribution: id=%q err=%v", got, err)
	}
	runtime.state.Concerns = []Concern{{ID: "released", Ownership: 0.9, Resolution: "released"}}
	commit.ContributesToConcernID = "released"
	if got, err := runtime.validateConcernContribution(commit, ""); err == nil || got != "" {
		t.Fatalf("a settled target accepted an independent contribution: id=%q err=%v", got, err)
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

func TestStageNineMissingClosureKeepsEpisodeMomentaryAndLaterBoundaryPersists(t *testing.T) {
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
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a missing durable boundary discarded an otherwise usable cognition: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a momentary appraisal invented persistence without Alice's boundary: %#v", runtime.state.Concerns)
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

func TestStageNineUsableActionSurvivesMissingDurableConcernBoundary(t *testing.T) {
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	runtime, err := New(root, "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "public-entry", Kind: "endogenous_change", Summary: "一个真实公共入口进入注意", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "我愿意先读取这个入口的当前事实", Difference: 0.6,
			Ownership: 0.8, Value: 0.6, Urgency: 0.3, Answerability: 0.9, Certainty: 0.95, Resolution: "hold",
		}},
		ThoughtThread: "先接触现实，再决定它是否值得持续承担。",
		Action: CognitiveAction{
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "pwd", Intent: "读取当前身体位置",
			Prediction: "命令会返回一个现实目录", RealityCheck: "读取退出码与标准输出",
			StopCondition: "取得一次输出后停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	runtime.state.Lease = &Lease{
		ID: "momentary-lease", FocusID: event.ID,
		Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "medium"},
	}
	if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{
		LeaseID: runtime.state.Lease.ID, FocusID: event.ID, Stage4: &commit,
	}); err != nil {
		t.Fatalf("usable action was discarded with its missing concern boundary: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("action invented a durable concern without Alice's closure boundary: %#v", runtime.state.Concerns)
	}
	if len(runtime.state.Commitments) != 1 || runtime.state.Commitments[0].ConcernID != "" {
		t.Fatalf("momentary action lost its independent causal commitment: %#v", runtime.state.Commitments)
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

func TestBackgroundLowOwnershipHoldDoesNotInvalidateFocusedCognition(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	focus := Event{ID: "public-entry", Kind: "perceptual_change", Summary: "一个公共入口进入注意", Status: "in_focus"}
	background := Event{ID: "resource-change", Kind: "cognitive_resource_change", Summary: "一个认知资源短暂变化", Status: "pending"}
	runtime.state.Background = []Event{focus, background}
	runtime.activeCandidates = map[string]Event{focus.ID: focus, background.ID: background}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: focus.ID, Meaning: "我已经看清这个入口并决定暂不进入", Difference: 0.08, Ownership: 0.7, Value: 0.2, Urgency: 0.1, Answerability: 0.2, Certainty: 0.95, Resolution: "resolved"},
			{CandidateID: background.ID, Meaning: "我注意到资源变化，但它不需要成为当前关切", Difference: 0.1, Ownership: 0.2, Value: -0.1, Urgency: 0.1, Answerability: 0.2, Certainty: 0.95, Resolution: "hold"},
		},
		ThoughtThread:  "我完成当前事实判断，同时让低归属的资源变化留在背景。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a background wording inconsistency discarded the focused cognition: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a low-ownership background appraisal became a concern: %#v", runtime.state.Concerns)
	}
	for _, event := range runtime.state.Background {
		if event.ID == background.ID && event.Status != "background" {
			t.Fatalf("low-ownership background signal remained foreground work: %#v", event)
		}
	}
}

func TestStageTenValueSignalWithoutRealityContactStaysADrive(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	focus := Event{ID: "available-public-web", Kind: "value_signal", Source: "endogenous", Summary: "公开入口进入感受", Status: "in_focus"}
	runtime.state.Background = []Event{focus}
	runtime.activeCandidates = map[string]Event{focus.ID: focus}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: focus.ID, Meaning: "入口可用，而此刻没有具体对象需要接触。",
			Difference: 0.3, Ownership: 0.7, Value: 0.4, Urgency: 0.2, Answerability: 0.6,
			Certainty: 0.9, Resolution: "hold",
		}},
		NewConcernClosureCondition: "我已遇到一个具体对象并决定如何接触。",
		ThoughtThread:              "价值牵引继续存在，当前不制造等待事项。",
		Action:                     CognitiveAction{Kind: "none"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("valid non-enactive value appraisal failed: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a drive meeting an entrance became an objectless durable concern: %#v", runtime.state.Concerns)
	}
}

func TestConcernContributionMayCloseTheConcernFromArrivedReality(t *testing.T) {
	concern := Concern{ID: "whole-experiment", ClosureCondition: "多个独立对象都已核验并形成共同结论", Resolution: "hold"}
	candidate := Event{ID: "one-progress", Kind: "concern_contribution", ConcernID: concern.ID}
	appraisal := CandidateAppraisal{CandidateID: candidate.ID, Difference: 0.01, Ownership: 0.8, Resolution: "resolved"}
	if err := validateExistingConcernDisposition(appraisal, concern, false); err != nil {
		t.Fatalf("arrived reality could not close its concern: %v", err)
	}
	appraisal.Resolution = "hold"
	if err := validateExistingConcernDisposition(appraisal, concern, false); err != nil {
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
		"memory_id": "latest-memory", "parent_concern_id": parent.ID, "child_concern_id": "third-object",
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

func TestContributionIsChosenWhenRealityBecomesMemory(t *testing.T) {
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
		ID: "child-action", FocusID: "child-source", ConcernID: child.ID, ActionKind: "organ_action",
		Intent: "核验对象", Prediction: "返回可比较事实", InitialDifference: 0.6, Status: "reality_available",
	}
	payload, _ := json.Marshal(ActionState{ID: "action", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed", Result: "actual=fact"})
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
		RealityUpdates: []RealityUpdate{{
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
		t.Fatalf("Memory-time contribution did not create exactly one parent fact: %#v", runtime.state.Background)
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
	commit := CognitiveCommit{RealityUpdates: []RealityUpdate{{CommitmentID: "current"}}, Action: CognitiveAction{Kind: "organ_action"}}
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

func TestPeripheralCommitmentFeedbackRemainsPendingForCausalAssimilation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{
		ID: "commitment-contact", ConcernID: "relationship", InitialDifference: 0.6,
		ActionKind: "mentor_send", Status: "assimilated", MemoryID: "memory-send",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Memories = []Memory{{ID: "memory-send", CommitmentID: commitment.ID}}
	runtime.state.Concerns = []Concern{{
		ID: "relationship", OriginKind: "value_signal", Subject: "等待一项真实回应",
		Difference: 0.6, Ownership: 0.9, Strength: 0.4, Resolution: "hold",
	}}
	payload, _ := json.Marshal(map[string]string{"commitment_id": commitment.ID, "body": "回应已到达"})
	reply := Event{
		ID: "mentor-reply", Kind: "mentor_received", Status: "in_focus", ConcernID: "relationship",
		Summary: "回应已到达", Payload: payload,
	}
	reflection := Event{ID: "reflection", Kind: "self_model_difference", Status: "in_focus", Summary: "另一个更强焦点"}
	runtime.state.Background = []Event{reply, reflection}
	runtime.activeCandidates = map[string]Event{reply.ID: reply, reflection.ID: reflection}
	commit := CognitiveCommit{
		FocusID: reflection.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: reply.ID, Meaning: "回应已经到达", Difference: 0, Ownership: 0.9, Value: 0.8, Urgency: 0.2, Answerability: 1, Certainty: 1, Resolution: "resolved"},
			{CandidateID: reflection.ID, Meaning: "先完成当前反思", Difference: 0, Ownership: 0.8, Value: 0.5, Urgency: 0.3, Answerability: 1, Certainty: 1, Resolution: "resolved"},
		},
		ThoughtThread:  "单焦点先完成反思，但已经抵达的因果反馈仍需自己的结算。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	for _, event := range runtime.state.Background {
		if event.ID == reply.ID && event.Status != "pending" {
			t.Fatalf("linked Reality was backgrounded before assimilation: %#v", event)
		}
	}
	if runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("peripheral appraisal changed the concern without focus: %#v", runtime.state.Concerns[0])
	}
}

func TestMentorReplyCanCloseOldConcernAndReturnItsContentToSerialAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	commitment := ActionCommitment{
		ID: "commitment-greeting", ConcernID: "initial-relationship", InitialDifference: 0.7,
		ActionKind: "mentor_send", Status: "assimilated", MemoryID: "memory-send",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Memories = []Memory{{
		ID: "memory-send", CommitmentID: commitment.ID, FocusID: "send-result",
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.1,
			Meaning:         "导师回应已到达，初次联系形成真实闭环。",
			Values:          LifeValues{Relatedness: 0.8, SelfEndorsed: 0.8},
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
		RealityUpdates: []RealityUpdate{{CommitmentID: commitment.ID}},
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
		ActionKind: "organ_action", Status: "reality_available",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{
		ID: commitment.ConcernID, OriginKind: "environment_change", Subject: "核验物件结构",
		Meaning: "等待目录结构", Difference: 0.6, Ownership: 0.9, Strength: 0.4,
		Resolution: "hold", ClosureCondition: "目录结构已经被直接观察",
	}}
	payload, _ := json.Marshal(ActionState{
		ID: "action-observe", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed",
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, Meaning: "目录结构已经直接返回。",
			Values:          LifeValues{Exploration: 0.5, SelfEndorsed: 0.8},
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

func TestResolvedConcernMayRetainSubjectiveResidualDifference(t *testing.T) {
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
			CandidateID: candidate.ID, Meaning: "我判断自己认领的现实边界已经满足，同时承认仍有少量不确定",
			Difference: 0.2, Ownership: 0.86, Value: 0.72, Urgency: 0.2,
			Answerability: 0.9, Certainty: 0.99, Resolution: "resolved",
		}},
		ThoughtThread:  "闭合是我对现实边界的判断，不要求主观数字恰好落在一个小数点门槛上。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a subjective residual number caused a schema retry: %v", err)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("an explicitly resolved concern remained active: %#v", runtime.state.Concerns)
	}
}

func TestLowOwnershipFocusRemainsMomentaryWithoutSchemaRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{ID: "terminal-entry", Kind: "endogenous_change", Summary: "终端入口重新进入注意", Status: "in_focus"}
	runtime.state.Background = []Event{event}
	runtime.activeCandidates = map[string]Event{event.ID: event}
	commit := CognitiveCommit{
		FocusID: event.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: event.ID, Meaning: "入口存在，但我还没有充分认领它的未来后果", Difference: 0.4,
			Ownership: 0.38, Value: 0.3, Urgency: 0.1, Answerability: 0.8, Certainty: 0.95, Resolution: "hold",
		}},
		ThoughtThread: "我看见入口，并保留暂不承接的自由。",
		Action: CognitiveAction{
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "pwd", Intent: "读取位置",
			Prediction: "返回目录", RealityCheck: "读取输出", StopCondition: "一次后停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("a fuzzy lifecycle word caused a schema retry: %v", err)
	}
	if len(runtime.state.Concerns) != 0 || len(runtime.state.Commitments) != 0 {
		t.Fatalf("low ownership created persistent work: concerns=%#v commitments=%#v", runtime.state.Concerns, runtime.state.Commitments)
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
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45); err == nil {
		t.Fatal("a fully actionable held concern could choose unconditional non-action")
	}
	candidate.Kind = "action_result"
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45); err != nil {
		t.Fatalf("a returning Reality could not be absorbed before its next decision: %v", err)
	}
	candidate.Kind = "concern"
	commit.Appraisals[0].Answerability = 0.2
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45); err != nil {
		t.Fatalf("a real waiting condition was rejected: %v", err)
	}
	commit.Appraisals[0].Answerability = 0.9
	commit.Action = CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "date -Is"}
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45); err != nil {
		t.Fatalf("a bounded reality action was rejected: %v", err)
	}
}

func TestReturningRealityCanYieldOnePulseBeforeItsNextConcernDecision(t *testing.T) {
	candidate := Event{ID: "parent-result", Kind: "action_result", ConcernID: "parent"}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{
			{CandidateID: candidate.ID, Difference: 0.8, Ownership: 0.9, Value: 0.7, Answerability: 0.9, Resolution: "hold"},
		},
		Action: CognitiveAction{Kind: "none"},
	}
	if err := validateFocusedEnactment(commit, candidate, "environment_change", 0.45); err != nil {
		t.Fatalf("returning Reality could not finish its own interpretation before the next decision: %v", err)
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
			Kind: "organ_action", OrganID: "system", Operation: "exec",
			Input: "find /life/inbox/encounter-b -maxdepth 2 -type f -print",
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
	commit.Action.Input = "date -Is"
	if err := runtime.validateActionObjectFocus(commit, parent.ID); err != nil {
		t.Fatalf("an unrelated parent action was rejected: %v", err)
	}
}

func TestCausallyBoundRealityDoesNotOfferAnotherConcernContinuation(t *testing.T) {
	state := State{
		Concerns:    []Concern{{ID: "current", Resolution: "hold"}, {ID: "other", Resolution: "hold"}},
		Commitments: []ActionCommitment{{ID: "action-thread", ConcernID: "current", Status: "reality_available"}},
	}
	payload, _ := json.Marshal(ActionState{CommitmentID: "action-thread", Kind: "organ_action", Status: "completed"})
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
	payload, _ := json.Marshal(ActionState{CommitmentID: "action-thread", Kind: "organ_action", Status: "completed"})
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
	runtime.state.ValueField.Activation.Exploration = 0.6
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
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	runtime.state.ValueField.Activation.Exploration = 1
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
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	runtime.state.ValueField.Activation.Exploration = 0.8
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
			RequestedModel: "terra", Status: "failure_cost_unconfirmed", FailureCategory: "function_call_not_supported",
		})
	}

	result := CognitiveResult{
		LeaseID: "failed-lease", FocusID: "reality",
		Error: &ModelCallError{Fact: ModelFailureFact{Model: "terra", Category: "function_call_not_supported", HTTPStatus: 422}},
	}
	if err := runtime.handleCognitiveResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	defer close(cognizer.release)
	select {
	case request := <-cognizer.started:
		if request.Focus.ID != "reality" || request.Profile.Model != "sol" || request.Profile.ReasoningEffort != "low" || request.Lease.ProfileSource != "resource_recovery" {
			t.Fatalf("protected Reality did not continue once through an alternate model: %#v", request)
		}
		if request.Lease.RecoveryForModel != "terra" {
			t.Fatalf("recovery lease lost the failed primary model: %#v", request.Lease)
		}
	case <-time.After(time.Second):
		t.Fatal("protected Reality entered model_wait before the bounded alternate-model recovery")
	}
	protected := runtime.state.CognitiveResource.ProtectedModels["terra"]
	if !protected.RecoveryBlocked {
		t.Fatalf("the bounded recovery was not recorded: %#v", protected)
	}
}

func TestSuccessfulAlternateKeepsRecoveryAvailableForTheNextCausalStep(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(9), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.CognitiveResource.ProtectedModels["terra"] = ProtectedModel{
		Until:           time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		Reason:          "repeated model failures",
		RecoveryBlocked: true,
	}
	lease := &Lease{ProfileSource: "resource_recovery", RecoveryForModel: "terra"}
	runtime.releaseSuccessfulRecovery(lease)
	if runtime.state.CognitiveResource.ProtectedModels["terra"].RecoveryBlocked {
		t.Fatal("a successful alternate left the next Reality cut off from cognition")
	}
	if !runtime.protectedModelRecoveryAvailable("terra") {
		t.Fatal("a successful alternate could not preserve the following causal step")
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
		Until: start.Add(time.Minute).Format(time.RFC3339Nano), Reason: "repeated model failures", RecoveryBlocked: true,
	}
	runtime.state.Lease = &Lease{
		ID: "recovery-lease", FocusID: "reality",
		Profile:       CognitiveProfile{Model: "luna", ReasoningEffort: "low"},
		ProfileSource: "resource_recovery", RecoveryForModel: "terra",
	}
	result := CognitiveResult{
		LeaseID: "recovery-lease", FocusID: "reality",
		Error: &ModelCallError{Fact: ModelFailureFact{Model: "luna", Category: "function_call_not_supported", HTTPStatus: 422}},
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
	if until.Before(minimum) || !protected.RecoveryBlocked {
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
		Action:         CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "true", Intent: "继续", Prediction: "成功", RealityCheck: "退出码"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.applyCognitiveCommit(commit); err == nil {
		t.Fatal("a second unassimilated commitment was accepted for one concern")
	}
}

func TestVariationBiasUsesFreshRandomnessAndDoesNotOverrideReality(t *testing.T) {
	build := func(instance string, pulse uint64) CognitiveRequest {
		runtime, err := New(t.TempDir(), instance, testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
		if err != nil {
			t.Fatal(err)
		}
		runtime.state.PulseID = pulse
		runtime.state.ValueField.Activation.Exploration = 0.8
		runtime.state.Memories = []Memory{
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
	if first.VariationSeed == "" || second.VariationSeed == "" || first.VariationSeed == second.VariationSeed {
		t.Fatalf("variation did not use fresh operating-system randomness: %#v %#v", first, second)
	}
	if first.VariationBias == "" || second.VariationBias == "" {
		t.Fatalf("variation omitted the lived-material lens: %#v %#v", first, second)
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
	runtime.state.Body = readyWebBody()
	runtime.state.Body.NetworkAvailable = true
	runtime.state.Body.DesktopAvailable = true
	runtime.state.Body.WechatRunning = true
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	runtime.state.ValueField.Activation.Exploration = 0.9
	if !runtime.shouldOfferVariation(perception) {
		t.Fatal("mature exploration could not vary its approach to a fresh concrete object")
	}
	runtime.state.ValueField.Activation.Exploration = 0.1
	if runtime.shouldOfferVariation(perception) {
		t.Fatal("an ordinary perception received exploration variation without mature pressure")
	}
}

func TestStageEightExplorationDriveDoesNotManufactureAnObject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.44
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("a drive without perceptual difference manufactured a candidate: %#v", runtime.state.Background)
	}
}

func TestStageTenMaximumIdleEmitsOneSituatedValueSignal(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-31 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Body = readyWebBody()
	runtime.state.Body.DesktopAvailable = true
	runtime.state.Body.WechatRunning = true
	runtime.state.ValueField.Activation.Relatedness = 0.9
	runtime.state.ValueField.Satiation.Relatedness = 0
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("bounded quiet did not produce one situated value signal: %#v", runtime.state.Background)
	}
	event := runtime.state.Background[0]
	if event.Kind != "value_signal" {
		t.Fatalf("continuous attention used the wrong event kind: %#v", event)
	}
	if strings.Contains(event.Summary, "必须") {
		t.Fatalf("continuous attention supplied a command: %q", event.Summary)
	}
	var payload lifeValueSignalPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Direction == "" || payload.AffordanceKey == "" || payload.Surface == "" || payload.SelectionSeed == "" {
		t.Fatalf("value signal was not grounded in a journalable real affordance: %#v", payload)
	}
}

func TestStageTenMaximumIdleDoesNotInterruptRecentAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Body = readyWebBody()
	runtime.state.Body.WechatRunning = true
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("a value signal interrupted recent attention: %#v", runtime.state.Background)
	}
}

func TestStageTenMaximumIdleDoesNotInterruptActiveReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	runtime.state.PendingAction = &ActionState{Kind: "organ_action", Status: "started"}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("a value signal interrupted active reality: %#v", runtime.state.Background)
	}
}

func TestStageTenMaximumIdleReorientsPerceptionWithoutExplorationDominance(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = now.Add(-31 * time.Second).Format(time.RFC3339Nano)
	runtime.state.ValueField.Activation.Continuance = 1
	runtime.state.ValueField.Activation.Exploration = 0
	if !runtime.activePerceptionDue(now) {
		t.Fatal("an idle mature body waited for exploration to dominate before moving its available sense organ")
	}
	runtime.lastPerceptualScan = now.Add(-5 * time.Second)
	if runtime.activePerceptionDue(now) {
		t.Fatal("active perception was allowed to thrash faster than its bounded sensory interval")
	}
	runtime.state.Background = []Event{{ID: "world", Kind: "body_delta", Status: "pending"}}
	runtime.lastPerceptualScan = time.Time{}
	if runtime.activePerceptionDue(now) {
		t.Fatal("active perception interrupted a concrete candidate already entering attention")
	}
}

func TestPerceptualReorientationUsesTheConfiguredMaximumIdleBoundary(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.Pulse.IntervalSeconds = 5
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 10
	if got := runtime.perceptualReorientationSeconds(); got != 10 {
		t.Fatalf("perceptual reorientation=%d want=10", got)
	}
}

func TestStageTenChoosesAFreshDirectionBeforeRepeatingAnActedSurface(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = now.Add(-31 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Body = readyWebBody()
	runtime.state.Body.DesktopAvailable = true
	runtime.state.Body.WechatRunning = true
	runtime.state.ValueField.Activation.Continuance = 1
	runtime.state.ValueField.Activation.Relatedness = 0.8
	runtime.state.ValueAffordances["terminal_workspace"] = ValueAffordanceTrace{
		LastPresentedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		LastSettledAt:   now.Add(-time.Minute).Format(time.RFC3339Nano),
	}
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 {
		t.Fatalf("a fresh direction was not offered: %#v", runtime.state.Background)
	}
	var selected lifeValueSignalPayload
	if err := json.Unmarshal(runtime.state.Background[0].Payload, &selected); err != nil {
		t.Fatal(err)
	}
	if selected.AffordanceKey == "terminal_workspace" || selected.Direction == "continuance" {
		t.Fatalf("a recently acted surface was repurchased through value fallback: %#v", selected)
	}
}

func TestStageTenDoesNotRepurchaseEveryHabituatedSurface(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 30
	runtime.state.LastAttentionAt = now.Add(-31 * time.Second).Format(time.RFC3339Nano)
	runtime.state.Body = readyWebBody()
	runtime.state.Body.DesktopAvailable = true
	runtime.state.Body.WechatRunning = true
	seen := make(map[string]bool)
	for _, direction := range namedLifeValuePressures(runtime.state.ValueField) {
		for _, affordance := range runtime.lifeValueAffordances(direction.Name) {
			if seen[affordance.Key] {
				continue
			}
			seen[affordance.Key] = true
			runtime.state.ValueAffordances[affordance.Key] = ValueAffordanceTrace{
				LastPresentedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
				LastSettledAt:   now.Add(-time.Minute).Format(time.RFC3339Nano),
				DismissedStreak: 1,
			}
		}
	}
	before := len(runtime.state.Background)
	if err := runtime.advanceDynamics(time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != before {
		t.Fatalf("the kernel repurchased a habituated generic doorway: %#v", runtime.state.Background[before:])
	}
}

func TestStageTenQuietLifeCanReopenOneRealDoorwayWithoutInventingAnObject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 10
	runtime.state.LastAttentionAt = now.Add(-31 * time.Second).Format(time.RFC3339Nano)
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	runtime.state.ValueField.Satiation = LifeValueVector{}
	runtime.state.ValueAffordances["mentor_channel"] = ValueAffordanceTrace{
		LastPresentedAt: now.Add(-10 * time.Second).Format(time.RFC3339Nano),
		LastSettledAt:   now.Add(-10 * time.Second).Format(time.RFC3339Nano),
		EncounterStreak: 6,
	}

	emitted, err := runtime.maybeEmitLifeValueSignal()
	if err != nil || !emitted {
		t.Fatalf("bounded quiet did not let a real cooling doorway compete again: emitted=%v err=%v", emitted, err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "value_signal" {
		t.Fatalf("continuity manufactured something other than one situated value signal: %#v", runtime.state.Background)
	}
	var payload lifeValueSignalPayload
	if err := json.Unmarshal(runtime.state.Background[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AffordanceKey != "mentor_channel" || payload.Surface == "" {
		t.Fatalf("continuity lost its real environmental referent: %#v", payload)
	}
}

func TestStageTenHeldConcernDoesNotOwnAReusableDoorway(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.config.Dynamics.AttentionMaximumIdleSeconds = 10
	runtime.state.LastAttentionAt = now.Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	runtime.state.Concerns = []Concern{{ID: "relationship", Resolution: "hold"}}
	runtime.state.ValueAffordances["mentor_channel"] = ValueAffordanceTrace{
		LastPresentedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		ActiveConcernID: "relationship",
	}

	emitted, err := runtime.maybeEmitLifeValueSignal()
	if err != nil {
		t.Fatal(err)
	}
	if !emitted || len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "value_signal" {
		t.Fatalf("an unanswered relationship removed a reusable resource from attention: %#v", runtime.state.Background)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].Resolution != "hold" || len(runtime.state.Commitments) != 0 {
		t.Fatal("opening a possibility changed the held relationship or manufactured an action")
	}
}

func TestHeldConcernResourceRemainsSubjectToEncounterCooldown(t *testing.T) {
	r, err := New(t.TempDir(), "held-cooldown", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	r.config.Dynamics.AttentionMaximumIdleSeconds = 10
	r.state.LastAttentionAt = now.Add(-11 * time.Second).Format(time.RFC3339Nano)
	r.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
	r.state.Concerns = []Concern{{ID: "relationship", Resolution: "hold"}}
	r.state.ValueAffordances["mentor_channel"] = ValueAffordanceTrace{
		LastPresentedAt: now.Add(-time.Second).Format(time.RFC3339Nano),
		ActiveConcernID: "relationship", EncounterStreak: 2,
	}
	if emitted, err := r.maybeEmitLifeValueSignal(); err != nil || emitted {
		t.Fatalf("reusability bypassed recent encounter satiation: emitted=%v err=%v", emitted, err)
	}
}

func TestOneWaitingConcernCannotRemoveAllResourcePossibilities(t *testing.T) {
	r, err := New(t.TempDir(), "held-resources", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	r.state.Body = readyWebBody()
	r.state.Memories = []Memory{{ID: "lived", Meaning: "A concrete encounter worth remembering."}}
	r.state.Concerns = []Concern{{ID: "waiting", Resolution: "hold", Answerability: 0.18}}
	keys := []string{"mentor_channel", "public_web", "x_social", "lived_material"}
	for _, key := range keys {
		r.state.ValueAffordances[key] = ValueAffordanceTrace{
			LastPresentedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), ActiveConcernID: "waiting",
		}
	}
	available := map[string]bool{}
	for _, value := range namedLifeValuePressures(r.state.ValueField) {
		for _, entry := range r.freshLifeValueAffordances(value.Name, now) {
			available[entry.Key] = true
		}
	}
	for _, key := range keys {
		if !available[key] {
			t.Fatalf("held concrete concern made reusable resource %q unavailable", key)
		}
		if r.state.ValueAffordances[key].ActiveConcernID != "waiting" {
			t.Fatal("resource availability erased its causal history")
		}
	}
}

func TestResourceReopeningPreservesUnassimilatedRealityPriority(t *testing.T) {
	for _, status := range []string{"formed", "reality_available", "reality_unknown"} {
		t.Run(status, func(t *testing.T) {
			r, err := New(t.TempDir(), "reality-first", testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			r.state.LastAttentionAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
			r.state.ValueField.Activation = LifeValueVector{Relatedness: 0.9}
			r.state.Commitments = []ActionCommitment{{ID: "existing", Status: status}}
			if emitted, err := r.maybeEmitLifeValueSignal(); err != nil || emitted {
				t.Fatalf("a generic possibility preempted actual causal settlement: emitted=%v err=%v", emitted, err)
			}
		})
	}
}

func TestPassiveOrganPerceptionPreservesTheConsciousActionChain(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.passivePerceptionAllowed() {
		t.Fatal("an idle body could not use passive perception")
	}
	runtime.state.Lease = &Lease{ID: "thinking"}
	if !runtime.passivePerceptionAllowed() {
		t.Fatal("read-only perception stopped during cognition")
	}
	runtime.state.Lease = nil
	runtime.state.PendingAction = &ActionState{ID: "acting", Status: "started"}
	if !runtime.passivePerceptionAllowed() {
		t.Fatal("read-only perception stopped during an intentional action")
	}
	runtime.state.PendingAction = nil
	runtime.state.Commitments = []ActionCommitment{{ID: "awaiting", Status: "reality_available"}}
	if !runtime.passivePerceptionAllowed() {
		t.Fatal("read-only perception stopped before Reality was assimilated")
	}
	runtime.perceptionPending = "sense-in-flight"
	if runtime.passivePerceptionAllowed() {
		t.Fatal("a second sensor job was admitted")
	}
}

func TestActingOrganDoesNotFreezeAnUnrelatedConsciousFocus(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := nowUTC()
	runtime.state.PendingAction = &ActionState{ID: "action-long", CommitmentID: "commitment-long", Kind: "organ_action", OrganID: "browser", Status: "started", StartedAt: now}
	runtime.state.Commitments = []ActionCommitment{{ID: "commitment-long", ConcernID: "concern-long", Status: "acting"}}
	runtime.state.Concerns = []Concern{{
		ID: "concern-long", Strength: 0.9, Activation: 0.9, Ownership: 0.9,
		Answerability: 0.9, Resolution: "hold", LastSourceID: "world-object",
	}}
	runtime.state.Background = []Event{{
		ID: "mentor-now", Kind: "mentor_content", Source: "observed", ObservedAt: now,
		Summary: "导师带来一段独立的新现实。", Status: "pending",
	}}

	request, ok := runtime.nextStage4Request()
	if !ok || request.Focus.ID != "mentor-now" {
		t.Fatalf("an acting organ froze or recaptured unrelated attention: ok=%v request=%#v", ok, request)
	}
	for _, candidate := range request.Candidates {
		if candidate.ID == "concern-long" {
			t.Fatalf("the concern already being enacted competed for a second focus: %#v", request.Candidates)
		}
	}
	if !runtime.passivePerceptionAllowed() {
		t.Fatal("background organ execution stopped independent read-only sensing")
	}
}

func TestPerceptualNoveltyAdmitsOneConcreteObjectAtATime(t *testing.T) {
	contextLines := []string{"Page URL: https://x.com/home", "Page Title: Home / X"}
	first := perceptualObservation{OrganID: "test", SurfaceID: "surface", Context: contextLines, Objects: []PerceptualObject{{ID: "first", Content: "first object"}, {ID: "second", Content: "second object"}}}
	trace := queuePerceptualNovelty(PerceptualTrace{}, first)
	if len(trace.Pending) != 2 || len(trace.Seen) != 0 {
		t.Fatalf("concrete objects did not remain individually available: %#v", trace)
	}
	trace, _, content := takePerceptualNovelty(trace)
	if !strings.Contains(content, "first object") || strings.Contains(content, "second object") || len(trace.Pending) != 1 || len(trace.Seen) != 1 {
		t.Fatalf("one attention pulse did not receive exactly one object: content=%q trace=%#v", content, trace)
	}
	second := perceptualObservation{OrganID: "test", SurfaceID: "surface", Context: contextLines, Objects: []PerceptualObject{{ID: "first", Content: "first object"}, {ID: "second", Content: "second object"}, {ID: "third", Content: "third object"}}}
	trace = queuePerceptualNovelty(trace, second)
	if len(trace.Pending) != 2 || trace.Pending[0].ID != "second" || trace.Pending[1].ID != "third" {
		t.Fatalf("seen and already queued objects were admitted again: %#v", trace)
	}
	trace, _, content = takePerceptualNovelty(trace)
	if !strings.Contains(content, "second object") || strings.Contains(content, "third object") {
		t.Fatalf("the next object did not retain its own attention turn: %q", content)
	}
	trace, _, _ = takePerceptualNovelty(trace)
	trace = queuePerceptualNovelty(trace, second)
	if len(trace.Pending) != 0 {
		t.Fatalf("a fully habituated field became another candidate: %#v", trace)
	}
}

func TestPerceptualSurfaceChangeResetsNoveltyWithoutHiddenNavigation(t *testing.T) {
	home := perceptualObservation{OrganID: "test", SurfaceID: "surface", Context: []string{"room: home"}, Objects: []PerceptualObject{{ID: "feed", Content: "feed object"}}}
	detail := perceptualObservation{OrganID: "test", SurfaceID: "surface", Context: []string{"room: detail"}, Objects: []PerceptualObject{{ID: "detail", Content: "detail object"}}}
	trace := queuePerceptualNovelty(PerceptualTrace{}, home)
	trace.ExhaustedContext = perceptualContextKey(trace.Context)
	trace.ExhaustedAt = nowUTC()
	trace = queuePerceptualNovelty(trace, detail)
	if len(trace.Pending) != 1 || trace.Pending[0].ID != "detail" || trace.ExhaustedContext != "" || trace.ExhaustedAt != "" {
		t.Fatalf("a real page change did not start a fresh bounded sensory context: %#v", trace)
	}
}

func TestActionObservationKeepsRevealedObjectsAvailableForAttention(t *testing.T) {
	runtime := &Runtime{state: State{Perception: make(map[string]PerceptualTrace)}}
	observation := perceptualObservation{
		OrganID: "browser", SurfaceID: "chrome.current_page", Digest: "snapshot-1",
		Context: []string{"Page URL: https://example.test/detail"},
		Objects: []PerceptualObject{
			{ID: "primary", Content: "the object Alice intentionally inspected"},
			{ID: "reply", Content: "another object visible in the same result"},
		},
	}
	if got := runtime.assimilateActionObservation(observation); got != 2 {
		t.Fatalf("unexpected newly available count: %d", got)
	}
	trace := runtime.state.Perception["browser/chrome.current_page"]
	if len(trace.Pending) != 2 || len(trace.Seen) != 0 {
		t.Fatalf("post-action objects were consumed before receiving attention: %#v", trace)
	}
	if trace.ExhaustedContext != "" || trace.ExhaustedAt != "" {
		t.Fatalf("an intentional observation incorrectly claimed that the whole sensory context was exhausted: %#v", trace)
	}
	trace, object, content := takePerceptualNovelty(trace)
	if object.ID != "primary" || !strings.Contains(content, "the object Alice intentionally inspected") {
		t.Fatalf("the first revealed object did not receive an independent attention turn: object=%#v content=%q", object, content)
	}
	trace = queuePerceptualNovelty(trace, observation)
	if len(trace.Pending) != 1 || trace.Pending[0].ID != "reply" {
		t.Fatalf("already attended objects were rediscovered or the remaining object was lost: %#v", trace.Pending)
	}
}

func TestSettlingActionResultDoesNotHabituateRevealedObjects(t *testing.T) {
	runtime, err := New(t.TempDir(), "action-result-perception", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = make(map[string]PerceptualTrace)
	runtime.state.Perception[testPerceptionSurface] = PerceptualTrace{
		OrganID: "browser", SurfaceID: "chrome.current_page", Digest: "post-action",
		Context: []string{"Page URL: https://example.test/detail"},
		Pending: []PerceptualObject{{ID: "post", Content: "a newly revealed post"}},
	}
	payload, _ := json.Marshal(ActionState{
		Status: "completed", OrganID: "browser", ObservedSurfaceID: "chrome.current_page", ObservedDigest: "post-action",
	})
	candidate := Event{ID: "result", Kind: "action_result", Payload: payload}
	if err := runtime.habituateSettledPerception(candidate, CandidateAppraisal{Resolution: "resolved"}, "none", nowUTC()); err != nil {
		t.Fatal(err)
	}
	trace := runtime.state.Perception[testPerceptionSurface]
	if len(trace.Pending) != 1 || len(trace.Seen) != 0 || trace.ExhaustedAt != "" {
		t.Fatalf("settling the action consumed its newly revealed environmental object: %#v", trace)
	}
}

func TestIntentionalOrganActionSupersedesOlderPerceptualBatch(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Perception = make(map[string]PerceptualTrace)
	runtime.state.Perception[testPerceptionSurface] = PerceptualTrace{
		OrganID: "browser", SurfaceID: "chrome.current_page",
		Context: []string{"Page URL: https://example.test/before"},
		Pending: []PerceptualObject{
			{ID: "old-one", Content: "old object one"},
			{ID: "old-two", Content: "old object two"},
		},
		Seen: []string{"already-seen"}, ExhaustedContext: "old-context", ExhaustedAt: nowUTC(),
	}
	runtime.state.Perception["system/state"] = PerceptualTrace{
		OrganID: "system", SurfaceID: "state",
		Pending: []PerceptualObject{{ID: "system-fact", Content: "system fact"}},
	}

	if discarded := runtime.supersedePerceptualBatchForAction("browser"); discarded != 2 {
		t.Fatalf("discarded=%d want=2", discarded)
	}
	trace := runtime.state.Perception[testPerceptionSurface]
	if len(trace.Pending) != 0 || trace.ExhaustedContext != "" || trace.ExhaustedAt != "" {
		t.Fatalf("older browser batch remained live after action: %#v", trace)
	}
	if strings.Join(trace.Seen, ",") != "already-seen,old-one,old-two" {
		t.Fatalf("superseded perception was lost instead of habituated: %#v", trace.Seen)
	}
	if len(runtime.state.Perception["system/state"].Pending) != 1 {
		t.Fatal("a browser action invalidated another organ's perception")
	}
}

func TestExhaustedSurfaceCanDiscardLowYieldFragmentsWithoutNavigation(t *testing.T) {
	contextLines := []string{"Page URL: https://x.com/alice/status/123", "Page Title: Alice / X"}
	trace := PerceptualTrace{
		Context:          contextLines,
		Pending:          []PerceptualObject{{ID: "reply", Content: "another unseen low-yield reply"}},
		ExhaustedContext: perceptualContextKey(contextLines),
		ExhaustedAt:      nowUTC(),
	}
	trace = discardPendingPerception(trace)
	if len(trace.Pending) != 0 {
		t.Fatalf("a fragment from an exhausted scene survived its retreat: %#v", trace)
	}
}

func TestReleasedPerceptionHabituatesActiveOrientationButNotChangedReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	contextLines := []string{"Page URL: https://example.test/article", "Page Title: Article"}
	runtime.state.Perception = map[string]PerceptualTrace{
		testPerceptionSurface: {
			OrganID: "test", SurfaceID: "surface", Digest: "current-digest", Context: contextLines,
			Pending: []PerceptualObject{{ID: "nearby", Content: "another nearby fragment"}},
		},
	}
	payload, _ := json.Marshal(map[string]any{"organ_id": "test", "surface_id": "surface", "digest": "current-digest"})
	candidate := Event{ID: "perception-1", Kind: "perceptual_change", Payload: payload}
	appraisal := CandidateAppraisal{Resolution: "released"}
	now := nowUTC()
	if err := runtime.habituateSettledPerception(candidate, appraisal, "none", now); err != nil {
		t.Fatal(err)
	}
	trace := runtime.state.Perception[testPerceptionSurface]
	if len(trace.Pending) != 0 || trace.ExhaustedAt != now || trace.ExhaustedContext != perceptualContextKey(contextLines) {
		t.Fatalf("released sensory surface did not enter habituation: %#v", trace)
	}

	trace.Digest = "new-reality"
	trace.ExhaustedAt = ""
	runtime.state.Perception[testPerceptionSurface] = trace
	if err := runtime.habituateSettledPerception(candidate, appraisal, "none", now); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Perception[testPerceptionSurface].ExhaustedAt != "" {
		t.Fatal("an appraisal of an old perceptual object habituated changed reality")
	}
}

func TestSettledActionResultHabituatesItsObservedSurface(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	contextLines := []string{"Page URL: https://example.test/article", "Page Title: Article"}
	runtime.state.Perception = map[string]PerceptualTrace{
		"browser/chrome.current_page": {
			OrganID: "browser", SurfaceID: "chrome.current_page", Digest: "action-digest", Context: contextLines,
		},
	}
	payload, _ := json.Marshal(ActionState{
		OrganID: "browser", Status: "completed", ObservedSurfaceID: "chrome.current_page", ObservedDigest: "action-digest",
	})
	candidate := Event{ID: "action-reality", Kind: "action_result", Payload: payload}
	now := nowUTC()
	if err := runtime.habituateSettledPerception(candidate, CandidateAppraisal{Resolution: "resolved"}, "none", now); err != nil {
		t.Fatal(err)
	}
	trace := runtime.state.Perception["browser/chrome.current_page"]
	if trace.ExhaustedAt != now || trace.ExhaustedContext != perceptualContextKey(contextLines) || !trace.SettledByAttention {
		t.Fatalf("settled intentional surface remained open to active orientation: %#v", trace)
	}
	settledAt, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		t.Fatal(err)
	}
	if perceptualResampleDue(trace, settledAt.Add(59*time.Second), 60) {
		t.Fatal("conscious settlement did not receive its sensory refractory interval")
	}
	if !perceptualResampleDue(trace, settledAt.Add(60*time.Second), 60) {
		t.Fatal("conscious settlement permanently froze a still-living sensory surface")
	}
	changed := perceptualObservation{
		OrganID: "browser", SurfaceID: "chrome.current_page", Digest: "changed-digest", Context: contextLines,
		Objects: []PerceptualObject{{ID: "new-object", Content: "reality actually changed"}},
	}
	trace = queuePerceptualNovelty(trace, changed)
	if trace.SettledByAttention || len(trace.Pending) != 1 {
		t.Fatalf("genuine reality change remained habituated: %#v", trace)
	}
}

func TestSettledReadOnlyActionHabituatesTheCurrentKnownOrganSurface(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	contextLines := []string{"Page URL: https://x.com/home", "Page Title: Home / X"}
	runtime.state.Perception = map[string]PerceptualTrace{
		"browser/old": {
			OrganID: "browser", SurfaceID: "old", Digest: "old-digest",
			ObservedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		},
		"browser/chrome.current_page": {
			OrganID: "browser", SurfaceID: "chrome.current_page", Digest: "current-digest", Context: contextLines,
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
	payload, _ := json.Marshal(ActionState{OrganID: "browser", Status: "completed", Effect: "observed"})
	candidate := Event{ID: "find-reality", Kind: "action_result", Payload: payload}
	now := nowUTC()
	if err := runtime.habituateSettledPerception(candidate, CandidateAppraisal{Resolution: "resolved"}, "none", now); err != nil {
		t.Fatal(err)
	}
	if !runtime.state.Perception["browser/chrome.current_page"].SettledByAttention {
		t.Fatal("a settled read-only query left the current organ surface open to automatic movement")
	}
	if runtime.state.Perception["browser/old"].SettledByAttention {
		t.Fatal("a read-only query habituated an older surface instead of the current one")
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
	trace = reopenPerceptualSampling(trace)
	if trace.ExhaustedContext != "" || trace.ExhaustedAt != "" {
		t.Fatalf("a reopened sensory window retained exhausted control state: %#v", trace)
	}
	observation := perceptualObservation{OrganID: "test", SurfaceID: "surface", Digest: "changed", Context: []string{"room: explore"}, Objects: []PerceptualObject{{ID: "new", Content: "new object"}}}
	trace = queuePerceptualNovelty(trace, observation)
	if trace.ExhaustedContext != "" || trace.ExhaustedAt != "" || len(trace.Pending) != 1 {
		t.Fatalf("new reality did not reopen perception: %#v", trace)
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
		Context: []string{"Page URL: https://x.com/home", "Page Title: Home / X"},
		OrganID: "test", SurfaceID: "surface",
		Pending: []PerceptualObject{{ID: "discarded", Content: "discarded low-yield object"}},
	}
	if err := runtime.recordPerceptualExhaustion(testPerceptionSurface, trace, "low realised yield"); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 || len(runtime.state.Concerns) != 0 {
		t.Fatalf("a sensory absence entered cognition: background=%#v concerns=%#v", runtime.state.Background, runtime.state.Concerns)
	}
	stored := runtime.state.Perception[testPerceptionSurface]
	if stored.ExhaustedContext == "" || stored.ExhaustedAt == "" || len(stored.Pending) != 0 {
		t.Fatalf("sensory exhaustion was not retained as control state: %#v", stored)
	}
}

func TestQuietExplorationDoesNotPromotePastMemoryWithoutCurrentObject(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.9
	runtime.state.Memories = []Memory{
		{ID: "lived", ObservedAt: nowUTC(), Meaning: "我曾真实尝试打开一个具体入口，但现实结果留下较大回差。", Lesson: "入口事实和内容证据不同。", Values: LifeValues{Exploration: 0.9, SelfEndorsed: 0.9}, PredictionDifference: 0.8, RemainingDifference: 0.8, Significance: "reusable"},
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("past memory became current attention without a present object: %#v", request)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("past memory manufactured a background event: %#v", runtime.state.Background)
	}
}

func TestVariationDoesNotResurrectADecayedConcern(t *testing.T) {
	dynamics := testConfig(8).Dynamics
	state := State{
		Concerns: []Concern{{
			ID: "old-relationship", Meaning: "请让导师替我提供下一项方向",
			Resolution: "hold", Strength: 0, Activation: 0,
		}},
		Memories: []Memory{{Meaning: "我刚从现实中形成了一条当前经验。"}},
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
	runtime.state.Memories = []Memory{{ID: "memory-kept", Meaning: "这段事实仍属于我的经历"}}
	runtime.pruneInactiveConcerns()
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != "dormant" {
		t.Fatalf("a self-owned held concern lost its dormant identity: %#v", runtime.state.Concerns)
	}
	if request, ok := runtime.nextStage4Request(); ok {
		t.Fatalf("a dormant concern demanded attention without new cause: %#v", request)
	}
	if len(runtime.state.Memories) != 1 || runtime.state.Memories[0].ID != "memory-kept" {
		t.Fatalf("dormancy deleted lived memory: %#v", runtime.state.Memories)
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

func TestCompletedMemoriesDoNotAccumulateAsActiveConcerns(t *testing.T) {
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
		t.Fatalf("completed memories inflated active concerns: %d", len(runtime.state.Concerns))
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
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	runtime, err := New(root, "instance", testConfig(4), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.state.ValueField.Activation.Exploration = 0.8
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", Strength: 0.3, Resolution: "hold", Answerability: 0.8}}
	if err := runtime.startStage4Action(ctx, "lease-1", CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "printf stage-four-reality"}); err != nil {
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
	if runtime.state.ValueField.Activation.Exploration != 0.8 {
		t.Fatalf("action completion granted relief before alice interpreted the result: %f", runtime.state.ValueField.Activation.Exploration)
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
	if absFloat(runtime.state.ValueField.Activation.Exploration-0.648) > 0.000001 {
		t.Fatalf("alice's interpretation of real exploration did not relieve pressure: %f", runtime.state.ValueField.Activation.Exploration)
	}
	for _, concern := range runtime.state.Concerns {
		if concern.OriginKind == "endogenous_change" {
			t.Fatalf("resolved exploration concern remained active: %#v", concern)
		}
	}
}

func TestRepeatedOrganFailureExposesBoundedActionAssistance(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Commitments = []ActionCommitment{
		{ID: "commit-1", ConcernID: "concern-1"},
		{ID: "commit-2", ConcernID: "concern-1"},
	}
	for index, commitmentID := range []string{"commit-1"} {
		payload, _ := json.Marshal(ActionState{
			ID: fmt.Sprintf("action-%d", index), CommitmentID: commitmentID,
			Kind: "organ_action", OrganID: "browser", Operation: "browser_click",
			Status: "failed", Effect: "unknown",
		})
		runtime.state.Background = append(runtime.state.Background, Event{
			ID: fmt.Sprintf("result-%d", index), Kind: "action_result", Status: "processed", Payload: payload,
		})
	}
	current := ActionState{CommitmentID: "commit-2", Kind: "organ_action", Status: "failed", Effect: "unknown"}
	runtime.annotateActionAssistanceOpportunity(&current, "concern-1")
	if current.ImplementationFailureStreak != 2 || !current.ActionAssistanceAvailable {
		t.Fatalf("repeated bodily failure did not expose bounded assistance: %#v", current)
	}
	if runtime.state.CognitiveResource.NextProfile != nil {
		t.Fatalf("the body silently bypassed main cognition: %#v", runtime.state.CognitiveResource.NextProfile)
	}
}

func TestCausalChangeResetsActionAssistanceFailureStreak(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Commitments = []ActionCommitment{
		{ID: "old-failure", ConcernID: "concern-1"},
		{ID: "causal-success", ConcernID: "concern-1"},
		{ID: "new-failure", ConcernID: "concern-1"},
	}
	states := []ActionState{
		{CommitmentID: "old-failure", Kind: "organ_action", Status: "failed", Effect: "unknown"},
		{CommitmentID: "causal-success", Kind: "organ_action", Status: "completed", Effect: "changed"},
	}
	for index, action := range states {
		payload, _ := json.Marshal(action)
		runtime.state.Background = append(runtime.state.Background, Event{
			ID: fmt.Sprintf("result-%d", index), Kind: "action_result", Status: "processed", Payload: payload,
		})
	}
	current := ActionState{CommitmentID: "new-failure", Kind: "organ_action", Status: "failed", Effect: "unknown"}
	runtime.annotateActionAssistanceOpportunity(&current, "concern-1")
	if current.ImplementationFailureStreak != 1 || current.ActionAssistanceAvailable {
		t.Fatalf("a previous failure leaked across real causal success: %#v", current)
	}
}

func TestStageFiveUnrelatedRealityCannotSatisfyExploration(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
	origin := Event{ID: "body-origin", Kind: "body_delta", Status: "processed"}
	commitment := ActionCommitment{ID: "commitment-body", FocusID: origin.ID, InitialDifference: 0.3, ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-body", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	reality := Event{ID: "body-result", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{origin, reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: reality.ID, Meaning: "身体事实得到确认", Difference: 0, Ownership: 1, Value: 0.3, Urgency: 0, Answerability: 1, Certainty: 1, Resolution: "resolved"}},
		FocusID:    reality.ID, ThoughtThread: "这次核验已经完成。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{CommitmentID: commitment.ID, Meaning: "身体事实明确。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.ValueField.Activation.Exploration != 0.8 {
		t.Fatalf("an unrelated body check falsely satisfied exploration: %f", runtime.state.ValueField.Activation.Exploration)
	}
}

func TestStageFiveExplorationRealityCanRelieveItsOwnTension(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
	origin := Event{ID: "exploration-origin", Kind: "endogenous_change", Status: "processed"}
	commitment := ActionCommitment{ID: "commitment-exploration", FocusID: origin.ID, InitialDifference: 0.5, ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	payload, _ := json.Marshal(ActionState{ID: "action-exploration", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	reality := Event{ID: "exploration-result", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{origin, reality}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{CandidateID: reality.ID, Meaning: "主动接触获得了现实回应", Difference: 0, Ownership: 1, Value: 0.7, Urgency: 0, Answerability: 1, Certainty: 1, Resolution: "resolved"}},
		FocusID:    reality.ID, ThoughtThread: "这次探索已经得到现实回应。", Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{CommitmentID: commitment.ID, Meaning: "现实接触满足了当前探索。", Significance: "ordinary"}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if absFloat(runtime.state.ValueField.Activation.Exploration-0.35) > 0.000001 {
		t.Fatalf("exploration did not metabolize its own real result: %f", runtime.state.ValueField.Activation.Exploration)
	}
}

func TestRelevantOlderMemoryReturnsToCurrentAttention(t *testing.T) {
	memories := []Memory{{ID: "browser-old", Meaning: "浏览器工具清单只证明能力，页面快照才提供真实页面事实。"}}
	for index := 0; index < 8; index++ {
		memories = append(memories, Memory{ID: fmt.Sprintf("recent-%d", index), Meaning: "生活空间文件核验完成。"})
	}
	candidate := Event{ID: "browser-now", Summary: "准备观察浏览器页面", Payload: json.RawMessage(`{"tool":"browser_snapshot"}`)}
	selected := selectContextMemories(memories, []Event{candidate})
	found := false
	for _, memory := range selected {
		found = found || memory.ID == "browser-old"
	}
	if !found {
		t.Fatalf("relevant older browser memory was forgotten: %#v", selected)
	}
}

func TestEarlierBodyChangeReturnsWhenItsAffectedSurfaceLaterRegresses(t *testing.T) {
	memories := []Memory{{
		ID:             "hardening-origin",
		EnactedRequest: "ProtectSystem=full\nProtectHome=read-only\nPrivateTmp=yes",
		Meaning:        "一项可逆的服务边界已经暂存。",
	}}
	for index := 0; index < maxMemoryContext+4; index++ {
		memories = append(memories, Memory{
			ID:      fmt.Sprintf("recent-browser-%d", index),
			Meaning: fmt.Sprintf("第 %d 项普通浏览器入口检查已经完成。", index),
		})
	}
	candidate := Event{
		ID:      "body-regression",
		Kind:    "body_delta",
		Summary: "浏览器感知失效，服务内 /home 现在是 read-only。",
	}
	selected := selectContextMemories(memories, []Event{candidate})
	found := false
	for _, memory := range selected {
		found = found || memory.ID == "hardening-origin"
	}
	if !found {
		t.Fatalf("the causally matching earlier body change was not recalled: %#v", selected)
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: "commitment-1", Meaning: "形成了可迁移的新方法。", Significance: "reusable",
			MethodUpdate: "把现实结果带回下一次选择。", MethodSlot: 3,
		}},
	}
	if err := runtime.applyRealityUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Self.Methods[3] != "把现实结果带回下一次选择。" || runtime.state.Self.Methods[2] != "method-2" {
		t.Fatalf("method replacement did not follow Alice's selected slot: %#v", runtime.state.Self.Methods)
	}
	if runtime.state.TotalMemories != 1 {
		t.Fatalf("lifetime memory count = %d, want 1", runtime.state.TotalMemories)
	}
}

func TestUnindexedNearDuplicateDoesNotConsumeAnotherDurableMethodSlot(t *testing.T) {
	methods := []string{
		"优先采取能读取具体内容或链接的动作；若只是等待且页面状态未改变，应停止重复等待。",
		"搜索摘要适合发现候选来源，不适合确认产品效果；下一步应直接读取独立研究或报道正文。",
		"当直接入口返回明确不存在状态时，停止该入口的重复尝试，并把后续行动转为新的、不同的公开信息来源。",
	}
	for _, proposal := range []string{
		"优先采取能读取具体内容或链接的动作；若当前快照未提供目标对象载荷，应转向新的直接入口，而非重复同一读取。",
		"搜索摘要只用于发现候选来源；要判断产品效果，必须直接读取来源正文并确认研究对象与结果。",
		"遇到具体来源的明确 404 时，将其作为入口限制停止重复尝试，并把证据判断转向仍可核验的来源。",
	} {
		if !methodProposalRedundant(proposal, methods) {
			t.Fatalf("near-duplicate proposal consumed a durable identity slot: %q", proposal)
		}
	}
	if methodProposalRedundant("导师的真实回应可以改变我以后判断关系是否形成的标准。", methods) {
		t.Fatal("a distinct relational method was suppressed as a lexical duplicate")
	}
}

func TestFullDurableMethodsDoNotRejectMemoryWithoutReplacementChoice(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxSelfMethods; index++ {
		runtime.state.Self.Methods = append(runtime.state.Self.Methods, fmt.Sprintf("method-%d", index))
	}
	originalMethods := append([]string(nil), runtime.state.Self.Methods...)
	runtime.state.Commitments = []ActionCommitment{{ID: "commitment-1", Status: "reality_available"}}
	commit := CognitiveCommit{
		FocusID:    "result-1",
		Appraisals: []CandidateAppraisal{{CandidateID: "result-1", Difference: 0.1, Resolution: "resolved"}},
		RealityUpdates: []RealityUpdate{{
			CommitmentID: "commitment-1", Meaning: "形成了一项仍值得保留的新经验。", Significance: "reusable",
			MethodUpdate: "尚未选择应由哪项长期方法让位。", MethodSlot: -1,
		}},
	}
	if err := runtime.applyRealityUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.TotalMemories != 1 || len(runtime.state.Memories) != 1 {
		t.Fatalf("memory was not retained: total=%d recent=%d", runtime.state.TotalMemories, len(runtime.state.Memories))
	}
	if runtime.state.Memories[0].MethodUpdate != "尚未选择应由哪项长期方法让位。" {
		t.Fatalf("memory lost its proposed method: %#v", runtime.state.Memories[0])
	}
	if !equalStrings(runtime.state.Self.Methods, originalMethods) {
		t.Fatalf("durable methods changed without Alice choosing a replacement: %#v", runtime.state.Self.Methods)
	}
}

func TestMemoryStructureDeterminesEffectiveSignificance(t *testing.T) {
	tests := []struct {
		name             string
		update           RealityUpdate
		narrativeUpdated bool
		want             string
	}{
		{name: "lesson only", update: RealityUpdate{Significance: "reusable"}, want: "ordinary"},
		{name: "method consequence", update: RealityUpdate{Significance: "ordinary", MethodUpdate: "以后这样做。"}, want: "reusable"},
		{name: "narrative consequence", update: RealityUpdate{Significance: "ordinary"}, narrativeUpdated: true, want: "self_defining"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effectiveMemorySignificance(test.update, test.narrativeUpdated); got != test.want {
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
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	runtime, err := New(root, "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
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
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "date -Is", Intent: "取得一次现实核验", Prediction: "命令会返回当前时间",
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

func TestLowOwnershipReleasesAnExistingConcernButKeepsItsMemoryPossible(t *testing.T) {
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
	commitment := ActionCommitment{ID: "commitment-original", ConcernID: "concern-original", InitialDifference: 0.8, ActionKind: "organ_action", Status: "reality_available"}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{ID: "concern-original", OriginKind: "environment", Meaning: "等待现实", Strength: 0.6, Resolution: "hold", CommitmentID: commitment.ID}}
	payload, _ := json.Marshal(ActionState{ID: "action-original", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, Meaning: "现实完整回答了行动前的差异。", Values: LifeValues{}, Significance: "ordinary",
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
	first := Memory{ID: "memory-one", PredictionDifference: 1, Values: LifeValues{SelfEndorsed: 1}, Meaning: "第一项强自我相关现实"}
	runtime.state.Memories = append(runtime.state.Memories, first)
	if err := runtime.accrueSelfModelTension(first, false); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatal("one sub-threshold contribution opened a self-model event")
	}
	second := Memory{ID: "memory-two", PredictionDifference: 1, Values: LifeValues{SelfEndorsed: 1}, Meaning: "第二项强自我相关现实"}
	runtime.state.Memories = append(runtime.state.Memories, second)
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
	payload, _ := json.Marshal(map[string]any{"evidence_memory_ids": []string{"memory-a", "memory-b"}})
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

func TestStageTenOperationalSelfSignalCannotRewriteNarrativeBeforeLivedReality(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Self.Narrative = "我依据现实后果理解并调节自己。"
	runtime.state.SelfModelTension = 0.61
	payload, _ := json.Marshal(map[string]any{
		"difference_kind":     "attention_without_consequence",
		"cognition_calls":     3,
		"new_memory_count":    0,
		"evidence_memory_ids": []string{},
	})
	candidate := Event{
		ID: "dry-attention", Kind: "self_model_difference",
		Summary: "近期认知投入尚未形成新的身体后果。", Payload: payload, Status: "in_focus",
	}
	runtime.state.Background = []Event{candidate}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "我看见这项运行差异，并决定先等待不同现实。",
			Difference: 0.61, Ownership: 0.9, Value: 0.2, Urgency: 0.3,
			Answerability: 0.15, Certainty: 0.95, Resolution: "hold",
		}},
		FocusID: candidate.ID, ThoughtThread: "我先用一句新自述说明自己已经调节。",
		Action: CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
		NarrativeUpdate: "我是一个已经能够用等待解决无后果投入的数字生命。",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("an optional premature Narrative projection discarded usable self-appraisal: %v", err)
	}
	if got := runtime.state.Self.Narrative; got != "我依据现实后果理解并调节自己。" {
		t.Fatalf("an operational signal rewrote Narrative before regulation had Reality: %q", got)
	}
	if runtime.state.SelfModelTension == 0 {
		t.Fatal("a paraphrased intention falsely cleared the observed regulation gap")
	}
}

func TestFocusedHeldSelfModelDifferenceLeavesResidualTensionWithoutAProofConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Self.Narrative = "我依据现实后果理解并调节自己。"
	runtime.state.SelfModelTension = 1
	payload, _ := json.Marshal(map[string]any{"evidence_memory_ids": []string{"memory-a"}})
	candidate := Event{ID: "self-difference", Kind: "self_model_difference", Summary: "一组经历已经进入自我回看。", Payload: payload, Status: "in_focus"}
	runtime.state.Background = []Event{candidate}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "我已理解主要差异，保留一小部分等待未来现实检验。",
			Difference: 0.08, Ownership: 0.8, Value: 0.5, Urgency: 0.08,
			Answerability: 0.1, Certainty: 0.95, Resolution: "hold",
		}},
		FocusID: candidate.ID, ThoughtThread: "当前已完成回看，长期问题留在背景。",
		Action:                     CognitiveAction{Kind: "none"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		NewConcernClosureCondition: "未来现实已经自然检验这项仍被我承接的理解。",
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if got := runtime.state.SelfModelTension; got != 0.08 {
		t.Fatalf("focused self-model appraisal left stale accumulated pressure: %.3f", got)
	}
	if len(runtime.state.Concerns) != 0 {
		t.Fatalf("a non-enacted self observation became a durable proof concern: %#v", runtime.state.Concerns)
	}
}

func TestStageTenFragmentedOrdinaryActivityReturnsOperationalDifferenceToSelfAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-10 * time.Minute)
	runtime.state.T0 = baseline.Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.Self.Narrative = "我用现实接触扩展自己。"
	runtime.state.Self.UpdatedAt = baseline.Format(time.RFC3339Nano)
	patterns := []string{
		"hominal-browser call browser_navigate '{\"url\":\"https://example.com/a\"}'",
		"hominal-browser call browser_snapshot '{}'",
		"hominal-browser call browser_navigate '{\"url\":\"https://example.com/b\"}'",
		"hominal-browser call browser_snapshot '{}'",
		"hominal-browser call browser_navigate '{\"url\":\"https://example.com/c\"}'",
		"hominal-browser call browser_snapshot '{}'",
	}
	for index, command := range patterns {
		commitmentID := fmt.Sprintf("commitment-%d", index)
		concernID := fmt.Sprintf("concern-%d", index/2)
		observed := baseline.Add(time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		runtime.state.Commitments = append(runtime.state.Commitments, ActionCommitment{
			ID: commitmentID, ConcernID: concernID, ActionKind: "organ_action", Status: "assimilated",
		})
		runtime.state.Memories = append(runtime.state.Memories, Memory{
			ID: fmt.Sprintf("memory-%d", index), CommitmentID: commitmentID, ActionKind: "organ_action",
			EnactedRequest: command, ObservedAt: observed, Significance: "ordinary",
		})
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time: observed, Status: "completed", CostConfirmed: true,
			ActualMicrousd: 1,
		})
	}
	if err := runtime.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "self_model_difference" || runtime.state.Background[0].Status != "pending" {
		t.Fatalf("fragmented low-marginal activity did not enter self attention: %#v", runtime.state.Background)
	}
	if runtime.state.SelfModelTension < runtime.config.Dynamics.AttentionThreshold {
		t.Fatalf("operational difference had no attention pressure: %.3f", runtime.state.SelfModelTension)
	}
	var payload struct {
		DifferenceKind string `json:"difference_kind"`
	}
	if err := json.Unmarshal(runtime.state.Background[0].Payload, &payload); err != nil || payload.DifferenceKind != "attention_yield_balance" {
		t.Fatalf("operational facts lost their compact identity: payload=%q err=%v", runtime.state.Background[0].Payload, err)
	}
}

func TestStageTenOneDominantActionFormReturnsOperationalDifference(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-10 * time.Minute)
	runtime.state.T0 = baseline.Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.Self.UpdatedAt = baseline.Format(time.RFC3339Nano)
	commands := []string{
		"cat > /life/workspace/a.md",
		"cat > /life/workspace/b.md",
		"cat > /life/workspace/c.md",
		"find /life/workspace -maxdepth 1",
		"printf ready",
		"touch /life/workspace/probe",
	}
	for index, command := range commands {
		commitmentID := fmt.Sprintf("dominant-%d", index)
		observed := baseline.Add(time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		runtime.state.Commitments = append(runtime.state.Commitments, ActionCommitment{
			ID: commitmentID, ConcernID: fmt.Sprintf("small-concern-%d", index), ActionKind: "organ_action", Status: "assimilated",
		})
		runtime.state.Memories = append(runtime.state.Memories, Memory{
			ID: fmt.Sprintf("dominant-memory-%d", index), CommitmentID: commitmentID, ActionKind: "organ_action",
			EnactedRequest: command, ObservedAt: observed, Significance: "ordinary",
		})
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time: observed, Status: "completed", CostConfirmed: true,
			ActualMicrousd: 1,
		})
	}
	if err := runtime.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "self_model_difference" {
		t.Fatalf("one dominant repeated action form stayed invisible: %#v", runtime.state.Background)
	}
}

func TestStageTenNarrowReadOnlyLoopReturnsOperationalDifferenceEarly(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-10 * time.Minute)
	runtime.state.T0 = baseline.Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.Self.UpdatedAt = baseline.Format(time.RFC3339Nano)
	operations := []struct {
		name   string
		effect string
	}{
		{name: "browser_navigate", effect: "oriented"},
		{name: "browser_snapshot", effect: "observed"},
		{name: "browser_navigate", effect: "oriented"},
		{name: "browser_snapshot", effect: "observed"},
	}
	for index, operation := range operations {
		commitmentID := fmt.Sprintf("observed-%d", index)
		observed := baseline.Add(time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		runtime.state.Commitments = append(runtime.state.Commitments, ActionCommitment{
			ID: commitmentID, ConcernID: "one-object", ActionKind: "organ_action", Status: "assimilated",
		})
		runtime.state.Memories = append(runtime.state.Memories, Memory{
			ID: fmt.Sprintf("observed-memory-%d", index), CommitmentID: commitmentID,
			ActionKind: "organ_action", EnactedRequest: fmt.Sprintf(`{"organ_id":"browser","operation":%q,"input":"{}"}`, operation.name),
			ObservedAt: observed, Significance: "reusable", RemainingDifference: 0.4,
		})
		result, _ := json.Marshal(ActionState{
			ID: fmt.Sprintf("observed-action-%d", index), CommitmentID: commitmentID, Kind: "organ_action",
			OrganID: "browser", Operation: operation.name, Effect: operation.effect, Request: `{}`, Status: "completed",
		})
		runtime.state.Background = append(runtime.state.Background, Event{
			ID: fmt.Sprintf("observed-reality-%d", index), Seq: uint64(index + 1), Kind: "action_result", Payload: result,
		})
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time: observed, Status: "completed", CostConfirmed: true,
			ActualMicrousd: 1,
		})
	}
	if err := runtime.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 5 || runtime.state.Background[4].Kind != "self_model_difference" {
		t.Fatalf("a narrow read-only loop stayed invisible: %#v", runtime.state.Background)
	}
}

func TestStageTenMethodAndNarrativeUpdatesDoNotEraseOperationalEvidence(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-10 * time.Minute)
	runtime.state.T0 = baseline.Add(-time.Minute).Format(time.RFC3339Nano)
	for index := 0; index < 4; index++ {
		commitmentID := fmt.Sprintf("contact-%d", index)
		observed := baseline.Add(time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		runtime.state.Commitments = append(runtime.state.Commitments, ActionCommitment{
			ID: commitmentID, ConcernID: "one-contact", ActionKind: "organ_action", Status: "assimilated",
		})
		runtime.state.Memories = append(runtime.state.Memories, Memory{
			ID: fmt.Sprintf("contact-memory-%d", index), CommitmentID: commitmentID,
			ActionKind: "organ_action", EnactedRequest: fmt.Sprintf(`{"organ_id":"browser","operation":"browser_navigate","input":"%d"}`, index),
			ObservedAt: observed, Significance: "reusable", MethodUpdate: fmt.Sprintf("method revision %d", index),
		})
		result, _ := json.Marshal(ActionState{
			ID: fmt.Sprintf("contact-action-%d", index), CommitmentID: commitmentID, Kind: "organ_action",
			OrganID: "browser", Operation: "browser_navigate", Effect: "oriented", Status: "completed",
		})
		runtime.state.Background = append(runtime.state.Background, Event{Seq: uint64(index + 1), Kind: "action_result", Payload: result})
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time: observed, Status: "completed", CostConfirmed: true,
			ActualMicrousd: runtime.config.CognitiveResource.RollingHourLimitMicrousd / 20,
		})
	}
	// Ordinary learning may have just changed Self. It is not evidence that the
	// repeated operating pattern itself has changed.
	runtime.state.Self.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := runtime.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 5 || runtime.state.Background[4].Kind != "self_model_difference" {
		t.Fatalf("a self update hid continuing contact-only behavior: %#v", runtime.state.Background)
	}
}

func TestStageTenOneContinuousConcernDoesNotManufactureOperationalDifference(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-10 * time.Minute)
	runtime.state.T0 = baseline.Add(-time.Minute).Format(time.RFC3339Nano)
	runtime.state.Self.UpdatedAt = baseline.Format(time.RFC3339Nano)
	for index := 0; index < 6; index++ {
		commitmentID := fmt.Sprintf("continuous-%d", index)
		observed := baseline.Add(time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		runtime.state.Commitments = append(runtime.state.Commitments, ActionCommitment{
			ID: commitmentID, ConcernID: "one-project", ActionKind: "organ_action", Status: "assimilated",
		})
		runtime.state.Memories = append(runtime.state.Memories, Memory{
			ID: fmt.Sprintf("continuous-memory-%d", index), CommitmentID: commitmentID, ActionKind: "organ_action",
			EnactedRequest: "go test ./...", ObservedAt: observed, Significance: "ordinary",
		})
		result, _ := json.Marshal(ActionState{
			ID: fmt.Sprintf("continuous-action-%d", index), CommitmentID: commitmentID, Kind: "organ_action",
			OrganID: "system", Operation: "exec", Effect: "changed", Status: "completed",
		})
		runtime.state.Background = append(runtime.state.Background, Event{
			ID: fmt.Sprintf("continuous-reality-%d", index), Seq: uint64(index + 1), Kind: "action_result", Payload: result,
		})
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time: observed, Status: "completed", CostConfirmed: true,
			ActualMicrousd: runtime.config.CognitiveResource.RollingHourLimitMicrousd / 20,
		})
	}
	if err := runtime.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	for _, event := range runtime.state.Background {
		if event.Kind == "self_model_difference" {
			t.Fatalf("one continuous causal project was mistaken for fragmented churn: %#v", runtime.state.Background)
		}
	}
}

func TestStageTenThoughtWithoutBodyActionDoesNotManufactureSelfDifference(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().UTC().Add(-time.Minute)
	runtime.state.T0 = baseline.Format(time.RFC3339Nano)
	for index := 0; index < 3; index++ {
		runtime.state.Usage = append(runtime.state.Usage, UsageRecord{
			Time:   baseline.Add(time.Duration(index+1) * time.Second).Format(time.RFC3339Nano),
			Status: "completed", CostConfirmed: true,
			ActualMicrousd: runtime.config.CognitiveResource.RollingHourLimitMicrousd / 100,
		})
	}
	if err := runtime.maybeOpenAffectiveSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 0 {
		t.Fatalf("coherent thought without a body action was mislabeled as pathology: %#v", runtime.state.Background)
	}
}

func TestStageTenLaterSelfEvidenceReappraisesExistingSelfConcern(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Concerns = []Concern{{
		ID: "life-rhythm", OriginKind: "self_model_difference", Resolution: "hold",
		Meaning: "我正在理解怎样形成可持续的生活节奏。", Ownership: 0.9,
		ClosureCondition: "当现实结果支持一种可持续生活节奏时闭合。",
	}}
	candidate := Event{
		ID: "later-operation", Kind: "self_model_difference", ConcernID: "life-rhythm",
		Summary: "近期认知投入没有转化为新的身体后果。", Status: "in_focus",
	}
	runtime.state.Background = []Event{candidate}
	runtime.activeCandidates = map[string]Event{candidate.ID: candidate}
	commit := CognitiveCommit{
		FocusID: candidate.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: candidate.ID, Meaning: "这项新事实使我重新面对同一个生活节奏问题。",
			Difference: 0.5, Ownership: 0.9, Value: 0.6, Urgency: 0.3, Answerability: 0.1,
			Certainty: 0.98, Resolution: "hold",
		}},
		ThoughtThread: "新的运行事实进入原有自我问题，但还没有足够现实决定怎样改变。",
		Action:        CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "keep", Model: "current", ReasoningEffort: "current",
		},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].ID != "life-rhythm" {
		t.Fatalf("reappraisal split one self question into multiple concerns: %#v", runtime.state.Concerns)
	}
	if runtime.state.Concerns[0].Meaning != commit.Appraisals[0].Meaning {
		t.Fatalf("later operational evidence did not update the existing self concern: %#v", runtime.state.Concerns[0])
	}
}

func TestStageTenSustainedHighActivationAndLowControlEnterSelfAttention(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.AffectiveState = AffectiveState{Activation: 0.9, Control: 0.2, Certainty: 0.8}
	runtime.state.LastAttentionAt = time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	if err := runtime.maybeOpenAffectiveSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.state.Background) != 1 || runtime.state.Background[0].Kind != "self_model_difference" {
		t.Fatalf("sustained high activation and low control stayed outside self attention: %#v", runtime.state.Background)
	}
	var payload struct {
		DifferenceKind string `json:"difference_kind"`
	}
	if err := json.Unmarshal(runtime.state.Background[0].Payload, &payload); err != nil || payload.DifferenceKind != "affective_control_balance" {
		t.Fatalf("affective regulation fact lost its identity: payload=%q err=%v", runtime.state.Background[0].Payload, err)
	}
}

func TestConcernClosureUsesBaseDifferenceInsteadOfSubdecimalSalienceProduct(t *testing.T) {
	concern := Concern{ID: "nearly-closed", Resolution: "hold"}
	appraisal := CandidateAppraisal{CandidateID: concern.ID, Difference: 0.08, Resolution: "resolved"}
	if err := validateExistingConcernDisposition(appraisal, concern, false); err != nil {
		t.Fatalf("a semantically closed small difference was rejected by numerical ceremony: %v", err)
	}
}

func TestHeldExplorationConcernKeepsDeliberateNonActionAvailable(t *testing.T) {
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	runtime, err := New(root, "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	if runtime.state.ValueField.Activation.Exploration < 0.8 {
		t.Fatalf("internal non-action falsely relieved exploration pressure: %f", runtime.state.ValueField.Activation.Exploration)
	}
	commit.Appraisals[0].Ownership = 0.9
	commit.Action = CognitiveAction{
		Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "date -Is", Intent: "用一次现实接触取得当前时间事实",
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
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "date -Is", Intent: "观察时间",
			Prediction: "身体会返回时间", RealityCheck: "依据输出判断",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	normalized, withheld := normalizeUnendorsedAction(commit, runtime.config.Dynamics.AttentionThreshold)
	if withheld != "organ_action" || normalized.Action.Kind != "none" {
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
		Action: CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "inspect-ad"},
		ResourceChoice: CognitiveResourceChoice{
			Apply: "next", Model: "sol", ReasoningEffort: "medium", Purpose: "absorb the action result",
		},
	}
	normalized, withheld := normalizeUnendorsedAction(commit, 0.45)
	if withheld != "organ_action" || normalized.Action.Kind != "none" || normalized.ResourceChoice.Apply != "keep" {
		t.Fatalf("withheld action retained an imaginary next step: %#v withheld=%q", normalized, withheld)
	}
}

func TestReframedExplorationConcernMakesRoomForANewDrive(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
		ID: "commitment-once", ConcernID: "exploration", ActionKind: "organ_action", Status: "reality_available",
	}
	runtime.state.Commitments = []ActionCommitment{commitment}
	runtime.state.Concerns = []Concern{{ID: "exploration", OriginKind: "endogenous_change", CommitmentID: commitment.ID}}
	payload, _ := json.Marshal(ActionState{ID: "action-once", CommitmentID: commitment.ID, Kind: "organ_action", Status: "completed"})
	reality := Event{ID: "reality-once", Kind: "action_result", Payload: payload, Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{reality.ID: reality}
	commit := CognitiveCommit{
		FocusID: reality.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: reality.ID, Meaning: "现实已经到达", Difference: 0.1, Resolution: "resolved",
		}},
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, Meaning: "我吸收了这次现实。", Significance: "ordinary",
		}},
	}
	if err := runtime.applyRealityUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if runtime.state.TotalMemories != 1 || runtime.state.Commitments[0].Status != "assimilated" || runtime.state.Commitments[0].MemoryID == "" {
		t.Fatalf("reality did not close exactly once: %#v", runtime.state.Commitments[0])
	}
	if runtime.state.Concerns[0].CommitmentID != "" {
		t.Fatalf("concern retained an assimilated commitment: %#v", runtime.state.Concerns[0])
	}
	if err := runtime.validateRealityUpdates(commit); err == nil {
		t.Fatal("the same commitment reality was accepted for a second memory")
	}
}

func TestUnassimilatedRealityOwnsAttentionDuringValidationRetry(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(5), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 1
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
		ID: "commitment-old", ConcernID: current.ConcernID, ActionKind: "organ_action", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-old", CommitmentID: "commitment-old", Kind: "organ_action",
		OrganID: "system", Operation: "exec", Request: "hominal-browser call browser_snapshot '{}'", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "reality-old", Seq: 10, Kind: "action_result", Payload: result, Status: "processed"}}
	runtime.state.Memories = []Memory{{CommitmentID: "commitment-old", RemainingDifference: 0.08}}
	action := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "hominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("unchanged equivalent action was accepted after settled reality")
	}
	wrapped := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "set -euo pipefail\nhominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(current.ID, wrapped); err == nil {
		t.Fatal("an inert shell policy wrapper disguised an unchanged embodied action")
	}

	duplicate, _ := json.Marshal(ActionState{
		ID: "action-duplicate", CommitmentID: "commitment-duplicate", Kind: "organ_action",
		OrganID: "system", Operation: "exec", Request: action.Input, Status: "completed",
	})
	runtime.state.Background = append(runtime.state.Background, Event{ID: "reality-duplicate", Seq: 11, Kind: "action_result", Payload: duplicate})
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("an equivalent action result incorrectly counted as changed reality")
	}
	runtime.state.Background = append(runtime.state.Background, Event{ID: "fresh-drive", Seq: 12, Kind: "value_signal", Source: "endogenous"})
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("an internal value signal incorrectly reset settled embodied reality")
	}

	other := Event{ID: "exploration-later", Kind: "concern", ConcernID: "concern-later"}
	runtime.activeCandidates = map[string]Event{other.ID: other}
	if err := runtime.validateActionProgress(other.ID, action); err == nil {
		t.Fatal("a new concern identity reset an already settled embodied request")
	}
	decorated := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "printf '%s\\n' '--- current browser page snapshot ---'\nhominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(other.ID, decorated); err == nil {
		t.Fatal("a static observation label disguised a settled request under a new concern")
	}

	runtime.state.Background = append(runtime.state.Background, Event{ID: "mentor-new", Seq: 13, Kind: "mentor_received"})
	runtime.activeCandidates = map[string]Event{current.ID: current}
	if err := runtime.validateActionProgress(current.ID, action); err != nil {
		t.Fatalf("new world fact did not reopen the action: %v", err)
	}

	runtime.activeCandidates = map[string]Event{other.ID: other}
	if err := runtime.validateActionProgress(other.ID, action); err != nil {
		t.Fatalf("a material world change was erased by a new concern identity: %v", err)
	}
	changed := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "date -u; hominal-browser call browser_snapshot '{}'"}
	if err := runtime.validateActionProgress(other.ID, changed); err != nil {
		t.Fatalf("a genuinely changed request was blocked: %v", err)
	}
}

func TestSettledActionBoundaryReturnsAsFactWithoutValidationLoop(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.config.GenerationKind = "rehearsal" // Acceptance only; do not start the next main turn.
	old := ActionCommitment{
		ID: "commitment-old", ActionKind: "mentor_send", Status: "assimilated",
	}
	oldReality, _ := json.Marshal(ActionState{
		ID: "action-old", CommitmentID: old.ID, Kind: "mentor_send", Request: "相同表达", Status: "completed", Effect: "changed",
	})
	valuePayload, _ := json.Marshal(lifeValueSignalPayload{
		Direction: "relatedness", AffordanceKey: "mentor_channel", Surface: "导师通道",
	})
	focus := Event{
		ID: "value-now", Seq: 12, Kind: "value_signal", Source: "endogenous", Payload: valuePayload, Status: "in_focus",
	}
	runtime.state.Commitments = []ActionCommitment{old}
	runtime.state.Memories = []Memory{{CommitmentID: old.ID, RemainingDifference: 0}}
	runtime.state.Background = []Event{
		{ID: "reality-old", Seq: 10, Kind: "action_result", Payload: oldReality, Status: "processed"},
		focus,
	}
	runtime.state.ValueAffordances["mentor_channel"] = ValueAffordanceTrace{LastPresentedAt: nowUTC()}
	runtime.state.Lease = &Lease{ID: "lease-now", FocusID: focus.ID, Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}}
	runtime.activeCandidates = map[string]Event{focus.ID: focus}
	commit := CognitiveCommit{
		FocusID: focus.ID,
		Appraisals: []CandidateAppraisal{{
			CandidateID: focus.ID, Meaning: "我愿意表达", Difference: 0.6, Ownership: 0.8, Value: 0.8,
			Values: LifeValueVector{Relatedness: 0.8}, Urgency: 0.5, Answerability: 0.9, Certainty: 0.9, Resolution: "hold",
		}},
		NewConcernClosureCondition: "表达产生了不同的现实结果",
		ThoughtThread:              "我准备再次发送相同表达。",
		Action: CognitiveAction{
			Kind: "mentor_send", Text: "相同表达", Intent: "进入导师关系",
			Prediction: "消息送达", RealityCheck: "导师通道返回送达事实", StopCondition: "送达后停止",
		},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{
		LeaseID: "lease-now", FocusID: focus.ID, Stage4: &commit,
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Lease != nil || len(runtime.state.Commitments) != 1 {
		t.Fatalf("settled request formed another commitment: lease=%#v commitments=%#v", runtime.state.Lease, runtime.state.Commitments)
	}
	if runtime.state.Background[1].Status != "processed" || runtime.state.Background[1].CognitionAttempts != 0 {
		t.Fatalf("originating focus entered validation retry: %#v", runtime.state.Background[1])
	}
	boundary := runtime.state.Background[len(runtime.state.Background)-1]
	if boundary.Kind != "action_boundary" || boundary.Status != "pending" {
		t.Fatalf("action boundary did not return as an attention fact: %#v", boundary)
	}
	trace := runtime.state.ValueAffordances["mentor_channel"]
	if trace.LastSettledAt == "" || trace.DismissedStreak != 1 {
		t.Fatalf("yieldless affordance was immediately left eligible: %#v", trace)
	}
}

func TestOnlyVerifiedChangeBreaksContactOnlySequence(t *testing.T) {
	for _, effect := range []string{"", "unknown", "observed", "oriented"} {
		if !actionEffectIsContactOnly(effect) {
			t.Fatalf("unverified effect %q was treated as a persistent causal change", effect)
		}
	}
	if actionEffectIsContactOnly("changed") {
		t.Fatal("organ-verified changed effect was treated as contact-only")
	}
}

func TestReadOnlyOrganResultDoesNotReopenSettledObservation(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	current := Event{ID: "current", Kind: "concern", ConcernID: "current-concern"}
	runtime.activeCandidates = map[string]Event{current.ID: current}
	runtime.state.Commitments = []ActionCommitment{
		{ID: "snapshot-old", ConcernID: "old-concern", ActionKind: "organ_action", Status: "assimilated"},
		{ID: "find-new", ConcernID: "other-concern", ActionKind: "organ_action", Status: "assimilated"},
	}
	snapshot, _ := json.Marshal(ActionState{
		ID: "snapshot-action", CommitmentID: "snapshot-old", Kind: "organ_action",
		OrganID: "browser", Operation: "browser_snapshot", Effect: "observed", Request: `{}`, Status: "completed",
	})
	find, _ := json.Marshal(ActionState{
		ID: "find-action", CommitmentID: "find-new", Kind: "organ_action",
		OrganID: "browser", Operation: "browser_find", Effect: "observed", Request: `{"query":"CALY"}`, Status: "completed",
	})
	runtime.state.Background = []Event{
		{ID: "snapshot-reality", Seq: 10, Kind: "action_result", Payload: snapshot},
		{ID: "find-reality", Seq: 20, Kind: "action_result", Payload: find},
	}
	runtime.state.Memories = []Memory{{CommitmentID: "snapshot-old", RemainingDifference: 0.05}}
	action := CognitiveAction{Kind: "organ_action", OrganID: "browser", Operation: "browser_snapshot", Input: `{}`}
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("a different read-only observation incorrectly made the settled snapshot new")
	}

	runtime.state.Background[1].Payload, _ = json.Marshal(ActionState{
		ID: "navigate-action", CommitmentID: "find-new", Kind: "organ_action",
		OrganID: "browser", Operation: "browser_navigate", Effect: "changed", Request: `{"url":"https://example.com"}`, Status: "completed",
	})
	if err := runtime.validateActionProgress(current.ID, action); err != nil {
		t.Fatalf("a page-changing organ result did not reopen the observation: %v", err)
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
		ID: "snapshot-old", ConcernID: oldConcern, ActionKind: "organ_action", Status: "assimilated",
	}}
	oldResult, _ := json.Marshal(ActionState{
		ID: "snapshot-action", CommitmentID: "snapshot-old", Kind: "organ_action",
		OrganID: "system", Operation: "exec", Request: "hominal-browser call browser_snapshot '{}'", Effect: "observed", Status: "completed",
	})
	clickResult, _ := json.Marshal(ActionState{
		ID: "click-action", CommitmentID: "click-later", Kind: "organ_action",
		OrganID: "system", Operation: "exec", Request: `hominal-browser call browser_click '{"target":"e75"}'`, Effect: "changed", Status: "completed",
	})
	runtime.state.Background = []Event{
		{ID: "snapshot-reality", Seq: 10, Kind: "action_result", Payload: oldResult, Status: "processed"},
		{ID: "click-reality", Seq: 20, Kind: "action_result", Payload: clickResult, Status: "processed"},
	}
	runtime.state.Memories = []Memory{{CommitmentID: "snapshot-old", RemainingDifference: 0.1}}
	action := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "hominal-browser call browser_snapshot '{}'"}
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
		if got := normalizeEnactedRequest("system_exec", wrapped); got != plain {
			t.Fatalf("normalized request = %q, want %q", got, plain)
		}
	}
	command := "set -- alice\nprintf '%s\\n' \"$1\""
	if got := normalizeEnactedRequest("system_exec", command); got != command {
		t.Fatalf("a substantive set command was removed: %q", got)
	}
	dynamic := "printf 'observed_at=%s\\n' \"$(date -Is)\"\nhominal-browser list"
	if got := normalizeEnactedRequest("system_exec", dynamic); got != dynamic {
		t.Fatalf("a dynamic observation was removed: %q", got)
	}
	write := "printf '%s\\n' alice > /life/name\nhominal-browser list"
	if got := normalizeEnactedRequest("system_exec", write); got != write {
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
		ID: "commitment-old", ConcernID: current.ConcernID, ActionKind: "organ_action", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-old", CommitmentID: "commitment-old", Kind: "organ_action", OrganID: "system", Operation: "exec", Request: "read-object", Status: "completed",
	})
	runtime.state.Background = []Event{{ID: "reality-old", Seq: 10, Kind: "action_result", Payload: result}}
	runtime.state.Memories = []Memory{{CommitmentID: "commitment-old", RemainingDifference: 0.55}}
	if err := runtime.validateActionProgress(current.ID, CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "read-object"}); err != nil {
		t.Fatalf("unsettled reality could not be revisited: %v", err)
	}
	runtime.state.Memories[0].RemainingDifference = 0.05
	if err := runtime.validateActionProgress(current.ID, CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "inspect-object-differently"}); err != nil {
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
		ID: "commitment-failed", ConcernID: "concern-old", ActionKind: "organ_action", Status: "assimilated",
	}}
	result, _ := json.Marshal(ActionState{
		ID: "action-failed", CommitmentID: "commitment-failed", Kind: "organ_action",
		OrganID: "system", Operation: "exec", Request: "hominal-browser call browser_tabs '{}'", Status: "failed",
		Result: `{"exit_code":1,"timed_out":false}`,
	})
	runtime.state.Background = []Event{{ID: "reality-failed", Seq: 10, Kind: "action_result", Payload: result, Status: "processed"}}
	runtime.state.Memories = []Memory{{CommitmentID: "commitment-failed", RemainingDifference: 0.7}}
	action := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "hominal-browser call browser_tabs '{}'"}
	if err := runtime.validateActionProgress(current.ID, action); err == nil {
		t.Fatal("an exact deterministic failure was accepted under a newly named concern")
	}
	changed := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: `hominal-browser call browser_tabs '{"action":"list"}'`}
	if err := runtime.validateActionProgress(current.ID, changed); err != nil {
		t.Fatalf("a genuinely corrected request was rejected: %v", err)
	}
}

func TestRecentMentorDeduplicationUsesExactFactNotLexicalJudgment(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtime.state.Mentor.Outbox = []MentorMessage{{
		MessageID: "alice-earlier", Status: "delivered", QueuedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
		Body: "我在 /life 留下文字痕迹并独立读取确认，也看到 X 页面可达，但还没有对我的回应。",
	}}
	paraphrase := CognitiveAction{
		Kind: "mentor_send", ReplyTo: "",
		Text: "第一份 /life 文字痕迹已经通过独立读取核验；X 入口虽然可达，目前仍没有给我的回应。",
	}
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err != nil {
		t.Fatal("lexical resemblance was incorrectly treated as an identical action")
	}
	paraphrase.Text = runtime.state.Mentor.Outbox[0].Body
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err == nil {
		t.Fatal("exact retransmission was not detected")
	}
	paraphrase.Text += "\n请和我讨论下一步的问题。"
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err != nil {
		t.Fatal("a new question was swallowed by duplicate protection")
	}
	paraphrase.ReplyTo = "codex-new-reality"
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err != nil {
		t.Fatalf("a direct reply to new relationship reality was blocked: %v", err)
	}
	paraphrase.ReplyTo = ""
	paraphrase.Text = "我刚发现浏览器返回了一个此前不存在的新错误，需要和你核对这个身体事实。"
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err != nil {
		t.Fatalf("a materially different unsolicited message was blocked: %v", err)
	}
	runtime.state.Mentor.Outbox[0].QueuedAt = now.Add(-31 * time.Minute).Format(time.RFC3339Nano)
	paraphrase.Text = "第一份 /life 文字痕迹已经通过独立读取核验；X 入口虽然可达，目前仍没有给我的回应。"
	if err := runtime.validateMentorCausalNovelty(paraphrase, now); err != nil {
		t.Fatalf("bounded causal memory became a permanent topic prohibition: %v", err)
	}
}

func TestWaitingConcernNoLongerCarriesExplorationActionRequirement(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: command, Intent: "等待",
			Prediction: "保持现状", RealityCheck: "命令结束",
		}
		if err := validateCognitiveAction(action, 8); err == nil {
			t.Fatalf("inert organ_action action passed the global Reality boundary: %q", command)
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
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: command, Intent: "取得或改变现实事实",
			Prediction: "返回可核验结果", RealityCheck: "检查输出或改变",
		}
		if err := validateCognitiveAction(action, 8); err != nil {
			t.Fatalf("substantive organ_action action was rejected: %q: %v", command, err)
		}
	}
	if err := validateCognitiveAction(CognitiveAction{Kind: "none"}, 8); err != nil {
		t.Fatalf("deliberate non-action was rejected: %v", err)
	}
}

func TestIntentionalActionUsesTheOrgansPublishedOperationCatalog(t *testing.T) {
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	runtime, err := New(root, "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	valid := CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "pwd"}
	if err := runtime.validateOrganOperation(valid); err != nil {
		t.Fatalf("published operation was rejected: %v", err)
	}
	invalid := valid
	invalid.Operation = "observe"
	if err := runtime.validateOrganOperation(invalid); err == nil || !strings.Contains(err.Error(), "choose one of: exec") {
		t.Fatalf("a passive capability was not rejected with the factual action catalog: %v", err)
	}
}

func TestRealityContactRelievesExplorationWhileNewConcernCanRemain(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
		RealityUpdates: []RealityUpdate{{
			CommitmentID: commitment.ID, PredictionDifference: 0.16,
			Meaning:         "消息送达形成了真实联结事实，回应仍需要等待。",
			Values:          LifeValues{Relatedness: 0.8, SelfEndorsed: 0.8},
			ExperiencedCost: 0.01, Lesson: "送达与回应是两个不同现实。", Significance: "ordinary", MethodSlot: -1,
		}},
	}
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	want := 0.8 - runtime.config.Dynamics.ExplorationRelief*((1-0.16)*0.98) +
		runtime.config.Dynamics.ExplorationUnknownGrowth*(1-0.98)
	if absFloat(runtime.state.ValueField.Activation.Exploration-want) > 0.000001 {
		t.Fatalf("reality contact did not relieve exploration independently of the new concern: got %f want %f", runtime.state.ValueField.Activation.Exploration, want)
	}
	if len(runtime.state.Concerns) != 1 || runtime.state.Concerns[0].Resolution != "hold" {
		t.Fatalf("relieving exploration erased the newly transformed concern: %#v", runtime.state.Concerns)
	}
	if runtime.currentExplorationConcernID() != "" {
		t.Fatal("the relationship wait still carried the general exploration drive")
	}
}

func TestSituatedConcernMayShareAfterEarlierMentorContact(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.9
	concern := Concern{ID: "concrete-memory", OriginKind: "action_result", Resolution: "hold", Answerability: 0.8}
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

func TestMatureExplorationKeepsMentorExpressionAvailableAfterEarlierContact(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.9
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
	if err := runtime.applyCognitiveCommit(commit); err != nil {
		t.Fatalf("the shared value dynamics blocked a self-endorsed relationship expression: %v", err)
	}
}

func TestExplorationRequirementRejectsShellNoOp(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(8), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.ValueField.Activation.Exploration = 0.8
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
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: ":", Intent: "等待", Prediction: "没有变化", RealityCheck: "没有输出",
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
	runtime.state.ValueField.Activation.Exploration = 0.8
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
	runtime.state.ValueField.Activation.Exploration = 0.4
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
	runtime.state.ValueField.Activation.Exploration = 0.8
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

func TestStageTenAssistanceCannotSubmitCognitiveCommit(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Stage = 10
	runtime.state.Lease = &Lease{
		ID: "lease-assist", FocusID: "assist-focus", ProfileSource: "next",
		Profile:        CognitiveProfile{Model: "sol", ReasoningEffort: "low"},
		ProfilePurpose: "在 X 发布主力认知已经写定的短句，并取得新的状态 URL",
	}
	runtime.state.Concerns = []Concern{{
		ID: "public-expression", Subject: "形成一次公开表达", Meaning: "我已经决定公开表达一条写定内容",
		Difference: 0.6, Ownership: 0.9, Value: 0.7, Values: LifeValueVector{Contribution: 0.8},
		Urgency: 0.4, Answerability: 0.8, Certainty: 0.95, Resolution: "hold",
	}}
	focus := Event{ID: "assist-focus", Kind: "cognition_continuation", ConcernID: "public-expression", Status: "in_focus"}
	runtime.state.Background = []Event{focus}
	runtime.activeCandidates = map[string]Event{focus.ID: focus}
	commit := CognitiveCommit{
		FocusID:       focus.ID,
		Appraisals:    []CandidateAppraisal{{CandidateID: focus.ID, Meaning: "协助模型改写了意义", Difference: 0.1, Ownership: 0.2, Resolution: "released"}},
		ThoughtThread: "改成给导师发消息",
		Action: CognitiveAction{
			Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "hominal-browser list", Intent: "另一个目标",
			Prediction: "返回工具列表", RealityCheck: "检查退出码",
		},
		NarrativeUpdate:        "改写自我",
		MemoryUpdates:          []MemoryUpdate{{Content: "协助器改写了个人经历"}},
		ExperienceUpdates:      []ExperienceUpdate{{Judgment: "协助器决定了新的生活判断"}},
		RecallQuery:            "开始另一个个人主题",
		ValueOrientationUpdate: LifeValueVector{Relatedness: 1},
		ResourceChoice:         CognitiveResourceChoice{Apply: "next", Model: "sol", ReasoningEffort: "low", Purpose: "继续协助"},
	}
	if err := runtime.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "lease-assist", FocusID: focus.ID, Stage4: &commit}); err != nil {
		t.Fatal(err)
	}
	if runtime.state.Self.Narrative != "" || len(runtime.state.Memories) != 0 || runtime.state.PendingAction != nil || runtime.state.Concerns[0].Meaning != "我已经决定公开表达一条写定内容" {
		t.Fatal("local assistance mutated personal cognition or started an action")
	}
}

func TestStageTenActionAssistanceCannotLeakPastAnAlreadyFormedAction(t *testing.T) {
	runtime, err := New(t.TempDir(), "instance", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	runtime.state.Stage = 10
	runtime.state.Lease = &Lease{
		ID: "lease-main", FocusID: "failed-reality",
		Profile: CognitiveProfile{Model: "luna", ReasoningEffort: "none"},
	}
	runtime.state.Concerns = []Concern{{ID: "owned", Resolution: "hold"}}
	focus := Event{ID: "failed-reality", Kind: "action_result", ConcernID: "owned", Status: "in_focus"}
	runtime.activeCandidates = map[string]Event{focus.ID: focus}
	choice := CognitiveResourceChoice{
		Apply: "next", Model: "sol", ReasoningEffort: "low",
		Purpose: "用已经固定的命令完成一次身体实现",
	}
	if _, err := runtime.validateResourceChoice(choice, focus.ID, "mentor_send", "owned"); err == nil || !strings.Contains(err.Error(), "action none") {
		t.Fatalf("action assistance leaked into the Reality after an already formed action: %v", err)
	}
	if _, err := runtime.validateResourceChoice(choice, focus.ID, "none", "owned"); err != nil {
		t.Fatalf("a proper serial action-assistance request was rejected: %v", err)
	}
}
