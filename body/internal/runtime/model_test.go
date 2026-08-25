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

func TestStageFourModelUsesOneForcedCognitiveCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected responses path %q", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		choice, ok := body["tool_choice"].(map[string]any)
		if !ok || choice["name"] != "cognitive_commit" {
			t.Fatalf("stage four tool was not forced: %#v", body["tool_choice"])
		}
		instructions, _ := body["instructions"].(string)
		if !strings.Contains(instructions, "探索张力的绝对强度") || !strings.Contains(instructions, "自行形成一个具体接触点") || !strings.Contains(instructions, "只有 candidates") {
			t.Fatalf("stage four omitted the meaning and agency of exploration pressure: %q", instructions)
		}
		input, _ := body["input"].(string)
		if !strings.Contains(input, `"background_concerns_not_candidates"`) || strings.Contains(input, `"active_concerns"`) || !strings.Contains(input, "previous focus was invalid") {
			t.Fatalf("stage four did not distinguish candidates, background and retry feedback: %q", input)
		}
		arguments, _ := json.Marshal(CognitiveCommit{
			Appraisals: []CandidateAppraisal{{
				CandidateID: "event-1", Meaning: "这是一次真实身体变化", Difference: 0.7,
				Ownership: 0.9, Value: 0.6, Urgency: 0.2, Answerability: 0.8, Certainty: 0.9, Resolution: "hold",
			}},
			FocusID:       "event-1",
			ThoughtThread: "我愿意先理解身体变化。",
			Action:        CognitiveAction{Kind: "none"},
		})
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":    "response-1",
			"model": "test-model",
			"output": []map[string]any{{
				"type": "function_call", "name": "cognitive_commit", "call_id": "call-1", "arguments": string(arguments),
			}},
			"usage": map[string]any{"input_tokens": 120, "output_tokens": 80, "total_tokens": 200},
		})
	}))
	defer server.Close()

	config := testConfig(4)
	config.Model.BaseURL = server.URL
	config.Model.Name = "test-model"
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-1"}, Stage: 4,
		Focus:      Event{ID: "event-1", Kind: "body_delta", Source: "observed", Summary: "body changed", LastCommitErr: "previous focus was invalid"},
		Candidates: []Event{{ID: "event-1", Kind: "body_delta", Source: "observed", Summary: "body changed", LastCommitErr: "previous focus was invalid"}},
		State:      State{Mentor: MentorState{Received: map[string]uint64{}}},
		Config:     config,
	}
	notices := make(chan WorkerNotice)
	usageSeen := make(chan UsageRecord, 1)
	go func() {
		notice := <-notices
		usageSeen <- notice.Payload.(UsageRecord)
		notice.Ack <- NoticeAck{Accepted: true}
	}()
	result := NewModelClient().Run(context.Background(), request, notices)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Stage4 == nil || result.Stage4.FocusID != "event-1" || result.Stage4.Action.Kind != "none" {
		t.Fatalf("unexpected stage-four result: %#v", result)
	}
	select {
	case usage := <-usageSeen:
		if usage.TotalTokens != 200 {
			t.Fatalf("usage total = %d", usage.TotalTokens)
		}
	case <-time.After(time.Second):
		t.Fatal("model usage was not committed through the event loop")
	}
}

func TestRuntimeSecretIsRedactedFromBodyResults(t *testing.T) {
	sensitiveValue := "runtime-sensitive-value"
	result := redactRuntimeSecret("before "+sensitiveValue+" after "+sensitiveValue, sensitiveValue)
	if result != "before <runtime-secret-redacted> after <runtime-secret-redacted>" {
		t.Fatalf("runtime secret was not redacted: %q", result)
	}
}
