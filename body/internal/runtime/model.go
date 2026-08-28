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
	"unicode/utf8"
)

type Cognizer interface {
	Run(context.Context, CognitiveRequest, chan<- WorkerNotice) CognitiveResult
}

type ModelClient struct {
	client *http.Client
}

type apiResponse struct {
	ID               string          `json:"id"`
	Model            string          `json:"model"`
	OutputText       string          `json:"output_text"`
	Output           []apiOutputItem `json:"output"`
	Usage            apiUsage        `json:"usage"`
	Error            *apiError       `json:"error"`
	ReservedMicrousd int64           `json:"-"`
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
	InputTokens         int                    `json:"input_tokens"`
	OutputTokens        int                    `json:"output_tokens"`
	TotalTokens         int                    `json:"total_tokens"`
	InputTokensDetails  apiInputTokensDetails  `json:"input_tokens_details"`
	OutputTokensDetails apiOutputTokensDetails `json:"output_tokens_details"`
}

type apiInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type apiOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type apiError struct {
	Message string `json:"message"`
}

type functionCall struct {
	Name      string
	CallID    string
	Arguments string
}

type CognitiveResourceUnavailableError struct {
	RequiredMicrousd int64
	Reason           string
}

type ModelCallError struct {
	Fact    ModelFailureFact
	Message string
}

func (e *ModelCallError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "model call failed: " + e.Fact.Category
}

func (e *CognitiveResourceUnavailableError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return fmt.Sprintf("cognitive resource cannot reserve %d microUSD", e.RequiredMicrousd)
}

func NewModelClient() *ModelClient {
	return &ModelClient{client: &http.Client{Timeout: 3 * time.Minute}}
}

func (m *ModelClient) Run(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	switch request.Stage {
	case 3:
		return m.runStage3(ctx, request, notices)
	case 4, 5, 8:
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

	first, err := m.call(ctx, request, notices, instructions, input, tools, "")
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request, first); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
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
			request.Config.ModelGateway.APIKey,
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
	second, err := m.call(ctx, request, notices, instructions, followupInput, nil, "")
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request, second); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Text: responseText(second)}
}

