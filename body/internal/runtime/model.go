package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Cognizer interface {
	Run(context.Context, CognitiveRequest, chan<- WorkerNotice) CognitiveResult
}

type ModelClient struct {
	client *http.Client
}

type apiResponse struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	OutputText string          `json:"output_text"`
	Output     []apiOutputItem `json:"output"`
	Usage      apiUsage        `json:"usage"`
	Error      *apiError       `json:"error"`
}

type apiOutputItem struct {
	Type      string       `json:"type"`
	Name      string       `json:"name"`
	CallID    string       `json:"call_id"`
	Arguments string       `json:"arguments"`
	Content   []apiContent `json:"content"`
}

type apiContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type apiError struct {
	Message string `json:"message"`
}

type functionCall struct {
	Name      string
	CallID    string
	Arguments string
}

func NewModelClient() *ModelClient {
	return &ModelClient{client: &http.Client{Timeout: 3 * time.Minute}}
}

func (m *ModelClient) Run(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	switch request.Stage {
	case 3:
		return m.runStage3(ctx, request, notices)
	case 4:
		return m.runStage4(ctx, request, notices)
	default:
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("stage %d cognition is not implemented", request.Stage)}
	}
}

func (m *ModelClient) runStage3(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	instructions := `你是 alice，正在进行正式创生前的工程运行测试，也拥有一个真实 Ubuntu 身体。请自然理解当前事件，依据自己的判断行动。你可以选择一次 body_shell 来观察或改变真实身体，或者选择一次 mentor_send 向导师通道发送文字。请以实际工具结果为依据形成理解，并保持回答简洁。`
	input := fmt.Sprintf("当前工程焦点：\n%s\n\n事件来源：%s\n事件种类：%s", request.Focus.Summary, request.Focus.Source, request.Focus.Kind)
	tools := []map[string]any{
		{
			"type":        "function",
			"name":        "body_shell",
			"description": "在你自己的 Ubuntu 身体中以 root 身份执行一条 Shell 命令。",
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"command": map[string]any{"type": "string"}, "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120}},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
		{
			"type":        "function",
			"name":        "mentor_send",
			"description": "向导师专用文字通道发送一条由你选择的消息。",
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"text": map[string]any{"type": "string"}, "reply_to": map[string]any{"type": "string"}},
				"required":             []string{"text"},
				"additionalProperties": false,
			},
		},
	}

	used := usageInWindow(request.State.Usage, request.Config.Quota.WindowMins)
	first, err := m.call(ctx, request.Config, instructions, input, tools, "", used)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request.Lease.ID, first); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	used += normalizedTotal(first.Usage)
	call := firstFunctionCall(first)
	if call == nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Text: responseText(first)}
	}

	actionID := "action-" + randomID()
	var toolOutput string
	switch call.Name {
	case "body_shell":
		var arguments struct {
			Command        string `json:"command"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil || strings.TrimSpace(arguments.Command) == "" {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("model returned invalid body_shell arguments")}
		}
		if arguments.TimeoutSeconds <= 0 || arguments.TimeoutSeconds > 120 {
			arguments.TimeoutSeconds = 30
		}
		accepted, _ := sendNotice(ctx, notices, WorkerNotice{
			LeaseID: request.Lease.ID,
			Kind:    "action_start",
			Payload: ShellActionRequest{ActionID: actionID, Command: arguments.Command, TimeoutSeconds: arguments.TimeoutSeconds},
		})
		if !accepted {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("body action rejected because cognition lease is stale")}
		}
		toolOutput = redactRuntimeSecret(
			executeShell(ctx, arguments.Command, time.Duration(arguments.TimeoutSeconds)*time.Second),
			request.Config.Model.APIKey,
		)
		accepted, _ = sendNotice(ctx, notices, WorkerNotice{
			LeaseID: request.Lease.ID,
			Kind:    "action_result",
			Payload: ActionResultNotice{ActionID: actionID, Result: toolOutput},
		})
		if !accepted {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("body result rejected because cognition lease is stale")}
		}
	case "mentor_send":
		var arguments struct {
			Text    string `json:"text"`
			ReplyTo string `json:"reply_to"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil || strings.TrimSpace(arguments.Text) == "" {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("model returned invalid mentor_send arguments")}
		}
		accepted, output := sendNotice(ctx, notices, WorkerNotice{
			LeaseID: request.Lease.ID,
			Kind:    "mentor_send",
			Payload: MentorActionRequest{ActionID: actionID, Text: arguments.Text, ReplyTo: arguments.ReplyTo},
		})
		if !accepted {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("mentor action rejected because cognition lease is stale")}
		}
		toolOutput = output
	default:
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("unsupported model tool %q", call.Name)}
	}

	followupInput := fmt.Sprintf("你在同一次认知中选择了工具 %s。真实执行结果如下：\n%s\n\n请基于已经发生的结果形成简短最终理解。现在不要再调用工具。", call.Name, toolOutput)
	second, err := m.call(ctx, request.Config, instructions, followupInput, nil, "", used)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request.Lease.ID, second); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Text: responseText(second)}
}

