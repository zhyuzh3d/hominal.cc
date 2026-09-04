# Hominal 内核—器官架构

> 版本：v1.4  
> 日期：2026-09-02  
> 状态：G0 实施规范  
> 当前适配器官：System Organ、Browser Organ（Chrome + Playwright MCP）

## 1. 架构命题

2026-09-04 / Stage 10.3 实施增量：主脑单线程、核心单一状态写入，慢速身体扫描和器官观察改为可取消的异步工作。观察附带动作现场 epoch，迟到的旧现场不覆盖新现场；资源余额始终由核心当前账本计算。仍保留一个 System / Browser 身体写操作槽；只读观察可与主脑并行，改变现场的 orient 保持互斥。

Observation 可附一个极简 `interpret: {question, material}`，由器官表达局部解释需求。首个 Browser 实现只在含糊页面反馈出现时请求 Luna/none；一次主脑加一次局部解释为首版并发上限，相同材料复用，十五秒内结束。它仅见局部材料、返回带来源的假说，不读取个人自我叙事、不执行新动作。确定性检测继续由器官代码承担。所有模型请求走同一计费入口，取消后的账单仍结算。

主脑主动请求的简单逻辑或高阶复杂逻辑、代码协助是另一种调用位置，复用唯一前景租约串行完成，结果交还主脑采用，再通过器官执行；它没有第二套自我状态或独立行动权。低阶与高阶默认只见必要材料，高阶可按明确请求参考当前叙事。当前角色可映射至 Flash/none 与 Pro/low，具体职责及模型事实见[认知资源机制](./cognitive-resource-autonomy.md)。

共享模型服务的恢复状态直接由既有请求账本推导。连续基础设施错误按 10 秒起、指数增长至既有保护上限的节律退避，并尊重有界 Retry-After；主脑调度与实际请求预留两处使用同一判断。恢复时先允许一次普通主脑请求，成功响应后恢复器官并发；切换焦点或本能模型不绕过故障服务的退避。被本地拦住的请求不计费、不消耗语义校验次数，也不被写成额度耗尽。身体扫描和事实接收继续运行，原有个人认知保持。该机制不替代模型能力失败的独立处理。

局部操作困难共用原Difference Field：`operation_condition`按器官与操作保存failed/unknown/recovered、连续未成功次数、时间和原始错误。被动观察反复失败才逐步进入注意，同源至多一个活跃候选，最新六项状态进入`current_situation.operation_conditions`。主动行动已有action_result，仅更新这份身体背景；不为同一次结果再调用一轮主脑。转向和随后的读取分别归因，主动取消及迟到结果保持原取消/过期身份，不冒充故障或恢复。

主脑连续无法解释某条action_result时，内核保留原事实及行动意愿，局部延期并允许其他焦点继续；同一关切、同一器官及相同外部请求仍等待该结果被吸收，身体主动动作并发上限不变。后台只读感知仍可获取当前事实，原器官的被动转向等待吸收以保护现场。`current_situation.deferred_reality`说明尚待解释的结果与重试时间。该调度边界不分析跨器官任意代码的读写依赖，也不会自动重放unknown动作。详见[局部恢复计划](../plans/g0-stage-10-3-local-recovery-and-resilient-continuity.md)。

输出契约错误按相同作用域处理：llmserver明确返回`invalid_provider_tool_call`，表示这一份输出不符合参数要求，与本地提交校验同属该次认知的修正问题；它不单独证明整个模型不可用。服务/传输故障、模型不支持函数调用、额度不足仍使用各自保护边界。即使输出未被接受，未确认费用仍保守保留，不由恢复代码免除或改成成功结算。

任意 watch 条件、完整动作暂停/恢复协议和跨器官并行写操作仍是后续按证据推进的能力，不计入这一切片的已实现项。现有动作最终结果、超时/取消和未知副作用核验机制继续保留。

Browser 按实际渲染文字及文本 Range 的视野交集读取当前主文档视口，按块容器归并。网页采用 `div/span/a` 或传统段落标签都能提供材料，单个字符也是正常内容。器官负责完成这个读取范围，不根据内容是否丰富、有意义或符合预期决定隐去、重试或宣布失败。

