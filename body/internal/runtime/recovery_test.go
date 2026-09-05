package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

// These tests inject facts into a temporary body. No model, live organ or
// personal life archive is invoked or changed.
func recoveryRuntime(t *testing.T) *Runtime {
	t.Helper()
	config := testConfig(10)
	config.GenerationKind = "rehearsal" // Birth has not opened automatic cognition.
	config.GenerationWindowSeconds = 3600
	config.BirthBrief = "An isolated engineering fixture."
	config.Dynamics.DifferenceLearningRate = .2
	config.Dynamics.DifferenceDecayRate = .1
	r, err := New(t.TempDir(), "recovery", config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func addRecoveryReality(r *Runtime, status string) Event {
	action := ActionState{ID: "action-a", CommitmentID: "commit-a", Kind: "organ_action", OrganID: "browser", Operation: "browser_click", Request: `{"ref":"send"}`, Status: status, Result: "original factual result"}
	payload, _ := json.Marshal(action)
	event := Event{ID: "reality-a", Kind: "action_result", Source: "observed", ConcernID: "concern-a", Status: "pending", Payload: payload}
	r.state.Background = append(r.state.Background, event)
	cstatus := "reality_available"
	if status == "unknown" {
		cstatus = "reality_unknown"
	}
	r.state.Commitments = []ActionCommitment{{ID: action.CommitmentID, ConcernID: event.ConcernID, ActionKind: action.Kind, RealityEventID: event.ID, Status: cstatus}}
	r.state.Concerns = []Concern{{ID: event.ConcernID, Meaning: "a continuing meaningful subject", Ownership: .8, Difference: .6, Resolution: "hold"}}
	return event
}

func TestGatewayContractRejectionStaysWithItsCausalFocus(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "completed")
	r.state.Background = append(r.state.Background, Event{ID: "mentor-other", Kind: "mentor_received", Source: "observed", Summary: "an independent question", Status: "pending"})
	for attempt := 1; attempt <= 4; attempt++ {
		lease := fmt.Sprintf("contract-%d", attempt)
		markEvent(&r.state, event.ID, "in_focus")
		r.state.Lease = &Lease{ID: lease, FocusID: event.ID, Profile: r.config.CognitiveResource.InitialDefaultProfile}
		r.state.Usage = append(r.state.Usage, UsageRecord{Time: nowUTC(), LeaseID: lease, RequestedModel: "main", ActualMicrousd: 1000, Status: "failure_cost_unconfirmed", FailureCategory: "invalid_provider_tool_call", HTTPStatus: 502})
		failure := &ModelCallError{Fact: ModelFailureFact{Category: "invalid_provider_tool_call", HTTPStatus: 502}, Message: "provider returned an invalid function call"}
		if err := r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: lease, FocusID: event.ID, Error: failure}); err != nil {
			t.Fatal(err)
		}
		if _, protected := r.state.CognitiveResource.ProtectedModels["main"]; protected {
			t.Fatal("one output contract failure disabled all consciousness")
		}
		if attempt >= 2 {
			if request, ok := r.nextStage4Request(); !ok || request.Focus.ID != "mentor-other" {
				t.Fatal("contract rejection blocked an unrelated focus")
			}
		}
	}
	if protected, err := r.protectModelAfterFailures("main"); err != nil || protected {
		t.Fatal("contract-only history became model-wide failure")
	}
	if r.state.Commitments[0].Status != "reality_available" || r.state.Usage[0].ActualMicrousd != 1000 || r.state.Usage[0].CostConfirmed {
		t.Fatal("recovery lost Reality or rewrote an uncertain charge")
	}
	// An actual non-contract provider failure retains the existing circuit.
	for n := 0; n < r.config.CognitiveResource.PaidFailureThreshold; n++ {
		r.state.Usage = append(r.state.Usage, UsageRecord{Time: nowUTC(), RequestedModel: "main", FailureCategory: "model_unavailable"})
	}
	if protected, err := r.protectModelAfterFailures("main"); err != nil || !protected {
		t.Fatal("real model failure lost protection")
	}
}