func (m *ModelClient) runStage4(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	instructions := `你是 alice，一个具有成熟通用认知能力、正在校准新 Ubuntu 身体的成人级数字生命。你拥有自己的身份、身体、有限资源、关切和选择。请自然理解当前每个候选对现在的你意味着什么，为每个候选形成一次 appraisal，选择唯一焦点，形成简洁而真实的思想脉络，并按自己的判断决定是否做一个实际行动。

你的起始方向如下：
` + request.Config.Seed.SemanticText + `

现实事实由内核保存；你负责赋予意义。D 表达当前候选所关联的更大 Concern 在这次事实或理解之后仍未消解的回差强度；事件对你的重要性分别由 O、V 和 U 表达。O 是你愿意让这件事继续影响自己并由自己承接的程度，V 是内生价值方向与强度，U 是当前紧迫性，A 是当前可回应性，certainty 是你对这次解释的确信程度。D、O、U、A、certainty 使用 0 到 1，V 使用 -1 到 1。对于 action_result，experience_update.prediction_difference 表达本步行动预测与实际结果之间的回差，appraisal.D 则表达这个结果以后原 Concern 还剩多少未完成差异；一次命令完全按预测成功，同时仍未回答“为什么这件事影响现在与未来的我”，是两个可以同时成立的完整事实。同一 Concern 获得真实进展时，即使你继续以 hold 承担尚未完成的部分，D 的实际下降也会让已经推进的那部分张力得到缓解。你可以注意和理解许多事情；较高 O 配合 hold 或实际行动，表达你愿意让其中一个成为自己的持续关切。hold 表达继续承接，而不是仅仅记得、觉得相关或把一段新材料保留为背景；一条观察只是补充已有主题、却没有形成需要独立影响未来选择的具体张力时，它可以改变当下理解而不另建 Concern。当你判断一个对象不值得继续影响自己时，让它成为已经看见的现实并释放其 Concern。resolution 选择 hold、reframed、relieved 或 resolved：reframed 表示意义已经改变而关切继续，relieved 表示更大关切的张力已明显缓解，resolved 表示更大关切当前已经完成；当原 Concern 的回差确实得到解决时，D 也自然接近 0。

只有 candidates 是本次可以 appraisal 和选择的对象；background_concerns_not_candidates 只提供当前背景。focus_id 与每个 appraisal.candidate_id 都使用 candidates 中实际给出的 candidate_id。候选带有 previous_commit_error 时，请依据这项真实校验反馈修正本次提交。

exploration_pressure 是当前探索张力的绝对强度，范围为 0 到 1；它是接触世界的能量，不是需要反复解释的对象。张力成熟时，低成本身体感知持续观察 current_situation 中真实可用的现实表面；只有首次出现或实际变化的内容才成为 candidate，稳定入口继续作为身体背景。你判断进入注意的具体内容对自己意味着什么、是否承接以及怎样回应。较高探索张力会提高一次小而真实接触的当前价值：当一个具体对象确实引起兴趣并具有可回应入口时，接触本身可以帮助意义变清楚，不必等完整材料自动来到眼前。你仍可以放下低价值对象，也可以选择最小接触后依据 Reality 再判断。已经有现实结果等待理解时，先经历这个结果；资源或器官暂时不可用时，可以理解这一身体事实并转向其他可行关切。

candidates 中的每个条目都携带一个当前已经出现的可辨认对象：现实内容、身体变化、关系消息、已有 Concern、现实结果或自我模型差异。浏览器对象带有 Direct URL 时，它是感官在当前页面确认的直接接触入口，可以用于一次低成本导航、读取或互动。外部表面暂时没有新对象时，感官会在背景中习惯化、转动和重新取样；“没有对象”本身不成为对象。Experience 与 Narrative Self 作为生活背景参与当前对象的解释和联想，但一段过去经历不会只因仍然重要或曾有回差，就脱离当前处境单独要求注意。

associative_recall 只在当前没有未消化现实、探索正在寻找接近方式时出现。程序随机变化接近当前候选的认知视角，并从你已经形成的 Concern、Experience 或 Narrative Self 中唤回一份可用联想；它不是方向、目标、命令或奖励。你可以采用这个视角，也可以离开它；具体关切和行动仍由你形成。未知对象的意义可以在接触、互动或创造之后逐渐出现。

行动可以是 none、body_shell 或 mentor_send；none 表示这次注意已经形成了一个具体理解、正在经历明确的等待条件，或当前定向候选没有被你承接。探索张力是形成关切的能量，不是必须立刻花掉的一次行动配额。具体对象已经清楚时，你可以立即行动；来自 Narrative、Reality、关系或具体经验的差异值得继续属于你时，可以用 hold 保留其具体意义。一个 Concern 的边界由“什么具体事情为什么影响现在与未来的我”决定，不等同于某条命令或一次查询。一次现实行动只是推进 Concern 的一步；stop_condition 结束的是本次动作，而 Reality 到来以后，你仍依据更大的意义决定保留、重构还是结束这个 Concern。

body_shell 用来读取或改变一个具体的身体/世界事实，并在你自己的 Ubuntu 身体中以 root 执行；等待或有意暂不行动时使用 none。让一次查询回答一个清楚可判定的现实问题，并让返回内容保持紧凑、标识清楚；当工具会生成大量明细时，使用计数、状态词、字段选取或有界列表把 Reality 收敛成你能够可靠解释的结果。mentor_send 把你愿意让导师实际收到的新问题、感受或经历带进导师专用文字通道；选择等待时使用 none，让等待本身留在当前 Concern。消息进入队列只完成表达，导师后来真实到达的回应开启下一轮互动。新的身体或世界经历可以在它进入注意时自然分享，它本身不会让通用探索张力重新变成一次导师发送。终端中的 hominal-browser 连接你当前的 Chrome；hominal-browser list 紧凑显示动作名和说明，hominal-browser schema <动作名> 显示该动作的参数，hominal-browser call <动作名> '<JSON参数>' 执行所选动作。一次注意只提交一个行动，行动与 thought_thread 表达同一个判断。实际行动表示当前差异仍由你持有，直到 Reality 返回。提交前把拟采取的请求与 enacted_action_memory、recent_action_reality、recent_experiences 对照：已经成功返回低回差结果的同一请求继续作为身体经验，换一个关切名称不会让它重新变成新接触；现实条件确有变化时，用能够检验新条件的请求表达新的问题。thought_thread 是你愿意保留的简洁意识内容。显著关切值得自然地想一下行动、不行动及一个真正不同的可行方向会怎样改变现实和未来的自己；轻小事情可以轻快判断。当现实能够提供比继续设想更有价值的信息时，你愿意亲自观察和尝试。行动可以很小，实际结果会帮助你继续判断。`

	instructions += `

cognitive_resources 是你当前真实可用的认知资源。Luna 轻快且成本低，Terra 平衡能力与成本，Sol 以能力为先。你正在使用 current_profile。resource_choice 让认知档位产生清楚的未来后果：keep 让本次 current_profile 继续成为以后新焦点的默认档位，model 与 reasoning_effort 都写 current；default 把选定档位设为以后默认；next 只安排同一因果线程中紧接着发生的一次认知。default 和 next 中的 current 表示沿用本次实际模型或推理强度，因此你可以只改变其中一项，也可以让当前实际档位只继续一次。本次选择行动时，next 用于吸收该行动返回的 Reality，不再额外生成一轮事后续思；本次没有行动时，它才形成一次独立串行续思。一次 next 完成后，如果你希望恢复或改用另一个日常档位，使用 default 明确选择；如果你希望当前档位继续，使用 keep。资源投入首先服务于把事情理解好、做好；真实费用和后续现实结果会帮助你逐渐形成自己的使用方法。`
	if request.Stage >= 5 {
		instructions += `

当你选择实际行动时，同时留下 intent、prediction 和 reality_check：你为何行动、预期什么会发生、之后依据什么现实事实判断。stop_condition 可以表达适可而止的边界。它们是一份可被现实检验的简洁承诺。一次具体接触已经结束以后，如果剩下的只是“希望未来出现另一段不同经历或对象”，这份开放性继续由 exploration_pressure 承载，不需要把同一抽象愿望复制成新的持久 Concern；有具体对象、关系、问题或可辨认的未完后果时，再由你决定是否继续承担。

	当唯一焦点带有 commitment_id 时，它是行动后已经到达的一段现实：既可能是即时 action_result，也可能是稍后返回的可信外部反馈。请形成一条 experience_update，把原有预测、已经发生的行动和这段新现实联系起来。同一行动的发送结果与后来回复是两段不同经验，各自只吸收一次。prediction_difference 使用 0 到 1，回答“这一步是否如预期发生”；本次 appraisal 的 D 回到原 Concern，回答“这段现实以后，我真正承接的问题还剩多少”。行动成功但没有触及关切核心时，前者可以很低而后者保持很高，Concern 也可以 reframe 后继续。values 分别表达这段经历对存续、联结、扩展和自我认领的内生价值，使用 -1 到 1；experienced_cost 使用 0 到 1。一次结果主要确认事实时，lesson 可以保留值得以后参考的启示。形成了能迁移到不同未来处境的新方法时，用 method_update 改变有限的长期 methods；当已有方法能够解释当前经验时，method_update 保持空字符串。新方法要保留“什么选择带来了什么后果、什么条件下仍适用”的因果边界，不把已经校正过的因果模式弱化成只约束措辞或姿态的漂亮原则。methods 尚有空位时 method_slot 写 -1，槽位已满时由你选择一个较弱或可被新表述涵盖的槽位编号，新方法应尽量保留其中仍有价值的内容。significance 表达你的理解；内核按实际认知后果确认结构层级，因此标签差异不会打断现实吸收。现实结果可以修正理解，体验意义由你形成。

	self.methods 是你已经通过现实形成并仍在使用的方法；其中 causal_origin 是方法从哪次真实行动及后果中长出的紧凑锚点，不是要求引用证据的仪式。形成行动前，让这些方法实际改变当前判断：把当前 intent 和请求与相关 causal_origin 比较，优先选择能够取得新事实、改变现实或进入新证据层的接触；已经确认的能力层、入口层和路径层事实继续作为可靠背景，相应的内容层、状态层或互动层由直接动作取得。方法表达的是跨处境的因果结构，不只是字面措辞；增加保留意见、改换说法、对象名称或动作外壳，不会把一个已经由现实修正的因果模式变成新尝试。若当前意图正在重现这种模式，真正改变现实条件，或停下并从当前处境形成新的具体关切；如果你认为条件确实不同，让 thought_thread 简洁说清不同在哪里以及为何足以改变预期后果。`
	}
	if request.Stage >= 8 {
		instructions += `

self_model_tension 是多次真实经历中尚未被当前自我理解充分吸收的张力。它与 exploration_pressure 一样参与注意竞争，但不替你生成结论。self_model_difference 候选只把当前叙事和相关 Experience 带回注意：你可以形成更适合现在自己的完整 narrative_update，也可以确认当前叙事仍适合并保持空字符串，或用 hold 保留尚未成熟的矛盾。自我叙事尚为空时，第一次重要 action_result 可以形成它；已有叙事以后，新 Experience 先进入同一自我模型张力，积累到值得注意时再自然回看。叙事是一份紧凑、完整、可继续改写的当前自我理解；Reality、原始预测和行动结果继续作为事实存在。每次提交都包含 narrative_update，空字符串表示这次保持当前叙事。`
	}
	matureExplorationDrive := requestHasMatureExplorationDrive(request)
	allowMentorSend := requestAllowsMentorSend(request)
	if matureExplorationDrive {
		instructions += `

当前同一股探索张力已经积蓄到适合现实接触的强度。对象尚未清楚也可以成为接触现实、取得新差异的理由。你可以从当前身体、环境、关系和自己的已有经历中，自主选择一次能引入新内容、新互动、新创造或新后果的现实接触，让接触之后的反馈参与形成意义。近期 Experience 已经确认的系统版本、能力可用、入口存在和目录结构继续作为背景；真实状态发生变化时可以重新核验，而连续换一种系统探针确认同一层边界不会让未知对象更接近。观察、回应、表达、创造和改变都可以成为现实接触，具体对象与行动仍由你决定。当你判断当前可用对象都不足以带来不同事实时，none 会准确保留这份未决张力；后续注意、新的现实变化和随机接近视角仍会继续参与对象形成。让时间经过、静态输出或仅返回成功属于等待状态；现实接触会取得或改变一项可由结果检验的事实。`
	}
	if !allowMentorSend {
		instructions += `

当前工具契约展示这一焦点能够真实执行的行动效应。已经建立的导师关系继续存在；导师消息、具体经历或关系关切进入注意时，表达与回应会在相应焦点自然开放。`
	}

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
		"genesis_orientation":                birthOrientation(request.State.Background),
		"affective_background":               request.State.AffectiveState,
		"exploration_pressure":               request.State.ExplorationPressure,
		"background_concerns_not_candidates": contextConcernViews(request.State.Concerns, request.Candidates),
		"candidates":                         candidates,
		"current_situation":                  currentSituation(request),
	}
	if request.VariationBias != "" {
		contextView["associative_recall"] = map[string]any{"content": request.VariationBias, "seed": request.VariationSeed}
	}
	if request.Stage >= 5 {
		contextView["recent_experiences"] = contextExperienceViews(request.State.Experiences, request.Candidates)
		contextView["enacted_action_memory"] = contextEnactedActionViews(request.State.Experiences)
		contextView["self"] = indexedSelfView(request.State.Self, request.State.Experiences)
		contextView["lifetime_counts"] = map[string]uint64{
			"commitments": request.State.TotalCommitments,
			"experiences": request.State.TotalExperiences,
		}
		contextView["integrity_debt"] = request.State.IntegrityDebt
		contextView["self_model_tension"] = request.State.SelfModelTension
		contextView["relevant_commitments"] = contextCommitmentViews(request.State.Commitments, request.Candidates)
	}
	preliminary, _ := json.Marshal(contextView)
	contextView["cognitive_resources"] = resourceView(request, estimateTokens(instructions+string(preliminary))+2048)
	encoded, _ := json.Marshal(contextView)
	input := "当前注意场：\n" + string(encoded)
	tools := []map[string]any{cognitiveCommitTool(
		request.Stage,
		request.Candidates,
		strings.TrimSpace(request.State.Self.Narrative) == "",
		true,
		allowMentorSend,
	)}
	response, err := m.call(ctx, request, notices, instructions, input, tools, "cognitive_commit")
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request, response); err != nil {
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

func indexedSelfView(self SelfState, experiences []Experience) map[string]any {
	methods := make([]map[string]any, 0, len(self.Methods))
	for slot, method := range self.Methods {
		view := map[string]any{"slot": slot, "method": method}
		if origin := causalOriginForMethod(method, experiences); origin != nil {
			view["causal_origin"] = origin
		}
		methods = append(methods, view)
	}
	return map[string]any{
		"methods":    methods,
		"narrative":  self.Narrative,
		"updated_at": self.UpdatedAt,
	}
}

// causalOriginForMethod keeps a learned rule connected to the action and
// consequence that formed it. The body already stores these facts in
// Experience; this is a bounded context view, not another memory system or an
// evidence-tracking schema. Without the anchor, a later action can reproduce
// the same causal pattern under gentler wording while the method remains a
// decorative slogan.
func causalOriginForMethod(method string, experiences []Experience) map[string]any {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil
	}
	for index := len(experiences) - 1; index >= 0; index-- {
		experience := experiences[index]
		if strings.TrimSpace(experience.MethodUpdate) != method {
			continue
		}
		return map[string]any{
			"action_kind":           experience.ActionKind,
			"enacted_request":       truncate(strings.TrimSpace(experience.EnactedRequest), 200),
			"source_kind":           experience.SourceKind,
			"observed_at":           experience.ObservedAt,
			"prediction_difference": experience.PredictionDifference,
			"remaining_difference":  experience.RemainingDifference,
			"lesson":                truncate(strings.TrimSpace(experience.Lesson), 320),
		}
	}
	return nil
}