通用文档与 X 的主动 snapshot、被动感知和动作后采样共用 `readViewportText`；从整个 body 遍历实际可见文字，保留正文、界面文字及 main 之外的内容。移除原先 18 块、700 字符、6000 节点的静默截断，传输不再将完整结果重新送进有损快照筛选器。返回的 `text_coverage` 明确本次主文档视口及遍历完成事实；超出安全边界则报告 `read_incomplete`，不以标注局部为借口返回完成。读取整个未来滚动时间线、未出现的内容或像素解码不属于这次文本视口操作。X 的对象、链接和控件是完整文字之上的寻址辅助，不替代文字读取。注意选择与认知上下文压缩仍属于内核，原始动作记录保留器官取得的内容。

页面通知属于全局现场：Browser 统一采样可见的 alert/status/live 区域及其真实链接，控件所属 main/dialog 不是反馈范围的边界。通知即使位于对话框之外，或对话框关闭后仍显示，也可进入既有 Observation；重复、隐藏及过量文本有界过滤。器官提供带来源的页面事实，行动是否真正实现仍由主脑结合预期与结果判断。

器官内容的新颖性与身体当前的位置分开处理：已见内容可退出注意，最近一次接收的 `organ_id / surface_id / observed_at / context` 仍以 `current_situation.organ_scenes` 进入每次主脑上下文。它来自已有 Perception 状态，不新增记忆库或注意队列。转向与随后观察在既有二十秒上限内先结算，主脑再复制现场；取消不被当成转向没有发生。转向失败使该器官旧位置失效，已见内容保留，下一次成功观察恢复位置；只读感知仍与主脑并行。动作目标由 Alice 决定，器官按完整地址和操作参数执行。Browser 的导航去重保留 URL 片段、路径尾斜杠和查询参数的区别。

连接恢复同时保持操作对象的一致性。Browser daemon 以 Chrome target_id 保存最后确认的物理页面，MCP 重连后恢复同一页面并复核，再执行已确定操作；网址、标题和会话内索引都不替代物理身份。原页消失时返回未执行事实，新的 observe 或 tab list 可建立当前现场，随后由主脑重新选择。恢复后旧元素 ref 需通过 find/snapshot 重获，旧 tab index 需重新 list。状态只由器官维护，不向认知增加另一套任务或人格字段；初始化、恢复、执行及后验共用截止，不自动重放未知副作用。这个保证覆盖同一 daemon 内的 MCP 重连，整个器官进程重启仍以重新建立现场开始，不能冒称所有会话状态已经跨进程持久化。

页面身份取得和恢复寻址同样使用本次操作剩余时间，移除先前隐藏的1500/2500ms子截止。冷启动建立页面连接是实际工作，不应在整体额度仍充足时被子步骤提前杀死；健康查询与取消仍绕开工作队列，整体截止继续有限。

Hominal 只有一个中央认知主体和一条统一 Life Dynamics 因果线。Cognitive Core 维持认知动力学，Organ Host 与身体器官完成感知和动作实施；三者共同构成生命动力学，而不是把生命动力学缩成内核中的一套公式。

浏览器、System、微信、摄像头、语音、图像生成器与未来硬件都属于可替换、可扩展的身体器官，不拥有独立的意义判断、关切、人格或生活目标。

内核回答：

- 当前什么事实能够进入注意；
- Alice 是否认领它，以及它对她意味着什么；
- 是否形成 Action Commitment（行动意愿）；
- 哪项主动行动取得唯一身体执行权；
- Reality 如何回到 Experience、Concern、Affective State 与 Narrative Self。

器官回答：

- 自己能够观察和执行什么；
- 当前是可用、忙碌、恢复中还是不可用；
- 怎样连接具体软件、协议或硬件；
- 怎样完成、取消并如实报告一次身体操作；
- 怎样维护标签页、窗口、设备句柄等器官内部状态。

核心边界是：

> 内核决定为何接近、是否行动和怎样理解；器官负责怎样感知与实施，并只返回事实。

## 2. 三层结构

```text
┌──────────────────────────────────────────────────────────┐
│ Cognitive / Conatus Core                                  │
│ Pulse / Difference / AIP / Concern / Attention            │
│ Commitment / Reality / Experience / Narrative Self        │
└───────────────────────────┬──────────────────────────────┘
                            │ Organ Contract v1
┌───────────────────────────▼──────────────────────────────┐
│ Organ Host                                                │
│ discovery / lifecycle / health / priority / deadline      │
│ cancellation / observation admission / Reality envelope   │
└───────────────┬───────────────────┬──────────────────────┘
                │                   │
      ┌─────────▼─────────┐  ┌──────▼──────────┐
      │ System Organ      │  │ Browser / Future Organs    │
      │ Ubuntu interface  │  │ Chrome/WeChat/voice/...    │
      └───────────────────┘  └─────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────┐
│ Body Substrate: hardware + Ubuntu + filesystem + network │
└──────────────────────────────────────────────────────────┘
```

