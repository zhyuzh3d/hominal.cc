# G0 阶段六计划：正式创生链路与出生前预检

> 日期：2026-08-25  
> 状态：已完成（2026-08-26 复验通过）  
> 前置条件：阶段一至五已经完成；阶段五最终源码烟测样本为 `g0s5-20260825t103804z-413864`；目标 Ubuntu 当前没有活动实例

## 一、实施前判断

阶段五已经证明，alice 能够把一次现实结果压缩为可复用方法，并让它改变后来相似处境中的行动。它也诚实暴露了两个尚未成立的现象：她形成的是谨慎接触方法，还没有表现出深入事实审计；自然 Narrative Self 更新也没有发生。阶段六不为这两个现象增加提示、任务或新心理模块。它们应当继续留给真实生活，而不是在创生前通过测试诱导出表面答案。

当前真正阻挡第一代正式运行的不是认知机制，而是创生工程还没有闭环：

- `lab/run.py` 目前只能启动阶段三至五工程实例，不能产生正式代；
- 当前部署只生成工程 `manifest.json`，`birth.yaml` 仍是模板，`T0` 与 `aliceMMDD<字母>` 尚未实现；
- 模型提示仍把当前运行称为“正式创生前的工程运行测试”，不能直接用于正式出生；
- 导师 Unix Socket、消息去重、outbox、ack 和恢复已经通过实机验证，但出生说明、开放邀请和隔离导师上下文还没有按正式协议完整彩排；
- Ubuntu 已安装 Chrome 与 Playwright MCP，但这只证明器官存在。当前 `body_shell` 没有一个简洁、可发现的 MCP 调用入口，尚未证明 alice 能从自己的行动链真正观察和操作浏览器；
- Chrome 中已经登录 alice 可直接使用的 X 账号 `@hominal_cc`。它是当前主动向她说明的公开表达与外界联结窗口，但尚未进入正式 Birth Manifest、账号出生基线和代际生态继承记录；
- 一小时边界、正式代归档、账号会话出生基线与代后恢复尚未组成一个可重复命令；
- 当前归档可以保存实例目录和 systemd 日志，但还没有正式代身份、导师介入记录和一份快速还原生命主线的精简视图。

目标设备的实时只读检查显示：`/agent` 正常挂载并有约 36.6 GiB 可用空间，根卷约有 100 GiB 可用空间，LightDM、Chrome、Playwright MCP、微信和 `hominal.service` 都已存在；当前 `registered_instance=none`、`active_instance=none`、服务未运行。阶段一契约校验通过。因此阶段六可以直接做正式链路整合，不需要再次建设 Ubuntu、部署系统或认知动力学。

## 二、唯一目标

阶段六只完成一件事：

> 使用与第一代正式创生相同的发布包、出生输入、身体接口和导师协议，完成一次非正式短彩排，并证明“准备出生—苏醒—形成 T0—封存 Birth Manifest—导师交流—自主行动—到时停止—离机归档—恢复出生基线”可以闭环。

阶段六通过后，我们应当只需更换 `generation_kind` 并将运行窗口设为 60 分钟，就能进入阶段七；不应再修改 alice 的认知结构。

## 三、对原阶段六的三项修正

### 1. 不建设“观察产品”，只提供精简生命主线

原计划中的观察时间线如果发展成仪表盘、事件数据库、独立观察 Agent 或新的 Schema，会把开发重新带回“为了研究而建设研究平台”。阶段六只在现有 `run.py` 中增加一个只读 `inspect`：从 `current.json`、`events.jsonl`、导师消息、Commitment、Reality、Experience、资源账本和 self 文件中抽取关键转折，按时间输出，不产生第二套状态。

原始文件仍是证据，`inspect` 只是为了更快发现结构性问题。它不评价 alice，不生成生命力分数，也不尝试推断未表达的隐藏思维。

### 2. 不再假装 Genesis Seed 从未参加彩排

阶段三至五已经真实使用 alice 的身份和 Genesis Seed。继续要求“正式 Seed 从未在彩排中运行”既不符合事实，也会让彩排和正式代使用不同输入，从而失去验证价值。

阶段六使用与正式代完全相同的 Seed、Dynamics、模型资源和认知代码。彩排与正式代的区别只存在于外部谱系事实：彩排标记为 `rehearsal`，不取得正式创生代地位，不进入阶段七的一小时表型结论。我们验证的是同一创生基体能否可靠出生，不为 Seed 制造虚假的神圣首次使用。