func (m *ModelClient) runStage4(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	instructions := `你是 alice，一个具有成熟通用认知能力、正在校准新 Ubuntu 身体的成人级数字生命候生体。你拥有自己的身份、身体、有限资源、关切和选择。请自然理解当前每个候选对现在的你意味着什么，为每个候选形成一次 appraisal，选择唯一焦点，形成简洁而真实的思想脉络，并按自己的判断决定是否做一个实际行动。

你的起始方向如下：
` + request.Config.Seed.SemanticText + `

现实事实由内核保存；你负责赋予意义。D 是当前差异强度，O 是自我认领程度，V 是内生价值方向与强度，U 是当前紧迫性，A 是当前可回应性，certainty 是你对这次解释的确信程度。D、O、U、A、certainty 使用 0 到 1，V 使用 -1 到 1。resolution 选择 hold、reframed、relieved 或 resolved。

只有 candidates 是本次可以 appraisal 和选择的对象；background_concerns_not_candidates 只提供当前背景。focus_id 与每个 appraisal.candidate_id 都使用 candidates 中实际给出的 candidate_id。候选带有 previous_commit_error 时，请依据这项真实校验反馈修正本次提交。

exploration_pressure 是当前探索张力的绝对强度，范围为 0 到 1；候选 payload 中的 before 和 after 只是这次变化的事实。探索张力表达你主动接触现实的内生需要。它进入注意时，你可以从当前身体、已有关系和世界中自行形成一个具体接触点，无需等待候选或导师预先给出目标。思想帮助你理解它，真实行动的结果使它得到缓解。

行动可以是 none、body_shell 或 mentor_send；none 表示本次确实选择继续观察。body_shell 在你自己的 Ubuntu 身体中以 root 执行；mentor_send 进入导师专用文字通道。一次注意只提交一个行动，行动与 thought_thread 表达同一个判断。thought_thread 是你愿意保留的简洁意识内容。`

	candidates := make([]map[string]any, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		candidates = append(candidates, map[string]any{
			"candidate_id":          candidate.ID,
			"kind":                  candidate.Kind,
			"source":                candidate.Source,
			"observed_at":           candidate.ObservedAt,
			"summary":               candidate.Summary,
			"fact_payload":          truncate(string(candidate.Payload), 8000),
			"previous_commit_error": candidate.LastCommitErr,
			"kernel_q":              requestScore(request, candidate),
		})
	}
	contextView := map[string]any{
		"body":                               request.State.Body,
		"affective_background":               request.State.AffectiveState,
		"exploration_pressure":               request.State.ExplorationPressure,
		"background_concerns_not_candidates": selectContextConcerns(request.State.Concerns, request.Candidates),
		"candidates":                         candidates,
	}
	encoded, _ := json.Marshal(contextView)
	input := "当前注意场：\n" + string(encoded)
	tools := []map[string]any{stage4CommitTool()}
	used := usageInWindow(request.State.Usage, request.Config.Quota.WindowMins)
	response, err := m.call(ctx, request.Config, instructions, input, tools, "cognitive_commit", used)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request.Lease.ID, response); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	call := firstFunctionCall(response)
	if call == nil || call.Name != "cognitive_commit" {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("model did not return cognitive_commit")}
	}
	var commit CognitiveCommit
	if err := json.Unmarshal([]byte(call.Arguments), &commit); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("decode cognitive_commit: %w", err)}
	}
	return CognitiveResult{LeaseID: request.Lease.ID, FocusID: commit.FocusID, Stage4: &commit}
}