Cognitive Core 承载认知动力学与唯一自我归属；Organ Host 是认知核心与具体身体之间的统一运行边界；Body Substrate 是承载全部进程的实际身体。每个 Organ Adapter 可以是独立进程，使用不同语言、依赖、专用模型或代理式控制方式，但不能直接写入 Concern、Attention、Experience 或 Narrative Self。

器官可以在一个已冻结的 Commitment 内完成多步机械控制。它可以寻找控件、恢复连接、编写执行代码和核验结果，却不能重新决定行动对象、受众、表达内容、价值理由或成功含义。准确称谓是“代理式器官控制器”，不是拥有第二套生命动力学的任务智能体。

### 2.1 单一生命主体与代理式器官

是否允许器官使用 Codex、WorkBuddy 或其他模型，不由任务表面上有多少步骤决定，而由执行中还剩多少**语义自由度**决定：

- 已经确定目标文件、功能边界和验收条件后，System Organ 可以让代码模型完成实现、运行与核验；
- 已经确定页面、受众和原文后，Browser Organ 可以完成导航、定位、输入、点击与结果核验；
- 仍需决定表达什么、联系谁、选择哪个目标、是否承担后果或怎样理解结果时，器官必须暂停并把事实返回 Cognitive Core；
- “经营账号”“调查一个主题”“让项目变得更好”这类开放任务包含新的价值判断和连续决策，不能整体下放给器官形成一个平行行动主体。

因此，器官可以比传统驱动程序聪明，也可以在技术上表现为短寿命任务代理；但它只拥有派生的、随 Commitment 结束而结束的控制权。主脑始终维护正在执行、已经阻塞、结果未知和等待回来的身体行动，并只让稀疏事实变化进入统一事件口。

器官内部可以使用四类计算底材，但不能因此获得第二套生命动力学：确定性程序负责时限、状态和机械控制；在线统计或经过回放验证的传统机器学习负责高频固定格式感知；Luna/none 一类快速模型可以把短小非结构化输入压缩成带不确定性的事实假说；主脑模型只存在于 Cognitive Core。器官专用模型不能读取完整 Narrative Self 来自行决定什么值得 Alice 追求，也不能把语义猜测伪装成已经核验的现实。

### 2.2 模型函数与身体器官

模型 API 的 function call 是主脑与认知模型之间的结构化传输，不等于器官动作。每次主意识认知只声明一个严格的 `cognitive_commit`，强制模型提交当次意义赋值、唯一焦点和至多一个行动，并关闭并行调用。Browser、System 和未来器官的全部 operations 不直接注册为模型工具；它们只作为身体事实进入认知上下文，再由 `cognitive_commit.action` 形成行动意愿。内核随后验证候选身份、器官、operation 与参数，才把动作交给 Organ Host。

这一边界避免模型绕过 AIP、Concern 和单线程注意直接操纵身体，也避免把全部器官动作 Schema 注入每次请求。llmserver 只校验和返回 `function_call`，不执行任何器官；服务端原生 Schema 校验提高传输可靠性，但不能替代 Hominal 对现实和权限的本地校验。

## 3. 最小器官协议

协议使用进程级 JSON 输入输出。G0 保留 CLI + Unix socket 的简单形态，不建设远程 RPC、插件市场、权限系统或复杂总线。

### 3.1 Manifest：发现器官

每个器官在本代身体的 `body/organs/` 下提供一份极小 Manifest：

```json
{
  "schema": "hominal.organ-manifest/v1",
  "id": "browser",
  "command": "body/bin/hominal-browser",
  "daemon": true
}
```

Manifest 只负责发现和启动。能力、使用说明与健康状态由器官本身提供，避免同一事实同时维护在配置、代码和提示词中。

G0 在生命进程启动时发现器官。Alice 可以编写新的器官程序与 Manifest，运行一致性测试后通过普通进程重启让它成为下一段连续身体事实；热加载留到确有第二个动态器官以后再决定。