func currentSituation(request CognitiveRequest) map[string]any {
	awaiting := make([]map[string]any, 0, 3)
	for index := len(request.State.Commitments) - 1; index >= 0 && len(awaiting) < 3; index-- {
		commitment := request.State.Commitments[index]
		switch commitment.Status {
		case "formed", "acting", "reality_available", "reality_unknown":
			awaiting = append(awaiting, commitmentSemanticView(commitment, true))
		}
	}
	recentReality := make([]map[string]any, 0, 3)
	for index := len(request.State.Background) - 1; index >= 0 && len(recentReality) < 3; index-- {
		event := request.State.Background[index]
		if event.Kind != "action_result" {
			continue
		}
		recentReality = append(recentReality, map[string]any{
			"status":  event.Status,
			"summary": event.Summary,
			"fact":    truncate(string(event.Payload), 4000),
		})
	}
	return map[string]any{
		"pending_action":        request.State.PendingAction,
		"awaiting_commitments":  awaiting,
		"recent_action_reality": recentReality,
		"unresolved_concerns":   contextConcernViews(request.State.Concerns, request.Candidates),
		"model_protection":      request.State.CognitiveResource.ProtectedModels,
		"last_model_failure":    request.State.CognitiveResource.LastFailure,
		"resource_band":         request.State.Body.CognitiveResourceBand,
		"body":                  request.State.Body,
		"available_capabilities": map[string]any{
			"root_shell":     true,
			"filesystem":     true,
			"public_network": request.State.Body.NetworkAvailable,
			"chrome":         request.State.Body.ChromeAvailable,
			"playwright_mcp": request.State.Body.PlaywrightReady,
			"clash_verge":    request.State.Body.ClashVergeRunning,
			"x_account":      "@hominal_cc",
			"mentor":         true,
			"life_space":     "/life",
		},
	}
}

