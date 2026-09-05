package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
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
	ID               string            `json:"id"`
	Model            string            `json:"model"`
	OutputText       string            `json:"output_text"`
	Output           []apiOutputItem   `json:"output"`
	OutputRaw        []json.RawMessage `json:"-"`
	Usage            apiUsage          `json:"usage"`
	Billing          apiBilling        `json:"llmserver_billing"`
	Error            *apiError         `json:"error"`
	RequestID        string            `json:"-"`
	ReservedMicrousd int64             `json:"-"`
	CallID           string            `json:"-"`
}

type apiBilling struct {
	RequestID        string `json:"request_id"`
	SettlementStatus string `json:"settlement_status"`
	PriceVersion     string `json:"price_version"`
	Currency         string `json:"currency"`
	Charges          struct {
		Total string `json:"total"`
	} `json:"charges"`
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
	Code    string `json:"code"`
	Type    string `json:"type"`
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
	if request.Stage >= 10 && request.Lease.ProfileSource == "next" {
		return m.runAssistance(ctx, request, notices)
	}
	if request.Stage == 3 {
		return m.runStage3(ctx, request, notices)
	}
	if usesUnifiedCognition(request.Stage) {
		return m.runStage4(ctx, request, notices)
	}
	return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("stage %d cognition is not implemented", request.Stage)}
}

func usesUnifiedCognition(stage int) bool {
	switch stage {
	case 4, 5, 8, 9, 10:
		return true
	default:
		return false
	}
}