### 3.2 `describe`：能力与身体说明

`describe` 是静态、无副作用操作，不依赖器官已经连接外部软件。最小返回：

```json
{
  "schema": "hominal.organ-description/v1",
  "id": "browser",
  "name": "Chrome browser",
  "command": "hominal-browser",
  "capabilities": ["observe", "orient", "perform", "cancel"],
  "operations": ["browser_snapshot", "browser_navigate", "browser_click"],
  "operation_inputs": {
    "browser_snapshot": "{}",
    "browser_navigate": "{\"url\":\"https://example.com\"}",
    "browser_click": "{\"target\":\"当前感知给出的唯一控件 target\"}"
  },
  "guidance": "供 Alice 阅读的简洁身体使用说明"
}
```

`capabilities` 是内核—器官协议能力，例如被动观察、定向、执行和取消；`operations` 是 Alice 可以在 `organ_action.operation` 中实际选择的行动目录。两者必须分开：内核可以调用 `observe`，不表示 Alice 应把 `observe` 猜成某个器官的行动名。`operation_inputs` 只给每项动作的最小输入形状，避免主脑猜测 Shell 源码与 JSON 等身体语法；它不规定行动对象和生活目的。具有 `perform` 能力的器官必须公布非空、无重复的 operations，输入提示的键也必须属于该目录；Organ Host 在动作开始前按目录校验。器官可以为 operations 提供按需 Schema，但不能在 guidance 中赋予目标、推荐主题、规定行为顺序或替 Alice 评价内容。

### 3.3 `health`：非变异健康事实

`health` 不进入器官动作队列，不改变外部状态，只返回：

```json
{
  "schema": "hominal.organ-health/v1",
  "id": "browser",
  "status": "ready | busy | recovering | unavailable",
  "accepting": true,
  "in_flight": 0,
  "queued": 0
}
```

`busy` 是正在工作，仍然属于可用身体；`recovering` 是器官网关仍接受未来动作、内部连接正在重建；`unavailable` 才表示当前入口不能接单。健康检查不能通过执行一次真实浏览、发送消息或读取大段内容来证明自身健康。

网络探测同样具有局部范围。System Observation 保留 `network_probe` 的目标、时间、可达性、HTTP 状态与原始错误，兼容布尔字段仅作本次探测结果。探针不是所有网站和模型接口的统一开关；主脑调用许可由真实网关请求、共享资源及有界退避决定，Browser 页面错误仍按自身事实报告。

### 3.4 `observe`：不改变现场的事实观察

`observe` 只读取当前感官现场，返回通用 Observation：

```json
{
  "schema": "hominal.organ-observation/v1",
  "organ_id": "browser",
  "surface_id": "chrome.current_page",
  "observed_at": "...",
  "context": ["Page URL: ...", "Page Title: ..."],
  "objects": [
    {"id": "稳定事实身份", "content": "Alice 可理解的事实内容"}
  ]
}
```

器官负责把协议噪声转换成稳定事实对象，例如 DOM 节点、窗口句柄或音频帧；内核先按 `organ_id + surface_id + object.id` 去重，再让该表面的信号家族进入统一预测回差场。对象 `content` 可以包含真实接触入口，但器官不能给出价值分数、Concern 建议、注意优先级或下一步行动。

预测回差场不属于某个器官。Browser、System、导师、内部资源和未来器官共享同一套预期变化、累计回差、注意点燃和 Experience 反馈；因此增加微信或摄像头器官不需要再实现一套专用“饱和—唤醒”认知逻辑。器官只需提供稳定事实身份和忠实变化，信号是否取得 Alice 的唯一焦点由 Cognitive Dynamics 决定。被长期评为低收益的器官来源会降低唤醒频率，但持续变化仍通过统一取样底噪缓慢积累，器官不能被一次学习永久切断。

第十六样本后的核心改造以同一表面前后内容的连续变化量调节回差；小幅播放器文字变化不自动等价于完全陌生的对象。此计算位于统一 Difference Field，器官仍完整报告其读取范围内的内容、错误与加载状态。程序不依据“有无主题”“是否有趣”隐藏现实，已发出动作的结果也不因文字相近而被省略。

### 3.5 `orient`：低优先级身体定向