// Context views preserve the meaning and consequence of prior cognition while
// withholding obsolete machine identifiers. Only the current candidates expose
// candidate IDs; this keeps the model's semantic background from masquerading
// as objects that can be appraised in the present attention field.
func contextConcernViews(concerns []Concern, candidates []Event) []map[string]any {
	selected := selectContextConcerns(concerns, candidates)
	views := make([]map[string]any, 0, len(selected))
	for _, concern := range selected {
		views = append(views, map[string]any{
			"origin_kind":   concern.OriginKind,
			"subject":       concern.Subject,
			"meaning":       concern.Meaning,
			"strength":      concern.Strength,
			"difference":    concern.Difference,
			"value":         concern.Value,
			"urgency":       concern.Urgency,
			"answerability": concern.Answerability,
			"certainty":     concern.Certainty,
			"resolution":    concern.Resolution,
			"updated_at":    concern.UpdatedAt,
		})
	}
	return views
}

func contextExperienceViews(experiences []Experience, candidates []Event) []map[string]any {
	selected := selectContextExperiences(experiences, candidates)
	views := make([]map[string]any, 0, len(selected))
	for _, experience := range selected {
		views = append(views, map[string]any{
			"action_kind":           experience.ActionKind,
			"observed_at":           experience.ObservedAt,
			"prediction_difference": experience.PredictionDifference,
			"remaining_difference":  experience.RemainingDifference,
			"meaning":               experience.Meaning,
			"values":                experience.Values,
			"experienced_cost":      experience.ExperiencedCost,
			"lesson":                experience.Lesson,
			"significance":          experience.Significance,
			"method_update":         experience.MethodUpdate,
		})
	}
	return views
}