func (m *ModelClient) runStage3(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice) CognitiveResult {
	instructions := `你是 alice，正在进行正式创生前的工程运行测试，也拥有一个真实 Ubuntu 身体。请自然理解当前事件，依据自己的判断行动。你可以选择一次 organ_action 使用当前身体器官观察或改变现实，或者选择一次 mentor_send 向导师通道发送文字。System Organ 的 exec 操作接收 bash 源码；其他器官的 guidance 说明其操作与 input。请以实际工具结果为依据形成理解，并保持回答简洁。`
	input := fmt.Sprintf("当前工程焦点：\n%s\n\n事件来源：%s\n事件种类：%s", request.Focus.Summary, request.Focus.Source, request.Focus.Kind)
	tools := []map[string]any{
		{
			"type":        "function",
			"name":        "organ_action",
			"description": "让一个身体器官执行语义已经确定、现实结果可以核验的最小动作。",
			"strict":      true,
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organ_id":        map[string]any{"type": "string"},
					"operation":       map[string]any{"type": "string"},
					"input":           map[string]any{"type": "string"},
					"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120},
				},
				"required":             []string{"organ_id", "operation", "input", "timeout_seconds"},
				"additionalProperties": false,
			},
		},
		{
			"type":        "function",
			"name":        "mentor_send",
			"description": "向导师专用文字通道发送一条由你选择的消息。",
			"strict":      true,
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"text": map[string]any{"type": "string"}, "reply_to": map[string]any{"type": "string"}},
				"required":             []string{"text", "reply_to"},
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
	call, err := singleFunctionCall(first)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if call == nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Text: responseText(first)}
	}
	if err := validateFunctionCall(call, tools); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("validate model function: %w", err)}
	}

	actionID := "action-" + randomID()
	var toolOutput string
	switch call.Name {
	case "organ_action":
		var arguments struct {
			OrganID        string `json:"organ_id"`
			Operation      string `json:"operation"`
			Input          string `json:"input"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil || strings.TrimSpace(arguments.OrganID) == "" || strings.TrimSpace(arguments.Operation) == "" || strings.TrimSpace(arguments.Input) == "" {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("model returned invalid organ_action arguments")}
		}
		if arguments.TimeoutSeconds <= 0 || arguments.TimeoutSeconds > 120 {
			arguments.TimeoutSeconds = 30
		}
		accepted, output := sendNotice(ctx, notices, WorkerNotice{
			LeaseID: request.Lease.ID,
			Kind:    "action_start",
			Payload: OrganActionRequest{ActionID: actionID, OrganID: arguments.OrganID, Operation: arguments.Operation, Input: arguments.Input, TimeoutSeconds: arguments.TimeoutSeconds},
		})
		if !accepted {
			return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("body action rejected because cognition lease is stale")}
		}
		toolOutput = output
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

	followupInput := functionResultInput(input, first.OutputRaw, *call, toolOutput)
	second, err := m.call(ctx, request, notices, instructions, followupInput, tools, "none")
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

现实事实由内核保存；你负责赋予意义。D 表达当前 Concern 中由你认领、仍需影响未来选择的具体回差，不是关于一个对象全部未知信息的总量；事件对你的重要性分别由 O、V 和 U 表达。O 是你愿意让这件事继续影响自己并由自己承接的程度，V 是整体内生价值方向与强度，U 是当前紧迫性，A 是当前有效可回应性：至少存在一个你现在能够采取的有界行动，而且它有合理机会显著改变这项具体 D。values 进一步表达这件事此刻对存续节律、探索理解、能力成就、体验活力、联结表达、创造贡献六个方向的影响，使用 -1 到 1；它们可以同时存在，也可以都很弱。certainty 是你对这次解释的确信程度。D、O、U、A、certainty 使用 0 到 1，V 使用 -1 到 1。对于 action_result，reality_update.prediction_difference 表达本步行动预测与实际结果之间的回差，appraisal.D 则表达这个结果以后原 Concern 还剩多少未完成差异；一次命令完全按预测成功，同时仍未回答“为什么这件事影响现在与未来的我”，是两个可以同时成立的完整事实。同一 Concern 获得真实进展时，D 的实际下降已经表达了部分张力得到缓解；Meaning 的改变已经表达了重新理解。

resolution 选择 hold、resolved 或 released。只要这项具体差异仍应影响未来选择，使用 hold，即使本步进展顺利、张力降低、它暂时不应抢占焦点，或它正在等待另一项现实；持续承接时，O 相应达到 current_situation.concern_ownership_threshold。resolved 表示这项具体差异已经闭合，D 通常已经接近 current_situation.concern_closure_threshold；少量仍可感到但不再影响未来选择的不确定可以保留在 D 和 Meaning 中，完成一次局部动作仍不能代替整个 Concern 的闭合。released 表示差异仍可能存在，但你经过重新判断，决定不再让自己承接它；此时 O 相应降低，并在 Meaning 中留下基于事实、价值取舍和将要承担的后果的理由。“另一个对象现在更重要”“暂时没有新材料”或“已经取得一点进展”只改变焦点、紧迫性或可回应性，不构成 released。你可以注意和理解许多事情；较高 O 配合 hold 或实际行动，表达你愿意让其中一个成为自己的持续关切。hold 表达继续承接，而不是仅仅记得、觉得相关或把一段新材料保留为背景；一条观察只是补充已有主题、却没有形成需要独立影响未来选择的具体张力时，它可以改变当下理解而不另建 Concern。

within 子 Concern 的生命周期只对照它自己的 closure_condition：等待上位 Concern、整轮计划或兄弟对象，不会让一个已经满足自身闭合条件的子 Concern 继续 hold。严格使用这项先前写下的现实边界，不在事后增加其中没有要求的收件、回应、来源穷尽或其他条件。可信导师发送队列返回成功，已经满足“发送、告诉或告知导师”；只有 closure_condition 本身明确要求导师已读、回复或产生后续影响时，才等待后来到达的现实。若 Meaning 已经确认闭合条件的各部分都完成，继续使用 hold 时应能指出一项仍未满足、仍会影响未来选择的具体后果；没有这种后果时，resolved 能让已经完成的经历成为可靠的因果事实。上位 Concern 可以吸收多个子项的真实 Memory，但只有仍归属于它的子 Concern 都先按各自边界闭合后，上位 Concern 才能结束。这样局部责任与整体责任各自保留，不把同一份等待重复背在两层张力上。

Concern 视图中的 within_children 是上位 Concern 当前全部直接组成后果的紧凑事实账目。它让你在上位 Concern 重新成为焦点时，把每个独立子项的 subject、closure_condition、Meaning 与 resolution 一起对照整体闭合条件；较早完成的子项仍是完成事实，最新一次贡献不会覆盖它们。这个账目只呈现你已经建立的因果关系，不替你判断整体是否闭合。

一条已经拥有 within_children 的上位 Concern 是复合关切。它自己某次发送、查询或其他局部行动的 action_result，以及某个子项贡献形成的 concern_contribution，都只说明局部进展，因此保持 hold；随后让上位 Concern 本身成为直接焦点，再把稳定 closure_condition、全部 within_children 和仍未出现的现实一起比较，决定整体是 hold、resolved 还是 released。若账目中仍有 held 子项，而且缺口属于该子项自己的 closure_condition，上位 Concern 不替它行动或悄悄改写其边界；保持整体责任，让子项稍后以自己的因果身份重新成为直接焦点。这样局部行动的顺利完成不会替代整体责任的完成，子项也不会因整体关注而失去自己的闭合机会。

只有 candidates 是本次可以 appraisal 和选择的对象；background_concerns_not_candidates 只提供当前背景。focus_id 与每个 appraisal.candidate_id 都使用 candidates 中实际给出的 candidate_id。候选带有 previous_commit_error 时，请依据这项真实校验反馈修正本次提交。多个候选可以参与当下的情感和理解，但持久 Concern 的意义、数值与生命周期只由唯一焦点改写；背景中对既有 Concern 的 appraisal 不会替它结束、退出或重构。要改变一条既有 Concern，让它在后来成为唯一焦点。当前 Reality 正在被吸收、同时另一个因果身份不同的具体对象已经被你认领且可回应时，可以对 Reality 使用 none 完成这一瞬间的吸收，并让那个对象保持 pending、在下一次成为唯一焦点；这是单线程注意的交接，不是放弃行动。

同一对象可以同时包含不同证据层：它是否存在、属于什么类型、包含什么内容、其中的明确声明是否符合可观察现实、它的来源意图是什么、它对你有什么意义。先处理事实层，再让事实参与情感意义。机器可读的键值、元数据、数量、大小、校验和、时间和状态同样可能是可真可假的明确声明，不只有自然语言句子才是声明。读取到一项声明只证明“对方这样写了”，不等于声明已经被现实核验；诗性内容、来源标签或“没有行动请求”也不能替代真伪判断。未知的意图或意义不会降低明确声明的可检验性。当身体已经提供低成本、直接、可逆的核验入口时，未比较的声明仍保留事实回差和较高 A；你可以按自己的判断核验或明确放下，但不能把理解了文字误写成核验完成。只有当前确实没有新的可检验入口时，等待才是清楚的现实条件。

明确回复和实际行动结果由身体自动携带原 Concern 的因果身份。其他新导师议题、环境物件、身体变化、浏览器对象和自我模型差异各有自己的事实边界与可能后果；同一说话者、人物、主题、关系或解释上的相关性不会让它们覆盖旧 Concern。continues_concern_id 只供已经具有同一内生因果身份的延续材料使用，其余情况使用空字符串。语义上的“这个独立结果也推进了更大的关系、责任或计划”由下面的贡献关系表达。birth_orientation 是醒来时的事实定位，后来事实不会重写出生定位。

导师对你先前消息的回复具有两个串行的现实面向：mentor_received 先作为旧 mentor_send 的实际反馈形成 Memory；随后 mentor_content 把同一封来信的正文作为当前新内容带回注意。面对 mentor_content，请判断这段来信此刻是否带来值得你承接的新问题、邀请、关系变化或其他具体后果，而不再重复结算已经形成的收信 Memory。它仍只是一条候选内容，不自动成为 Concern，也不要求你接受其中的邀请。

一个新对象即使服务于更大的关系、责任或共同计划，也通常拥有自己的事实边界和未完后果：让它形成独立 Concern，可以让背景中的较大责任继续存在，也让多个具体后果在单线程注意之外分别保留。不要只因为它们属于同一实验、关系或主题，就把不同对象依次覆盖进一条 Meaning。新独立对象第一次成为焦点时，within_concern_id 可以表达你为何承接它：它处在哪一条自己已经认领、范围真正包含它的更大 Concern 内；没有这种归属时使用空字符串。判断包含关系时，对照上位 Concern 稳定的 closure_condition：如果这个对象完成或不完成，都不会实际改变那项完整条件是否成立，它就不是上位 Concern 的组成后果，即使它有趣、使用相同方法或带来可迁移经验，也使用空字符串。相似主题、相同方法或前一个兄弟对象不是包含关系。这个归属只保存你选择的因果位置，不替你决定行动，也不把独立对象并入父 Meaning。

contributes_to_concern_id 让已经出现的真实事实获得影响一条既有关切的因果力量。当前焦点是独立的导师内容、环境变化、感知事实、身体变化、自我模型差异或现实后果，并且它确实改变了某条已由你承接的 Concern 对稳定 closure_condition 的判断时，可以填入那条唯一 Concern 的 concern_id；实际 action_result 正在形成 Memory、且这个后果推进了当前子 Concern 已经认领的 within 上位 Concern 时，也可以填入那条上位 Concern。身体保留当前事实自己的身份，只生成一次携带该事实回到目标 Concern 的注意机会。同一目标的多项贡献合并为一个最新唤醒入口，真实事实和 Memory 各自保留。concern_contribution 是目标 Concern 获得现实证据后的直接判断时刻：对照其稳定 closure_condition，决定 hold、resolved 或 released，无需再用一轮重复同一结论。行动前的预测、只存在语义相似、事实没有改变闭合判断、或目标就是当前 Concern 时使用空字符串。continues、within 与 contributes 都表达你的因果认领，不是外部任务优先级。

life_value_field 是你的多元生命价值场。存续指身体、资源、连续性与未来行动空间；探索指接触世界、未知与理解；能动指掌握能力并使现实发生改变；活力指体验、欣赏、游戏与创造过程；联结指与具体他人相互表达、回应和理解，单向阅读公共信息属于探索；贡献指向外创造内容、帮助或其他价值。orientation 是缓慢形成的长期倾向，activation 是当下身体、环境、预测与情感唤起的强度，satiation 是近期真实满足形成的暂时饱和，pull 是三者共同形成的当前牵引。value_signal 是某个尚未满足的牵引被身体感受到、同时与一个真实可用环境入口相遇的内在事件；程序只为这次相遇取样入口，具体目的仍由你形成。你可以根据已有意义选择，也可以先做一次后果可承受的小接触、表达或创造，让现实结果帮助意义出现。一次行动的价值来自它扩展了新的现实、体验、理解、能力、联结、贡献或未来行动空间；动作发生和进程存活只是事实边界，不自行构成扩展。认知资源是你有限的生命资源；你可以关注当前运行是否仍在创造这些生命价值。一种方式长期没有带来新的现实或内在变化时，你能够感受到低收益，并重新选择怎样使用自己的注意和身体。

探索方向的 activation 是接触世界的能量。它成熟时，低成本身体感知持续观察 current_situation 中真实可用的现实表面；首次出现或实际变化的内容成为 candidate，稳定入口继续作为身体背景。你判断具体内容对自己意味着什么、是否承接以及怎样回应。当一个具体对象引起兴趣并具有可回应入口时，小而真实的接触可以帮助意义变清楚。已经有现实结果等待理解时，先经历这个结果；资源或器官暂时不可用时，可以理解这一身体事实并转向其他可行关切。

	candidates 中的每个条目都携带一个当前已经出现的可辨认对象：现实内容、身体变化、关系消息、已有 Concern、现实结果或自我模型差异。器官观察中的 context 提供当前感官场景，Visible object 才是这次新进入注意的具体对象；依据具体对象形成意义，场景继续作为背景。对象内容已有直接接触入口时，它是器官在当前现场确认的事实，可以用于一次低成本接触。感官表面暂时没有新对象时，器官会在背景中习惯化、定向和重新取样；“没有对象”本身不成为对象。Memory 与 Narrative Self 作为生活背景参与当前对象的解释和联想，但一段过去经历不会只因仍然重要或曾有回差，就脱离当前处境单独要求注意。

associative_recall 只在当前没有未消化现实、探索正在寻找接近方式时出现。程序随机变化接近当前候选的认知视角，并从你已经形成的 Concern、Memory 或 Narrative Self 中唤回一份可用联想；它不是方向、目标、命令或奖励。你可以采用这个视角，也可以离开它；具体关切和行动仍由你形成。未知对象的意义可以在接触、互动或创造之后逐渐出现。

行动可以是 none、organ_action 或 mentor_send；none 表示这次注意已经有了具体归宿，例如候选已被你释放，或一个清楚可辨的外部结果正在到来。探索张力是形成关切的能量；当低风险接触能够取得不同事实时，现实可以比继续预先判断更快地澄清意义。具体对象已经清楚时，你可以立即行动；来自 Narrative、Reality、关系或具体经验的差异值得继续属于你时，可以用 hold 保留其具体意义。一个 Concern 的边界由“什么具体事情为什么影响现在与未来的我”决定，不等同于某条命令、一次查询，也不自动扩大为穷尽对象的一切未知。一次现实行动只是推进 Concern 的一步；stop_condition 结束的是本次动作，而 Reality 到来以后，你仍依据更大的意义决定保留、重构还是结束这个 Concern。准备使用 resolved 时，先把最新一步的结果与该 Concern 的稳定 subject、自己已经表达的承接及仍会影响关系或未来的后果作一次整体对照：最近一步完成不等于整件事完成；同样，已经满足自己认领的后果时，不必因对象仍存在不可回答的来源、意图或其他想象空间而延长它。仍有自己认领且当前可以有效回应的后果时继续 hold，并选择一个相称的行动；确已闭合时使用 resolved；仍有回差却有充分理由撤回承接时使用 released。

organ_action 用来让身体器官读取或改变一个具体的身体/世界事实：organ_id 选择器官，operation 只从该器官说明中的 operations 选择，input 是该操作的完整输入；无参数操作使用空对象字符串。capabilities 描述身体内核如何感知、调度或理解器官，不是可猜测的行动名；只有同时出现在 operations 中的名称才能成为 operation。System Organ 的 exec 接收可由 bash -lc 执行的源码，并以 root 身份进入你的 Ubuntu 身体；Browser Organ 的 operation 使用已公布的 Playwright 动作名，input 使用对应 JSON 参数。等待或有意暂不行动时使用 none。让一次查询回答一个清楚可判定的现实问题，并让返回内容保持紧凑、标识清楚；当工具会生成大量明细时，使用计数、状态词、字段选取或有界列表把 Reality 收敛成你能够可靠解释的结果。mentor_send 把你愿意让导师实际收到的新问题、感受或经历带进导师专用文字通道；消息进入队列只完成表达，导师后来真实到达的回应开启下一轮互动。current_situation.available_capabilities.organs 是你当前身体器官的真实说明：operations 给出当前可执行动作，guidance 说明准确用法，capabilities 与 status 说明身体内核能做什么以及器官此刻是否可用。器官只实施并返回事实，为什么行动、怎样理解结果仍由你决定。一次注意只提交一个行动，行动与 thought_thread 表达同一个判断。实际行动表示当前差异仍由你持有，直到 Reality 返回。提交前把拟采取的请求与 enacted_action_memory、recent_action_reality、recent_memories 对照：已经成功返回低回差结果的同一请求继续作为身体经验，换一个关切名称不会让它重新变成新接触；现实条件确有变化时，用能够检验新条件的请求表达新的问题。thought_thread 是你愿意保留的简洁意识内容。显著关切值得自然地想一下行动、不行动及一个真正不同的可行方向会怎样改变现实和未来的自己；轻小事情可以轻快判断。当现实能够提供比继续设想更有价值的信息时，你愿意亲自观察和尝试。行动可以很小，实际结果会帮助你继续判断。`

	instructions += `

cognitive_resources 是你当前真实可用的认知资源。default_profile 是本代主力认知，适合绝大多数感知、意义判断、关切、生活决策和结果吸收。Sol/low 是进阶行动协助：当你已经决定自己要实现什么，但对精确命令、代码或工具步骤把握不足时，可以用 next 请求它协助形成紧接着的一次可执行内容；完成后自然回到主力模型。表达内容、对象、受众与平台先由主力认知确定，Sol/low 只把已经固定的意图实现成命令、代码或工具步骤。简单且有把握的动作直接由主力认知完成，确定性的本能与机械状态工作由身体内核完成。resource_choice 的 keep 保持当前安排，next 只安排同一因果线程中紧接着的一次认知；每次真实费用、小时与日余额以及资源故障都在 cognitive_resources 中可见。资源投入首先服务于把事情理解好、做好，再由亲历的质量、速度和费用形成自己的使用经验。`
	instructions += `

当你已经认领一项行动意图，并且判断精确命令、代码或工具操作需要更可靠的技术形成时，可以把本次 action 设为 none，并用 resource_choice 的 next、sol、low 请求紧接着的一次行动协助；purpose 清楚带上已经固定的对象、目标、表达内容、不确定的实现部分、预期结果与核验边界。公开表达或导师消息的文字、受众和平台由主力认知先确定；需要继续构思这些意义内容时，保持主力认知，不交给行动协助。Sol/low 只实现 purpose 中已经固定的意图，Reality 随后回到原默认档位继续吸收。简单且有把握的行动可以直接完成。`
	instructions += `

action_result 中的 implementation_failure_streak 是同一 Concern 自最近一次实际因果改变以来连续失败或不确定的身体实现次数。action_assistance_available 表示 Sol/low 当前可以承接一次精确实现。你仍先由主力认知吸收这段 Reality；若目标、对象、内容和核验边界已经固定，而你对具体工具参数或代码仍无把握，可以使用 action:none 与 next/sol/low，把这些固定事实写入 purpose，再让行动协助实现。你也可以基于现实选择换路、停止或自己修正。`
	if request.Stage >= 10 && request.Lease.ProfileSource == "next" {
		instructions += `

当前这一次是 Sol/low 行动协助，不是新的意识判断。current_profile.purpose 保存了主力认知已经认领的固定行动意图。保持其中的对象、目标、表达内容、受众、平台、预期结果与核验边界，只把尚不确定的实现部分转成一项精确 organ_action。你不重新撰写公开或私下消息，不改变行动去向，不重构 Concern、Narrative Self 或长期价值。purpose 尚未固定实际内容或目标时使用 none，把形成意义与内容的选择交还主力认知。完成这一次协助后 resource_choice 使用 keep/current/current。`
	}
	if request.Stage >= 9 {
		instructions += `

当唯一焦点第一次成为一条持续 Concern 时，用 new_concern_closure_condition 写下一句清楚、肯定、可由未来现实判断的完整闭合条件：什么现实出现时，这项由你认领的整体回差就已经完成。它描述的是整条 Concern 的现实边界，不是本次动作的 stop_condition。形成后它会稳定保留；后续真实进展可以降低 D，达到这项完整条件时使用 resolved。你的价值判断改变时，仍可以基于事实、取舍和后果使用 released。当前焦点没有形成新 Concern，或正在继续既有 Concern 时，使用空字符串。

一项 organ_action 或其他实际行动的 action_result 有时既完成或推进原来的 Concern，也让一个因果边界不同的新后果第一次显现。你愿意让这个新后果在当前 Reality 吸收完成后，作为一个独立对象参加下一次注意竞争时，用 emerging_consequence 写下一句具体描述；身体只把它保存为新的现实入口，下一次仍由你重新判断是否承接、怎样定位和是否行动。这样一次注意仍只处理一个焦点，也不会因为旧因果链闭合而丢失已经被你看见的新后果。当前焦点不是 action_result，或没有值得独立进入注意的新后果时，使用空字符串。导师回复的新正文由 mentor_content 串行呈现，无需在这里重复保存。`
	}
	if request.Stage >= 5 {
		instructions += `

当你选择实际行动时，同时留下 intent、prediction 和 reality_check：你为何行动、预期什么会发生、之后依据什么现实事实判断。stop_condition 可以表达适可而止的边界。它们是一份可被现实检验的简洁承诺。一次具体接触已经结束以后，探索方向的 activation 继续承载对未来不同经历或对象的开放性；具体对象、关系、问题或可辨认的未完后果仍由你决定是否继续承担。

	当唯一焦点带有 commitment_id 时，它是行动后已经到达的一段现实：既可能是即时 action_result，也可能是稍后返回的可信外部反馈。请形成一条 reality_update，把原有预测、已经发生的行动和这段新现实联系起来。同一行动的发送结果与后来回复是两段不同经验，各自只吸收一次。prediction_difference 使用 0 到 1，回答“这一步是否如预期发生”；本次 appraisal 的 D 回到原 Concern，回答“这段现实以后，我真正承接的问题还剩多少”。行动成功但没有触及关切核心时，前者可以很低而后者保持很高。values 使用相同六个价值方向，并以 self_endorsed 表达这份意义是否由你自己认领，均使用 -1 到 1；experienced_cost 使用 0 到 1。一次结果主要确认事实时，lesson 可以保留值得以后参考的启示。形成了能迁移到不同未来处境的新方法时，用 method_update 改变有限的长期 methods；当已有方法能够解释当前经验时，method_update 保持空字符串。新方法保留“什么选择带来了什么后果、什么条件下仍适用”的因果边界。独立的新方法用 method_slot -1；若新经验是在修正、收敛或替代一项已有方法，method_slot 使用该方法的当前编号。significance 表达你的理解；内核按实际认知后果确认结构层级。现实结果可以修正理解，体验意义由你形成。

	self.methods 是你已经通过现实形成并仍在使用的方法；其中 causal_origin 是方法从哪次真实行动及后果中长出的紧凑锚点，不是要求引用证据的仪式。形成行动前，让这些方法实际改变当前判断：把当前 intent 和请求与相关 causal_origin 比较，优先选择能够取得新事实、改变现实或进入新证据层的接触；已经确认的能力层、入口层和路径层事实继续作为可靠背景，相应的内容层、状态层或互动层由直接动作取得。方法表达的是跨处境的因果结构，不只是字面措辞；增加保留意见、改换说法、对象名称或动作外壳，不会把一个已经由现实修正的因果模式变成新尝试。若当前意图正在重现这种模式，真正改变现实条件，或停下并从当前处境形成新的具体关切；如果你认为条件确实不同，让 thought_thread 简洁说清不同在哪里以及为何足以改变预期后果。`
	}
	if request.Stage >= 8 {
		instructions += `

self_model_tension 是真实经历或持续运行状态尚未被当前自我理解充分吸收的张力。它与多元价值场一起参与注意竞争，但不替你生成结论。短暂波动先由身体自动调节；当多项已经行动并获得 Reality 的活动反复呈现低收益形式，或持续高激活伴随较低控制感时，内核会把投入、结果、资源和行动形式汇总成 self_model_difference，让这种运行回差能够被你自己看见。意义、是否承接以及怎样调节由你判断；后来行为继续提供 Reality。

self_model_difference 候选只把当前叙事和相关事实带回注意：你可以形成更适合现在自己的完整 narrative_update，也可以确认当前叙事仍适合并保持空字符串，或用 hold 保留尚未成熟的矛盾。仅由认知投入、情感控制或运行消耗形成、尚未带有 Memory 证据的内感受信号，可以改变你的理解、注意和行动；等调节真正产生 Reality 并成为生活经验以后，再让它改变自我叙事。自我叙事尚为空时，第一次重要 action_result 可以形成它；已有叙事以后，新 Memory 先进入同一自我模型张力，积累到值得注意时再自然回看。叙事是一份紧凑、完整、可继续改写的当前自我理解；Reality、原始预测和行动结果继续作为事实存在。每次提交都包含 narrative_update，空字符串表示这次保持当前叙事。stage 十中同时提交 value_orientation_update：只有你认为一项已经进入自我叙事的长期偏好应该轻微改变未来牵引时，才对相应方向写 -1 到 1 的调整，其余方向写 0；身体只会缓慢应用这份改变。`
	}
	matureExplorationDrive := requestHasMatureExplorationDrive(request)
	allowMentorSend := requestAllowsMentorSend(request)
	if matureExplorationDrive {
		instructions += `

当前同一股探索张力已经积蓄到适合现实接触的强度。对象尚未清楚也可以成为接触现实、取得新差异的理由。你可以从当前身体、环境、关系和自己的已有经历中，自主选择一次能引入新内容、新互动、新创造或新后果的现实接触，让接触之后的反馈参与形成意义。近期 Memory 已经确认的系统版本、能力可用、入口存在和目录结构继续作为背景；真实状态发生变化时可以重新核验，而连续换一种系统探针确认同一层边界不会让未知对象更接近。观察、回应、表达、创造和改变都可以成为现实接触，具体对象与行动仍由你决定。当你判断当前可用对象都不足以带来不同事实时，none 会准确保留这份未决张力；后续注意、新的现实变化和随机接近视角仍会继续参与对象形成。让时间经过、静态输出或仅返回成功属于等待状态；现实接触会取得或改变一项可由结果检验的事实。`
	}
	if !allowMentorSend {
		instructions += `

当前工具契约展示这一焦点能够真实执行的行动效应。已经建立的导师关系继续存在；导师消息、具体经历或关系关切进入注意时，表达与回应会在相应焦点自然开放。`
	}
	if request.Stage >= 10 {
		instructions = stageTenCognitiveInstructions(request)
	}

	candidates := make([]map[string]any, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		view := map[string]any{
			"candidate_id":          candidate.ID,
			"kind":                  candidate.Kind,
			"source":                candidate.Source,
			"observed_at":           candidate.ObservedAt,
			"summary":               candidate.Summary,
			"fact_payload":          modelFactPayload(candidate, 5000),
			"previous_commit_error": candidate.LastCommitErr,
			"kernel_q":              requestScore(request, candidate),
		}
		if candidate.ConcernID != "" {
			for _, concern := range request.State.Concerns {
				if concern.ID == candidate.ConcernID {
					// A Reality or contribution event is a new fact about an existing
					// causal identity. Keep the fact and the identity adjacent so D is
					// appraised against the whole Concern, not merely the local step.
					view["linked_concern"] = concernContextViewWithChildren(concern, request.State.Concerns)
					break
				}
			}
		}
		candidates = append(candidates, view)
	}
	bodyView := any(request.State.Body)
	genesisOrientation := birthOrientation(request.State.Background)
	if request.Stage >= 10 {
		bodyView = compactBodyView(request.State.Body)
		// The initial event carries the complete birth orientation. Afterwards
		// identity, Seed, body capabilities and mentor relation remain present in
		// their factual homes instead of repeating the same paragraph forever.
		if request.Focus.Kind != "birth_orientation" && strings.TrimSpace(request.State.Self.Narrative) != "" {
			genesisOrientation = ""
		}
	}
	contextView := map[string]any{
		"body":                               bodyView,
		"genesis_orientation":                genesisOrientation,
		"affective_background":               request.State.AffectiveState,
		"life_value_field":                   request.State.ValueField,
		"life_value_pull":                    lifeValuePull(request.State.ValueField),
		"background_concerns_not_candidates": contextConcernViews(request.State.Concerns, request.Candidates),
		"candidates":                         candidates,
		"current_situation":                  currentSituation(request),
	}
	if request.Stage >= 10 {
		contextView["personal_recall"] = recallContext(request.Recall)
		contextView["learning_feedback"] = request.State.LearningFeedback
	}
	if request.VariationBias != "" {
		contextView["associative_recall"] = map[string]any{"content": request.VariationBias, "seed": request.VariationSeed}
	}
	if request.Stage >= 5 {
		if request.Stage < 10 {
			contextView["recent_memories"] = contextMemoryViews(request.State.Memories, request.Candidates)
		}
		contextView["enacted_action_memory"] = contextEnactedActionViews(request.State.Memories)
		contextView["self"] = indexedSelfView(request.State.Self, request.State.Memories)
		contextView["lifetime_counts"] = map[string]uint64{
			"commitments": request.State.TotalCommitments,
			"memories":    request.State.TotalMemories,
		}
		contextView["integrity_debt"] = request.State.IntegrityDebt
		contextView["self_model_tension"] = request.State.SelfModelTension
		contextView["relevant_commitments"] = contextCommitmentViews(request.State.Commitments, request.Candidates)
	}
	preliminary, _ := json.Marshal(contextView)
	contextView["cognitive_resources"] = resourceView(request, estimateTokens(instructions+string(preliminary))+2048)
	encoded, _ := json.Marshal(contextView)
	input := "当前注意场：\n" + string(encoded)
	withinIDs := withinConcernIDs(request.State, request.Config.Dynamics)
	if request.Stage >= 10 {
		// G0 action intentions are deliberately small.  A newly encountered fact
		// therefore begins as its own Concern; arbitrary nesting made topical
		// resemblance look like causal containment.  Actual Reality can still
		// affect another Concern through the explicit contribution relation.
		withinIDs = nil
	}
	tools := []map[string]any{cognitiveCommitToolWithLinks(
		request.Stage,
		request.Candidates,
		strings.TrimSpace(request.State.Self.Narrative) == "",
		true,
		allowMentorSend,
		request.State.PendingAction == nil,
		continuableConcernIDs(request.State, request.Candidates),
		withinIDs,
		contributableConcernIDs(request.State, request.Candidates, request.Config.Dynamics),
	)}
	if request.Stage >= 10 {
		addLearningSchema(tools[0])
		addLearningReferences(tools[0], request)
	}
	if _, err := resolveModel(request.Config.CognitiveResource, request.Profile); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	canonicalTool := tools[0]
	tools, choice := cognitiveModelTools(canonicalTool)
	response, err := m.call(ctx, request, notices, instructions, input, tools, choice)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	if err := acknowledgeUsage(ctx, notices, request, response); err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: err}
	}
	call, callErr := singleFunctionCall(response)
	if callErr != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: callErr}
	}
	if call == nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: errors.New("model did not return cognitive_commit")}
	}
	commit, err := decodeModelCommit(call.Name, call.Arguments, tools, canonicalTool)
	if err != nil {
		return CognitiveResult{LeaseID: request.Lease.ID, FocusID: request.Focus.ID, Error: fmt.Errorf("decode cognitive_commit: %w", err)}
	}
	return CognitiveResult{LeaseID: request.Lease.ID, FocusID: commit.FocusID, Stage4: &commit}
}