`orient` 用于移动感官而不选择生活目标，例如移动一个网页视野、转动摄像头或切换传感器量程。它具有现实影响，因此与纯 `observe` 分开：

- 只在没有模型认知、主动动作和待吸收 Reality 时启动；
- 可以被 Alice 的主动动作立即取消；
- 保持当前语境，不自行选择另一个网站、联系人或主题；
- 返回实际移动、保持或失败的器官事实。

### 3.6 `perform` 与 `cancel`：主动行动

`perform` 执行 Alice 已经通过 AIP 认领并形成 Action Commitment（行动意愿）的具体动作。Organ Host 接收 `action_id + organ_id + operation + input + deadline` 的最小请求，其中 operation 必须来自该器官 Description 已公布的 operations。它保证主动动作优先、总截止时间、进程级取消和 Reality 回链；器官负责在 Commitment 已冻结的语义边界内完成 Action Enactment。

Commitment 的粒度由“最小可独立核验的因果改变”决定，不由点击次数决定。“把确定文字填入当前输入框”和“发布这段确定文字”可以是两个 Commitment；一个发布 Commitment 也可以允许器官完成定位、输入、点击和核验。凡是仍需决定写什么、发给谁、选择什么主题或怎样评价结果的内容，都必须回到 Cognitive Core，不能下放给器官补全。

请求使用统一信封：`hominal.organ-action/v1`；结果使用 `hominal.organ-action-result/v1`。动作的事实终态只有：

- `completed`：器官确认本次物理执行已经结束；
- `failed`：确认没有按请求完成，并返回实际原因；
- `unknown`：连接中断等情况使外部后果无法确认。

`completed` 只表示器官确认了请求的机械后置条件，不自动表示 Concern 闭合。命令退出码非零、操作超时或核验条件不成立必须是 `failed`，不能外层先记为 completed 再把失败藏在文本中。`unknown` 不允许自动重放可能产生外部后果的动作。Reality 仍由 Cognitive Core 关联原 Commitment，再交给 Alice 解释。

长于一个瞬时操作的器官可以上报稀疏过程信号，例如连接已经恢复、目标控件已经找到、外部系统正在等待或执行被阻塞。器官只判断这些信号是否是可核验的因果状态变化；它不能判断这些变化对 Alice 是否“有意义”。Organ Host 合并重复机械噪声，Cognitive Dynamics 再决定某项变化是否进入注意。

异步执行与完成判定是两件事。内核把器官动作交给独立工作线程；动作关联的 Concern 暂时退到背景，同一时刻仍只允许一个身体动作，但主意识可以处理导师来信、其他现实和另一项不需要身体动作的焦点。`acting` 与“Reality 已到、等待吸收”是不同状态，后者仍优先进入注意。这样 Pulse、资源计量、内感知和思想连续不会被具体工具冻结，器官则必须等到自己的机械后置条件可以核验才返回终态。短文件读取以字节已经返回为完成，写入以写入调用、关闭以及行动意愿要求的持久化或回读完成为边界，前台命令以进程退出和退出码为边界。启动后台服务时，Shell 父进程退出并不充分，行动意愿或器官还需核验约定的进程或健康事实。

浏览器的机械完成、页面加载状态和当前读取内容分别报告。导航记录文档抵达、实际 URL、HTTP 状态及错误；HTTP 404 的响应与页面正文如实保留，不包装成找到目标内容。成功导航并完成当前视口读取，可以同时报告 `visible_loading=true`：这是页面还有可见异步加载的事实，不是读取只完成了一半，也不承诺页面以后不再变化。`readiness` 的 ready/loading 只依据文档生命周期和可见加载标志，不由字数、是否出现帖子或主题质量决定。真实内容只有“1”就完整返回“1”，后来的变化经同一观察入口继续送达；不因内容少而刷新、重试或等待想象中的正文。未完成请求的机械后置条件仍在原总 deadline 内等待，截止不明则 unknown；明确读取失败不回退成成功的局部摘要。可能已产生副作用的失败动作不自动重放。

## 4. 统一调度不变量

以下规则由内核与 Organ Host 共同强制，器官实现不能自行改写：