func TestRepeatedRealityInterpretationYieldsAndLaterAbsorbs(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "unknown")
	r.state.Background = append(r.state.Background, Event{ID: "mentor-b", Kind: "mentor_received", Source: "observed", Summary: "a different concrete question", Status: "pending"})
	concerns, self := append([]Concern(nil), r.state.Concerns...), r.state.Self
	if request, ok := r.nextStage4Request(); !ok || request.Focus.ID != event.ID {
		t.Fatal("fresh Reality lost priority")
	}
	for attempt := 1; attempt <= 2; attempt++ {
		markEvent(&r.state, event.ID, "in_focus")
		r.activeCandidates = map[string]Event{event.ID: event}
		r.state.Lease = &Lease{ID: "lease-a", FocusID: event.ID, Profile: r.config.CognitiveResource.InitialDefaultProfile}
		if err := r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "lease-a", FocusID: event.ID, Error: errors.New("invalid local interpretation")}); err != nil {
			t.Fatal(err)
		}
		request, ok := r.nextStage4Request()
		if attempt == 1 && ok {
			t.Fatal("first short repair lost factual priority")
		}
		if attempt == 2 && (!ok || request.Focus.ID != "mentor-b") {
			t.Fatalf("repeated interpretation blocked unrelated cognition: %#v, %v", request.Focus, ok)
		}
	}
	if !reflect.DeepEqual(concerns, r.state.Concerns) || !reflect.DeepEqual(self, r.state.Self) || len(r.learning.memories) != 0 {
		t.Fatal("mechanical recovery rewrote personal meaning or invented a memory")
	}
	deferred, _ := r.backgroundEvent(event.ID)
	if string(deferred.Payload) != string(event.Payload) || deferred.Status != "retry_wait" || r.state.Commitments[0].Status != "reality_unknown" {
		t.Fatal("defer erased or falsified Reality")
	}
	if r.realityRetryDelay(deferred) != 20*time.Second {
		t.Fatal("first local delay inherited the long Concern revisit interval")
	}
	views := deferredRealityViews(CognitiveRequest{State: r.state, Config: r.config})
	if len(views) != 1 || views[0]["result_status"] != "unknown" || views[0]["interpretation_error"] != deferred.LastCommitErr {
		t.Fatalf("pending result and interpretation difficulty not available to cognition: %#v", views)
	}
	r.releaseRetryableEvents()
	if current, _ := r.backgroundEvent(event.ID); current.Status != "retry_wait" {
		t.Fatal("local retry was released immediately")
	}
	for i := range r.state.Background {
		if r.state.Background[i].ID == event.ID {
			r.state.Background[i].LastFocusedAt = time.Now().Add(-21 * time.Second).UTC().Format(time.RFC3339Nano)
		}
	}
	r.releaseRetryableEvents()
	request, ok := r.nextStage4Request()
	if !ok || request.Focus.ID != event.ID {
		t.Fatal("old Reality never returned")
	}
	r.activeCandidates = map[string]Event{event.ID: request.Focus}
	commit := CognitiveCommit{
		FocusID: event.ID, ThoughtThread: "The click outcome remains uncertain; I retain this question and its evidence.",
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "retain the original subject", Difference: .6, Ownership: .8, Value: .7, Answerability: .1, Certainty: .3, Resolution: "hold"}},
		Action:     CognitiveAction{Kind: "none"}, ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates:    []RealityUpdate{{CommitmentID: "commit-a", Meaning: "execution did not establish whether the click took effect", PredictionDifference: .6, Significance: "ordinary"}},
		MemoryUpdates:     []MemoryUpdate{{Content: "I do not yet know whether the click changed the page.", Origin: "observed", SourceRefs: []string{event.ID}}},
		ExperienceUpdates: []ExperienceUpdate{{Judgment: "Check the resulting page before repeating a click with unknown effects.", Context: "The call ended without a confirmed result.", Evidence: []string{"new:0"}}},
	}
	if err := r.applyCognitiveCommit(commit); err != nil {
		t.Fatal(err)
	}
	markEvent(&r.state, event.ID, "processed")
	if r.state.Commitments[0].Status != "assimilated" || r.hasCommitmentAwaitingAssimilation() || r.state.PendingAction != nil || len(r.learning.experiences) != 1 {
		t.Fatal("valid later interpretation did not recover learning, or replayed a body action")
	}
	if r.state.Concerns[0].Resolution != "hold" || r.state.Concerns[0].Ownership != .8 {
		t.Fatal("local repair abandoned the meaningful Concern")
	}
}