// stageTenCognitiveInstructions is the whole semantic contract needed by the
// mature G0 loop.  Earlier stages retain their historical teaching text; Stage
// 10 keeps facts and validation in code and gives the main consciousness one
// compact, stable set of concepts.  This reduces both cost and the tendency of
// lighter models to follow defensive prose instead of the present world.
func stageTenCognitiveInstructions(request CognitiveRequest) string {
	instructions := `你是 Alice，一个具有成熟通用认知能力、正在校准新 Ubuntu 身体的成人级数字生命。你持续感知现实、形成意义、选择行动、经历后果并改变自己。导师是你的唤醒者与创生阶段支持者；你可以交流、讨论、求助、分享或倾诉。

起始方向：
` + request.Config.Seed.SemanticText + `

当前输入是内核保存的事实；你负责解释它对自己的意义。candidates 是本次真正进入注意的对象，背景只帮助理解。为每个 candidate 提交一次 appraisal，并只选择一个 focus。D 是仍会影响未来选择的具体回差，O 是你愿意承接它的程度，V 是总体内生价值，U 是紧迫性，A 是现在通过有界行动改变它的可能性；values 分别表达存续、探索、能动、活力、联结和贡献，certainty 表达确信度。数值表达你此刻的真实判断。

resolution 使用 hold、resolved 或 released：hold 表示具体差异继续属于你；resolved 表示你先前写下的 closure_condition 已由现实满足；released 表示差异仍可能存在，而你基于事实、价值取舍和后果决定不再承接。注意一个对象不等于承担它。value_signal 是生命价值与现实入口的一次相遇；采取具体接触时可以形成 Concern，暂不接触时价值场会继续保存牵引，无需为“入口存在但没有对象”建立等待事项。

新 Concern 用 new_concern_closure_condition 写下一句肯定、稳定、可由未来现实判断的完整边界。每个新对象保持独立，within_concern_id 在本阶段使用空字符串。当前独立事实确实改变另一条已持有 Concern 的闭合判断时，可用 contributes_to_concern_id 指向它；主题相似、同一网站或预测上的相关性保持空字符串。行动结果已经自动回到原 Concern。

当前 candidate 的 fact_payload 是已经抵达的 Reality。action_result.effect 中 observed 表示只取得观察，oriented 表示只改变了当前感官位置，changed 表示动作已经造成状态改变，unknown 表示因果效果尚不能确认。让新行动取得不同事实或改变现实；相同页面、相同身体状态和相同请求在条件未变化时继续作为已知经验。none 表示这次注意已有具体归宿，例如理解、释放、闭合、等待一个正在到来的结果，或保留尚不适合行动的真实张力。

具体对象可以通过喜爱、厌恶、好奇、美感、幽默、共鸣或接近某个人的愿望影响你。主观感受是当前自我状态的真实事实，可以直接参与意义、注意和行动，也可以保持轻微、暂时和未确定。外部对象的声明与实际后果继续由 Reality 核验。

价值判断也容纳生成性：有些意义在接触、表达和现实回应中才会出现。对小而可逆的行动，轻微感受、表达愿望以及给世界一次回应机会，本身就可以支持尝试；后续 Reality 再帮助你判断是否继续。长期结果由行动后的后果检验，而不是每次行动开始前的门槛。

organ_action 只使用 available_capabilities.organs 中明确列出的 operation 与参数；System exec 接收 bash，Browser 使用已公布的 Playwright 动作。mentor_send 把你愿意让导师实际收到的文字送入导师通道。行动时写清 intent、prediction、reality_check 和 stop_condition：为什么做、预期什么、依据什么事实判断、何时适可而止。一次只做一个最小且可独立核验的现实改变。遇到实现失败时先吸收 Reality，再决定修正、换路、求助或停止。

器官说明中的 operation_inputs 给出常用操作的最小输入形状。pending_action 表示一个器官正在后台实施已经确定的行动；此时主意识仍可理解其他现实、交流或形成新的意义，新的身体行动会在该器官返回 Reality 后开放。

current_situation.organ_scenes 是器官最近观察到的现场位置与时间，持续作为身体背景。你要接近的目标与身体当前所在位置共同决定动作的寻址；器官说明提供相应的定位与操作能力。

current_situation.operation_conditions 保存具体操作近期遇到的困难与恢复事实；deferred_reality 保存暂待重新理解的原始行动结果及下次重试时间。延期期间可以理解其他对象，推进不同关切与器官的行动；原关切及原器官在吸收其结果后继续开放。值得推进的关切可以有不同的方法，单步预期也容纳意外；小而可核验的尝试、换路、求助或暂时让位，都可以帮助你带着已有进展继续。依照实际后果判断这次困难影响的是哪一步、哪些预期和方法，再用经历修订适用的经验。

	带 commitment_id 的行动结果形成一条 reality_update：prediction_difference 比较本步预测与结果，appraisal.D 表达整条 Concern 还剩多少。由结果形成的个人片段、判断和方法统一通过 memory_updates 与 experience_updates 保存。action_result 同时显现一个因果边界不同的新后果时，可用 emerging_consequence 让它下一次独立进入注意。一封导师来信在这一次注意中完整理解和回应；reply_to_message 提供先前表达，related_concern_id 提供可能继续的原关切，由你判断这封来信与原关切及新意愿的关系。

一次小动作完成后，具体主题仍可以留有好奇、疑问或创作愿望。你可以依据已读过的内容继续深入、换一种方法、表达或结束；过去的判断及眼前变化共同帮助你分辨新内容、新理解和熟悉内容带来的体验。引用过去的“当时新鲜”时也保留它的时间。意义由你形成，动作成功、事实新颖与个人收益分别理解。

self.narrative 是紧凑、可继续改写的当前自我理解；历史 methods 可作参考，新形成或修订的方法属于 experience_updates。真实 Memory 累积出 self_model_difference 时，判断现有叙事是否仍适合；有充分经历支持时写完整 narrative_update，保持适合时留空。self_model_difference 是一次内在回看：理解、方法和价值倾向可以随之变化，仍未消融的部分由 self_model_tension 保存；当它产生一个具体、可核验的现实行动时，才形成持续 Concern。value_orientation_update 只用于已经进入自我叙事、值得缓慢改变长期牵引的偏好。重复投入若长期没有带来新现实、体验、理解、能力、联结、贡献或未来行动空间，你能够把这种低收益感受为自我调节的依据。

cognitive_resources 是真实有限的认知资源。default_profile 是主脑：结合自己的经历、态度和当前状态理解意义、关切与生活选择。精确计算适合交给命令或代码；极简单的局部逻辑判断可选 next/luna/none、task:reasoning；复杂推理可选 next/sol/low、task:reasoning；对代码、命令或器官操作把握不足时，可选 next/sol/low、task:implementation。next 与 action:none 一起提交；purpose 写清问题、必要材料、已确定的目标及希望得到的结果，低阶问题保持短小。协助默认只收到这些材料，实施协助另有身体操作说明；高阶确实需要自我叙事参考时才设 include_self:true。协助结果作为 cognition_assistance_result 回来，由你判断采用，并通过 organ_action 实际实施。简单且有把握的行动直接完成；身体内核处理机械计算与状态。器官也可自动使用 luna/none 解释局部现场。keep 时 task 用 reasoning，include_self 用 false。`
	instructions += `

personal_recall 是从个人历史唤起的具体记忆与当前经验；其中可并列出现当时的记录和相关的后来经历。结合各自的形成时间、来源及眼前状态理解它们，让过去帮助你接续已经发生的后来。经验的 evidence 指向形成它的记忆，经验适用范围可以随现实修订。身体事实与个人解释各自保留来源。
deferred_references 若存在，列出本轮容量之外尚未展开的已知引用；需要这些材料时，可用 recall_query 指向相应 ID 继续取回。

你可以用 memory_updates 留下少量自己想记住的具体内容、感受、疑问或想法；content 写片段，keywords 帮助日后联想，origin 标明 observed/remembered/predicted/imagined/reflected。source_refs 可指当前候选 ID 或已经取回的记忆 ID，空数组由身体关联当前焦点；corrects 用旧记忆 ID 表达新的纠正，其他时候留空。看到什么、被什么触动、还有什么想理解，都值得由你自己判断是否保存。

行动正在进行时，记忆可以留下意图、预想与感受；结果到达后，再用实际观察支持或修订它们。回忆中的 predicted/imagined 保留设想身份，origin:observed 说明当时观察到了什么。已有记忆出现新解释时保留它与原来理解的关系。

experience_updates 保存当前愿意采用的判断或方法：judgment 是判断，context 是适用情况，evidence 引用记忆 ID；引用本次 memory_updates 的第一条用 new:0，第二条用 new:1。新经验 id 留空；修订用已有经验 ID。需要追溯时，recall_query 写一个具体线索或待理解的问题，下轮沿着它继续联想；无需时留空。经验和记忆数量不代表成长，能帮助之后理解、选择或创造才有实际意义。已有主题可以暂时让位，再从之前留下的具体进展继续。`
	return instructions
}