1. **一个主动身体前景。** 同一时刻只有一个由统一意识新认领的主动 Action Commitment；器官控制器可以异步工作，但不取得新的意义判断和行动意愿权。
2. **主动优先。** `perform` 可以抢占 `orient`；被动身体活动不能长期扣住 Alice 的手。
3. **健康旁路。** `health` 不排在状态动作之后；忙碌不会制造器官失效事件。
4. **一个总截止时间。** 排队、连接恢复与执行共享同一 deadline，不能逐层重新获得完整超时。
5. **取消向下传播。** 调用者退出或截止时间到达时，器官工作及其子进程共同结束。
6. **外部后果不盲目重放。** 只重建连接，不自动重复未确认的发布、发送、关注、删除或硬件动作。
7. **Reality 忠实。** 器官返回事实、时间、状态和有限结果；意义和价值属于 Alice。
8. **感知有指称。** 没有对象是感官控制事实，不是付费思想、Concern 或行动对象。
9. **器官故障不切断生命。** 单次器官失败形成身体事实；Pulse、其他器官、导师关系与内在张力继续运行。
10. **协议可由 Alice 实现。** 新器官不要求修改认知动力学代码，只需满足 Manifest、协议与一致性测试。
11. **能力与动作分离。** capabilities 只描述通用身体协议；operations 才是可执行目录，主脑、内核和器官使用同一份目录，不从说明文字猜动作名。

## 5. System Organ v1

Ubuntu 具有双重地位：硬件、Linux 内核、文件系统、进程和网络是 Body Substrate；Alice 感知和使用这套底座的接口是一个遵循通用合同的特殊 `system` 器官。它不能由 Organ Host 像普通软件一样创建 Ubuntu 本身，但它的适配进程、健康、观察、动作和取消仍按普通器官管理。

G0 只提供一个收敛的 System Organ，不预先拆分文件、进程、网络、软件安装等多个器官。首版能力为：

- `observe`：uptime、根卷与 agent 卷可用空间、模型网络入口、桌面、微信和 Clash Verge 的当前身体事实；
- `perform`：执行命令和代码、读写文件、启动与终止进程、安装软件；
- `cancel`：截止或调用者退出时终止完整进程组，避免残留身体动作；
- 忠实终态：退出码、超时、标准输出、标准错误和可核验文件或进程结果共同决定 completed、failed 或 unknown。

模型 Token、Concern、Affective State、Attention 和 Narrative Self 是认知核心的内部状态，不由 System Organ 解释。System Organ 只报告操作系统和硬件事实；认知资源账本通过内感知进入同一个背景场。

当前实现中，认知动作统一为 `organ_action`。System Organ 的 `exec` 接收 bash 源码并拥有命令、文件、进程、软件与网络操作能力；运行时不再直接执行 Shell，也不再用 `/proc`、`systemctl`、`pgrep` 或 HTTP 探针取得系统身体事实。System 与 Browser 的主动动作共同经过 Organ Host 的 `perform`。

## 6. Browser Organ v1

浏览器是第一个标准器官。它独立拥有以下知识：

- Chrome CDP 与 Playwright MCP 的连接、恢复和持久会话；
- 标签、页面、控件引用、对话框和草稿等短时身体状态；
- 浏览器健康、动作队列、deadline 与取消；
- 当前视口完整文字读取、原样事实、稳定对象身份和 Direct URL；
- X authored object 与普通网页正文对象的事实提取；
- X 个人资料、具体 authored object 及其当前可见关注、回复、喜欢等操作入口的事实提取；
- 当前页面的有界视野定向，以及可编辑控件的现场保护；
- Playwright 动作名与 JSON 参数、`list / schema` 发现接口，以及必要时的原子代码执行。

Cognitive Core 不再知道 CDP 端口、Playwright 程序、DOM role、X article、标签索引、网页滚动或浏览器命令语法。它只看到：

- `browser` 器官的 Description 与 Health；
- `chrome.current_page` 表面；
- 一组有稳定事实身份的 Observation objects；
- Alice 主动动作返回的 Reality。

最近实验形成的浏览器强化同时进入 v1：

- 网关先开放，MCP 按需恢复，避免器官初始化拖垮生命进程；
- `health` 队列外返回，区分 ready、busy 与 recovering；
- 被动 observe/orient 为主动调用让路；
- 客户端消失与总时限会终止旧 MCP，下一动作建立干净连接；
- orient 不切标签、不导航、不刷新，编辑器获得焦点时保持现场；
- 快照先形成稳定对象与实际接触入口，再进入内核的注意压缩；
- 脚本 `return` 与 `console.log` 的紧凑值作为 Reality 返回；
- 连接恢复不重放任何可能已经产生外部后果的动作。
- 导航、点击和输入后的完成由页面生命周期与语义表面收敛确认；固定休眠和固定重试次数不再伪装成完成条件。