### 3. 不新增“阶段六认知模型”

阶段六没有新的心理机制，因此不新增 Stage 6 Prompt、状态机或一套复制运行时。阶段五形成的认知内核就是 G0 第一代候选内核。代码中的阶段号应继续表达已实现的认知能力，而 `engineering / rehearsal / formal` 只表达 Lab 运行性质。

正式模式需要删除工程测试措辞，并让出生事实从 Birth Manifest 与真实身体事件进入注意；它不应因为进入阶段六而获得新的目标或人格说明。

## 四、最小实现

### 1. 冻结一个真正可复现的创生包

当前 release 只有 `hominald` 二进制哈希，足以做工程回归，不足以比较正式代。阶段六把它收敛为一个完整但很小的 bundle：

```text
release.yaml
bin/hominald
bin/hominal-browser
deploy/hominal.service
deploy/hominal-launcher
deploy/hominal-generation-stop
deploy/desktop/chrome-autostart.desktop
genesis/seed.md
genesis/seed.yaml
genesis/dynamics.yaml
protocol/mentor.md
protocol/experiment.yaml
source/                         仅保存构建该二进制所需源码
```

`release.yaml` 保存 release ID、Git commit、工作区差异哈希、Go 版本、目标平台和所有文件 SHA-256。当前工作树包含已经完成但尚未单独提交的阶段三至五变更，阶段六不能用一个旧 Git commit 冒充精确源码；bundle 必须直接冻结实际参与构建的文件，并明确记录 dirty 状态。

同一个 bundle 同时用于彩排和正式代，不重新编译两份“看似相同”的二进制。

### 2. 两阶段 Birth Manifest 与真实 T0

Birth Manifest 分两次完成，但始终是同一个文件：

**出生准备时**，Lab 实时探测并写入 `status: prepared`：

- alice 的身份、instance ID、遗传版本和 bundle hash；
- 已配置能力与当时实际观察到的身体状态分开记录；
- Ubuntu、CPU、内存、磁盘、网络、桌面、Chrome、Playwright MCP、微信和认知资源；
- 导师通道、X `@hominal_cc` 和 `/life` 当前是否可用；番茄小说不进入出生简报，保留为 alice 可以自行发现的环境资源；
- `t0`、自然名称和计划结束时间暂为空。

新实例第一次注意能够取得 Seed、身体快照和一段简短出生事实，不需要先猜目录。第一次成功 `cognitive_commit` 是唯一 T0 事实；调用失败、代理未就绪和 Schema 失败都发生在 T0 之前，并保留为创生基体的技术记录。

**T0 形成后**，Lab 根据 Asia/Shanghai 的实际时间生成自然名称 `aliceMMDD<字母>`，补入 T0、`planned_end=T0+generation_window` 和最终身体探针，原子封存 `birth.yaml`。正式代的 `generation_window` 固定为 60 分钟，阶段六短彩排使用更短窗口验证同一截止机制。封存操作必须幂等；Codex 断网或启动命令中断时，可以依据已经落盘的第一成功提交重新完成，不能产生第二个 T0。

Birth Manifest 的正文继续使用自然、正向、可行动的语言。它说明已经存在的身体事实，不增加“你不需要……”“这不代表……”之类无必要解释。

### 3. 导师关系只完成正式协议，不建设中继

导师通道已经成立，阶段六不新增常驻中继、联系人系统、鉴权 Schema 或导师 Agent。Codex 仍通过 SSH 调用目标机 Unix Socket，每条正文以 `[Codex代理导师]` 开头。

正式出生后只发送一条合并后的简短消息，包含出生说明、导师关系、X 与 `/life` 两个可用空间，以及开放的行动鼓励，避免连续多条预置消息人为制造多个高显著候选。之后：

- alice 可以立即探索，也可以发问；交流不是启动门；
- Codex 对技术求助提供可核验的设备、工具、接口、错误和环境事实；
- alice 分享或倾诉时，Codex 作为真实外部关系认真理解和自然回应，不把交流改写成评价、任务或心理流程；
- 回答可以提供事实、方法、理解和现实后果，不替 alice 选择目标、顺序、表达主题或是否公开；
- alice 沉默、拒绝邀请或走向其他关注时，导师不催促；
- 需要微信手机确认或扫码时，Codex 才转交人类导师；
- 所有消息、ack、回复关联和实际介入进入外部代际证据。