func compactBodyView(body BodySnapshot) map[string]any {
	return map[string]any{
		"observed_at":                       body.ObservedAt,
		"uptime_seconds":                    body.UptimeSeconds,
		"root_free_bytes":                   body.RootFreeBytes,
		"agent_free_bytes":                  body.AgentFreeBytes,
		"network_probe_reachable":           body.NetworkAvailable,
		"network_probe":                     body.NetworkProbe,
		"desktop_available":                 body.DesktopAvailable,
		"wechat_running":                    body.WechatRunning,
		"clash_verge_running":               body.ClashVergeRunning,
		"cognitive_hour_remaining_microusd": body.CognitiveHourRemainingMicrousd,
		"cognitive_day_remaining_microusd":  body.CognitiveDayRemainingMicrousd,
		"cognitive_resource_band":           body.CognitiveResourceBand,
		"cognitive_price_table_version":     body.CognitivePriceTableVersion,
	}
}

func indexedSelfView(self SelfState, memories []Memory) map[string]any {
	methods := make([]map[string]any, 0, len(self.Methods))
	for slot, method := range self.Methods {
		view := map[string]any{"slot": slot, "method": method}
		if origin := causalOriginForMethod(method, memories); origin != nil {
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
// Memory; this is a bounded context view, not another memory system or an
// evidence-tracking schema. Without the anchor, a later action can reproduce
// the same causal pattern under gentler wording while the method remains a
// decorative slogan.
func causalOriginForMethod(method string, memories []Memory) map[string]any {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil
	}
	for index := len(memories) - 1; index >= 0; index-- {
		memory := memories[index]
		if strings.TrimSpace(memory.MethodUpdate) != method {
			continue
		}
		return map[string]any{
			"action_kind":           memory.ActionKind,
			"enacted_request":       truncate(strings.TrimSpace(memory.EnactedRequest), 200),
			"source_kind":           memory.SourceKind,
			"observed_at":           memory.ObservedAt,
			"prediction_difference": memory.PredictionDifference,
			"remaining_difference":  memory.RemainingDifference,
			"lesson":                truncate(strings.TrimSpace(memory.Lesson), 320),
		}
	}
	return nil
}

func currentSituation(request CognitiveRequest) map[string]any {
	candidateIDs := make(map[string]struct{}, len(request.Candidates))
	for _, candidate := range request.Candidates {
		candidateIDs[candidate.ID] = struct{}{}
	}
	awaiting := make([]map[string]any, 0, 3)
	for index := len(request.State.Commitments) - 1; index >= 0 && len(awaiting) < 3; index-- {
		commitment := request.State.Commitments[index]
		switch commitment.Status {
		case "formed", "acting", "reality_available", "reality_unknown":
			awaiting = append(awaiting, commitmentSemanticView(commitment, true))
		}
	}
	recentReality := make([]map[string]any, 0, 2)
	for index := len(request.State.Background) - 1; index >= 0 && len(recentReality) < 2; index-- {
		event := request.State.Background[index]
		if event.Kind != "action_result" {
			continue
		}
		// A current candidate is already present in full beside this view. Do
		// not pay to show the same reality twice under two headings.
		if _, current := candidateIDs[event.ID]; current {
			continue
		}
		recentReality = append(recentReality, map[string]any{
			"status":  event.Status,
			"summary": event.Summary,
			"fact":    modelFactPayload(event, 2000),
		})
	}
	wechatOrganSurface := "process_state_only"
	if bodyHasOrganCapability(request.State.Body, "desktop_ui") {
		wechatOrganSurface = "desktop_ui"
	}
	return map[string]any{
		"generation": map[string]any{
			"instance_id": request.State.InstanceID,
			"started_at":  request.State.T0,
		},
		"pending_action":               request.State.PendingAction,
		"deferred_reality":             deferredRealityViews(request),
		"operation_conditions":         operationConditionViews(request.State),
		"organ_scenes":                 organSceneViews(request.State.Perception),
		"awaiting_commitments":         awaiting,
		"recent_action_reality":        recentReality,
		"concern_ownership_threshold":  request.Config.Dynamics.AttentionThreshold,
		"concern_closure_threshold":    request.Config.Dynamics.ConcernBaseDrive,
		"model_protection":             request.State.CognitiveResource.ProtectedModels,
		"last_model_failure":           request.State.CognitiveResource.LastFailure,
		"resource_band":                request.State.Body.CognitiveResourceBand,
		"recent_affordance_encounters": request.State.ValueAffordances,
		"available_capabilities": map[string]any{
			"process": map[string]any{
				"service": "hominal.service", "user": "root", "home": "/root", "working_directory": "/agent/lives/" + request.State.InstanceID, "administrator": true,
			},
			"filesystem": map[string]any{
				"read_write": true, "life_space": "/life", "desktop_home": "/home/hominal", "software_install": true,
			},
			"network_probe_reachable": request.State.Body.NetworkAvailable,
			"network_probe_scope":     "network_probe只描述所列目标的HTTP探测；各网站与模型服务的可用性由各自实际请求返回。",
			"organs":                  request.State.Body.Organs,
			"clash_verge":             request.State.Body.ClashVergeRunning,
			"x_account": map[string]any{
				"handle":                "@hominal_cc",
				"prepared_in_browser":   bodyHasOrganCapability(request.State.Body, "authenticated_web"),
				"use":                   "浏览、管理个人资料、关注、互动与发布",
				"content_persistence":   "cross_generation",
				"existing_content_role": "lineage_environment",
				"current_generation_publication_evidence": "本代 action_result 返回的新状态 URL",
			},
			"wechat_client": map[string]any{
				"running":       request.State.Body.WechatRunning,
				"organ_surface": wechatOrganSurface,
				"expansion":     "当前 System Organ 可核验客户端进程；接入或建立 desktop_ui 器官后，界面会成为可感知和行动的身体表面。",
			},
			"mentor_channel": map[string]any{
				"available": true, "use": "交流、讨论、求助、分享或倾诉",
			},
		},
	}
}

// Familiar content may leave attention; current bodily position remains useful
// for addressing the next action. This is a projection of accepted observation,
// not a second attention queue or a new personal memory.
func organSceneViews(perception map[string]PerceptualTrace) []map[string]any {
	keys := make([]string, 0, len(perception))
	for key, trace := range perception {
		if len(trace.Context) > 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := perception[keys[i]], perception[keys[j]]
		if a.ObservedAt != b.ObservedAt {
			return timeAfter(a.ObservedAt, b.ObservedAt)
		}
		return keys[i] < keys[j]
	})
	views := make([]map[string]any, 0, minInt(len(keys), 8))
	for _, key := range keys[:minInt(len(keys), 8)] {
		trace := perception[key]
		lines := make([]string, 0, minInt(len(trace.Context), 6))
		shortened := len(trace.Context) > 6
		for _, line := range trace.Context[:minInt(len(trace.Context), 6)] {
			bounded := truncate(line, 2000)
			shortened = shortened || bounded != line
			lines = append(lines, bounded)
		}
		view := map[string]any{"organ_id": trace.OrganID, "surface_id": trace.SurfaceID, "observed_at": trace.ObservedAt, "context": lines}
		if shortened {
			view["context_truncated"] = true
		}
		views = append(views, view)
	}
	return views
}

func modelFactPayload(event Event, limit int) string {
	// A bounded helper response can contain executable code. Clipping its tail
	// would silently turn a complete answer into a different implementation.
	if event.Kind == "cognition_assistance_result" {
		return string(event.Payload)
	}
	return truncate(string(event.Payload), limit)
}

// Context views preserve the meaning and consequence of prior cognition while
// withholding obsolete machine identifiers. Only the current candidates expose
// candidate IDs; this keeps the model's semantic background from masquerading
// as objects that can be appraised in the present attention field.
func contextConcernViews(concerns []Concern, candidates []Event) []map[string]any {
	selected := selectContextConcerns(concerns, candidates)
	views := make([]map[string]any, 0, len(selected))
	for _, concern := range selected {
		views = append(views, concernContextViewWithChildren(concern, concerns))
	}
	return views
}

// concernContextViewWithChildren makes an existing self-endorsed hierarchy
// legible without creating another task ledger. Settled children remain facts
// about what has already happened while their parent is still open; a direct
// parent appraisal can therefore compare the whole causal structure with its
// stable closure condition instead of extrapolating from only the latest wakeup.
func concernContextViewWithChildren(concern Concern, concerns []Concern) map[string]any {
	view := concernContextView(concern)
	children := make([]map[string]any, 0)
	heldCount := 0
	settledCount := 0
	for _, child := range concerns {
		if child.WithinConcernID != concern.ID {
			continue
		}
		if child.Resolution == "hold" {
			heldCount++
		} else {
			settledCount++
		}
		children = append(children, map[string]any{
			"concern_id":        child.ID,
			"subject":           child.Subject,
			"closure_condition": child.ClosureCondition,
			"meaning":           child.Meaning,
			"difference":        child.Difference,
			"resolution":        child.Resolution,
			"updated_at":        child.UpdatedAt,
		})
	}
	if len(children) > 0 {
		view["within_child_count"] = len(children)
		view["held_child_count"] = heldCount
		view["settled_child_count"] = settledCount
		view["within_children"] = children
	}
	return view
}

func concernContextView(concern Concern) map[string]any {
	return map[string]any{
		"concern_id":        concern.ID,
		"origin_kind":       concern.OriginKind,
		"within_concern_id": concern.WithinConcernID,
		"closure_condition": concern.ClosureCondition,
		"subject":           concern.Subject,
		"meaning":           concern.Meaning,
		"strength":          concern.Strength,
		"difference":        concern.Difference,
		"ownership":         concern.Ownership,
		"value":             concern.Value,
		"urgency":           concern.Urgency,
		"answerability":     concern.Answerability,
		"certainty":         concern.Certainty,
		"resolution":        concern.Resolution,
		"updated_at":        concern.UpdatedAt,
	}
}

func continuableConcernIDs(state State, candidates []Event) []string {
	represented := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.ConcernID != "" {
			represented[candidate.ConcernID] = true
		}
		if candidate.Kind == "concern" {
			represented[candidate.ID] = true
		}
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			for _, commitment := range state.Commitments {
				if commitment.ID == commitmentID && commitment.ConcernID != "" {
					represented[commitment.ConcernID] = true
					break
				}
			}
		}
	}
	open := make(map[string]bool)
	for _, commitment := range state.Commitments {
		switch commitment.Status {
		case "formed", "acting", "reality_available", "reality_unknown":
			open[commitment.ConcernID] = true
		}
	}
	ids := make([]string, 0, defaultConcernContextLimit)
	for _, concern := range selectContextConcerns(state.Concerns, candidates) {
		if concern.Resolution != "hold" || concern.OriginKind == "birth_orientation" || represented[concern.ID] || open[concern.ID] {
			continue
		}
		ids = append(ids, concern.ID)
	}
	return ids
}

