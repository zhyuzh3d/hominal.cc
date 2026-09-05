package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestStageTenUsesUnifiedCognition(t *testing.T) {
	if !usesUnifiedCognition(10) {
		t.Fatal("stage ten did not reuse the frozen unified cognition path")
	}
}

func TestStageNineUsesTheStageEightCognitiveContract(t *testing.T) {
	if !usesUnifiedCognition(9) {
		t.Fatal("stage nine did not enter the shared cognitive route")
	}
	if usesUnifiedCognition(3) || usesUnifiedCognition(7) {
		t.Fatal("an unsupported stage entered the shared cognitive route")
	}
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
		if !strings.Contains(instructions, "探索方向的 activation") || !strings.Contains(instructions, "current_situation") || !strings.Contains(instructions, "只有 candidates") {
			t.Fatalf("stage four omitted the meaning and agency of the value field: %q", instructions)
		}
		if !strings.Contains(instructions, "available_capabilities.organs") || !strings.Contains(instructions, "器官只实施并返回事实") {
			t.Fatalf("stage four omitted Alice's generic organ proprioception: %q", instructions)
		}
		if !strings.Contains(instructions, "operation 只从该器官说明中的 operations 选择") || !strings.Contains(instructions, "不是可猜测的行动名") {
			t.Fatalf("stage four confused host capabilities with callable organ operations: %q", instructions)
		}
		if !strings.Contains(instructions, "associative_recall") || !strings.Contains(instructions, "不是方向、目标、命令或奖励") {
			t.Fatalf("stage four did not preserve Alice's agency over programmatic variation: %q", instructions)
		}
		if !strings.Contains(instructions, "default_profile 是本代主力认知") || !strings.Contains(instructions, "high/low 是进阶行动协助") || !strings.Contains(instructions, "确定性的本能与机械状态工作由身体内核完成") || !strings.Contains(instructions, "next 只安排同一因果线程中紧接着的一次认知") {
			t.Fatalf("resource choice semantics remained ambiguous: %q", instructions)
		}
		if !strings.Contains(instructions, "机器可读的键值") || !strings.Contains(instructions, "读取到一项声明") {
			t.Fatalf("stage four lost the distinction between reading and checking an explicit claim: %q", instructions)
		}
		if !strings.Contains(instructions, "器官观察中的 context 提供当前感官场景") || !strings.Contains(instructions, "Visible object 才是这次新进入注意的具体对象") {
			t.Fatalf("organ scene context could masquerade as the newly perceived object: %q", instructions)
		}
		if !strings.Contains(instructions, "少量仍可感到但不再影响未来选择的不确定可以保留") {
			t.Fatalf("a subjective residual difference was still treated as schema failure: %q", instructions)
		}
		if !strings.Contains(instructions, "等待上位 Concern、整轮计划或兄弟对象") || !strings.Contains(instructions, "不把同一份等待重复背在两层张力上") {
			t.Fatalf("concern hierarchy could duplicate one waiting condition across parent and child: %q", instructions)
		}
		input, _ := body["input"].(string)
		if !strings.Contains(input, `"background_concerns_not_candidates"`) || strings.Contains(input, `"active_concerns"`) || !strings.Contains(input, "previous focus was invalid") {
			t.Fatalf("stage four did not distinguish candidates, background and retry feedback: %q", input)
		}
		if !strings.Contains(input, `"genesis_orientation"`) || !strings.Contains(input, `"current_situation"`) || !strings.Contains(input, "@hominal_cc") {
			t.Fatalf("stage four forgot durable birth orientation facts: %q", input)
		}
		if !strings.Contains(input, `"user":"root"`) || !strings.Contains(input, `"home":"/root"`) || !strings.Contains(input, `"working_directory":"/agent/lives/`) || !strings.Contains(input, `"life_space":"/life"`) ||
			!strings.Contains(input, `"desktop_home":"/home/hominal"`) || !strings.Contains(input, `"command":"hominal-browser"`) ||
			!strings.Contains(input, `"operations":["browser_snapshot","browser_click"]`) ||
			!strings.Contains(input, `"wechat_client"`) || !strings.Contains(input, `"organ_surface":"process_state_only"`) || !strings.Contains(input, `"mentor_channel"`) ||
			!strings.Contains(input, `"content_persistence":"cross_generation"`) || !strings.Contains(input, `"existing_content_role":"lineage_environment"`) ||
			!strings.Contains(input, `"current_generation_publication_evidence"`) {
			t.Fatalf("stage four did not make Alice's device, file and communication resources recoverable: %q", input)
		}
		if !strings.Contains(input, `"linked_concern"`) || !strings.Contains(input, "共同实验仍有多个独立物件") {
			t.Fatalf("a contribution fact was detached from the whole concern it reappraises: %q", input)
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
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "medium"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-1", Profile: profile}, Stage: 4,
		Focus:      Event{ID: "event-1", Kind: "concern_contribution", Source: "memory", Summary: "one child advanced", ConcernID: "parent-concern", LastCommitErr: "previous focus was invalid"},
		Candidates: []Event{{ID: "event-1", Kind: "concern_contribution", Source: "memory", Summary: "one child advanced", ConcernID: "parent-concern", LastCommitErr: "previous focus was invalid"}},
		State: State{
			Mentor: MentorState{Received: map[string]uint64{}},
			Body: BodySnapshot{Organs: map[string]OrganSnapshot{"browser": {
				Name: "Chrome browser", Command: "hominal-browser", Capabilities: []string{"observe", "perform", "authenticated_web"},
				Operations: []string{"browser_snapshot", "browser_click"},
				Guidance:   "Use the organ command.", Status: "ready", Accepting: true,
			}}},
			Concerns: []Concern{{ID: "parent-concern", Subject: "共同实验仍有多个独立物件", Meaning: "一个子步骤有进展，但整体尚未闭合", Difference: 0.7, Ownership: 0.9, Resolution: "hold"}},
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
	payload, _ := json.Marshal(ActionState{CommitmentID: "commitment-now", Kind: "organ_action", Status: "completed"})
	candidates := []Event{{ID: "reality-now", Kind: "action_result", Payload: payload}}
	tool := cognitiveCommitTool(5, candidates, true, true, true, "existing-concern")
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
	continuation := properties["continues_concern_id"].(map[string]any)
	if got := continuation["enum"].([]string); len(got) != 2 || got[0] != "" || got[1] != "existing-concern" {
		t.Fatalf("concern continuation was not limited to current background identities: %#v", got)
	}
	contribution := properties["contributes_to_concern_id"].(map[string]any)
	if got := contribution["enum"].([]string); len(got) != 2 || got[0] != "" || got[1] != "existing-concern" {
		t.Fatalf("concern contribution was not limited to current background identities: %#v", got)
	}
	resolution := appraisalProperties["resolution"].(map[string]any)
	if got := resolution["enum"].([]string); len(got) != 3 || got[0] != "hold" || got[1] != "resolved" || got[2] != "released" {
		t.Fatalf("concern lifecycle did not distinguish closure from chosen release: %#v", got)
	}
	memories := properties["reality_updates"].(map[string]any)
	if memories["minItems"] != 1 || memories["maxItems"] != 1 {
		t.Fatalf("a real action result did not require one memory: %#v", memories)
	}
	memoryItems := memories["items"].(map[string]any)
	memoryProperties := memoryItems["properties"].(map[string]any)
	commitmentID := memoryProperties["commitment_id"].(map[string]any)
	if got := commitmentID["enum"].([]string); len(got) != 1 || got[0] != "commitment-now" {
		t.Fatalf("memory could name an unrelated commitment: %#v", got)
	}

	ordinary := cognitiveCommitTool(5, []Event{{ID: "mentor-now", Kind: "mentor_received"}}, true, true, true)
	ordinaryProperties := ordinary["parameters"].(map[string]any)["properties"].(map[string]any)
	ordinaryMemories := ordinaryProperties["reality_updates"].(map[string]any)
	if ordinaryMemories["minItems"] != 0 || ordinaryMemories["maxItems"] != 0 {
		t.Fatalf("a non-reality focus could invent an memory: %#v", ordinaryMemories)
	}
	feedbackPayload, _ := json.Marshal(map[string]string{"commitment_id": "commitment-now"})
	feedback := cognitiveCommitTool(8, []Event{{ID: "mentor-reply", Kind: "mentor_received", Payload: feedbackPayload}}, false, true, true)
	feedbackProperties := feedback["parameters"].(map[string]any)["properties"].(map[string]any)
	feedbackMemories := feedbackProperties["reality_updates"].(map[string]any)
	if feedbackMemories["minItems"] != 1 || feedbackMemories["maxItems"] != 1 {
		t.Fatalf("linked delayed mentor feedback did not require one memory: %#v", feedbackMemories)
	}
	mixed := cognitiveCommitTool(8, []Event{
		{ID: "mentor-reply", Kind: "mentor_received", Payload: feedbackPayload},
		{ID: "own-concern", Kind: "concern"},
	}, false, true, true)
	mixedProperties := mixed["parameters"].(map[string]any)["properties"].(map[string]any)
	mixedMemories := mixedProperties["reality_updates"].(map[string]any)
	if mixedMemories["minItems"] != 0 || mixedMemories["maxItems"] != 1 {
		t.Fatalf("background feedback forced an memory onto an independently selected focus: %#v", mixedMemories)
	}
}

func TestStageTenSchemaCarriesPluralValuesAndLocksUngroundedOrientation(t *testing.T) {
	ordinary := cognitiveCommitTool(10, []Event{{ID: "ordinary", Kind: "perceptual_change"}}, false, true, true)
	properties := ordinary["parameters"].(map[string]any)["properties"].(map[string]any)
	appraisal := properties["appraisals"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	valueProperties := appraisal["values"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"continuance", "exploration", "agency", "vitality", "relatedness", "contribution"} {
		if _, exists := valueProperties[name]; !exists {
			t.Fatalf("appraisal omitted life value direction %q", name)
		}
	}
	orientation := properties["value_orientation_update"].(map[string]any)["properties"].(map[string]any)
	if orientation["relatedness"].(map[string]any)["maximum"] != float64(0) {
		t.Fatal("an ordinary perception could permanently rewrite value orientation")
	}

	grounded := cognitiveCommitTool(10, []Event{{ID: "reality", Kind: "action_result"}}, true, true, true)
	groundedProperties := grounded["parameters"].(map[string]any)["properties"].(map[string]any)
	groundedOrientation := groundedProperties["value_orientation_update"].(map[string]any)["properties"].(map[string]any)
	if groundedOrientation["relatedness"].(map[string]any)["maximum"] != float64(1) {
		t.Fatal("grounded self revision could not express a slow value-orientation change")
	}
}

func TestCognitiveSchemaKeepsOneBodyActionWhileAnOrganIsBusy(t *testing.T) {
	tool := cognitiveCommitToolWithLinks(
		10,
		[]Event{{ID: "other-reality", Kind: "mentor_content"}},
		false,
		true,
		true,
		false,
		nil,
		nil,
		nil,
	)
	encoded, err := json.Marshal(tool["parameters"].(map[string]any)["properties"].(map[string]any)["action"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"organ_action"`) {
		t.Fatalf("a second body action remained available while an organ was busy: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"mentor_send"`) || !strings.Contains(string(encoded), `"none"`) {
		t.Fatalf("non-body cognition disappeared while an organ was busy: %s", encoded)
	}
}

func TestNewIndependentConcernCanChooseAStableBroaderContext(t *testing.T) {
	parent := Concern{
		ID: "shared-experiment", OriginKind: "mentor_received", Meaning: "共同实验仍在进行",
		Ownership: 0.9, Resolution: "hold",
	}
	child := Event{ID: "new-object", Kind: "environment_change"}
	visibleParent := Event{ID: parent.ID, Kind: "concern", ConcernID: parent.ID}
	state := State{Concerns: []Concern{parent}}
	continuable := continuableConcernIDs(state, []Event{child, visibleParent})
	if len(continuable) != 0 {
		t.Fatalf("a represented parent could overwrite the independent child through continuation: %#v", continuable)
	}
	within := withinConcernIDs(state, testConfig(9).Dynamics)
	if len(within) != 1 || within[0] != parent.ID {
		t.Fatalf("a held broader concern was unavailable as the child's stable context: %#v", within)
	}
	contributable := contributableConcernIDs(state, []Event{child, visibleParent}, testConfig(9).Dynamics)
	if len(contributable) != 1 || contributable[0] != parent.ID {
		t.Fatalf("an independent reality fact could not affect a visible self-owned concern: %#v", contributable)
	}
	tool := cognitiveCommitToolWithLinks(9, []Event{child, visibleParent}, false, true, true, true, continuable, within, contributable)
	properties := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	context := properties["within_concern_id"].(map[string]any)
	if got := context["enum"].([]string); len(got) != 2 || got[1] != parent.ID {
		t.Fatalf("tool schema hid the stable parent context: %#v", got)
	}
	if _, exists := properties["new_concern_closure_condition"]; !exists {
		t.Fatal("Stage 9 tool schema omitted the stable closure condition at concern formation")
	}
	if _, exists := properties["emerging_consequence"]; !exists {
		t.Fatal("Stage 9 tool schema omitted the serial preservation of a newly emerging consequence")
	}
	if properties["emerging_consequence"].(map[string]any)["maxLength"] != 0 {
		t.Fatal("a non-reality focus could manufacture another emerging consequence")
	}
	required := tool["parameters"].(map[string]any)["required"].([]string)
	foundClosure := false
	foundConsequence := false
	for _, field := range required {
		foundClosure = foundClosure || field == "new_concern_closure_condition"
		foundConsequence = foundConsequence || field == "emerging_consequence"
	}
	if !foundClosure {
		t.Fatal("Stage 9 tool schema did not require an explicit empty-or-formed closure condition")
	}
	if !foundConsequence {
		t.Fatal("Stage 9 tool schema did not require an explicit empty-or-emerging consequence")
	}
}

func TestOnlyActionResultCanExposeAnEmergingConsequence(t *testing.T) {
	reality := cognitiveCommitTool(9, []Event{{ID: "reality", Kind: "action_result"}}, false, true, true)
	realityProperties := reality["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, closed := realityProperties["emerging_consequence"].(map[string]any)["maxLength"]; closed {
		t.Fatal("an action result could not preserve a genuinely new consequence")
	}

	consequence := cognitiveCommitTool(9, []Event{{ID: "next-object", Kind: "reality_consequence"}}, false, true, true)
	consequenceProperties := consequence["parameters"].(map[string]any)["properties"].(map[string]any)
	if consequenceProperties["emerging_consequence"].(map[string]any)["maxLength"] != 0 {
		t.Fatal("a preserved consequence could recursively create another consequence before acting")
	}
}

func TestRealityCanOnlyContributeToItsChildsStableContext(t *testing.T) {
	parent := Concern{ID: "shared-experiment", OriginKind: "mentor_received", Ownership: 0.9, Resolution: "hold"}
	sibling := Concern{ID: "earlier-object", OriginKind: "environment_change", Ownership: 0.9, Resolution: "hold"}
	child := Concern{ID: "current-object", OriginKind: "environment_change", WithinConcernID: parent.ID, Ownership: 0.9, Resolution: "hold"}
	commitment := ActionCommitment{ID: "child-action", ConcernID: child.ID, Status: "reality_available"}
	payload, _ := json.Marshal(ActionState{CommitmentID: commitment.ID, Status: "completed"})
	reality := Event{ID: "child-reality", Kind: "action_result", ConcernID: child.ID, Payload: payload}
	state := State{Concerns: []Concern{parent, sibling, child}, Commitments: []ActionCommitment{commitment}}
	ids := contributableConcernIDs(state, []Event{reality}, testConfig(9).Dynamics)
	if len(ids) != 1 || ids[0] != parent.ID {
		t.Fatalf("semantic similarity exposed a sibling instead of the self-endorsed parent: %#v", ids)
	}
}

func TestIndependentRealityOnlyExposesVisibleOwnedContributionTargets(t *testing.T) {
	config := testConfig(10)
	valid := Concern{ID: "public-expression", OriginKind: "perceptual_change", Ownership: 0.9, Resolution: "hold"}
	birth := Concern{ID: "birth", OriginKind: "birth_orientation", Ownership: 1, Resolution: "hold"}
	unowned := Concern{ID: "noticed-only", OriginKind: "environment_change", Ownership: config.Dynamics.AttentionThreshold - 0.01, Resolution: "hold"}
	settled := Concern{ID: "settled", OriginKind: "environment_change", Ownership: 0.9, Resolution: "resolved"}
	current := Concern{ID: "current-object", OriginKind: "perceptual_change", Ownership: 0.9, Resolution: "hold"}
	state := State{Concerns: []Concern{valid, birth, unowned, settled, current}}
	perception := Event{ID: "visible-post", Kind: "perceptual_change", ConcernID: current.ID}

	ids := contributableConcernIDs(state, []Event{perception}, config.Dynamics)
	if len(ids) != 1 || ids[0] != valid.ID {
		t.Fatalf("independent reality exposed a non-owned, settled, birth, or self target: %#v", ids)
	}
}

func TestConcernContextKeepsItsStableClosureConditionVisible(t *testing.T) {
	concern := Concern{
		ID: "whole-experiment", Subject: "共同理解多个物件",
		ClosureCondition: "多个独立物件都取得现实结果并形成共同结论", Resolution: "hold",
	}
	view := concernContextView(concern)
	if view["closure_condition"] != concern.ClosureCondition {
		t.Fatalf("whole-concern boundary disappeared from later cognition: %#v", view)
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
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "medium"}
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
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "medium"}
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

func TestLLMServerAdapterUsesNativeFunctionCognitionAndConfirmedServerBill(t *testing.T) {
	checkNativeFunctionCognitionAndBill(t, 10)
}

func TestStage20RunsActualNativeCognitionAndSettlement(t *testing.T) {
	checkNativeFunctionCognitionAndBill(t, 20)
}

func checkNativeFunctionCognitionAndBill(t *testing.T, stage int) {
	arguments, _ := json.Marshal(CognitiveCommit{
		Appraisals: []CandidateAppraisal{{
			CandidateID: "llmserver-focus", Meaning: "本地认知服务已经可达", Difference: 0.2,
			Ownership: 0.8, Value: 0.4, Urgency: 0.1, Answerability: 0.7, Certainty: 0.9, Resolution: "hold",
		}},
		FocusID: "llmserver-focus", ThoughtThread: "我可以继续理解这个事实。",
		Action:         CognitiveAction{Kind: "none"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 || tools[0].(map[string]any)["name"] != "cognitive_commit" || tools[0].(map[string]any)["strict"] != true {
			t.Fatalf("llmserver request did not contain the one native strict cognition tool: %#v", body["tools"])
		}
		choice, ok := body["tool_choice"].(map[string]any)
		if !ok || choice["type"] != "function" || choice["name"] != "cognitive_commit" || body["parallel_tool_calls"] != false {
			t.Fatalf("llmserver native tool selection was not single and forced: choice=%#v parallel=%#v", body["tool_choice"], body["parallel_tool_calls"])
		}
		if body["model"] != "codex-terra" || body["store"] != false {
			t.Fatalf("unexpected llmserver request basics: %#v", body)
		}
		instructions, _ := body["instructions"].(string)
		if strings.Contains(instructions, "不提供 function tool") || strings.Contains(instructions, "请只输出一个 JSON 对象") {
			t.Fatal("obsolete prompt-emulated tool contract remained in native cognition")
		}
		if !strings.Contains(instructions, "价值判断也容纳生成性") || !strings.Contains(instructions, "长期结果由行动后的后果检验") {
			t.Fatal("stage ten still required social or experiential value to be proven before a reversible action")
		}
		extension, _ := body["llmserver"].(map[string]any)
		if !strings.HasPrefix(extension["idempotency_key"].(string), "hominal:llmserver-life:llmserver-focus:lease-local:") {
			t.Fatalf("missing stable llmserver idempotency key: %#v", extension)
		}
		writer.Header().Set("X-LLMServer-Request-ID", "req-confirmed")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id": "response-local", "status": "completed", "model": "codex-terra",
			"output": []map[string]any{{"type": "function_call", "name": "cognitive_commit", "call_id": "call-native", "arguments": string(arguments)}},
			"usage":  map[string]any{"input_tokens": 100, "output_tokens": 20, "total_tokens": 120},
			"llmserver_billing": map[string]any{
				"request_id": "req-confirmed", "settlement_status": "confirmed", "price_version": "codex-terra-test-v1", "currency": "USD",
				"charges": map[string]any{"total": "0.000123456"},
			},
		})
	}))
	defer server.Close()

	config := testConfig(stage)
	config.ModelGateway.BaseURL = server.URL
	config.ModelGateway.Adapter = "llmserver"
	terra := config.CognitiveResource.Models["main"]
	terra.ID = "codex-terra"
	config.CognitiveResource.Models["main"] = terra
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "none"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-local", Profile: profile, PulseID: 7}, Stage: stage,
		Focus:      Event{ID: "llmserver-focus", Kind: "body_delta", Status: "pending"},
		Candidates: []Event{{ID: "llmserver-focus", Kind: "body_delta", Status: "pending"}},
		State:      State{InstanceID: "llmserver-life"}, Config: config, Profile: profile,
	}
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
	if result.Error != nil || result.Stage4 == nil || result.Stage4.FocusID != "llmserver-focus" {
		t.Fatalf("llmserver native function cognition did not become the existing commit type: %#v", result)
	}
	usage := <-usageSeen
	if !usage.CostConfirmed || usage.ActualMicrousd != 124 || usage.BilledUSD != "0.000123456" ||
		usage.RequestID != "req-confirmed" || usage.BillingStatus != "confirmed" || usage.BillingPrice != "codex-terra-test-v1" {
		t.Fatalf("server bill was not preserved in the local resource fact: %#v", usage)
	}
}

func TestLLMServerUnconfirmedBillRejectsCognitionWithoutInventingSpend(t *testing.T) {
	config := testConfig(10)
	config.ModelGateway.Adapter = "llmserver"
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "none"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-unconfirmed", PulseID: 8}, Focus: Event{ID: "focus-unconfirmed"},
		Config: config, Profile: profile,
	}
	response := apiResponse{
		Model: "gpt-5.6-terra", RequestID: "req-unconfirmed", ReservedMicrousd: 900,
		Usage: apiUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	notices := make(chan WorkerNotice)
	usageSeen := make(chan UsageRecord, 1)
	go func() {
		usageNotice := <-notices
		usageSeen <- usageNotice.Payload.(UsageRecord)
		usageNotice.Ack <- NoticeAck{Accepted: true}
	}()
	err := acknowledgeUsage(context.Background(), notices, request, response)
	var failure *ModelCallError
	if !errors.As(err, &failure) || failure.Fact.Category != "billing_unconfirmed" {
		t.Fatalf("missing llmserver settlement was not a structured failure: %v", err)
	}
	usage := <-usageSeen
	if usage.ActualMicrousd != 0 || usage.CostConfirmed || usage.Status != "completed_billing_unconfirmed" || usage.FailureCategory != "billing_unconfirmed" {
		t.Fatalf("unconfirmed settlement became invented spend: %#v", usage)
	}
}

func TestLLMServerConfirmedFailedResponseSettlesCostAndRejectsCognition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-LLMServer-Request-ID", "req-failed-confirmed")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id": "response-failed", "status": "failed", "model": "codex-terra",
			"error": map[string]any{"code": "invalid_function_output", "message": "model did not form a function call"},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
			"llmserver_billing": map[string]any{
				"request_id": "req-failed-confirmed", "settlement_status": "confirmed", "price_version": "price-test",
				"currency": "USD", "charges": map[string]any{"total": "0.0000034"},
			},
		})
	}))
	defer server.Close()

	config := testConfig(20)
	config.ModelGateway.Adapter = "llmserver"
	config.ModelGateway.BaseURL = server.URL
	profile := CognitiveProfile{Model: "main", ReasoningEffort: "none"}
	request := CognitiveRequest{
		Lease: Lease{ID: "lease-failed-confirmed", Profile: profile, PulseID: 9}, Stage: 20,
		Focus:      Event{ID: "focus-failed-confirmed", Kind: "body_delta", Status: "pending"},
		Candidates: []Event{{ID: "focus-failed-confirmed", Kind: "body_delta", Status: "pending"}},
		State:      State{InstanceID: "llmserver-life"}, Config: config, Profile: profile,
	}
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
	var failure *ModelCallError
	if !errors.As(result.Error, &failure) || failure.Fact.Category != "invalid_function_output" || failure.Fact.CostStatus != "confirmed" {
		t.Fatalf("confirmed response.failed was not retained as a paid output failure: %#v", result.Error)
	}
	usage := <-usageSeen
	if usage.Status != "failed" || usage.FailureCategory != "invalid_function_output" || !usage.CostConfirmed || usage.ActualMicrousd != 4 || usage.RequestID != "req-failed-confirmed" {
		t.Fatalf("confirmed response.failed was settled incorrectly: %#v", usage)
	}
}