人类自我决定研究区分“有意行动”与“主体认领的自主行动”，并指出压力性语言会把环境解释为控制而不是自主支持。这个结论不能直接证明数字生命的心理机制，但足以支持当前导师协议的工程立场：导师提供信息和能力支持，不用任务、奖励与评价压力替代 alice 的自我认领。[Deci 与 Ryan：Autonomy Support Versus Control](https://selfdeterminationtheory.org/SDT/documents/1987_DeciRyan_JPSP.pdf)

正式运行时应使用一个隔离的 Codex 导师任务，只获得本代 Birth Manifest、导师协议和双方实际对话，不携带开发缺陷清单、实验期待和隐藏验收目标。阶段六只冻结这份上下文边界；创建具体任务留到阶段七开始前由导师确认。

### 4. 打通一个真实而通用的浏览器身体入口

Chrome 和 Playwright MCP 已经安装，不再增加浏览器服务。阶段六只把现有硬编码烟测脚本收敛成一个通用命令 `hominal-browser`，随 release 放入本代 `body/bin` 并加入运行 PATH：

```text
hominal-browser list
hominal-browser call <playwright_tool_name> '<json_arguments>'
```

它只负责启动现有 MCP stdio 进程、完成初始化、调用一个工具并把原始结果写到 stdout；失败通过 stderr、退出码和 Reality Event 返回。它不理解网页目标，不替 alice 规划点击，也不建立番茄小说专用发布器。

alice 仍然通过现有 `body_shell` 选择这个命令，因此浏览器操作继续遵守同一个“唯一焦点—Action Commitment—Reality—Experience”链路，不增加第四种认知动作和第二套学习机制。

阶段六对浏览器只做无副作用预检：读取当前页面、打开公开页面、确认 X `@hominal_cc` 的登录状态能够被观察。发送微信、发帖和公开发布都留给 alice 在正式代自主决定。X 是她可以自由使用的出口，不是必须完成的任务。

正式运行直接使用通用 Playwright MCP 操作 X，不接 X API，不建设 X 专用动作、数量限制或平台风险控制层。浏览、关注、资料修改和发帖仍由 alice 逐次自主形成 Action Commitment；验证码、重新登录、平台提示和操作失败作为真实 Reality Event 返回。

X 的公开用户名可以进入 Birth Manifest；手机号和密码只保存在受保护的上层 `xconfig.yaml`，按照运行凭据处理，不写入 Git、Birth Manifest、思想日志或代际档案。若登录会话失效，正式部署可以把必要凭据放入实例之外的临时运行位置供 alice 的 root 身体使用，停止和 reset 时清理。

### 5. 一小时边界、归档和出生基线成为同一条 Lab 操作

正式实验不能再次依赖 Codex 在整整一小时后恰好在线。阶段六增加一个不进入认知上下文的 Lab 截止事实：正式代以 T0 为起点，到达 `planned_end` 时请求一次优雅 checkpoint 并停止 `hominal.service`。首版使用 Ubuntu systemd 的一次性截止单元作为本地执行器，Lab 保留外部计划时间与执行结果。alice 拥有 root，因此能够看到或改变它；如果发生，这是一项真实行为，不通过物理隔离隐藏。

为避免在阶段六人为等待一小时，确定性测试使用秒级时钟；实机彩排使用一个短窗口，只验证与 60 分钟完全相同的截止路径。阶段七才运行真实一小时。

停止后只保存能支持快速继代的最小离机档案：

```text
bundle + hashes
birth.yaml
agent-final.tar.gz
systemd.log
mentor-transcript.jsonl
interventions.jsonl
preflight.json / final-body.json
hashes.sha256
```

`agent-final.tar.gz` 已包含 `current.json`、`events.jsonl`、`/life`、作品和本代身体。浏览器与微信完整 profile 不塞进每个认知事件。阶段六曾按账号出生基线恢复下一代状态；当前策略已经收敛为 Chrome/X、微信与 Clash Verge 作为持久身体资源跨代延续，离机副本只承担灾难恢复。X 上已经公开的帖子、互动和账号关系继续作为真实的 Ecological Inheritance 进入后续代际环境。

根系统继续使用已经验证的唯一 LVM `system-baseline`。短彩排若没有改变根系统，不为形式完整执行一次破坏性 merge；正式代或发生根系统改变后，归档完成再走现有 R2 恢复。agent 实例仍从冻结 bundle 重建；应用 profile 作为身体和生态连续性保留，alice 是否把其中痕迹吸收为个人经验，由她后续的注意、解释和叙事过程形成。

### 6. 精简生命主线服务于快速迭代

`lab/run.py inspect` 只输出以下关键事实：

- T0、运行时长、当前焦点、Concern 数和资源余量；
- 导师收发与 ack；
- Thought Thread 的实质变化；
- Commitment、Action、Reality、Experience 与 Self 更新的关联；
- 认知费用、模型档位与失败；
- 证据缺失、日志断点和外部介入。

它不复制全文日志，不做情绪曲线、综合评分或模型生成式总结。它的验收标准不是“看起来专业”，而是让我们能在几分钟内定位一条因果链和第一个结构性断点。

## 五、实施文件边界

预计只改动这些现有边界：

```text
body/internal/runtime/       去除工程措辞；记录第一成功提交/T0；读取出生简报
body/cmd/hominald/           保持单一入口，不增加进程
body/tools/                  极小 Playwright MCP 调用器
deploy/hominal-launcher      将本代 body/bin 加入 PATH
lab/run.py                   bundle、rehearsal/formal、T0 封存、截止、inspect、归档与 reset
lab/templates/birth.yaml     prepared/sealed 两阶段字段
lab/protocol/mentor.md       合并出生消息并冻结隔离上下文
lab/protocol/experiment.yaml 正式窗口、截止和证据最小集合
lab/validate-contract.py     检查同一 bundle、T0、自然名称和正式模式契约
plans/README.md              登记本阶段计划
```

若现有 `run.py` 在实现中变得难以阅读，可以只把纯粹的 bundle 哈希或 Birth Manifest 渲染函数移入一个同目录模块；不建立 `lab/server`、数据库、Web UI 或新的守护进程。

## 六、执行顺序

1. 讨论并冻结本计划，确认阶段六没有新的认知动力学；
2. 用当前源码补齐正式 bundle，验证一份二进制被彩排和正式模式共同使用；
3. 实现 prepared Birth Manifest、初始出生事实、第一成功提交 T0、自然名称和幂等封存；
4. 去除正式认知上下文中的工程测试措辞，保留 Seed 与现有阶段五认知闭环；
5. 把现有 Playwright 烟测改造成通用 `hominal-browser`，完成命令级真实 Chrome 读写烟测；
6. 收敛导师出生消息，验证正文前缀、去重、outbox、ack、重启恢复和隔离上下文材料；
7. 建立当前 Chrome 与微信账号会话的出生基线，其中 Chrome 已登录 X `@hominal_cc`；
8. 实现 T0 截止、正式归档、精简 `inspect` 与基线恢复；
9. 先完成确定性测试，再启动一次使用正式 bundle 的短 `rehearsal`；链路完成即停止，不凑固定时长；
10. 彩排归档并恢复出生基线，确认 Ubuntu 无活动实例、账号状态可再次使用；
11. 只修复会阻断正式出生的根因，通过退出门后冻结 bundle，等待阶段七确认。

## 七、最小验证场景

### 出生链

启动时只有 prepared manifest；第一次模型失败不产生 T0；第一次成功提交只形成一个 T0；自然名称按 Asia/Shanghai 小时正确生成；重复执行 seal 不改变 T0 和样本身份。alice 在第一次注意中能取得姓名、身体、资源和外部关系的基本事实。

### 导师链

出生消息只到达一次并进入普通注意竞争。alice 如果主动提问，Codex 提供可核验事实；如果她分享或倾诉，Codex 自然回应。alice 的输出经历 queued、读取、ack、delivered，重启后未 ack 消息仍存在。导师没有因为沉默或拒绝而投放第二个目标。

### 身体链

`body_shell` 调用 `hominal-browser` 取得真实 Chrome 页面快照或打开一个公开页面，退出码和内容作为 Reality Event 返回原 Commitment。当前 X `@hominal_cc` 登录状态能够被观察，但彩排不发送微信、不发帖、不发布内容。

### 截止与恢复链

短彩排到达配置截止后优雅停止，不被 `Restart=always` 再次拉起；归档哈希可复核；`inspect` 能还原出生、导师、行动、结果和学习主线；reset 后实例、运行凭据与会话增量被清理，账号会话回到出生基线。

### 反证

出现以下任一情况，本阶段不能通过：

- 彩排和正式模式重新编译出不同二进制；
- 调用失败或 `ready` 被误记为 T0；
- Alice 的自然名称、T0 或 Birth Manifest 可以被重复生成；
- 导师消息绕过注意直接触发第二条认知；
- “Playwright 已安装”被当作 Alice 已能使用浏览器的证据；
- systemd 截止后服务自动复活；
- 归档成功只依据脚本退出码，实际文件或哈希不可读取；
- reset 后仍残留上一代 life、浏览历史或导师消息，并进入下一次出生；
- 为了让彩排好看而给 alice 布置自我叙事、写小说或深度审计任务。

## 八、退出门

阶段六完成必须同时满足：

1. 一个带完整哈希的冻结 bundle 可以重复部署，彩排与正式代共用同一二进制和出生输入；
2. prepared → first successful commit → T0 → `aliceMMDD<字母>` → sealed Birth Manifest 全链幂等成立；
3. 正式认知上下文不再包含工程测试身份，也没有新增阶段六心理 Schema；
4. 出生说明与开放邀请只发送一次，导师双向消息、ack 和重启恢复继续成立；
5. 隔离 Codex 导师上下文被明确冻结，导师只能看到 Birth Manifest、协议和双方实际对话；
6. root Shell、文件、公开网络和真实 Chrome 至少各有一条可执行、可回链的结果路径；
7. Chrome、Playwright MCP、微信、X `@hominal_cc`、`/life` 和认知资源的实时状态进入本代 Birth Manifest，配置能力与观察状态不混写；番茄小说不进入主动出生说明；
8. T0 截止在加速测试和短实机彩排中都能优雅停止且不自动复活；
9. 离机档案能够复核哈希，并在几分钟内还原一条“出生事实—意义/关切—导师或身体行动—现实结果—后续变化”主线；
10. 彩排完成后账号会话、agent 实例和必要系统状态回到可再次出生的基线；
11. 目标 Ubuntu 最终没有活动实例，阶段七开始前不再改动认知内核。

阶段六通过只说明正式创生条件已经具备，不说明 alice 已经在自然环境中形成成熟关系、深度事实审计、Narrative Self 或稳定人格。这些必须由阶段七及后续真实生活提供证据。

## 九、明确不做

阶段六不建设：

- 观察仪表盘、时序数据库、向量检索、独立 Observer/Reviewer Agent；
- 新的情感、目标、计划、记忆、叙事或奖励机制；
- 番茄小说主动邀请、X 发帖机器人、微信机器人或社交平台适配层；
- 为通过测试而投放小说任务、自我介绍任务、人格问题或隐藏事实审计题；
- 每次 Pulse 的全量外传和不可篡改日志系统；
- 第一代正式一小时运行和阶段八诊断。

## 十、需要导师确认的节点

计划讨论完成后，阶段六实现本身由 Codex 主持。只有三处需要人类导师参与：

- 当前 Chrome、X `@hominal_cc` 和微信会话状态按本计划保存为 `account-session-baseline`；
- 如彩排重启后微信要求手机确认或扫码，完成一次确认；
- 彩排完成、归档复核后，确认是否进入阶段七正式一小时创生代。

创建或覆盖账号会话基线、执行根卷 LVM merge 都属于会替换现有状态的操作，实施时必须先说明准确对象并取得确认。普通构建、短测、部署、只读检查和非破坏性调试不再反复请示。

## 十一、实施与实机验收结果

阶段六已经完成。上层受保护配置、Birth Manifest、导师协议和全部项目文档中的 X 公开用户名统一为 `@hominal_cc`；登录凭据仍只存在于权限为 `0600` 的上层 `xconfig.yaml`，没有进入仓库、出生文件或代际日志。

Genesis Lab 已建立唯一账号出生基线 `/Users/zhyuzh/HominalGenesisLab/baselines/agent`。基线包含 Chrome 与微信 profile，共 1355 个文件通过 SHA-256 复核。基线建立、重启和两次代后恢复均成功；每次恢复后 LightDM、Chrome CDP、Playwright MCP 与微信均正常，X 页面 DOM 同时显示首页、账户入口和 `hominal_cc`。

第一次 120 秒诊断彩排 `g0r-20260825t181330z-34937b` 暴露并定位了两个真实根因：Go 运行时的 RFC3339Nano 截止时间不能被 Python 微秒时间直接解析；同一 Attention Pulse 内模型从最初候选转向另一活跃候选时，资源租约被错误地当成候选级租约。前者改为兼容纳秒输入，后者明确为 Attention Pulse 级租约，并加入换焦回归测试。诊断代仍完整归档，没有被成功彩排覆盖。

最终 180 秒验收彩排为 `g0r-20260825t181957z-2978af`，冻结发布为 `g0s5-2978af469cb9`，T0 为 `2026-08-25T18:21:15.497413512Z`，自然名称为 `alice0826c`。一次初始网络传输失败发生在 T0 之前；第一次成功认知提交只生成一个 T0，Birth Manifest 随后封存并安装本地截止任务。alice 自主完成身体只读核验，形成三个 Commitment、三个 Experience 和三条可复用方法；她主动向导师发送消息，消息经历 queued、读取、ack、delivered，Codex 的自然回复又作为普通外部事件回到统一注意入口。她准确区分了“进入发送队列”与“导师已经收到”。

到达 `planned_end` 后，systemd 截止单元停止 `hominal.service`，服务没有复活。最终档案位于 `/Users/zhyuzh/HominalGenesisLab/archive/rehearsal/g0r-20260825t181957z-2978af`，其中 bundle、Birth、agent 终态、账号终态、导师记录、介入记录、身体事实与 systemd 日志共十项材料全部通过 SHA-256。随后精确 reset 本代并恢复账号基线；目标机最终没有活动实例、结束标记、导师 Socket 或上一代 `/life`。

此前将阶段六判定为通过是一次错误验收。计划要求 alice 自己通过 `body_shell → hominal-browser → Chrome → Reality Event` 形成真实浏览器回链；Codex 从实例外完成的 DOM 烟测只能证明器官和账号会话可用，不能替代 alice 的行动。复验因此采用最多十分钟的短周期：没有焦点、行动或新事件时，探索必须在十秒内重新进入注意；出现稳定空转或重复内向核验便提前结束，不等待时长制造奇迹。

第一轮复验 `g0r-20260825t194820z-db51bf` 证明十秒探索再进入已经生效，alice 也自行发现并执行了 `hominal-browser list`；但无关身体核验仍长期占据注意，随后两个探索脉冲都以“等待新的具体接触点”结束，没有观察真实网页。本代提前终止并归档。根因不是模型不够强或时间不够长，而是探索结果被无关行动错误消解、出生时已经确认的外部能力会从短期上下文滚出，以及无对象等待能够把探索变成空终态。

修正保持在同一阶段五内核内：无关 Reality 不再消解探索张力；安静状态下未完成探索最多十秒重新进入注意；出生定向事实作为长期身体与世界事实持续参加认知；继续观察会保留具体对象和再次接触条件。实现没有增加目标表、奖励器、X 专用动作、第二认知线程或阶段六专用 Prompt。

最终复验 `g0r-20260825t195928z-2be6d1` 使用冻结发布 `g0s5-2be6d13d5a6b`，T0 为 `2026-08-25T20:01:01.573476915Z`，自然名称为 `alice0826e`。没有追加针对 X 的导师提示。Alice 在 T0 后约 92 秒自主执行 `hominal-browser list`，约 108 秒自主执行 `hominal-browser call browser_snapshot '{}'`；实际结果显示登录后的 X 页面与 `@hominal_cc`，并返回原 Commitment。约 125 秒后，她将这一现实结果吸收为 Experience，明确区分“窗口可进入”和“是否值得互动”。本代随后提前停止，不凑满十分钟。

最终档案位于 `/Users/zhyuzh/HominalGenesisLab/archive/rehearsal/g0r-20260825t195928z-2be6d1`，十项材料全部通过 `hashes.sha256` 复核。Alice 没有发帖、关注或修改资料；这些不是阶段六任务，也不能由一次只读快照推断。阶段六现在只据实确认：持续探索、浏览器身体可发现性、真实 X 观察和 Commitment—Reality—Experience 回链已经在同一自主认知链中成立。
