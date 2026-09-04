package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func assistanceTestRequest(profile CognitiveProfile, task string, includeSelf bool) CognitiveRequest {
	payload, _ := json.Marshal(assistanceContract{Purpose: "判断给定条件是否足够", Profile: profile, Task: task, IncludeSelf: includeSelf})
	return CognitiveRequest{
		Stage: 10, Config: testConfig(10), Profile: profile,
		Lease: Lease{ID: "local-lease", Profile: profile, ProfileSource: "next", ProfilePurpose: "判断给定条件是否足够"},
		Focus: Event{ID: "local-focus", Kind: "cognition_continuation", Status: "pending", Payload: payload},
		State: State{Stage: 10, InstanceID: "test-life", Self: SelfState{Narrative: "PRIVATE-NARRATIVE"},
			Concerns: []Concern{{Meaning: "PRIVATE-CONCERN"}}, Body: readyWebBody()},
	}
}

func TestAssistanceContextSeparatesRoles(t *testing.T) {
	for _, tc := range []struct {
		model, task string
		self, valid bool
	}{
		{"luna", "reasoning", false, true},
		{"luna", "reasoning", true, false},
		{"luna", "implementation", false, false},
		{"sol", "reasoning", false, true},
		{"sol", "reasoning", true, true},
		{"sol", "implementation", false, true},
		{"sol", "unknown", false, false},
	} {
		t.Run(tc.model+tc.task+map[bool]string{true: "self", false: "local"}[tc.self], func(t *testing.T) {
			effort := "none"
			if tc.model == "sol" {
				effort = "low"
			}
			request := assistanceTestRequest(CognitiveProfile{Model: tc.model, ReasoningEffort: effort}, tc.task, tc.self)
			view, err := assistanceContext(request)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if err != nil {
				return
			}
			encoded, _ := json.Marshal(view)
			if strings.Contains(string(encoded), "PRIVATE-NARRATIVE") != tc.self || strings.Contains(string(encoded), "PRIVATE-CONCERN") {
				t.Fatalf("personal context leaked: %s", encoded)
			}
			_, tools := view["available_capabilities"]
			if tools != (tc.task == "implementation") {
				t.Fatalf("unexpected tool context: %s", encoded)
			}
		})
	}
}

func TestAssistanceNativeCallAndSharedAccounting(t *testing.T) {
	for _, model := range []string{"luna", "sol"} {
		t.Run(model, func(t *testing.T) {
			effort := "none"
			if model == "sol" {
				effort = "low"
			}
			request := assistanceTestRequest(CognitiveProfile{Model: model, ReasoningEffort: effort}, "reasoning", false)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				wire, _ := json.Marshal(body)
				if strings.Contains(string(wire), "PRIVATE-") || strings.Contains(string(wire), "cognitive_commit") {
					t.Error("assistance carried main cognition context/schema")
				}
				if body["tool_choice"].(map[string]any)["name"] != "assistance_result" || body["parallel_tool_calls"] != false {
					t.Error("assistance did not use a single native function")
				}
				if model == "luna" && body["max_output_tokens"] != float64(200) {
					t.Error("low-tier output is not compact")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "result", "status": "completed", "model": request.Config.CognitiveResource.Models[model].ID,
					"output":            []map[string]any{{"type": "function_call", "name": "assistance_result", "call_id": "call-1", "arguments": `{"answer":"条件不足，需要第三项事实。"}`}},
					"usage":             map[string]any{"input_tokens": 120, "output_tokens": 20, "total_tokens": 140},
					"llmserver_billing": map[string]any{"request_id": "bill-1", "settlement_status": "confirmed", "currency": "USD", "charges": map[string]any{"total": "0.001"}},
				})
			}))
			defer server.Close()
			request.Config.ModelGateway.BaseURL = server.URL
			request.Config.ModelGateway.Adapter = "llmserver"
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			notices := make(chan WorkerNotice)
			billed := make(chan UsageRecord, 1)
			go func() {
				for {
					select {
					case notice := <-notices:
						if notice.Kind == "model_usage" {
							billed <- notice.Payload.(UsageRecord)
						}
						notice.Ack <- NoticeAck{Accepted: true}
					case <-ctx.Done():
						return
					}
				}
			}()
			result := NewModelClient().Run(ctx, request, notices)
			if result.Error != nil || result.Assistance == nil || result.Stage4 != nil {
				t.Fatalf("invalid result: %#v", result)
			}
			select {
			case usage := <-billed:
				if !usage.CostConfirmed || usage.ActualMicrousd != 1000 {
					t.Fatalf("missing shared bill: %#v", usage)
				}
			default:
				t.Fatal("missing usage settlement")
			}
		})
	}
}