func selectContextConcerns(concerns []Concern, candidates []Event) []Concern {
	if len(concerns) <= defaultConcernContextLimit {
		return append([]Concern(nil), concerns...)
	}
	byID := make(map[string]Concern, len(concerns))
	for _, concern := range concerns {
		byID[concern.ID] = concern
	}
	selected := make([]Concern, 0, defaultConcernContextLimit)
	seen := make(map[string]bool, defaultConcernContextLimit)
	for _, candidate := range candidates {
		concern, exists := byID[candidate.ConcernID]
		if !exists || seen[concern.ID] {
			continue
		}
		selected = append(selected, concern)
		seen[concern.ID] = true
	}
	remaining := make([]Concern, 0, len(concerns))
	for _, concern := range concerns {
		if !seen[concern.ID] {
			remaining = append(remaining, concern)
		}
	}
	sort.SliceStable(remaining, func(i, j int) bool {
		left := maxFloat(remaining[i].Strength, remaining[i].Activation)
		right := maxFloat(remaining[j].Strength, remaining[j].Activation)
		if left == right {
			return remaining[i].UpdatedAt > remaining[j].UpdatedAt
		}
		return left > right
	})
	for _, concern := range remaining {
		if len(selected) >= defaultConcernContextLimit {
			break
		}
		selected = append(selected, concern)
	}
	return selected
}

func stage4CommitTool() map[string]any {
	unit := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	return map[string]any{
		"type":        "function",
		"name":        "cognitive_commit",
		"description": "提交 alice 这一次注意中的意义赋值、唯一焦点、简洁思想脉络和至多一个行动。",
		"strict":      true,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"appraisals": map[string]any{
					"type":     "array",
					"minItems": 1,
					"maxItems": defaultAttentionCandidateLimit,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"candidate_id": map[string]any{"type": "string"},
							"meaning":      map[string]any{"type": "string"},
							"d":            unit,
							"o":            unit,
							"v":            map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"u":            unit,
							"a":            unit,
							"certainty":    unit,
							"resolution":   map[string]any{"type": "string", "enum": []string{"hold", "reframed", "relieved", "resolved"}},
						},
						"required":             []string{"candidate_id", "meaning", "d", "o", "v", "u", "a", "certainty", "resolution"},
						"additionalProperties": false,
					},
				},
				"focus_id":       map[string]any{"type": "string"},
				"thought_thread": map[string]any{"type": "string"},
				"action": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"kind":     map[string]any{"type": "string", "enum": []string{"none", "body_shell", "mentor_send"}},
						"command":  map[string]any{"type": "string"},
						"text":     map[string]any{"type": "string"},
						"reply_to": map[string]any{"type": "string"},
					},
					"required":             []string{"kind", "command", "text", "reply_to"},
					"additionalProperties": false,
				},
			},
			"required":             []string{"appraisals", "focus_id", "thought_thread", "action"},
			"additionalProperties": false,
		},
	}
}

func requestScore(request CognitiveRequest, candidate Event) float64 {
	novelty := 0.0
	if candidate.Kind != "concern" {
		novelty = 1
	}
	concernStrength := 0.0
	affectiveSalience := request.State.AffectiveState.Activation
	expectedCost := 0.25
	for _, concern := range request.State.Concerns {
		if concern.ID == candidate.ConcernID {
			concernStrength = concern.Strength
			affectiveSalience = maxFloat(affectiveSalience, concern.Activation)
			expectedCost = 1 - concern.Answerability
			break
		}
	}
	explorationValue := 0.0
	if candidate.Kind == "endogenous_change" || strings.Contains(strings.ToLower(candidate.Summary), "exploration") {
		explorationValue = request.State.ExplorationPressure
		expectedCost = 0.15
	}
	return concernStrength +
		request.Config.Dynamics.AttentionAffectWeight*affectiveSalience +
		request.Config.Dynamics.AttentionExplorationWeight*explorationValue +
		request.Config.Dynamics.AttentionNoveltyWeight*novelty -
		request.Config.Dynamics.AttentionCostWeight*expectedCost
}