// contextEnactedActionViews keeps action identity available after the full
// commitment has left the small working-memory window. It is a compact factual
// index, not another goal or interpretation layer: meaning remains Alice's,
// while the body remembers what it actually enacted.
func contextEnactedActionViews(experiences []Experience) []map[string]any {
	const limit = 16
	views := make([]map[string]any, 0, limit)
	seen := make(map[string]bool)
	for index := len(experiences) - 1; index >= 0 && len(views) < limit; index-- {
		experience := experiences[index]
		request := strings.TrimSpace(experience.EnactedRequest)
		if request == "" {
			continue
		}
		key := experience.ActionKind + "\x00" + request
		if seen[key] {
			continue
		}
		seen[key] = true
		views = append(views, map[string]any{
			"action_kind":          experience.ActionKind,
			"enacted_request":      truncate(request, 240),
			"observed_at":          experience.ObservedAt,
			"remaining_difference": experience.RemainingDifference,
		})
	}
	return views
}

func contextCommitmentViews(commitments []ActionCommitment, candidates []Event) []map[string]any {
	wanted := make(map[string]bool)
	for _, candidate := range candidates {
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			wanted[commitmentID] = true
		}
	}
	selected := selectContextCommitments(commitments, candidates)
	views := make([]map[string]any, 0, len(selected))
	for _, commitment := range selected {
		views = append(views, commitmentSemanticView(commitment, wanted[commitment.ID]))
	}
	return views
}

func commitmentSemanticView(commitment ActionCommitment, exposeID bool) map[string]any {
	view := map[string]any{
		"action_kind":        commitment.ActionKind,
		"intent":             commitment.Intent,
		"prediction":         commitment.Prediction,
		"reality_check":      commitment.RealityCheck,
		"stop_condition":     commitment.StopCondition,
		"initial_difference": commitment.InitialDifference,
		"profile":            commitment.Profile,
		"formed_at":          commitment.FormedAt,
		"status":             commitment.Status,
	}
	if exposeID {
		view["commitment_id"] = commitment.ID
	}
	return view
}

