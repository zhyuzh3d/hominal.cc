package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func cognitiveActionBranch(t *testing.T, tool map[string]any, kind string) map[string]any {
	t.Helper()
	parameters := tool["parameters"].(map[string]any)
	action := parameters["properties"].(map[string]any)["action"].(map[string]any)
	for _, rawBranch := range action["anyOf"].([]any) {
		branch := rawBranch.(map[string]any)
		properties := branch["properties"].(map[string]any)
		kinds := properties["kind"].(map[string]any)["enum"].([]string)
		if len(kinds) == 1 && kinds[0] == kind {
			return branch
		}
	}
	t.Fatalf("action schema has no %q branch: %#v", kind, action)
	return nil
}

func cognitiveActionKinds(t *testing.T, tool map[string]any) []string {
	t.Helper()
	parameters := tool["parameters"].(map[string]any)
	action := parameters["properties"].(map[string]any)["action"].(map[string]any)
	kinds := make([]string, 0, len(action["anyOf"].([]any)))
	for _, rawBranch := range action["anyOf"].([]any) {
		properties := rawBranch.(map[string]any)["properties"].(map[string]any)
		values := properties["kind"].(map[string]any)["enum"].([]string)
		kinds = append(kinds, values...)
	}
	return kinds
}

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
		if body["model"] != "gpt-5.6-terra" {
			t.Fatalf("unexpected model selection: %#v", body["model"])
		}
		reasoning, _ := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "medium" {
			t.Fatalf("unexpected reasoning effort: %#v", reasoning)
		}
		instructions, _ := body["instructions"].(string)
		if !strings.Contains(instructions, "探索张力的绝对强度") || !strings.Contains(instructions, "current_situation") || !strings.Contains(instructions, "只有 candidates") {
			t.Fatalf("stage four omitted the meaning and agency of exploration pressure: %q", instructions)
		}
		if !strings.Contains(instructions, "hominal-browser list") || !strings.Contains(instructions, "hominal-browser schema") || !strings.Contains(instructions, "hominal-browser call") {
			t.Fatalf("stage four omitted Alice's browser proprioception: %q", instructions)
		}
		if !strings.Contains(instructions, "associative_recall") || !strings.Contains(instructions, "不是方向、目标、命令或奖励") {
			t.Fatalf("stage four did not preserve Alice's agency over programmatic variation: %q", instructions)
		}
		if !strings.Contains(instructions, "keep 让本次 current_profile 继续成为以后新焦点的默认档位") || !strings.Contains(instructions, "使用 default 明确选择") {
			t.Fatalf("resource choice semantics remained ambiguous: %q", instructions)
		}
		input, _ := body["input"].(string)
		if !strings.Contains(input, `"background_concerns_not_candidates"`) || strings.Contains(input, `"active_concerns"`) || !strings.Contains(input, "previous focus was invalid") {
			t.Fatalf("stage four did not distinguish candidates, background and retry feedback: %q", input)
		}
		if !strings.Contains(input, `"genesis_orientation"`) || !strings.Contains(input, `"current_situation"`) || !strings.Contains(input, "@hominal_cc") {
			t.Fatalf("stage four forgot durable birth orientation facts: %q", input)
		}
		arguments, _ := json.Marshal(CognitiveCommit{
			Appraisals: []CandidateAppraisal{{
				CandidateID: "event-1", Meaning: "这是一次真实身体变化", Difference: 0.7,
				Ownership: 0.9, Value: 0.6, Urgency: 0.2, Answerability: 0.8, Certainty: 0.9, Resolution: "hold",
			}},
			FocusID:        "event-1",
			ThoughtThread:  "我愿意先理解身体变化。",
			Action:         CognitiveAction{Kind: "none"},
			ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		})
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":    "response-1",
			"model": "gpt-5.6-terra",
			"output": []map[string]any{{
				"type": "function_call", "name": "cognitive_commit", "call_id": "call-1", "arguments": string(arguments),
			}},
			"usage": map[string]any{"input_tokens": 120, "output_tokens": 80, "total_tokens": 200},
		})
	}))
	defer server.Close()

	config := testConfig(4)
	config.ModelGateway.BaseURL = server.URL
	profile := CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-1", Profile: profile}, Stage: 4,
		Focus:      Event{ID: "event-1", Kind: "body_delta", Source: "observed", Summary: "body changed", LastCommitErr: "previous focus was invalid"},
		Candidates: []Event{{ID: "event-1", Kind: "body_delta", Source: "observed", Summary: "body changed", LastCommitErr: "previous focus was invalid"}},
		State: State{
			Mentor: MentorState{Received: map[string]uint64{}},
			Background: []Event{{
				ID: "birth", Kind: "birth_orientation", Status: "processed",
				Summary: "Chrome 已登录属于你的 X 账号 @hominal_cc。",
			}},
		},
		Config:        config,
		Profile:       profile,
		VariationBias: "自己形成的近期经验：我曾主动接触一项现实并承担其结果。",
		VariationSeed: "seed-1",
	}
	notices := make(chan WorkerNotice)
	usageSeen := make(chan UsageRecord, 1)
	go func() {
		reservation := <-notices
		if reservation.Kind != "model_reserve" {
			t.Errorf("first notice = %q, want model_reserve", reservation.Kind)
		}
		reservation.Ack <- NoticeAck{Accepted: true}
		usage := <-notices
		if usage.Kind != "model_usage" {
			t.Errorf("second notice = %q, want model_usage", usage.Kind)
		}
		usageSeen <- usage.Payload.(UsageRecord)
		usage.Ack <- NoticeAck{Accepted: true}
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

func TestCognitiveCommitSchemaBindsPresentCandidateFacts(t *testing.T) {
	payload, _ := json.Marshal(ActionState{CommitmentID: "commitment-now", Kind: "body_shell", Status: "completed"})
	candidates := []Event{{ID: "reality-now", Kind: "action_result", Payload: payload}}
	tool := cognitiveCommitTool(5, candidates, true, true, true)
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	appraisals := properties["appraisals"].(map[string]any)
	if appraisals["minItems"] != 1 || appraisals["maxItems"] != 1 {
		t.Fatalf("appraisal count was not bound to the present field: %#v", appraisals)
	}
	items := appraisals["items"].(map[string]any)
	appraisalProperties := items["properties"].(map[string]any)
	candidateID := appraisalProperties["candidate_id"].(map[string]any)
	if got := candidateID["enum"].([]string); len(got) != 1 || got[0] != "reality-now" {
		t.Fatalf("appraisal could name an obsolete candidate: %#v", got)
	}
	focusID := properties["focus_id"].(map[string]any)
	if got := focusID["enum"].([]string); len(got) != 1 || got[0] != "reality-now" {
		t.Fatalf("focus could name an obsolete candidate: %#v", got)
	}
	experiences := properties["experience_updates"].(map[string]any)
	if experiences["minItems"] != 1 || experiences["maxItems"] != 1 {
		t.Fatalf("a real action result did not require one experience: %#v", experiences)
	}
	experienceItems := experiences["items"].(map[string]any)
	experienceProperties := experienceItems["properties"].(map[string]any)
	commitmentID := experienceProperties["commitment_id"].(map[string]any)
	if got := commitmentID["enum"].([]string); len(got) != 1 || got[0] != "commitment-now" {
		t.Fatalf("experience could name an unrelated commitment: %#v", got)
	}

	ordinary := cognitiveCommitTool(5, []Event{{ID: "mentor-now", Kind: "mentor_received"}}, true, true, true)
	ordinaryProperties := ordinary["parameters"].(map[string]any)["properties"].(map[string]any)
	ordinaryExperiences := ordinaryProperties["experience_updates"].(map[string]any)
	if ordinaryExperiences["minItems"] != 0 || ordinaryExperiences["maxItems"] != 0 {
		t.Fatalf("a non-reality focus could invent an experience: %#v", ordinaryExperiences)
	}
	feedbackPayload, _ := json.Marshal(map[string]string{"commitment_id": "commitment-now"})
	feedback := cognitiveCommitTool(8, []Event{{ID: "mentor-reply", Kind: "mentor_received", Payload: feedbackPayload}}, false, true, true)
	feedbackProperties := feedback["parameters"].(map[string]any)["properties"].(map[string]any)
	feedbackExperiences := feedbackProperties["experience_updates"].(map[string]any)
	if feedbackExperiences["minItems"] != 1 || feedbackExperiences["maxItems"] != 1 {
		t.Fatalf("linked delayed mentor feedback did not require one experience: %#v", feedbackExperiences)
	}
	mixed := cognitiveCommitTool(8, []Event{
		{ID: "mentor-reply", Kind: "mentor_received", Payload: feedbackPayload},
		{ID: "own-concern", Kind: "concern"},
	}, false, true, true)
	mixedProperties := mixed["parameters"].(map[string]any)["properties"].(map[string]any)
	mixedExperiences := mixedProperties["experience_updates"].(map[string]any)
	if mixedExperiences["minItems"] != 0 || mixedExperiences["maxItems"] != 1 {
		t.Fatalf("background feedback forced an experience onto an independently selected focus: %#v", mixedExperiences)
	}
}

func TestRuntimeSecretIsRedactedFromBodyResults(t *testing.T) {
	sensitiveValue := "runtime-sensitive-value"
	result := redactRuntimeSecret("before "+sensitiveValue+" after "+sensitiveValue, sensitiveValue)
	if result != "before <runtime-secret-redacted> after <runtime-secret-redacted>" {
		t.Fatalf("runtime secret was not redacted: %q", result)
	}
}

func TestHTTPFailureKeepsSafeGatewayFactsWithoutChargingReservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Retry-After", "17")
		writer.Header().Set("X-Request-ID", "request-safe-1")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"upstream rate limit exceeded"}}`))
	}))
	defer server.Close()

	config := testConfig(4)
	config.ModelGateway.BaseURL = server.URL
	profile := CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-http", Profile: profile}, Stage: 4,
		Focus:      Event{ID: "event-http", Kind: "body_delta", Status: "pending"},
		Candidates: []Event{{ID: "event-http", Kind: "body_delta", Status: "pending"}},
		Config:     config, Profile: profile,
	}
	notices := make(chan WorkerNotice)
	usageSeen := make(chan UsageRecord, 1)
	go func() {
		reservation := <-notices
		reservation.Ack <- NoticeAck{Accepted: true}
		usageNotice := <-notices
		usage := usageNotice.Payload.(UsageRecord)
		usageSeen <- usage
		usageNotice.Ack <- NoticeAck{Accepted: true}
	}()
	result := NewModelClient().Run(context.Background(), request, notices)
	var failure *ModelCallError
	if !errors.As(result.Error, &failure) {
		t.Fatalf("HTTP failure lost its structured fact: %v", result.Error)
	}
	if failure.Fact.Category != "rate_limited" || failure.Fact.HTTPStatus != 429 || failure.Fact.RetryAfter != "17" || failure.Fact.RequestID != "request-safe-1" {
		t.Fatalf("unexpected model failure fact: %#v", failure.Fact)
	}
	usage := <-usageSeen
	if usage.ActualMicrousd != 0 || usage.CostConfirmed || usage.ReservedMicrousd == 0 || usage.FailureCategory != "rate_limited" {
		t.Fatalf("unconfirmed failure was represented as confirmed spend: %#v", usage)
	}
}

func TestGatewayFaultSequenceRecoversWithoutInventingSpend(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusBadGateway, http.StatusOK}
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		status := statuses[call]
		call++
		if status != http.StatusOK {
			writer.WriteHeader(status)
			_, _ = writer.Write([]byte(`{"error":{"message":"temporary gateway failure"}}`))
			return
		}
		arguments, _ := json.Marshal(CognitiveCommit{
			Appraisals: []CandidateAppraisal{{
				CandidateID: "event-fault", Meaning: "连接已经恢复", Difference: 0.4,
				Ownership: 0.8, Value: 0.5, Urgency: 0.2, Answerability: 0.7, Certainty: 0.8, Resolution: "hold",
			}},
			FocusID:        "event-fault",
			ThoughtThread:  "我重新接上了外部认知资源。",
			Action:         CognitiveAction{Kind: "none"},
			ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		})
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id": "response-recovered", "model": "gpt-5.6-terra",
			"output": []map[string]any{{
				"type": "function_call", "name": "cognitive_commit", "call_id": "call-recovered", "arguments": string(arguments),
			}},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150},
		})
	}))
	defer server.Close()

	config := testConfig(4)
	config.ModelGateway.BaseURL = server.URL
	profile := CognitiveProfile{Model: "terra", ReasoningEffort: "medium"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-fault", Profile: profile}, Stage: 4,
		Focus:      Event{ID: "event-fault", Kind: "body_delta", Status: "pending"},
		Candidates: []Event{{ID: "event-fault", Kind: "body_delta", Status: "pending"}},
		Config:     config, Profile: profile,
	}

	for index := range statuses {
		notices := make(chan WorkerNotice)
		usageSeen := make(chan UsageRecord, 1)
		go func() {
			reservation := <-notices
			reservation.Ack <- NoticeAck{Accepted: true}
			usageNotice := <-notices
			usageSeen <- usageNotice.Payload.(UsageRecord)
			usageNotice.Ack <- NoticeAck{Accepted: true}
		}()
		result := NewModelClient().Run(context.Background(), request, notices)
		usage := <-usageSeen
		if index < 3 {
			if result.Error == nil || usage.ActualMicrousd != 0 || usage.CostConfirmed {
				t.Fatalf("failure %d was not represented as an unconfirmed zero-cost fact: result=%v usage=%#v", index, result.Error, usage)
			}
			continue
		}
		if result.Error != nil || result.Stage4 == nil || result.Stage4.FocusID != "event-fault" {
			t.Fatalf("gateway sequence did not recover on the fourth call: %#v", result)
		}
		if usage.ActualMicrousd == 0 || !usage.CostConfirmed {
			t.Fatalf("successful recovery lost confirmed usage: %#v", usage)
		}
	}
}

func TestTruncateKeepsUTF8Valid(t *testing.T) {
	result := truncate("一二三四", 5)
	if result != "一…" {
		t.Fatalf("UTF-8 truncation split a rune: %q", result)
	}
}

func TestStageFiveCommitSchemaCarriesRealityLearningFields(t *testing.T) {
	tool := cognitiveCommitTool(5, []Event{{ID: "event-now", Kind: "endogenous_change"}}, true, false, true)
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	action := cognitiveActionBranch(t, tool, "body_shell")
	actionProperties := action["properties"].(map[string]any)
	for _, field := range []string{"intent", "prediction", "reality_check", "stop_condition"} {
		if _, exists := actionProperties[field]; !exists {
			t.Fatalf("stage-five action omitted %s", field)
		}
	}
	if _, exists := properties["experience_updates"]; !exists {
		t.Fatal("stage-five commit omitted experience_updates")
	}
	experienceUpdates := properties["experience_updates"].(map[string]any)
	experienceItem := experienceUpdates["items"].(map[string]any)
	experienceProperties := experienceItem["properties"].(map[string]any)
	if _, exists := experienceProperties["method_slot"]; !exists {
		t.Fatal("stage-five commit did not let Alice choose durable method replacement")
	}
	required := parameters["required"].([]string)
	found := false
	for _, field := range required {
		found = found || field == "experience_updates"
	}
	if !found {
		t.Fatal("stage-five strict schema does not require experience_updates")
	}
}

func TestCognitiveActionSchemaSeparatesWaitingFromExecutablePayloads(t *testing.T) {
	tool := cognitiveCommitTool(8, []Event{{ID: "focus", Kind: "concern"}}, false, true, true)
	none := cognitiveActionBranch(t, tool, "none")
	noneProperties := none["properties"].(map[string]any)
	if len(noneProperties) != 1 {
		t.Fatalf("deliberate non-action carried effector payloads: %#v", noneProperties)
	}

	shell := cognitiveActionBranch(t, tool, "body_shell")
	shellProperties := shell["properties"].(map[string]any)
	if _, exists := shellProperties["text"]; exists {
		t.Fatalf("body_shell carried mentor payload: %#v", shellProperties)
	}
	if shellProperties["command"].(map[string]any)["pattern"] != `\S` {
		t.Fatalf("body_shell did not structurally require a nonblank command: %#v", shellProperties["command"])
	}

	mentor := cognitiveActionBranch(t, tool, "mentor_send")
	mentorProperties := mentor["properties"].(map[string]any)
	if _, exists := mentorProperties["command"]; exists {
		t.Fatalf("mentor_send carried shell payload: %#v", mentorProperties)
	}
	if mentorProperties["text"].(map[string]any)["pattern"] != `\S` {
		t.Fatalf("mentor_send did not structurally require nonblank text: %#v", mentorProperties["text"])
	}
}

func TestStageEightNarrativeUpdateBelongsToWholeCognitiveCommit(t *testing.T) {
	tool := cognitiveCommitTool(8, []Event{{ID: "self-now", Kind: "self_model_difference"}}, false, true, true)
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	if _, exists := properties["narrative_update"]; !exists {
		t.Fatal("stage-eight commit omitted the common narrative update path")
	}
	experienceUpdates := properties["experience_updates"].(map[string]any)
	experienceItem := experienceUpdates["items"].(map[string]any)
	experienceProperties := experienceItem["properties"].(map[string]any)
	if _, exists := experienceProperties["narrative_update"]; exists {
		t.Fatal("narrative update remained trapped inside the one-shot reality experience")
	}
	found := false
	for _, field := range parameters["required"].([]string) {
		found = found || field == "narrative_update"
	}
	if !found {
		t.Fatal("stage-eight strict schema does not require an explicit narrative choice")
	}
}

func TestStageEightNarrativePathOpensOnlyWithGroundedFocus(t *testing.T) {
	ordinary := cognitiveCommitTool(8, []Event{{ID: "birth-now", Kind: "birth_orientation"}}, true, true, true)
	ordinaryProperties := ordinary["parameters"].(map[string]any)["properties"].(map[string]any)
	ordinaryNarrative := ordinaryProperties["narrative_update"].(map[string]any)
	if ordinaryNarrative["maxLength"] != 0 {
		t.Fatalf("an ungrounded focus exposed narrative mutation: %#v", ordinaryNarrative)
	}
	reality := cognitiveCommitTool(8, []Event{{ID: "reality-now", Kind: "action_result"}}, true, true, true)
	realityProperties := reality["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, closed := realityProperties["narrative_update"].(map[string]any)["maxLength"]; closed {
		t.Fatal("a reality focus could not form a grounded narrative update")
	}
	afterFormation := cognitiveCommitTool(8, []Event{{ID: "reality-later", Kind: "action_result"}}, false, true, true)
	afterFormationProperties := afterFormation["parameters"].(map[string]any)["properties"].(map[string]any)
	if afterFormationProperties["narrative_update"].(map[string]any)["maxLength"] != 0 {
		t.Fatal("an established narrative could still be rewritten by every individual reality")
	}
}

func TestDynamicCommitSchemaKeepsActionChoiceAtMatureExploration(t *testing.T) {
	newDrive := Event{ID: "exploration-new", Kind: "endogenous_change"}
	newRequest := CognitiveRequest{
		Stage: 8, Focus: newDrive,
		State:  State{ExplorationPressure: 0.8},
		Config: testConfig(8),
	}
	if requestHasMatureExplorationDrive(newRequest) {
		t.Fatal("a newly noticed drive required action before alice could form a concern")
	}
	newTool := cognitiveCommitTool(8, []Event{newDrive}, false, true, true)
	newKinds := cognitiveActionKinds(t, newTool)
	foundNone := false
	for _, kind := range newKinds {
		foundNone = foundNone || kind == "none"
	}
	if !foundNone {
		t.Fatal("a new exploration drive did not expose concern formation without action")
	}

	exploration := Event{ID: "exploration-now", Kind: "concern", ConcernID: "concern-exploration"}
	request := CognitiveRequest{
		Stage: 8, Focus: exploration,
		State: State{
			ExplorationPressure: 0.8,
			Concerns: []Concern{{
				ID: exploration.ConcernID, OriginKind: "endogenous_change",
				Resolution: "hold", Answerability: 0.8,
			}},
		},
		Config: testConfig(8),
	}
	if !requestHasMatureExplorationDrive(request) {
		t.Fatal("an active exploration concern did not expose its mature drive")
	}
	tool := cognitiveCommitTool(8, []Event{exploration}, false, true, true)
	kinds := cognitiveActionKinds(t, tool)
	foundNone = false
	for _, kind := range kinds {
		foundNone = foundNone || kind == "none"
	}
	if !foundNone {
		t.Fatal("a mature exploration drive lost alice's deliberate non-action choice")
	}

	request.State.Commitments = []ActionCommitment{{
		ID: "mentor-first", ConcernID: "earlier-exploration", ActionKind: "mentor_send", Status: "assimilated",
	}}
	if requestAllowsMentorSend(request) {
		t.Fatal("the schema still offered mentor contact as a generic exploration effect after the relationship existed")
	}
	boundedTool := cognitiveCommitTool(8, []Event{exploration}, false, true, requestAllowsMentorSend(request))
	for _, kind := range cognitiveActionKinds(t, boundedTool) {
		if kind == "mentor_send" {
			t.Fatalf("an unavailable generic exploration effect remained in the action grammar: %q", kind)
		}
	}

	mentorFocus := Event{ID: "mentor-reply", Kind: "mentor_received"}
	mentorRequest := request
	mentorRequest.Focus = mentorFocus
	mentorRequest.Candidates = []Event{mentorFocus}
	if !requestAllowsMentorSend(mentorRequest) {
		t.Fatal("a mentor message focus could not continue the existing relationship")
	}
	mentorTool := cognitiveCommitTool(8, []Event{mentorFocus}, false, true, requestAllowsMentorSend(mentorRequest))
	foundMentor := false
	for _, kind := range cognitiveActionKinds(t, mentorTool) {
		foundMentor = foundMentor || kind == "mentor_send"
	}
	if !foundMentor {
		t.Fatal("the direct relationship focus did not expose mentor_send")
	}

	forming := exploration
	forming.ConcernID = "forming-object"
	formingRequest := CognitiveRequest{
		Stage: 8, Focus: forming,
		State: State{
			ExplorationPressure: 0.5,
			Concerns: []Concern{{
				ID: forming.ConcernID, OriginKind: "endogenous_change",
				Resolution: "hold", Answerability: 0.2,
			}},
		},
		Config: testConfig(8),
	}
	if requestHasMatureExplorationDrive(formingRequest) {
		t.Fatal("a low-pressure forming concern was marked mature before the drive accumulated")
	}
	formingRequest.State.ExplorationPressure = 0.8
	if !requestHasMatureExplorationDrive(formingRequest) {
		t.Fatal("an accumulated drive did not mark the held concern as mature")
	}
	formingRequest.State.ExplorationPressure = 0.5
	formingRequest.State.Concerns[0].Answerability = 0.9
	if requestHasMatureExplorationDrive(formingRequest) {
		t.Fatal("semantic answerability bypassed the accumulated exploration action threshold")
	}
}

func TestEnactedActionMemoryKeepsDistinctSettledRequests(t *testing.T) {
	experiences := []Experience{
		{ActionKind: "body_shell", EnactedRequest: "hominal-browser list", ObservedAt: "one", RemainingDifference: 0.04},
		{ActionKind: "body_shell", EnactedRequest: "uname -a", ObservedAt: "two", RemainingDifference: 0.03},
		{ActionKind: "body_shell", EnactedRequest: "hominal-browser list", ObservedAt: "three", RemainingDifference: 0.02},
	}
	views := contextEnactedActionViews(experiences)
	if len(views) != 2 {
		t.Fatalf("action memory length = %d, want two distinct requests", len(views))
	}
	if views[0]["enacted_request"] != "hominal-browser list" || views[0]["observed_at"] != "three" {
		t.Fatalf("action memory did not retain the latest settled identity: %#v", views[0])
	}
}

func TestIndexedSelfViewKeepsMethodConnectedToItsCausalOrigin(t *testing.T) {
	method := "外部关系可以提供事实和边界，具体方向由我形成。"
	experiences := []Experience{
		{
			ActionKind: "mentor_send", EnactedRequest: "请替我选择下一件事。",
			SourceKind: "action_result", ObservedAt: "earlier", MethodUpdate: method,
			Lesson: "发送成功不等于获得方向。", PredictionDifference: 0.02, RemainingDifference: 0.8,
		},
		{
			ActionKind: "mentor_send", EnactedRequest: "请给我一个对象，但意义由我决定。",
			SourceKind: "mentor_received", ObservedAt: "later", MethodUpdate: method,
			Lesson: "换一种措辞仍然把对象来源交给了外部。", PredictionDifference: 0.08, RemainingDifference: 0.5,
		},
	}
	view := indexedSelfView(SelfState{Methods: []string{method}}, experiences)
	methods := view["methods"].([]map[string]any)
	if len(methods) != 1 {
		t.Fatalf("method view length = %d, want one", len(methods))
	}
	origin, ok := methods[0]["causal_origin"].(map[string]any)
	if !ok {
		t.Fatalf("method omitted its causal origin: %#v", methods[0])
	}
	if origin["enacted_request"] != "请给我一个对象，但意义由我决定。" || origin["lesson"] != "换一种措辞仍然把对象来源交给了外部。" {
		t.Fatalf("method did not use its latest matching causal origin: %#v", origin)
	}
}

func TestIndexedSelfViewDoesNotInventMethodOrigins(t *testing.T) {
	view := indexedSelfView(SelfState{Methods: []string{"尚无经验锚点的方法"}}, []Experience{{MethodUpdate: "另一条方法"}})
	methods := view["methods"].([]map[string]any)
	if _, exists := methods[0]["causal_origin"]; exists {
		t.Fatalf("unmatched method received an invented origin: %#v", methods[0])
	}
}