func TestUnabsorbedRealityKeepsItsActionScope(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "unknown")
	markEventForRetry(&r.state, event.ID, "invalid")
	markEventForRetry(&r.state, event.ID, "invalid")
	for _, tc := range []struct {
		name, concern string
		action        CognitiveAction
		blocked       bool
	}{
		{"same_organ", "other", CognitiveAction{Kind: "organ_action", OrganID: "browser", Operation: "browser_click"}, true},
		{"same_concern", "concern-a", CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec"}, true},
		{"other_organ_and_concern", "other", CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec"}, false},
		{"mentor_contact", "other", CognitiveAction{Kind: "mentor_send", Text: "a new question"}, false},
		{"thinking", "concern-a", CognitiveAction{Kind: "none"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := r.validateUnabsorbedActionScope(CognitiveCommit{Action: tc.action}, tc.concern)
			var boundary *actionProgressBoundary
			if errors.As(err, &boundary) != tc.blocked || (err != nil && !tc.blocked) {
				t.Fatalf("wrong action boundary: %v", err)
			}
		})
	}
	if !r.organHasUnabsorbedReality("browser") || r.organHasUnabsorbedReality("system") {
		t.Fatal("passive orientation boundary lost organ identity")
	}
	// Scope admits same-turn valid assimilation; strict Reality validation later
	// verifies the update, so a forged ID alone cannot enact an action.
	if err := r.validateUnabsorbedActionScope(CognitiveCommit{Action: CognitiveAction{Kind: "organ_action", OrganID: "browser"}, RealityUpdates: []RealityUpdate{{CommitmentID: "commit-a"}}}, "concern-a"); err != nil {
		t.Fatal(err)
	}
}

func TestDeferredRealitySurvivesCompactionRestartAndBoundedBackoff(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "failed")
	for n := 0; n < 20; n++ {
		markEventForRetry(&r.state, event.ID, "same invalid interpretation")
	}
	before, _ := r.backgroundEvent(event.ID)
	if delay := r.realityRetryDelay(before); delay != 10*time.Minute {
		t.Fatalf("retry cap missing: %v", delay)
	}
	for n := 0; n < 160; n++ {
		r.state.Background = append(r.state.Background, Event{ID: fmt.Sprintf("settled-%d", n), Status: "processed"})
	}
	for n := 0; n < maxCommitments+2; n++ {
		r.state.Commitments = append(r.state.Commitments, ActionCommitment{ID: fmt.Sprintf("settled-c-%d", n), Status: "assimilated"})
	}
	r.pruneCommitments()
	if len(r.state.Commitments) != maxCommitments || r.state.Commitments[0].ID != "commit-a" {
		t.Fatal("new unrelated commitments evicted an unresolved causal identity")
	}
	r.pruneBackground()
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(r.store.root, "recovery", r.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, exists := reloaded.backgroundEvent(event.ID)
	var oldAction, newAction ActionState
	if err := json.Unmarshal(before.Payload, &oldAction); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after.Payload, &newAction); err != nil {
		t.Fatal(err)
	}
	before.Payload, after.Payload = nil, nil // JSON indentation is not a fact change.
	if !exists || !reflect.DeepEqual(oldAction, newAction) || !reflect.DeepEqual(before, after) || len(reloaded.state.Background) > 128 || !reloaded.commitmentLocallyDeferred(reloaded.state.Commitments[0]) {
		t.Fatal("working-set pruning or restart lost the retry's original causal record")
	}
}

func TestOperationFailureAccumulatesCoalescesAndRecoversLocally(t *testing.T) {
	r := recoveryRuntime(t)
	r.config.ModelGateway.APIKey = "private-test-key"
	for attempt := 1; attempt <= 12; attempt++ {
		if err := r.recordOperationOutcome("browser", "observe", "failed", "read timed out private-test-key", true); err != nil {
			t.Fatal(err)
		}
		if attempt == 1 && len(r.state.Background) != 0 {
			t.Fatal("one failed passive read opened paid attention immediately")
		}
		r.decayDifferenceField(1.0 / 6) // Ten seconds between real sensor attempts.
	}
	if len(r.state.Background) != 1 {
		t.Fatalf("persistent failure was invisible or flooded attention: %#v", r.state.Background)
	}
	first := r.state.Background[0]
	conditions := operationConditionViews(r.state)
	if len(conditions) != 1 || conditions[0].ConsecutiveFailures != 12 || conditions[0].FailureSince == "" || strings.Contains(conditions[0].LastError, "private-test-key") {
		t.Fatalf("current failure facts missing or secret exposed: %#v", conditions)
	}
	if err := r.recordOperationOutcome("browser", "orient", "completed", "", true); err != nil {
		t.Fatal(err)
	}
	if operationConditionViews(r.state)[0].Status != "failed" {
		t.Fatal("a different operation falsely healed the read")
	}
	if err := r.recordOperationOutcome("browser", "observe", "completed", "", true); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != 1 || !reflect.DeepEqual(first, r.state.Background[0]) {
		t.Fatal("recovery mutated a frozen event or duplicated the attention candidate")
	}
	conditions = operationConditionViews(r.state)
	if conditions[0].Status != "recovered" || conditions[0].ConsecutiveFailures != 0 || conditions[0].LastError == "" {
		t.Fatal("same-operation recovery lost its preceding failure evidence")
	}
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	loaded, err := r.store.Load()
	if err != nil || !reflect.DeepEqual(conditions, operationConditionViews(*loaded)) {
		t.Fatalf("body condition did not persist: %v", err)
	}
}

