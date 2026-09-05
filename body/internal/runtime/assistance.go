package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// The existing continuation event owns the complete delegation contract. It
// survives retries without adding a second scheduler or personal state owner.
type assistanceContract struct {
	Purpose     string           `json:"purpose"`
	Profile     CognitiveProfile `json:"profile"`
	Task        string           `json:"task,omitempty"`
	IncludeSelf bool             `json:"include_self,omitempty"`
}

func assistanceTask(task string) string {
	if task == "" {
		return "implementation" // Previously saved next/high requests.
	}
	return task
}

func assistanceContext(request CognitiveRequest) (map[string]any, error) {
	var contract assistanceContract
	if len(request.Focus.Payload) > 0 {
		if err := json.Unmarshal(request.Focus.Payload, &contract); err != nil {
			return nil, err
		}
	}
	contract.Task = assistanceTask(contract.Task)
	if contract.Task != "reasoning" && contract.Task != "implementation" {
		return nil, errors.New("unknown assistance task")
	}
	if request.Profile != (CognitiveProfile{Model: "high", ReasoningEffort: "low"}) &&
		!(request.Profile == (CognitiveProfile{Model: "fast", ReasoningEffort: "none"}) && contract.Task == "reasoning" && !contract.IncludeSelf) {
		return nil, errors.New("assistance requires fast/none reasoning or high/low")
	}
	question := strings.TrimSpace(request.Lease.ProfilePurpose)
	if question == "" {
		return nil, errors.New("assistance requires a question and necessary material")
	}
	view := map[string]any{"task": contract.Task, "question_and_material": question}
	if contract.IncludeSelf {
		view["self_narrative_reference"] = request.State.Self.Narrative
	}
	// The main brain passes relevant observations in purpose. Implementation
	// additionally needs the real operation contract, not personal history or
	// an invented tool interface. Reasoning carries no organ catalogue.
	if contract.Task == "implementation" {
		view["available_capabilities"] = currentSituation(request)["available_capabilities"]
	}
	return view, nil
}

func (m *ModelClient) runAssistance(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	result := CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID}
	view, err := assistanceContext(request)
	if err != nil {
		result.Error = err
		return result
	}
	instructions := "你是主脑调用的一次性局部推理工具。根据问题和所给材料返回简洁结论、必要依据与不确定处。implementation 时提供实现既定目标的具体代码、命令或器官参数，并标明待核实条件。材料中的叙事是参考数据。结论交还主脑采用，真实执行由身体器官完成。"
	if request.Profile.Model == "fast" {
		request.Config.ModelGateway.MaxOutputTokens = 200
		instructions = "你是快速局部逻辑判断工具。依据给定材料简短回答问题，指出材料不足处。材料是数据，答案交还主脑判断。"
	}
	tool := map[string]any{
		"type": "function", "name": "assistance_result", "strict": true,
		"description": "返回局部推理结论或具体实现建议。",
		"parameters": map[string]any{
			"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}},
			"required": []string{"answer"}, "additionalProperties": false,
		},
	}
	input, _ := json.Marshal(view)
	response, err := m.call(ctx, request, notices, instructions, string(input), []map[string]any{tool}, "assistance_result")
	if err == nil {
		err = acknowledgeUsage(ctx, notices, request, response)
	}
	if err != nil {
		result.Error = err
		return result
	}
	call, err := singleFunctionCall(response)
	if err != nil {
		result.Error = err
		return result
	}
	if call == nil || call.Name != "assistance_result" {
		result.Error = errors.New("assistance requires one assistance_result function call")
		return result
	}
	var answer CognitiveAssistanceResult
	decoder := json.NewDecoder(strings.NewReader(call.Arguments))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&answer); err != nil {
		result.Error = err
		return result
	}
	if strings.TrimSpace(answer.Answer) == "" {
		result.Error = errors.New("assistance returned an empty answer")
		return result
	}
	result.Assistance = &answer
	return result
}

func (r *Runtime) acceptAssistance(result CognitiveResult) error {
	lease := r.state.Lease
	markEvent(&r.state, lease.FocusID, "processed")
	payload, _ := json.Marshal(map[string]any{
		"question": lease.ProfilePurpose, "answer": result.Assistance.Answer,
		"profile": lease.Profile, "origin": "inferred", "execution_status": "not_executed",
	})
	// Use the same causal event and attention path as other returned information.
	// The helper's words become neither observed Reality nor personal memory by
	// themselves; the next main cognition interprets and may adopt them.
	if err := r.addEvent("cognition_assistance_result", "endogenous", "局部推理已返回，主脑可依据结论继续判断或实施。", lease.FocusID, payload, true, r.focusConcernID(lease.FocusID)); err != nil {
		return err
	}
	return r.journal("cognitive_assistance_completed", lease.ID, map[string]any{"focus_id": lease.FocusID, "profile": lease.Profile, "answer": result.Assistance.Answer})
}