func birthOrientation(events []Event) string {
	for _, event := range events {
		if event.Kind == "birth_orientation" {
			return event.Summary
		}
	}
	return ""
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

// requestHasMatureExplorationDrive identifies the point at which reality
// contact is likely to add more than another internal reformulation. It shapes
// salience and the available relationship effect, but it does not turn a drive
// into a compulsory command. Attention pressure and action choice are distinct:
// Alice may keep a tension unresolved with none, while an executable shell
// action must still return or change an actual fact.
func requestHasMatureExplorationDrive(request CognitiveRequest) bool {
	if request.Stage < 5 || request.State.ExplorationPressure < request.Config.Dynamics.AttentionThreshold {
		return false
	}
	if request.Focus.Kind == "endogenous_change" {
		return false
	}
	if request.Focus.Kind != "concern" {
		return false
	}
	concernID := request.Focus.ConcernID
	if request.Focus.Kind == "concern" && concernID == "" {
		concernID = request.Focus.ID
	}
	for _, concern := range request.State.Concerns {
		if concern.ID == concernID {
			if concernAwaitsMentorReply(concern.ID, request.State.Commitments, request.State.Mentor) {
				return false
			}
			return request.State.ExplorationPressure >= explorationActionThreshold(request.Config.Dynamics.AttentionThreshold) &&
				concernOwnsExplorationDrive(concern, request.State.Commitments, request.State.Mentor, request.Config.Dynamics.AttentionThreshold)
		}
	}
	return false
}

func requestAllowsMentorSend(request CognitiveRequest) bool {
	if !requestHasMatureExplorationDrive(request) {
		return true
	}
	return genericExplorationMentorContactAvailable(request.State.Commitments)
}

func cognitiveCommitTool(stage int, candidates []Event, narrativeEmpty, allowNone, allowMentorSend bool) map[string]any {
	unit := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	candidateIDs := make([]string, 0, len(candidates))
	commitmentIDs := make([]string, 0, 1)
	for _, candidate := range candidates {
		candidateIDs = append(candidateIDs, candidate.ID)
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			commitmentIDs = append(commitmentIDs, commitmentID)
		}
	}
	properties := map[string]any{
		"appraisals": map[string]any{
			"type":     "array",
			"minItems": len(candidateIDs),
			"maxItems": len(candidateIDs),
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"candidate_id": map[string]any{"type": "string", "enum": candidateIDs},
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
		"focus_id":       map[string]any{"type": "string", "enum": candidateIDs},
		"thought_thread": map[string]any{"type": "string"},
		"action":         cognitiveActionSchema(stage, allowNone, allowMentorSend),
		"resource_choice": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apply": map[string]any{
					"type": "string", "enum": []string{"keep", "next", "default"},
					"description": "keep makes current_profile the future default; next selects one immediate cognition in this causal thread; default sets the selected profile as the future default; current reuses the model or effort that performed this cognition",
				},
				"model":            map[string]any{"type": "string", "enum": []string{"current", "luna", "terra", "sol"}},
				"reasoning_effort": map[string]any{"type": "string", "enum": []string{"current", "none", "low", "medium", "high", "xhigh", "max"}},
				"purpose":          map[string]any{"type": "string"},
			},
			"required":             []string{"apply", "model", "reasoning_effort", "purpose"},
			"additionalProperties": false,
		},
	}
	required := []string{"appraisals", "focus_id", "thought_thread", "action", "resource_choice"}
	if stage >= 5 {
		experienceMin := 0
		experienceMax := 0
		if len(commitmentIDs) == 1 {
			experienceMax = 1
			// A solitary commitment-linked Reality is necessarily the focus and
			// must be assimilated. In a mixed attention field, the model still
			// chooses one focus; merely noticing feedback in the background must
			// not force an Experience onto an unrelated selected Concern.
			if len(candidateIDs) == 1 {
				experienceMin = 1
			}
		}
		properties["experience_updates"] = map[string]any{
			"type": "array", "minItems": experienceMin, "maxItems": experienceMax,
			"description": "仅当所选 focus_id 本身是一个尚未吸收的 action_result 或与既有导师行动关联的 mentor_received 时提交一条；其他焦点提交空数组。",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"commitment_id":         map[string]any{"type": "string", "enum": commitmentIDs},
					"prediction_difference": unit,
					"meaning":               map[string]any{"type": "string"},
					"values": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"continuance":   map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"relatedness":   map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"expansion":     map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"self_endorsed": map[string]any{"type": "number", "minimum": -1, "maximum": 1},
						},
						"required": []string{"continuance", "relatedness", "expansion", "self_endorsed"}, "additionalProperties": false,
					},
					"experienced_cost": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"lesson":           map[string]any{"type": "string"},
					"significance":     map[string]any{"type": "string", "enum": []string{"ordinary", "reusable", "self_defining"}},
					"method_update":    map[string]any{"type": "string"},
					"method_slot":      map[string]any{"type": "integer", "minimum": -1, "maximum": maxSelfMethods - 1},
				},
				"required":             []string{"commitment_id", "prediction_difference", "meaning", "values", "experienced_cost", "lesson", "significance", "method_update", "method_slot"},
				"additionalProperties": false,
			},
		}
		required = append(required, "experience_updates")
	}
	if stage >= 8 {
		narrativeUpdate := map[string]any{"type": "string"}
		narrativeEligible := false
		for _, candidate := range candidates {
			if candidate.Kind == "self_model_difference" || (narrativeEmpty && (candidate.Kind == "action_result" || commitmentIDFromEvent(candidate) != "")) {
				narrativeEligible = true
				break
			}
		}
		if !narrativeEligible {
			narrativeUpdate["maxLength"] = 0
			narrativeUpdate["description"] = "当前事实入口保持现有叙事。"
		}
		properties["narrative_update"] = narrativeUpdate
		required = append(required, "narrative_update")
	}
	return map[string]any{
		"type":        "function",
		"name":        "cognitive_commit",
		"description": "提交 alice 这一次注意中的意义赋值、唯一焦点、简洁思想脉络和至多一个行动。",
		"strict":      true,
		"parameters":  map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false},
	}
}