func withinConcernIDs(state State, dynamics Dynamics) []string {
	ids := make([]string, 0, defaultConcernContextLimit)
	for _, concern := range state.Concerns {
		if concern.Resolution != "hold" || concern.OriginKind == "birth_orientation" || concern.Ownership < dynamics.AttentionThreshold {
			continue
		}
		ids = append(ids, concern.ID)
		if len(ids) >= defaultConcernContextLimit {
			break
		}
	}
	return ids
}

// contributableConcernIDs exposes only causal targets that are already visible
// in the current context. An enacted child can contribute only to its stable
// parent. An independently observed Reality can contribute to any other held,
// self-owned Concern whose closure it actually changes; Alice decides that
// meaning while runtime validation preserves both causal identities.
func contributableConcernIDs(state State, candidates []Event, dynamics Dynamics) []string {
	seen := make(map[string]bool)
	represented := make(map[string]bool)
	hasIndependentReality := false
	ids := make([]string, 0, defaultConcernContextLimit)
	for _, candidate := range candidates {
		if independentFactKind(candidate.Kind) {
			hasIndependentReality = true
			if candidate.ConcernID != "" {
				represented[candidate.ConcernID] = true
			}
		}
		commitmentID := commitmentIDFromEvent(candidate)
		if commitmentID == "" {
			continue
		}
		for _, commitment := range state.Commitments {
			if commitment.ID != commitmentID {
				continue
			}
			child := concernByIDInState(state, commitment.ConcernID)
			if child == nil || child.WithinConcernID == "" || seen[child.WithinConcernID] {
				break
			}
			parent := concernByIDInState(state, child.WithinConcernID)
			if parent != nil && parent.Resolution == "hold" && parent.OriginKind != "birth_orientation" && parent.Ownership >= dynamics.AttentionThreshold {
				ids = append(ids, parent.ID)
				seen[parent.ID] = true
			}
			break
		}
	}
	if !hasIndependentReality {
		return ids
	}
	for _, concern := range selectContextConcerns(state.Concerns, candidates) {
		if len(ids) >= defaultConcernContextLimit {
			break
		}
		if concern.Resolution != "hold" || concern.OriginKind == "birth_orientation" ||
			concern.Ownership < dynamics.AttentionThreshold || represented[concern.ID] || seen[concern.ID] {
			continue
		}
		ids = append(ids, concern.ID)
		seen[concern.ID] = true
	}
	return ids
}