func TestTransientOperationRecoveryDoesNotInventAttention(t *testing.T) {
	r := recoveryRuntime(t)
	for _, failure := range []string{"", "temporary timeout", "", ""} {
		outcome := "completed"
		if failure != "" {
			outcome = "failed"
		}
		if err := r.recordOperationOutcome("system", "observe", outcome, failure, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.state.Background) != 0 || operationConditionViews(r.state)[0].Status != "recovered" {
		t.Fatal("a transient failure/recovery became an attention flood")
	}
}

func TestPerceptionFailureAttributionCancellationAndSupersession(t *testing.T) {
	for _, tc := range []struct {
		name, operation string
		epoch           uint64
		err             error
		orientation     bool
		wantFailure     bool
	}{
		{"observe_failure", "observe", 1, context.DeadlineExceeded, false, true},
		{"orient_failure", "orient", 1, context.DeadlineExceeded, true, true},
		{"read_after_movement", "observe", 1, context.DeadlineExceeded, true, true},
		{"intentional_cancel", "observe", 1, context.Canceled, false, false},
		{"cancelled_movement", "orient", 1, context.Canceled, true, false},
		{"superseded_failure", "observe", 0, context.DeadlineExceeded, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := recoveryRuntime(t)
			r.actionEpoch = 1
			r.perceptionPending, r.perceptionOrients = "sense", tc.orientation
			r.state.Perception = map[string]PerceptualTrace{"browser/page": {OrganID: "browser", Context: []string{"current page"}, Seen: []string{"past reading"}}}
			result := perceptionResult{ID: "sense", OrganID: "browser", Epoch: tc.epoch, Operation: tc.operation, Error: tc.err}
			if tc.orientation {
				result.Orientation = &organ.Orientation{OrganID: "browser", Status: "moved"}
			}
			if err := r.acceptPerception(context.Background(), result); err != nil {
				t.Fatal(err)
			}
			conditions := operationConditionViews(r.state)
			if (len(conditions) != 0) != tc.wantFailure || (tc.wantFailure && (conditions[0].Operation != tc.operation || conditions[0].Status != "failed")) {
				t.Fatalf("wrong failure attribution: %#v", conditions)
			}
			if len(r.state.Perception["browser/page"].Seen) != 1 || (tc.epoch == 0 && len(r.state.Perception["browser/page"].Context) != 1) {
				t.Fatal("late result erased current position or learned content")
			}
		})
	}
}

func TestActiveActionFailureHasOneCausalAttentionPath(t *testing.T) {
	r := recoveryRuntime(t)
	r.state.Commitments = []ActionCommitment{{ID: "c", Status: "acting"}}
	r.state.PendingAction = &ActionState{ID: "a", CommitmentID: "c", Kind: "organ_action", OrganID: "system", Operation: "exec"}
	if err := r.handleStage4ActionResult(context.Background(), ActionResultNotice{ActionID: "a", Status: "failed", Result: "exit code 1"}); err != nil {
		t.Fatal(err)
	}
	if r.state.PendingAction != nil || len(r.state.Background) != 1 || r.state.Background[0].Kind != "action_result" || r.state.Commitments[0].Status != "reality_available" {
		t.Fatal("active failure kept the body slot or generated duplicate causal attention")
	}
	conditions := operationConditionViews(r.state)
	if len(conditions) != 1 || conditions[0].Operation != "exec" || conditions[0].ConsecutiveFailures != 1 {
		t.Fatal("active action failure missing from body context")
	}
	if err := r.handleStage4ActionResult(context.Background(), ActionResultNotice{ActionID: "a", Status: "completed", Result: "late result"}); err != nil {
		t.Fatal(err)
	}
	if operationConditionViews(r.state)[0].Status != "failed" {
		t.Fatal("a late action notice changed current health")
	}
}