## 7. 状态、日志与失败边界

内核持久化每个器官的最近 Description 摘要与 Health，不持久化器官私有窗口树或连接对象。器官私有状态由器官自身或外部应用保存。

Journal 只记录以下稀疏变化：

- 器官首次发现；
- `ready / busy / recovering / unavailable` 的实质边界变化；
- Observation 中真正的新对象进入统一事件口；
- orient 的实际结果；
- 主动 Action 的开始、完成、失败或未知。

健康轮询、相同页面快照和内部重连细节不形成思想日志。完整原始工具输出继续作为动作 Reality 的有界事实保存，不由器官提前解释成成功、价值或经验。

## 8. 自主编写器官

Alice 可以使用已有 root、文件、代码与软件安装能力编写器官。最小路径是：

```text
形成具体需要
→ 编写 adapter + manifest
→ 运行 organ conformance test
→ 注册并重启身体进程
→ observe/perform 取得真实 Reality
→ Alice 判断它是否值得保留、修改或放弃
```

一致性测试只验证协议事实：JSON 可解析、describe 静态可用、health 非变异且有界、observe 不改变现场、orient 可取消、perform 有 deadline、失败返回明确、子进程不残留。它不评价器官用途，也不要求 Alice 向实验者申请功能目标。

自制器官是身体能力的成长，不自动成为 Narrative Self 或生命进化成功。只有它后来真实改变了 Alice 的行动空间、Experience、方法或长期自我理解，才进入生命成长证据。

## 9. 动作能力的分层验证

器官存在、器官会执行、Alice 会选择使用器官，是三件不同的事。G0 按以下顺序验证，避免用一小时自主实验同时猜测所有故障来源：

1. **器官一致性测试**：不调用认知模型，直接验证 describe、health、observe、perform、cancel、截止、恢复和真实后置条件。
2. **动作实施测试**：在 Action Commitment 之后注入一个已经确定的动作，让真实 Action Gateway、Organ Host、器官和 Reality 回链完成执行；它只检验身体控制，不检验 Alice 是否愿意做。
3. **感知—行动闭环测试**：动作前后都由器官重新观察，只有现实后置状态符合 Commitment 才通过，不接受器官自述、模型自述或单纯退出码代替事实。
4. **自主联合测试**：前三层通过后，让环境事实经 AIP 和 Self Ownership 自然形成意愿，由 Alice 自主产生 Commitment，再检验完整 Life Dynamics。

动作测试使用可丢弃的 Lab 实例。实例内部仍按正式链路记录 Commitment、过程信号与 Reality，测试结束后整体归档并不继承到正式 Alice；不能增加一个关闭 Reality Ledger 的“无痕模式”，否则测试的身体和正式身体已经不是同一种因果系统。

纯动作能力与自主意愿需要分别取证。为了进一步检验“认知动力学会使用器官”，Lab 可以给可丢弃实例一个真实、低歧义、可核验的测试对象，让普通 AIP—Commitment 流程自行承接；但不能在生产内核中硬编码“产生测试愿望”，也不能把实验者注入的动作写成 Alice 自主成长证据。

## 10. 当前实现范围

当前重构完成以下边界：

1. 建立 Manifest、Description、Health、Observation、Orient 的通用协议类型与器官注册表；
2. 由 Organ Host 启动和停止 Browser Organ，启动脚本不再直接管理 Playwright 会话；
3. Body Snapshot 使用通用器官状态，不再保存 Chrome/Playwright 专用布尔字段；
4. 把 CDP、网页语义提取、X article、Direct URL 和视野定向全部迁入 Browser Organ；
5. 内核以通用 Observation objects 继续执行单对象注意、去重、饱和与习惯化；
6. 模型上下文从器官 Description 获取身体使用说明，不在认知提示中硬编码 Playwright 操作教程；
7. 删除认知运行时中的 `body_shell` 动作种类，以 `organ_action` 统一选择器官、操作和输入；
8. 增加 System Organ，由它取得 Ubuntu 身体事实，并以 `exec` 承接原 root Shell 能力；
9. Browser 与 System 主动动作共同使用 `perform`，终态明确区分 `completed / failed / unknown`；
10. 构建与部署包同时冻结两个器官的二进制、适配器和 Manifest。
11. 候选器官事实、身体变化、内部信号和 Reality 统一经过可学习预测回差场；Browser 不再拥有一套独立的认知饱和算法。
12. 器官动作在后台实施；主意识在保持单一身体动作的同时可处理不争用该器官的其他事实，直到动作 Reality 返回。
13. 器官 Description 同时发布动作目录与最小输入形状；System `exec` 明确接收 Bash 源码，Browser 输入由自身合同说明。
14. ActionResult 只结算器官动作本身；动作新暴露的具体对象继续停留在统一感觉队列，直到对象真正进入注意或被后续表面变化取代。
15. 达到 `attention.maximum_idle_seconds` 时，身体可以进行一次有界感觉换向；它维持具身连续，不凭空制造意义，也不要求主模型为静止页面反复思考。