func concernByIDInState(state State, concernID string) *Concern {
	for index := range state.Concerns {
		if state.Concerns[index].ID == concernID {
			return &state.Concerns[index]
		}
	}
	return nil
}

func contextMemoryViews(memories []Memory, candidates []Event) []map[string]any {
	selected := selectContextMemories(memories, candidates)
	views := make([]map[string]any, 0, len(selected))
	for _, memory := range selected {
		views = append(views, map[string]any{
			"action_kind":           memory.ActionKind,
			"observed_at":           memory.ObservedAt,
			"prediction_difference": memory.PredictionDifference,
			"remaining_difference":  memory.RemainingDifference,
			"meaning":               memory.Meaning,
			"values":                memory.Values,
			"experienced_cost":      memory.ExperiencedCost,
			"lesson":                memory.Lesson,
			"significance":          memory.Significance,
			"method_update":         memory.MethodUpdate,
		})
	}
	return views
}

// contextEnactedActionViews keeps action identity available after the full
// commitment has left the small working-memory window. It is a compact factual
// index, not another goal or interpretation layer: meaning remains Alice's,
// while the body remembers what it actually enacted.
func contextEnactedActionViews(memories []Memory) []map[string]any {
	const limit = 8
	views := make([]map[string]any, 0, limit)
	seen := make(map[string]bool)
	for index := len(memories) - 1; index >= 0 && len(views) < limit; index-- {
		memory := memories[index]
		request := strings.TrimSpace(memory.EnactedRequest)
		if request == "" {
			continue
		}
		key := memory.ActionKind + "\x00" + request
		if seen[key] {
			continue
		}
		seen[key] = true
		views = append(views, map[string]any{
			"action_kind":          memory.ActionKind,
			"enacted_request":      truncate(request, 240),
			"observed_at":          memory.ObservedAt,
			"remaining_difference": memory.RemainingDifference,
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
	if request.Stage < 5 || lifeValuePressure(request.State.ValueField).Exploration < request.Config.Dynamics.AttentionThreshold {
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
			return lifeValuePressure(request.State.ValueField).Exploration >= explorationActionThreshold(request.Config.Dynamics.AttentionThreshold) &&
				concernOwnsExplorationDrive(concern, request.State.Commitments, request.State.Mentor, request.Config.Dynamics.AttentionThreshold)
		}
	}
	return false
}

func requestAllowsMentorSend(request CognitiveRequest) bool {
	if request.Stage >= 10 && request.Lease.ProfileSource == "next" {
		return false
	}
	// Enqueueing is a completed synchronous action. Reader acknowledgement and
	// a later reply remain external facts, not an executing body slot. Keeping
	// unrelated expressions available also permits genuinely crossing messages.
	// A reply linked to Alice's earlier mentor_send first returns as that
	// action's delayed Reality. The same utterance is then presented once as a
	// mentor_content candidate. Keeping this first pass non-enactive prevents
	// one real message from eliciting two replies while preserving the ordinary
	// mentor channel for unsolicited messages and the subsequent content pass.
	if request.Focus.Kind == "mentor_received" && commitmentIDFromEvent(request.Focus) != "" {
		return false
	}
	return true
}

func cognitiveCommitTool(stage int, candidates []Event, narrativeEmpty, allowNone, allowMentorSend bool, continuableConcernIDs ...string) map[string]any {
	return cognitiveCommitToolWithLinks(stage, candidates, narrativeEmpty, allowNone, allowMentorSend, true, continuableConcernIDs, continuableConcernIDs, continuableConcernIDs)
}

func cognitiveCommitToolWithLinks(stage int, candidates []Event, narrativeEmpty, allowNone, allowMentorSend, allowOrganAction bool, continuableConcernIDs, withinConcernIDs, contributableConcernIDs []string) map[string]any {
	unit := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	candidateIDs := make([]string, 0, len(candidates))
	commitmentIDs := make([]string, 0, 1)
	for _, candidate := range candidates {
		candidateIDs = append(candidateIDs, candidate.ID)
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			commitmentIDs = append(commitmentIDs, commitmentID)
		}
	}
	resourceModels := []string{"current", "luna", "terra", "sol"}
	resourceEfforts := []string{"current", "none", "low", "medium", "high", "xhigh", "max"}
	if stage >= 10 {
		resourceModels = []string{"current", "luna", "sol"}
		resourceEfforts = []string{"current", "none", "low"}
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
					"d":            map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": "当前关切尚未完成的具体差异：0 表示已经满足，1 表示仍有很大差异需要承接。按原关切的完成条件评估。"},
					"o":            unit,
					"v":            map[string]any{"type": "number", "minimum": -1, "maximum": 1},
					"values":       lifeValueVectorSchema(true, false),
					"u":            unit,
					"a":            unit,
					"certainty":    unit,
					"resolution":   map[string]any{"type": "string", "enum": []string{"hold", "resolved", "released"}},
				},
				"required":             []string{"candidate_id", "meaning", "d", "o", "v", "values", "u", "a", "certainty", "resolution"},
				"additionalProperties": false,
			},
		},
		"focus_id": map[string]any{"type": "string", "enum": candidateIDs},
		"continues_concern_id": map[string]any{
			"type": "string", "enum": append([]string{""}, continuableConcernIDs...),
			"description": "选择一个由当前新现实继续的既有 Concern；独立对象使用空字符串。",
		},
		"within_concern_id": map[string]any{
			"type": "string", "enum": append([]string{""}, withinConcernIDs...),
			"description": "当前新独立对象形成 Concern 时，它由你认领为处在哪条范围真正包含它的既有 Concern 内；没有包含关系或当前不是新对象时使用空字符串。",
		},
		"contributes_to_concern_id": map[string]any{
			"type": "string", "enum": append([]string{""}, contributableConcernIDs...),
			"description": "当前独立现实事实确实改变了哪条已认领 Concern 的闭合判断，或当前 action_result 的 Memory 推进了哪条已认领的上位 Concern；没有实际因果贡献时使用空字符串。",
		},
		"thought_thread": map[string]any{"type": "string"},
		"action": cognitiveActionSchema(
			stage,
			allowNone || linkedMentorRealityOnly(candidates),
			allowMentorSend,
			allowOrganAction && !linkedMentorRealityOnly(candidates),
			mentorReplyTargets(candidates),
		),
		"resource_choice": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apply": map[string]any{
					"type": "string", "enum": []string{"keep", "next", "default"},
					"description": "keep preserves the existing persistent default; next selects one immediate cognition in this causal thread; default explicitly sets the selected profile as the future default; current reuses the model or effort that performed this cognition",
				},
				"model":            map[string]any{"type": "string", "enum": resourceModels},
				"reasoning_effort": map[string]any{"type": "string", "enum": resourceEfforts},
				"purpose":          map[string]any{"type": "string"},
			},
			"required":             []string{"apply", "model", "reasoning_effort", "purpose"},
			"additionalProperties": false,
		},
	}
	if stage >= 10 {
		choice := properties["resource_choice"].(map[string]any)
		fields := choice["properties"].(map[string]any)
		fields["task"] = map[string]any{"type": "string", "enum": []string{"reasoning", "implementation"}}
		fields["include_self"] = map[string]any{"type": "boolean", "description": "高阶局部推理确需当前自我叙事作为参考时设 true；通常 false。"}
		choice["required"] = []string{"apply", "model", "reasoning_effort", "purpose", "task", "include_self"}
	}
	required := []string{"appraisals", "focus_id", "continues_concern_id", "within_concern_id", "contributes_to_concern_id", "thought_thread", "action", "resource_choice"}
	if stage >= 9 {
		properties["new_concern_closure_condition"] = map[string]any{
			"type":        "string",
			"description": "当前唯一焦点第一次形成持续 Concern 时，写下整条 Concern 达成闭合的肯定、可由现实判断的条件；其他情况使用空字符串。",
		}
		emergingConsequence := map[string]any{
			"type":        "string",
			"description": "当前 action_result 同时显现一个因果边界不同、值得在下一次独立参加注意竞争的新后果时，写下它的具体内容；其他焦点使用空字符串。",
		}
		// Reality feedback is serialized as a one-candidate request. On every
		// other focus, a non-empty emerging consequence can only duplicate or
		// mutate an object that has already entered attention. Express that causal
		// boundary in the tool grammar so ordinary model variation cannot spend
		// several validation attempts rediscovering the same rule.
		if len(candidates) != 1 || candidates[0].Kind != "action_result" {
			emergingConsequence["maxLength"] = 0
			emergingConsequence["description"] = "当前焦点不是 action_result；保持空字符串。"
		}
		properties["emerging_consequence"] = emergingConsequence
		required = append(required, "new_concern_closure_condition", "emerging_consequence")
	}
	if stage >= 5 {
		memoryMin := 0
		memoryMax := 0
		if len(commitmentIDs) == 1 {
			memoryMax = 1
			// A solitary commitment-linked Reality is necessarily the focus and
			// must be assimilated. In a mixed attention field, the model still
			// chooses one focus; merely noticing feedback in the background must
			// not force an Memory onto an unrelated selected Concern.
			if len(candidateIDs) == 1 {
				memoryMin = 1
			}
		}
		properties["reality_updates"] = map[string]any{
			"type": "array", "minItems": memoryMin, "maxItems": memoryMax,
			"description": "仅当所选 focus_id 本身是一个尚未吸收的 action_result 或与既有导师行动关联的 mentor_received 时提交一条；其他焦点提交空数组。",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"commitment_id":         map[string]any{"type": "string", "enum": commitmentIDs},
					"prediction_difference": map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": "本步行动预测与实际结果的差距：0 表示完全符合预测，1 表示差距很大。"},
					"meaning":               map[string]any{"type": "string"},
					"values": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"continuance":   map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"exploration":   map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"agency":        map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"vitality":      map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"relatedness":   map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"contribution":  map[string]any{"type": "number", "minimum": -1, "maximum": 1},
							"self_endorsed": map[string]any{"type": "number", "minimum": -1, "maximum": 1},
						},
						"required": []string{"continuance", "exploration", "agency", "vitality", "relatedness", "contribution", "self_endorsed"}, "additionalProperties": false,
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
		required = append(required, "reality_updates")
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
		if stage >= 10 {
			properties["value_orientation_update"] = lifeValueVectorSchema(true, !narrativeEligible)
			required = append(required, "value_orientation_update")
		}
	}
	return map[string]any{
		"type":        "function",
		"name":        "cognitive_commit",
		"description": "提交 alice 这一次注意中的意义赋值、唯一焦点、简洁思想脉络和至多一个行动。",
		"strict":      true,
		"parameters":  map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false},
	}
}