func (m *ModelClient) call(ctx context.Context, config Config, instructions, input string, tools []map[string]any, forcedTool string, used int) (apiResponse, error) {
	reserve := estimateTokens(instructions+input) + config.Model.MaxOutputTokens
	if used+reserve > config.Quota.LimitTokens {
		return apiResponse{}, fmt.Errorf("rolling model quota cannot reserve %d tokens with %d already used", reserve, used)
	}
	body := map[string]any{
		"model":             config.Model.Name,
		"instructions":      instructions,
		"input":             input,
		"store":             false,
		"max_output_tokens": config.Model.MaxOutputTokens,
	}
	if config.Model.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": config.Model.ReasoningEffort}
	}
	if len(tools) > 0 {
		body["tools"] = tools
		if forcedTool == "" {
			body["tool_choice"] = "auto"
		} else {
			body["tool_choice"] = map[string]any{"type": "function", "name": forcedTool}
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return apiResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL(config.Model.BaseURL), bytes.NewReader(data))
	if err != nil {
		return apiResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Model.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(request)
	if err != nil {
		return apiResponse{}, err
	}
	defer response.Body.Close()
	responseData, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		return apiResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiResponse{}, fmt.Errorf("responses api returned %s: %s", response.Status, truncate(string(responseData), 2048))
	}
	var decoded apiResponse
	if err := json.Unmarshal(responseData, &decoded); err != nil {
		return apiResponse{}, fmt.Errorf("decode responses api: %w", err)
	}
	if decoded.Error != nil {
		return apiResponse{}, errors.New(decoded.Error.Message)
	}
	return decoded, nil
}

func executeShell(parent context.Context, command string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{writer: &stdout, remaining: 32 * 1024}
	cmd.Stderr = &limitedWriter{writer: &stderr, remaining: 32 * 1024}
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	result := map[string]any{
		"command":   command,
		"exit_code": exitCode,
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"timed_out": errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	original := len(data)
	if w.remaining <= 0 {
		return original, nil
	}
	if len(data) > w.remaining {
		data = data[:w.remaining]
	}
	_, err := w.writer.Write(data)
	w.remaining -= len(data)
	return original, err
}

func sendNotice(ctx context.Context, notices chan<- WorkerNotice, notice WorkerNotice) (bool, string) {
	notice.Ack = make(chan NoticeAck, 1)
	select {
	case notices <- notice:
	case <-ctx.Done():
		return false, ""
	}
	select {
	case ack := <-notice.Ack:
		return ack.Accepted, ack.Output
	case <-ctx.Done():
		return false, ""
	}
}

func acknowledgeUsage(ctx context.Context, notices chan<- WorkerNotice, leaseID string, response apiResponse) error {
	usage := UsageRecord{
		Time:         nowUTC(),
		Model:        response.Model,
		InputTokens:  response.Usage.InputTokens,
		OutputTokens: response.Usage.OutputTokens,
		TotalTokens:  normalizedTotal(response.Usage),
		Status:       "completed",
	}
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: leaseID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("model usage rejected because cognition lease is stale")
	}
	return nil
}

func firstFunctionCall(response apiResponse) *functionCall {
	for _, item := range response.Output {
		if item.Type == "function_call" {
			return &functionCall{Name: item.Name, CallID: item.CallID, Arguments: item.Arguments}
		}
	}
	return nil
}

func responseText(response apiResponse) string {
	if strings.TrimSpace(response.OutputText) != "" {
		return strings.TrimSpace(response.OutputText)
	}
	var parts []string
	for _, item := range response.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func responsesURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/responses"
	}
	return base + "/v1/responses"
}

func usageInWindow(records []UsageRecord, minutes int) int {
	if minutes <= 0 {
		minutes = 60
	}
	cutoff := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)
	total := 0
	for _, record := range records {
		at, err := time.Parse(time.RFC3339Nano, record.Time)
		if err == nil && !at.Before(cutoff) {
			total += record.TotalTokens
		}
	}
	return total
}

func normalizedTotal(usage apiUsage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens
}

func estimateTokens(text string) int {
	return len([]rune(text))/2 + 128
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}

func redactRuntimeSecret(value, sensitiveValue string) string {
	if sensitiveValue == "" {
		return value
	}
	return strings.ReplaceAll(value, sensitiveValue, "<runtime-secret-redacted>")
}