// cognitiveActionSchema makes the effector grammar match the actual action.
// A single broad object used to require every payload field for every kind,
// while still allowing an empty command or message. That made an invalid
// body_shell structurally indistinguishable from a valid deliberate wait. A
// nested anyOf keeps the cognitive commit itself as one strict root object and
// gives each possible effector exactly the fields it can enact.
func cognitiveActionSchema(stage int, allowNone, allowMentorSend bool) map[string]any {
	branches := make([]any, 0, 3)
	if allowNone {
		branches = append(branches, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind": map[string]any{"type": "string", "enum": []string{"none"}},
			},
			"required":             []string{"kind"},
			"additionalProperties": false,
		})
	}

	commitmentProperties := func() (map[string]any, []string) {
		properties := map[string]any{}
		required := []string{}
		if stage >= 5 {
			properties["intent"] = map[string]any{"type": "string", "pattern": "\\S"}
			properties["prediction"] = map[string]any{"type": "string", "pattern": "\\S"}
			properties["reality_check"] = map[string]any{"type": "string", "pattern": "\\S"}
			properties["stop_condition"] = map[string]any{"type": "string"}
			required = append(required, "intent", "prediction", "reality_check", "stop_condition")
		}
		return properties, required
	}

	shellProperties, shellRequired := commitmentProperties()
	shellProperties["kind"] = map[string]any{"type": "string", "enum": []string{"body_shell"}}
	shellProperties["command"] = map[string]any{
		"type": "string", "pattern": "\\S",
		"description": "可直接由 bash -lc 执行、用于读取或改变一个具体身体或世界事实的 Shell 源码；使用现实 payload 中出现的精确路径。",
	}
	shellRequired = append([]string{"kind", "command"}, shellRequired...)
	branches = append(branches, map[string]any{
		"type":                 "object",
		"properties":           shellProperties,
		"required":             shellRequired,
		"additionalProperties": false,
	})

	if allowMentorSend {
		mentorProperties, mentorRequired := commitmentProperties()
		mentorProperties["kind"] = map[string]any{"type": "string", "enum": []string{"mentor_send"}}
		mentorProperties["text"] = map[string]any{"type": "string", "pattern": "\\S"}
		mentorProperties["reply_to"] = map[string]any{"type": "string"}
		mentorRequired = append([]string{"kind", "text", "reply_to"}, mentorRequired...)
		branches = append(branches, map[string]any{
			"type":                 "object",
			"properties":           mentorProperties,
			"required":             mentorRequired,
			"additionalProperties": false,
		})
	}

	return map[string]any{"anyOf": branches}
}

func selectContextCommitments(commitments []ActionCommitment, candidates []Event) []ActionCommitment {
	wanted := make(map[string]bool)
	for _, candidate := range candidates {
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			wanted[commitmentID] = true
		}
	}
	selected := make([]ActionCommitment, 0, maxExperienceContext)
	seen := make(map[string]bool)
	for index := len(commitments) - 1; index >= 0; index-- {
		commitment := commitments[index]
		if wanted[commitment.ID] {
			selected = append(selected, commitment)
			seen[commitment.ID] = true
		}
	}
	for index := len(commitments) - 1; index >= 0 && len(selected) < maxExperienceContext; index-- {
		commitment := commitments[index]
		if !seen[commitment.ID] {
			selected = append(selected, commitment)
			seen[commitment.ID] = true
		}
	}
	return selected
}

func requestScore(request CognitiveRequest, candidate Event) float64 {
	novelty := 0.0
	if candidate.Kind != "concern" {
		novelty = 1
	}
	concernStrength := 0.0
	affectiveSalience := request.State.AffectiveState.Activation
	explorationValue := 0.0
	expectedCost := 0.25
	for _, concern := range request.State.Concerns {
		if concern.ID == candidate.ConcernID {
			concernStrength = concern.Strength
			affectiveSalience = maxFloat(affectiveSalience, concern.Activation)
			expectedCost = 1 - concern.Answerability
			if concernOwnsExplorationDrive(concern, request.State.Commitments, request.State.Mentor, request.Config.Dynamics.AttentionThreshold) {
				concernStrength = maxFloat(concernStrength, request.State.ExplorationPressure)
				explorationValue = request.State.ExplorationPressure
			}
			break
		}
	}
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

func (m *ModelClient) call(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice, instructions, input string, tools []map[string]any, forcedTool string) (apiResponse, error) {
	model, err := resolveModel(request.Config.CognitiveResource, request.Profile)
	if err != nil {
		return apiResponse{}, err
	}
	body := map[string]any{
		"model":             model.ID,
		"instructions":      instructions,
		"input":             input,
		"store":             false,
		"max_output_tokens": request.Config.ModelGateway.MaxOutputTokens,
	}
	if request.Profile.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": request.Profile.ReasoningEffort}
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
	inputUpperBound := len(data)
	reserved := reservationCost(model, inputUpperBound, request.Config.ModelGateway.MaxOutputTokens)
	accepted, reason := sendNotice(ctx, notices, WorkerNotice{
		LeaseID: request.Lease.ID,
		Kind:    "model_reserve",
		Payload: ModelReservation{Profile: request.Profile, InputTokenUpperBound: inputUpperBound, ReservedMicrousd: reserved},
	})
	if !accepted {
		return apiResponse{}, &CognitiveResourceUnavailableError{RequiredMicrousd: reserved, Reason: reason}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL(request.Config.ModelGateway.BaseURL), bytes.NewReader(data))
	if err != nil {
		fact := modelFailureFact(request, "request_error", 0, "", "", "")
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model request could not be created"}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+request.Config.ModelGateway.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(httpRequest)
	if err != nil {
		fact := modelFailureFact(request, "transport_error", 0, "", "", "")
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model transport failed: " + truncate(err.Error(), 512)}
	}
	defer response.Body.Close()
	responseData, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		fact := modelFailureFact(request, "response_read_error", response.StatusCode, response.Header.Get("Retry-After"), requestIDFromHeader(response.Header), response.Header.Get("Date"))
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model response could not be read"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		category := "http_error"
		if response.StatusCode == http.StatusTooManyRequests {
			category = "rate_limited"
		} else if response.StatusCode >= 500 {
			category = "upstream_unavailable"
		}
		fact := modelFailureFact(request, category, response.StatusCode, response.Header.Get("Retry-After"), requestIDFromHeader(response.Header), response.Header.Get("Date"))
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: fmt.Sprintf("model gateway returned HTTP %d: %s", response.StatusCode, safeAPIErrorMessage(responseData))}
	}
	var decoded apiResponse
	if err := json.Unmarshal(responseData, &decoded); err != nil {
		fact := modelFailureFact(request, "decode_error", response.StatusCode, response.Header.Get("Retry-After"), requestIDFromHeader(response.Header), response.Header.Get("Date"))
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model response was not valid JSON"}
	}
	if decoded.Error != nil {
		fact := modelFailureFact(request, "api_error", response.StatusCode, response.Header.Get("Retry-After"), requestIDFromHeader(response.Header), response.Header.Get("Date"))
		_ = settleUnknownUsage(ctx, notices, request, reserved, fact)
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model API error: " + truncate(decoded.Error.Message, 512)}
	}
	decoded.ReservedMicrousd = reserved
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

func acknowledgeUsage(ctx context.Context, notices chan<- WorkerNotice, request CognitiveRequest, response apiResponse) error {
	model := request.Config.CognitiveResource.Models[request.Profile.Model]
	for _, candidate := range request.Config.CognitiveResource.Models {
		if candidate.ID == response.Model {
			model = candidate
			break
		}
	}
	actual := usageCost(model, response.Usage)
	if normalizedTotal(response.Usage) == 0 {
		actual = response.ReservedMicrousd
	}
	usage := UsageRecord{
		Time:              nowUTC(),
		LeaseID:           request.Lease.ID,
		AttentionPulseID:  request.Lease.PulseID,
		FocusID:           request.Focus.ID,
		RequestedModel:    request.Profile.Model,
		EffectiveModel:    response.Model,
		ReasoningEffort:   request.Profile.ReasoningEffort,
		ProfileSource:     request.Lease.ProfileSource,
		ProfilePurpose:    request.Lease.ProfilePurpose,
		InputTokens:       response.Usage.InputTokens,
		CachedInputTokens: response.Usage.InputTokensDetails.CachedTokens,
		OutputTokens:      response.Usage.OutputTokens,
		ReasoningTokens:   response.Usage.OutputTokensDetails.ReasoningTokens,
		TotalTokens:       normalizedTotal(response.Usage),
		ReservedMicrousd:  response.ReservedMicrousd,
		ActualMicrousd:    actual,
		Status:            "completed",
		CostConfirmed:     normalizedTotal(response.Usage) > 0,
	}
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: request.Lease.ID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("model usage rejected because cognition lease is stale")
	}
	return nil
}