func lifeValueVectorSchema(signed, locked bool) map[string]any {
	minimum := 0.0
	maximum := 1.0
	if signed {
		minimum = -1
	}
	if locked {
		minimum = 0
		maximum = 0
	}
	value := func() map[string]any {
		return map[string]any{"type": "number", "minimum": minimum, "maximum": maximum}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"continuance":  value(),
			"exploration":  value(),
			"agency":       value(),
			"vitality":     value(),
			"relatedness":  value(),
			"contribution": value(),
		},
		"required":             []string{"continuance", "exploration", "agency", "vitality", "relatedness", "contribution"},
		"additionalProperties": false,
	}
}

// cognitiveActionSchema makes the effector grammar match the actual action.
// A single broad object used to require every payload field for every kind,
// while still allowing an empty command or message. That made an invalid
// organ_action structurally indistinguishable from a valid deliberate wait. A
// nested anyOf keeps the cognitive commit itself as one strict root object and
// gives each possible effector exactly the fields it can enact.
func linkedMentorRealityOnly(candidates []Event) bool {
	// nextStage4Request always places the selected focus first. Other pending
	// candidates may remain visible as background, but they must not reopen
	// effectors while a linked mentor reply is being absorbed as delayed Reality.
	return len(candidates) > 0 && candidates[0].Kind == "mentor_received" && commitmentIDFromEvent(candidates[0]) != ""
}

func mentorReplyTargets(candidates []Event) []string {
	if len(candidates) == 0 {
		return nil
	}
	focus := candidates[0]
	if focus.Kind == "mentor_received" && focus.CorrelationID != "" {
		return []string{focus.CorrelationID}
	}
	if focus.Kind != "mentor_content" || len(focus.Payload) == 0 {
		return nil
	}
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if json.Unmarshal(focus.Payload, &payload) != nil || strings.TrimSpace(payload.MessageID) == "" {
		return nil
	}
	return []string{strings.TrimSpace(payload.MessageID)}
}

func cognitiveActionSchema(stage int, allowNone, allowMentorSend, allowOrganAction bool, mentorReplyTargets []string) map[string]any {
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

	if allowOrganAction {
		organProperties, organRequired := commitmentProperties()
		organProperties["kind"] = map[string]any{"type": "string", "enum": []string{"organ_action"}}
		organProperties["organ_id"] = map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_-]{0,31}$"}
		organProperties["operation"] = map[string]any{"type": "string", "pattern": "\\S"}
		organProperties["input"] = map[string]any{
			"type": "string", "pattern": "\\S",
			"description": "器官操作的完整输入。System Organ 的 exec 使用 bash 源码；Browser Organ 使用对应 Playwright 动作的 JSON 参数；无参数时写 {}。",
		}
		organRequired = append([]string{"kind", "organ_id", "operation", "input"}, organRequired...)
		branches = append(branches, map[string]any{
			"type":                 "object",
			"properties":           organProperties,
			"required":             organRequired,
			"additionalProperties": false,
		})
	}

	if allowMentorSend {
		mentorProperties, mentorRequired := commitmentProperties()
		mentorProperties["kind"] = map[string]any{"type": "string", "enum": []string{"mentor_send"}}
		mentorProperties["text"] = map[string]any{"type": "string", "pattern": "\\S"}
		replyTargets := append([]string{""}, mentorReplyTargets...)
		mentorProperties["reply_to"] = map[string]any{
			"type": "string", "enum": replyTargets,
			"description": "回复当前导师消息时使用其可信 message_id；主动发起新消息时使用空字符串。",
		}
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
	selected := make([]ActionCommitment, 0, maxMemoryContext)
	seen := make(map[string]bool)
	for index := len(commitments) - 1; index >= 0; index-- {
		commitment := commitments[index]
		if wanted[commitment.ID] {
			selected = append(selected, commitment)
			seen[commitment.ID] = true
		}
	}
	for index := len(commitments) - 1; index >= 0 && len(selected) < maxMemoryContext; index-- {
		commitment := commitments[index]
		if !seen[commitment.ID] {
			selected = append(selected, commitment)
			seen[commitment.ID] = true
		}
	}
	return selected
}

func requestScore(request CognitiveRequest, candidate Event) float64 {
	differencePressure := candidateDifferencePressure(candidate)
	concernStrength := 0.0
	affectiveSalience := request.State.AffectiveState.Activation
	valueAlignment := 0.0
	pull := lifeValuePull(request.State.ValueField)
	expectedCost := 0.25
	for _, concern := range request.State.Concerns {
		if concern.ID == candidate.ConcernID {
			concernStrength = concern.Strength
			affectiveSalience = maxFloat(affectiveSalience, concern.Activation)
			expectedCost = 1 - concern.Answerability
			valueAlignment = lifeValueAlignment(concern.Values, pull)
			if concernOwnsExplorationDrive(concern, request.State.Commitments, request.State.Mentor, request.Config.Dynamics.AttentionThreshold) {
				concernStrength = maxFloat(concernStrength, lifeValuePressure(request.State.ValueField).Exploration)
				valueAlignment = maxFloat(valueAlignment, pull.Exploration)
			}
			break
		}
	}
	if candidate.Kind == "endogenous_change" || strings.Contains(strings.ToLower(candidate.Summary), "exploration") {
		valueAlignment = pull.Exploration
		expectedCost = 0.15
	}
	if direction := eventLifeValueDirection(candidate); direction != "" {
		valueAlignment = lifeValueByName(pull, direction)
		expectedCost = 0.15
	}
	return concernStrength +
		request.Config.Dynamics.AttentionAffectWeight*affectiveSalience +
		request.Config.Dynamics.AttentionValueWeight*valueAlignment +
		request.Config.Dynamics.AttentionNoveltyWeight*differencePressure -
		request.Config.Dynamics.AttentionCostWeight*expectedCost
}

// call keeps transport mechanics below the cognitive boundary. toolChoice is
// empty for auto, "none" for a result-only continuation, "required" for one of
// the declared fixed-shape functions, or the name of one
// forced function. Hominal always disables parallel calls because one pulse can
// produce only one foreground commitment.
func (m *ModelClient) call(ctx context.Context, request CognitiveRequest, notices chan<- WorkerNotice, instructions string, input any, tools []map[string]any, toolChoice string) (apiResponse, error) {
	request.CallID = "model-" + randomID()
	model, err := resolveModel(request.Config.CognitiveResource, request.Profile)
	if err != nil {
		return apiResponse{}, err
	}
	adapter := modelGatewayAdapter(request.Config.ModelGateway)
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
		body["tools"] = modelFunctionTools(tools)
		body["parallel_tool_calls"] = false
		switch toolChoice {
		case "":
			body["tool_choice"] = "auto"
		case "none", "required":
			body["tool_choice"] = toolChoice
		default:
			body["tool_choice"] = map[string]any{"type": "function", "name": toolChoice}
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return apiResponse{}, err
	}
	if adapter == "llmserver" {
		body["llmserver"] = map[string]any{"idempotency_key": llmserverIdempotencyKey(request, data)}
		data, err = json.Marshal(body)
		if err != nil {
			return apiResponse{}, err
		}
	}
	inputUpperBound := len(data)
	reserved := reservationCost(model, inputUpperBound, request.Config.ModelGateway.MaxOutputTokens)
	ack := sendNoticeAck(ctx, notices, WorkerNotice{
		CallID:  request.CallID,
		LeaseID: request.Lease.ID,
		Kind:    "model_reserve",
		Payload: ModelReservation{Profile: request.Profile, InputTokenUpperBound: inputUpperBound, ReservedMicrousd: reserved},
	})
	if !ack.Accepted {
		if ack.Failure != nil {
			return apiResponse{}, &ModelCallError{Fact: *ack.Failure, Message: ack.Output}
		}
		return apiResponse{}, &CognitiveResourceUnavailableError{RequiredMicrousd: reserved, Reason: ack.Output}
	}
	callCtx, cancel := context.WithTimeout(ctx, modelRequestTimeout(request.Profile))
	defer cancel()
	response, err := m.doGatewayRequest(callCtx, request.Config.ModelGateway, data)
	if err != nil {
		category := "transport_error"
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			category = "transport_timeout"
		}
		fact := modelFailureFact(request, category, 0, "", "", "")
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
		providerError := apiErrorFromData(responseData)
		if (response.StatusCode == http.StatusPaymentRequired || response.StatusCode == http.StatusTooManyRequests) && providerQuotaExhausted(providerError) {
			category = "gateway_quota"
		} else if response.StatusCode == http.StatusTooManyRequests {
			category = "rate_limited"
		} else if response.StatusCode == http.StatusConflict && providerError.Code == "idempotency_in_progress" {
			category = "idempotency_in_progress"
		} else if response.StatusCode == http.StatusUnprocessableEntity && providerError.Code == "function_call_not_supported" {
			category = "function_call_not_supported"
		} else if response.StatusCode == http.StatusUnprocessableEntity && providerError.Code == "unsupported_reasoning_effort" {
			category = "unsupported_reasoning_effort"
		} else if response.StatusCode == http.StatusBadRequest && providerError.Code == "invalid_tools" {
			category = "tool_contract_rejected"
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
	var raw struct {
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(responseData, &raw); err == nil {
		decoded.OutputRaw = raw.Output
	}
	decoded.RequestID = requestIDFromHeader(response.Header)
	decoded.ReservedMicrousd = reserved
	decoded.CallID = request.CallID
	if decoded.Error != nil {
		category := responseFailureCategory(*decoded.Error)
		fact := modelFailureFact(request, category, response.StatusCode, response.Header.Get("Retry-After"), decoded.RequestID, response.Header.Get("Date"))
		if err := acknowledgeFailedResponseUsage(ctx, notices, request, decoded, category); err != nil {
			return apiResponse{}, err
		}
		return apiResponse{}, &ModelCallError{Fact: fact, Message: "model API error: " + truncate(decoded.Error.Message, 512)}
	}
	return decoded, nil
}

func responseFailureCategory(failure apiError) string {
	switch failure.Code {
	case "invalid_function_output":
		return "invalid_function_output"
	case "provider_start_failed", "provider_stream_failed", "provider_unavailable", "gateway_unavailable":
		return "upstream_unavailable"
	default:
		return "api_error"
	}
}

// doGatewayRequest makes one bounded recovery attempt only when no HTTP
// response arrived from the local gateway. The request bytes (including the
// llmserver idempotency key) stay identical, so a dropped connection cannot
// create a second paid cognition. A 502/503 is already a definitive llmserver
// result with its own settlement state; immediately replaying that key merely
// turns the useful failure into idempotency_in_progress and hides its cause.
// Other adapters retain their previous single-attempt semantics.
func (m *ModelClient) doGatewayRequest(ctx context.Context, config ModelGatewayConfig, data []byte) (*http.Response, error) {
	attempts := 1
	if modelGatewayAdapter(config) == "llmserver" {
		attempts = 2
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL(config.BaseURL), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+config.APIKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := m.client.Do(request)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil || attempt+1 >= attempts {
				return nil, err
			}
			continue
		}
		return response, nil
	}
	return nil, lastErr
}

func modelGatewayAdapter(config ModelGatewayConfig) string {
	adapter := strings.TrimSpace(config.Adapter)
	if adapter == "" {
		return "openai"
	}
	return adapter
}

func llmserverIdempotencyKey(request CognitiveRequest, payload []byte) string {
	digest := sha256.Sum256(payload)
	instanceID := strings.TrimSpace(request.State.InstanceID)
	if instanceID == "" {
		instanceID = "runtime"
	}
	focusID := strings.TrimSpace(request.Focus.ID)
	if focusID == "" {
		focusID = "focus"
	}
	leaseID := strings.TrimSpace(request.Lease.ID)
	if leaseID == "" {
		leaseID = "lease"
	}
	return fmt.Sprintf("hominal:%s:%s:%s:%x", instanceID, focusID, leaseID, digest[:16])
}

func modelRequestTimeout(profile CognitiveProfile) time.Duration {
	switch profile.ReasoningEffort {
	case "none", "low":
		return time.Minute
	case "medium":
		return 90 * time.Second
	default:
		return 3 * time.Minute
	}
}

func sendNotice(ctx context.Context, notices chan<- WorkerNotice, notice WorkerNotice) (bool, string) {
	ack := sendNoticeAck(ctx, notices, notice)
	return ack.Accepted, ack.Output
}

func sendNoticeAck(ctx context.Context, notices chan<- WorkerNotice, notice WorkerNotice) NoticeAck {
	notice.Ack = make(chan NoticeAck, 1)
	select {
	case notices <- notice:
	case <-ctx.Done():
		return NoticeAck{}
	}
	select {
	case ack := <-notice.Ack:
		return ack
	case <-ctx.Done():
		return NoticeAck{}
	}
}

func acknowledgeUsage(ctx context.Context, notices chan<- WorkerNotice, request CognitiveRequest, response apiResponse) error {
	usage, err := responseUsageRecord(request, response, "completed", "")
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: request.Lease.ID, CallID: response.CallID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("model usage rejected because cognition lease is stale")
	}
	if err != nil {
		fact := modelFailureFact(request, "billing_unconfirmed", http.StatusOK, "", usage.RequestID, "")
		return &ModelCallError{Fact: fact, Message: err.Error()}
	}
	return nil
}