func TestAssistanceSerialReturnRetryAndNoPersonalMutation(t *testing.T) {
	cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 2), release: make(chan struct{})}
	r, err := New(t.TempDir(), "serial", testConfig(10), cognizer)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Stage = 10
	r.config.GenerationKind = "rehearsal" // Keep result acceptance synchronous; no birth activation.
	r.state.Self.Narrative = "unchanged"
	choice := CognitiveResourceChoice{Apply: "next", Model: "luna", ReasoningEffort: "none", Task: "reasoning", Purpose: "判断局部条件"}
	profile, err := r.validateResourceChoice(choice, "question", "none")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.applyResourceChoice(choice, profile, "question"); err != nil {
		t.Fatal(err)
	}
	next := r.state.CognitiveResource.NextProfile
	// Removing the consumed scheduling hint must preserve a failed request's role.
	r.state.CognitiveResource.NextProfile = nil
	got, source, purpose := activeProfileDecision(r.state, r.config.CognitiveResource, next.FocusID)
	if got != profile || source != "next" || purpose != choice.Purpose {
		t.Fatalf("lost retry contract: %v %s %s", got, source, purpose)
	}
	r.state.Lease = &Lease{ID: "helper", FocusID: next.FocusID, Profile: profile, ProfileSource: "next", ProfilePurpose: purpose}
	r.maybeStartCognition(context.Background())
	if len(cognizer.started) != 0 {
		t.Fatal("second foreground inference started")
	}
	if err = r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "helper", FocusID: next.FocusID, Assistance: &CognitiveAssistanceResult{Answer: "可以采用此局部结论"}}); err != nil {
		t.Fatal(err)
	}
	if r.state.Lease != nil || r.state.Self.Narrative != "unchanged" || len(r.state.Memories) != 0 || r.state.PendingAction != nil {
		t.Fatal("helper changed personal state or executed")
	}
	request, ok := r.nextStage4Request()
	if !ok || request.Focus.Kind != "cognition_assistance_result" || !strings.Contains(string(request.Focus.Payload), `"origin":"inferred"`) {
		t.Fatalf("result not available to main cognition: %#v", request)
	}
	got, source, _ = activeProfileDecision(r.state, r.config.CognitiveResource, request.Focus.ID)
	if got != r.config.CognitiveResource.InitialDefaultProfile || source == "next" {
		t.Fatalf("did not return to main: %v %s", got, source)
	}
	count := len(r.state.Background)
	if err = r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "helper", FocusID: next.FocusID, Assistance: &CognitiveAssistanceResult{Answer: "late duplicate"}}); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != count {
		t.Fatal("late helper result entered cognition")
	}
}

func TestAssistanceCodeReachesMainIntact(t *testing.T) {
	answer := strings.Repeat("# implementation detail\n", 300) + "printf 'complete-code-tail'"
	payload, _ := json.Marshal(map[string]any{"answer": answer, "origin": "inferred", "execution_status": "not_executed"})
	event := Event{ID: "code-result", Kind: "cognition_assistance_result", Payload: payload}
	request := CognitiveRequest{Stage: 10, Config: testConfig(10), Focus: event, Candidates: []Event{event}, Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}, Lease: Lease{ID: "main-after-code"}}
	input := isolatedModelInput(t, request)
	if !strings.Contains(input, "complete-code-tail") {
		t.Fatal("main received silently truncated implementation")
	}
}