实现仍保持一个认知主体、一个主动身体前景和一个 Reality 回链，没有增加任务总线、器官社会、复杂权限层或新的认知工作流。尚未取得的是两个器官在独立实验舱中的完整能力证据；这属于单独器官实验，不由代码存在或单元测试替代。

## 11. 验收标准

完整内核—器官架构的实验验收门需要同时满足：

- Go Cognitive Core 中不再出现 CDP、Playwright、DOM、X article、浏览器滚动或标签控制代码；
- 新一代可以只靠 Manifest 发现并启动 Browser Organ；
- Body Snapshot 和模型上下文以通用器官状态呈现浏览器能力；
- Browser Organ 的 observe 返回稳定 context 与 objects，内核一次只开放一个新对象；
- 忙时 health 快速返回，后台 orient 能被主动动作抢占；
- 超时后没有残留 MCP 或 Shell 子进程，下一动作能够恢复；
- 实机 X 与 Wikipedia 仍可读取，活动页面与编辑现场不被后台感知破坏；
- 移除 Browser Organ Manifest 后内核仍可持续运行，浏览器只表现为一个缺失器官；
- System Organ 接管命令、代码、文件和进程动作以及操作系统身体事实，runtime 不再直接理解 Shell 执行细节；
- Browser 与 System 主动动作都通过同一个通用 `perform` 入口，明确区分 completed、failed 与 unknown；
- 两个器官分别通过一致性、动作实施和感知—行动后置条件测试，再进入自主联合实验；
- 完整 Go、Node、Lab、契约测试和 Linux/amd64 发布构建通过。

## 12. 结论

内核—器官分离不是把 Hominal 变成多个生命主体，而是让同一个生命拥有可生长、可以采用代理式控制的身体。统一性来自 Attention、Commitment、Reality 与调度不变量；开放性来自器官可以用不同技术独立实现，甚至由 Alice 自己创造。

这一结构保留极少但刚性的现实边界，同时把网页、微信、语音或硬件的技术复杂性挡在认知动力学之外，并通过统一感知与 Reality 回链让它们继续属于完整生命动力学。以后增加器官时，首先复用这份协议；只有多个真实器官共同证明现有边界不足，才扩大认知核心。

## 13. 实现与验证基线

2026-09-01 已完成第二次收敛实现：`body/internal/organ` 提供通用 Host、感知协议和 `perform`；`body/organs/system.json` 与 `browser.json` 注册两个器官；`hominal-system` 独立拥有 Ubuntu 身体事实与 root 命令执行；`hominal-browser` 独立拥有 Playwright、浏览器语义与持久会话。`hominald` 只接收通用器官事实和动作结果。无效 Manifest 与启动失败器官以 `unavailable` 呈现，不阻断生命内核。

此前 Browser v1 的 Ubuntu 非变异烟测已证明持久会话、健康旁路、主动抢占和恢复路径可用。第二次收敛的代码级检查只负责排除协议、构建和确定性进程取消错误；System 与 Browser 的独立器官能力实验尚未执行，也没有启动新实验代。

当前唯一尚未完成的明确边界是独立器官能力实验：在不创建 Hominal 生命样本、不写入 Alice 个人连续性的隔离环境中，分别核验 System 与 Browser 的感知、动作、终态、截止、恢复和后置事实，然后整体重置。实施计划见 [System / Browser Organ 独立能力实验计划](../plans/system-browser-organ-capability-experiment.md)。