func settleUnknownUsage(ctx context.Context, notices chan<- WorkerNotice, request CognitiveRequest, reserved int64, fact ModelFailureFact) error {
	model := request.Config.CognitiveResource.Models[request.Profile.Model]
	usage := UsageRecord{
		Time:             nowUTC(),
		LeaseID:          request.Lease.ID,
		AttentionPulseID: request.Lease.PulseID,
		FocusID:          request.Focus.ID,
		RequestedModel:   request.Profile.Model,
		EffectiveModel:   model.ID,
		ReasoningEffort:  request.Profile.ReasoningEffort,
		ProfileSource:    request.Lease.ProfileSource,
		ProfilePurpose:   request.Lease.ProfilePurpose,
		ReservedMicrousd: reserved,
		ActualMicrousd:   0,
		Status:           "failure_cost_unconfirmed",
		CostConfirmed:    false,
		FailureCategory:  fact.Category,
		HTTPStatus:       fact.HTTPStatus,
		RetryAfter:       fact.RetryAfter,
		RequestID:        fact.RequestID,
		GatewayDate:      fact.GatewayDate,
	}
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: request.Lease.ID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("unknown model usage rejected because cognition lease is stale")
	}
	return nil
}

func modelFailureFact(request CognitiveRequest, category string, status int, retryAfter, requestID, gatewayDate string) ModelFailureFact {
	return ModelFailureFact{
		ObservedAt:  nowUTC(),
		Model:       request.Profile.Model,
		Category:    category,
		HTTPStatus:  status,
		RetryAfter:  truncate(strings.TrimSpace(retryAfter), 128),
		RequestID:   truncate(strings.TrimSpace(requestID), 256),
		GatewayDate: truncate(strings.TrimSpace(gatewayDate), 128),
		CostStatus:  "unconfirmed",
	}
}

func requestIDFromHeader(header http.Header) string {
	for _, name := range []string{"x-request-id", "request-id", "cf-ray", "x-amzn-requestid"} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func safeAPIErrorMessage(data []byte) string {
	var decoded struct {
		Error   *apiError `json:"error"`
		Message string    `json:"message"`
	}
	if json.Unmarshal(data, &decoded) == nil {
		if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
			return truncate(strings.TrimSpace(decoded.Error.Message), 512)
		}
		if strings.TrimSpace(decoded.Message) != "" {
			return truncate(strings.TrimSpace(decoded.Message), 512)
		}
	}
	return "upstream response contained no safe error detail"
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
	end := maximum
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + "…"
}

func redactRuntimeSecret(value, sensitiveValue string) string {
	if sensitiveValue == "" {
		return value
	}
	return strings.ReplaceAll(value, sensitiveValue, "<runtime-secret-redacted>")
}