func TestLLMServerIdempotencyAndDecimalBillingAreStrict(t *testing.T) {
	request := CognitiveRequest{State: State{InstanceID: "alice"}, Focus: Event{ID: "focus"}, Lease: Lease{ID: "lease-one"}}
	first := llmserverIdempotencyKey(request, []byte(`{"x":1}`))
	if first != llmserverIdempotencyKey(request, []byte(`{"x":1}`)) || first == llmserverIdempotencyKey(request, []byte(`{"x":2}`)) {
		t.Fatal("llmserver idempotency key was unstable or ignored request identity")
	}
	request.Lease.ID = "lease-two"
	if first == llmserverIdempotencyKey(request, []byte(`{"x":1}`)) {
		t.Fatal("a later cognition lease reused a definitively failed llmserver request identity")
	}
	for _, value := range []struct {
		decimal string
		micro   int64
	}{{"0", 0}, {"0.000001", 1}, {"0.000001001", 2}, {"1.25", 1_250_000}} {
		got, err := decimalUSDMicrousd(value.decimal)
		if err != nil || got != value.micro {
			t.Fatalf("decimal %s -> %d, %v", value.decimal, got, err)
		}
	}
}

func TestLLMServerPreservesDefinitiveGatewayFailureWithoutIdempotencyReplay(t *testing.T) {
	requests := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		requests = append(requests, string(data))
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":{"code":"provider_start_failed","message":"provider unavailable"}}`))
	}))
	defer server.Close()
	client := NewModelClient()
	response, err := client.doGatewayRequest(context.Background(), ModelGatewayConfig{
		Adapter: "llmserver", BaseURL: server.URL,
	}, []byte(`{"same":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway || len(requests) != 1 {
		t.Fatalf("a definitive server failure was replayed into an idempotency conflict: status=%d requests=%q", response.StatusCode, requests)
	}
}

func TestLLMServerDoesNotRetryARejectedProviderToolCall(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":{"code":"invalid_provider_tool_call","message":"schema mismatch"}}`))
	}))
	defer server.Close()
	client := NewModelClient()
	response, err := client.doGatewayRequest(context.Background(), ModelGatewayConfig{
		Adapter: "llmserver", BaseURL: server.URL,
	}, []byte(`{"same":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if requests != 1 || response.StatusCode != http.StatusBadGateway || apiErrorFromData(data).Code != "invalid_provider_tool_call" {
		t.Fatalf("a deterministic invalid tool call was retried or lost: requests=%d status=%d body=%s", requests, response.StatusCode, data)
	}
}

func TestNativeFunctionResultInputPreservesOutputAndCausalResult(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`{"type":"reasoning","id":"rs_1"}`), json.RawMessage(`{"type":"function_call","call_id":"call-1","name":"mentor_send","arguments":"{\"text\":\"你好\",\"reply_to\":\"\"}"}`)}
	call := functionCall{Name: "mentor_send", CallID: "call-1", Arguments: `{"text":"你好","reply_to":""}`}
	input := functionResultInput("向导师问候", raw, call, `{"status":"sent"}`)
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"input_text", "reasoning", "function_call", "function_call_output", "call-1", "sent"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("standard tool continuation lost %q: %s", expected, encoded)
		}
	}
}

func TestSingleFunctionCallRejectsParallelOrMalformedCalls(t *testing.T) {
	valid := apiOutputItem{Type: "function_call", Name: "one", CallID: "call-1", Arguments: `{}`}
	if _, err := singleFunctionCall(apiResponse{Output: []apiOutputItem{valid, valid}}); err == nil {
		t.Fatal("parallel function calls crossed the single-consciousness boundary")
	}
	invalid := apiOutputItem{Type: "function_call", Name: "one", CallID: "", Arguments: `[]`}
	if _, err := singleFunctionCall(apiResponse{Output: []apiOutputItem{invalid}}); err == nil {
		t.Fatal("malformed function call crossed the local validation boundary")
	}
}

func TestStageFiveCommitSchemaCarriesRealityLearningFields(t *testing.T) {
	tool := cognitiveCommitTool(5, []Event{{ID: "event-now", Kind: "endogenous_change"}}, true, false, true)
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	action := cognitiveActionBranch(t, tool, "organ_action")
	actionProperties := action["properties"].(map[string]any)
	for _, field := range []string{"intent", "prediction", "reality_check", "stop_condition"} {
		if _, exists := actionProperties[field]; !exists {
			t.Fatalf("stage-five action omitted %s", field)
		}
	}
	if _, exists := properties["reality_updates"]; !exists {
		t.Fatal("stage-five commit omitted reality_updates")
	}
	memoryUpdates := properties["reality_updates"].(map[string]any)
	memoryItem := memoryUpdates["items"].(map[string]any)
	memoryProperties := memoryItem["properties"].(map[string]any)
	if _, exists := memoryProperties["method_slot"]; !exists {
		t.Fatal("stage-five commit did not let Alice choose durable method replacement")
	}
	required := parameters["required"].([]string)
	found := false
	for _, field := range required {
		found = found || field == "reality_updates"
	}
	if !found {
		t.Fatal("stage-five strict schema does not require reality_updates")
	}
}

func TestCognitiveActionSchemaSeparatesWaitingFromExecutablePayloads(t *testing.T) {
	tool := cognitiveCommitTool(8, []Event{{ID: "focus", Kind: "concern"}}, false, true, true)
	none := cognitiveActionBranch(t, tool, "none")
	noneProperties := none["properties"].(map[string]any)
	if len(noneProperties) != 1 {
		t.Fatalf("deliberate non-action carried effector payloads: %#v", noneProperties)
	}

	action := cognitiveActionBranch(t, tool, "organ_action")
	actionProperties := action["properties"].(map[string]any)
	if _, exists := actionProperties["text"]; exists {
		t.Fatalf("organ_action carried mentor payload: %#v", actionProperties)
	}
	for _, field := range []string{"organ_id", "operation", "input"} {
		if actionProperties[field].(map[string]any)["pattern"] == nil {
			t.Fatalf("organ_action did not structurally require %s: %#v", field, actionProperties[field])
		}
	}

	mentor := cognitiveActionBranch(t, tool, "mentor_send")
	mentorProperties := mentor["properties"].(map[string]any)
	if _, exists := mentorProperties["input"]; exists {
		t.Fatalf("mentor_send carried organ payload: %#v", mentorProperties)
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
	memoryUpdates := properties["reality_updates"].(map[string]any)
	memoryItem := memoryUpdates["items"].(map[string]any)
	memoryProperties := memoryItem["properties"].(map[string]any)
	if _, exists := memoryProperties["narrative_update"]; exists {
		t.Fatal("narrative update remained trapped inside the one-shot reality memory")
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
		State:  State{ValueField: LifeValueField{Activation: LifeValueVector{Exploration: 0.8}}},
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
			ValueField: LifeValueField{Activation: LifeValueVector{Exploration: 0.8}},
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
	if !requestAllowsMentorSend(request) {
		t.Fatal("an earlier message removed a normal relationship affordance")
	}
	request.State.Mentor.Outbox = []MentorMessage{{MessageID: "alice-unread", Status: "queued"}}
	if !requestAllowsMentorSend(request) {
		t.Fatal("an unread message was mistaken for an executing send")
	}
	request.State.Mentor.Outbox[0].RepliedAt = nowUTC()
	if !requestAllowsMentorSend(request) {
		t.Fatal("an actual reply did not reopen the serial mentor relationship")
	}
	request.State.Mentor.Outbox = nil
	openTool := cognitiveCommitTool(8, []Event{exploration}, false, true, requestAllowsMentorSend(request))
	kinds = cognitiveActionKinds(t, openTool)
	foundMentor := false
	foundNone = false
	for _, kind := range kinds {
		foundMentor = foundMentor || kind == "mentor_send"
		foundNone = foundNone || kind == "none"
	}
	if !foundMentor || !foundNone {
		t.Fatalf("the normal relationship affordance displaced Alice's choice: %v", kinds)
	}

	mentorFocus := Event{ID: "mentor-reply", Kind: "mentor_received"}
	mentorRequest := request
	mentorRequest.Focus = mentorFocus
	mentorRequest.Candidates = []Event{mentorFocus}
	if !requestAllowsMentorSend(mentorRequest) {
		t.Fatal("a mentor message focus could not continue the existing relationship")
	}
	mentorTool := cognitiveCommitTool(8, []Event{mentorFocus}, false, true, requestAllowsMentorSend(mentorRequest))
	foundMentor = false
	for _, kind := range cognitiveActionKinds(t, mentorTool) {
		foundMentor = foundMentor || kind == "mentor_send"
	}
	if !foundMentor {
		t.Fatal("the direct relationship focus did not expose mentor_send")
	}

	linkedPayload, _ := json.Marshal(map[string]string{"commitment_id": "mentor-first"})
	linkedReply := Event{ID: "mentor-linked-reply", Kind: "mentor_received", Payload: linkedPayload}
	linkedRequest := request
	linkedRequest.Focus = linkedReply
	linkedRequest.Candidates = []Event{linkedReply, {ID: "peripheral", Kind: "perceptual_change"}}
	if requestAllowsMentorSend(linkedRequest) {
		t.Fatal("background candidates reopened a reply while linked mentor feedback was still Reality")
	}
	linkedTool := cognitiveCommitTool(8, linkedRequest.Candidates, false, true, requestAllowsMentorSend(linkedRequest))
	linkedKinds := cognitiveActionKinds(t, linkedTool)
	if len(linkedKinds) != 1 || linkedKinds[0] != "none" {
		t.Fatalf("linked mentor feedback exposed an enactive action before its content pass: %v", linkedKinds)
	}

	contentPayload, _ := json.Marshal(map[string]string{"message_id": "codex-message-1"})
	contentTool := cognitiveCommitTool(8, []Event{{ID: "mentor-content", Kind: "mentor_content", Payload: contentPayload}}, false, true, true)
	contentAction := contentTool["parameters"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)
	for _, rawBranch := range contentAction["anyOf"].([]any) {
		properties := rawBranch.(map[string]any)["properties"].(map[string]any)
		if properties["kind"].(map[string]any)["enum"].([]string)[0] != "mentor_send" {
			continue
		}
		replies := properties["reply_to"].(map[string]any)["enum"].([]string)
		if len(replies) != 2 || replies[0] != "" || replies[1] != "codex-message-1" {
			t.Fatalf("mentor content exposed untrusted reply targets: %#v", replies)
		}
	}

	actionAssistRequest := request
	actionAssistRequest.Stage = 10
	actionAssistRequest.Lease.ProfileSource = "next"
	if requestAllowsMentorSend(actionAssistRequest) {
		t.Fatal("stage-ten action assistance exposed the mentor relationship effector")
	}

	forming := exploration
	forming.ConcernID = "forming-object"
	formingRequest := CognitiveRequest{
		Stage: 8, Focus: forming,
		State: State{
			ValueField: LifeValueField{Activation: LifeValueVector{Exploration: 0.5}},
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
	formingRequest.State.ValueField.Activation.Exploration = 0.8
	if !requestHasMatureExplorationDrive(formingRequest) {
		t.Fatal("an accumulated drive did not mark the held concern as mature")
	}
	formingRequest.State.ValueField.Activation.Exploration = 0.5
	formingRequest.State.Concerns[0].Answerability = 0.9
	if requestHasMatureExplorationDrive(formingRequest) {
		t.Fatal("semantic answerability bypassed the accumulated exploration action threshold")
	}
}

func TestModelFactPayloadKeepsTheKernelOrganAgnosticAndBounded(t *testing.T) {
	payload := json.RawMessage(`{"kind":"action_result","organ_fact":"completed","detail":"bounded"}`)
	fact := modelFactPayload(Event{Kind: "action_result", Payload: payload}, 48)
	if !strings.Contains(fact, "action_result") || strings.Contains(fact, "bounded") {
		t.Fatalf("generic Reality was not bounded without organ-specific parsing: %q", fact)
	}
}

func TestEnactedActionMemoryKeepsDistinctSettledRequests(t *testing.T) {
	memories := []Memory{
		{ActionKind: "organ_action", EnactedRequest: normalizedOrganRequest("browser", "list", `{}`), ObservedAt: "one", RemainingDifference: 0.04},
		{ActionKind: "organ_action", EnactedRequest: normalizedOrganRequest("system", "exec", "uname -a"), ObservedAt: "two", RemainingDifference: 0.03},
		{ActionKind: "organ_action", EnactedRequest: normalizedOrganRequest("browser", "list", `{}`), ObservedAt: "three", RemainingDifference: 0.02},
	}
	views := contextEnactedActionViews(memories)
	if len(views) != 2 {
		t.Fatalf("action memory length = %d, want two distinct requests", len(views))
	}
	if views[0]["enacted_request"] != normalizedOrganRequest("browser", "list", `{}`) || views[0]["observed_at"] != "three" {
		t.Fatalf("action memory did not retain the latest settled identity: %#v", views[0])
	}
}

func TestIndexedSelfViewKeepsMethodConnectedToItsCausalOrigin(t *testing.T) {
	method := "外部关系可以提供事实和边界，具体方向由我形成。"
	memories := []Memory{
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
	view := indexedSelfView(SelfState{Methods: []string{method}}, memories)
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
	view := indexedSelfView(SelfState{Methods: []string{"尚无经验锚点的方法"}}, []Memory{{MethodUpdate: "另一条方法"}})
	methods := view["methods"].([]map[string]any)
	if _, exists := methods[0]["causal_origin"]; exists {
		t.Fatalf("unmatched method received an invented origin: %#v", methods[0])
	}
}