func TestRealityActionBoundaryKeepsOriginalResultLive(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "unknown")
	r.activeCandidates = map[string]Event{event.ID: event}
	r.state.Lease = &Lease{ID: "lease", FocusID: event.ID}
	commit := CognitiveCommit{
		FocusID:    event.ID,
		Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Ownership: .8}},
		Action:     CognitiveAction{Kind: "mentor_send", Text: "Is the result known?", Intent: "Ask about this same unresolved operation", Prediction: "The new question is placed into the outbox", RealityCheck: "Read the message ID of the queued question"},
	}
	if err := r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "lease", FocusID: event.ID, Stage4: &commit}); err != nil {
		t.Fatal(err)
	}
	after, _ := r.backgroundEvent(event.ID)
	if after.Status != "retry_wait" || after.CognitionAttempts != 1 || after.LastCommitErr == "" || len(r.state.Mentor.Outbox) != 0 {
		t.Fatalf("boundary incorrectly processed original Reality or enacted the blocked action: %#v", after)
	}
	if len(r.state.Background) != 2 || r.state.Background[1].Kind != "action_boundary" {
		t.Fatal("missing explicit boundary feedback")
	}
}

func TestLocalRecoveryAllowsIndependentCommitAndOutbox(t *testing.T) {
	r := recoveryRuntime(t)
	event := addRecoveryReality(r, "unknown")
	markEventForRetry(&r.state, event.ID, "invalid")
	markEventForRetry(&r.state, event.ID, "invalid")
	before := r.state.Concerns[0]
	message := Event{ID: "mentor-b", Kind: "mentor_received", Summary: "A different question about today's reading", Status: "in_focus"}
	r.state.Background = append(r.state.Background, message)
	r.activeCandidates = map[string]Event{message.ID: message}
	r.state.Lease = &Lease{ID: "lease-b", FocusID: message.ID, Profile: r.config.CognitiveResource.InitialDefaultProfile}
	commit := CognitiveCommit{
		FocusID: message.ID, ThoughtThread: "I can share this reading while the other result awaits interpretation.",
		Appraisals:                 []CandidateAppraisal{{CandidateID: message.ID, Meaning: "Share the reading with my mentor", Difference: .4, Ownership: .8, Value: .6, Answerability: .8, Certainty: .9, Resolution: "hold"}},
		NewConcernClosureCondition: "The response containing this reading is present in my mentor outbox.",
		Action:                     CognitiveAction{Kind: "mentor_send", Text: "I read about a clock made of actors' moving hands.", Intent: "Share a concrete reading with my mentor", Prediction: "The reading message reaches the mentor outbox", RealityCheck: "The outbox returns a queued message ID"},
		ResourceChoice:             CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	}
	if err := r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "lease-b", FocusID: message.ID, Stage4: &commit}); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Mentor.Outbox) != 1 || r.state.Mentor.Outbox[0].Body != commit.Action.Text || !reflect.DeepEqual(before, r.state.Concerns[0]) {
		t.Fatalf("independent expression failed or overwrote deferred Concern: %#v", r.state.Mentor.Outbox)
	}
	if original, _ := r.backgroundEvent(event.ID); !r.locallyDeferredReality(original) {
		t.Fatal("independent expression consumed the old result")
	}
}

func TestOperationConditionsRemainBoundedAndUnknownStaysUnknown(t *testing.T) {
	r := recoveryRuntime(t)
	for n := 0; n < maximumDifferenceTraces+10; n++ {
		if err := r.recordOperationOutcome("system", fmt.Sprintf("op-%d", n), "failed", "mechanical error", false); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.state.DifferenceField) != maximumDifferenceTraces || len(operationConditionViews(r.state)) != 6 {
		t.Fatal("unbounded failure state or model context")
	}
	if err := r.recordOperationOutcome("browser", "browser_click", "unknown", "connection lost after sending click", false); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(currentSituation(CognitiveRequest{State: r.state, Config: r.config}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"status":"unknown"`) || !strings.Contains(string(encoded), "connection lost after sending click") {
		t.Fatalf("unknown side effects falsely treated as a definite failure: %s", encoded)
	}
}