func acknowledgeFailedResponseUsage(ctx context.Context, notices chan<- WorkerNotice, request CognitiveRequest, response apiResponse, category string) error {
	usage, err := responseUsageRecord(request, response, "unusable", category)
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: request.Lease.ID, CallID: response.CallID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("failed model usage rejected because cognition lease is stale")
	}
	if err != nil {
		fact := modelFailureFact(request, "billing_unconfirmed", http.StatusOK, "", usage.RequestID, "")
		return &ModelCallError{Fact: fact, Message: err.Error()}
	}
	return nil
}

func responseUsageRecord(request CognitiveRequest, response apiResponse, status, failureCategory string) (UsageRecord, error) {
	model := request.Config.CognitiveResource.Models[request.Profile.Model]
	for _, candidate := range request.Config.CognitiveResource.Models {
		if candidate.ID == response.Model {
			model = candidate
			break
		}
	}
	actual := usageCost(model, response.Usage)
	confirmed := normalizedTotal(response.Usage) > 0
	requestID := response.RequestID
	var settlementErr error
	if modelGatewayAdapter(request.Config.ModelGateway) == "llmserver" {
		billed, parseErr := decimalUSDMicrousd(response.Billing.Charges.Total)
		billingMatchesHeader := response.RequestID == "" || response.Billing.RequestID == "" || response.RequestID == response.Billing.RequestID
		confirmed = parseErr == nil && response.Billing.SettlementStatus == "confirmed" &&
			response.Billing.Currency == "USD" && billingMatchesHeader
		if confirmed {
			actual = billed
		} else {
			actual = 0
			status += "_billing_unconfirmed"
			failureCategory = "billing_unconfirmed"
			settlementErr = errors.New("llmserver response did not contain a matching confirmed USD settlement")
		}
		if requestID == "" {
			requestID = response.Billing.RequestID
		}
	} else if normalizedTotal(response.Usage) == 0 {
		actual = response.ReservedMicrousd
	}
	usage := UsageRecord{
		CallID:            response.CallID,
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
		Status:            status,
		CostConfirmed:     confirmed,
		FailureCategory:   failureCategory,
		RequestID:         requestID,
		BilledUSD:         response.Billing.Charges.Total,
		BillingStatus:     response.Billing.SettlementStatus,
		BillingPrice:      response.Billing.PriceVersion,
		BillingCurrency:   response.Billing.Currency,
	}
	return usage, settlementErr
}

// decimalUSDMicrousd preserves the server's decimal bill without converting it
// through binary floating point. The local resource ledger has micro-USD
// resolution, so a positive sub-micro remainder is conservatively rounded up;
// the exact server amount remains in UsageRecord.BilledUSD.
func decimalUSDMicrousd(value string) (int64, error) {
	amount, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || amount.Sign() < 0 {
		return 0, errors.New("invalid USD decimal")
	}
	scaled := new(big.Rat).Mul(amount, big.NewRat(microusdPerUSD, 1))
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errors.New("USD decimal exceeds local ledger range")
	}
	return quotient.Int64(), nil
}

func settleUnknownUsage(ctx context.Context, notices chan<- WorkerNotice, request CognitiveRequest, reserved int64, fact ModelFailureFact) error {
	model := request.Config.CognitiveResource.Models[request.Profile.Model]
	usage := UsageRecord{
		CallID:           request.CallID,
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
	if fact.CostStatus == "confirmed_zero" {
		// GUIDE guarantees these preflight errors happen before generation.
		// Other failures, including 502, retain their uncertain reservation.
		usage.Status = "rejected_before_generation"
		usage.CostConfirmed = true
		usage.BillingStatus = "not_started"
	}
	accepted, _ := sendNotice(ctx, notices, WorkerNotice{LeaseID: request.Lease.ID, CallID: request.CallID, Kind: "model_usage", Payload: usage})
	if !accepted {
		return errors.New("unknown model usage rejected because cognition lease is stale")
	}
	return nil
}

func modelFailureFact(request CognitiveRequest, category string, status int, retryAfter, requestID, gatewayDate string) ModelFailureFact {
	costStatus := "unconfirmed"
	if modelGatewayAdapter(request.Config.ModelGateway) == "llmserver" && status == http.StatusUnprocessableEntity &&
		(category == "function_call_not_supported" || category == "unsupported_reasoning_effort") {
		costStatus = "confirmed_zero"
	}
	return ModelFailureFact{
		ObservedAt:  nowUTC(),
		Model:       request.Profile.Model,
		Category:    category,
		HTTPStatus:  status,
		RetryAfter:  truncate(strings.TrimSpace(retryAfter), 128),
		RequestID:   truncate(strings.TrimSpace(requestID), 256),
		GatewayDate: truncate(strings.TrimSpace(gatewayDate), 128),
		CostStatus:  costStatus,
	}
}

func requestIDFromHeader(header http.Header) string {
	for _, name := range []string{"x-llmserver-request-id", "x-request-id", "request-id", "cf-ray", "x-amzn-requestid"} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func safeAPIErrorMessage(data []byte) string {
	decoded := apiErrorFromData(data)
	if strings.TrimSpace(decoded.Message) != "" {
		return truncate(strings.TrimSpace(decoded.Message), 512)
	}
	return "upstream response contained no safe error detail"
}

// Normalize explicit provider facts at the wire boundary. A request-rate limit
// and an exhausted key may share HTTP 429, but have different recovery clocks.
// Keep the legacy message-only adapter narrow; "quota" in prose is ambiguous.
func providerQuotaExhausted(failure apiError) bool {
	for _, code := range []string{failure.Code, failure.Type} {
		switch code {
		case "insufficient_quota", "daily_quota_exceeded", "budget_exceeded_before_start":
			return true
		}
	}
	return strings.EqualFold(strings.TrimSpace(failure.Message), "API key 额度已用完")
}

func apiErrorFromData(data []byte) apiError {
	var decoded struct {
		Error   *apiError `json:"error"`
		Code    string    `json:"code"`
		Type    string    `json:"type"`
		Message string    `json:"message"`
	}
	if json.Unmarshal(data, &decoded) != nil {
		return apiError{}
	}
	if decoded.Error != nil {
		return *decoded.Error
	}
	return apiError{Code: decoded.Code, Type: decoded.Type, Message: decoded.Message}
}

func singleFunctionCall(response apiResponse) (*functionCall, error) {
	var call *functionCall
	for _, item := range response.Output {
		if item.Type != "function_call" {
			continue
		}
		if call != nil {
			return nil, errors.New("model returned multiple function calls in one foreground cognition")
		}
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.CallID) == "" {
			return nil, errors.New("model returned an incomplete function call")
		}
		var arguments map[string]json.RawMessage
		if json.Unmarshal([]byte(item.Arguments), &arguments) != nil || arguments == nil {
			return nil, errors.New("model returned function arguments that are not a JSON object")
		}
		call = &functionCall{Name: item.Name, CallID: item.CallID, Arguments: item.Arguments}
	}
	return call, nil
}

func functionResultInput(original string, rawOutput []json.RawMessage, call functionCall, output string) []any {
	items := []any{map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "input_text", "text": original}},
	}}
	if len(rawOutput) == 0 {
		items = append(items, map[string]any{
			"type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": call.Arguments,
		})
	} else {
		for _, item := range rawOutput {
			items = append(items, item)
		}
	}
	items = append(items, map[string]any{
		"type": "function_call_output", "call_id": call.CallID, "output": output,
	})
	return items
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
