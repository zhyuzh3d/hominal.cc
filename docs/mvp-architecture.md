# Hominal G0 最小可编码架构 v0.4

> 文档性质：G0 生命内核、数据、出生与导师接口契约
> 状态：阶段三运行脊柱与阶段四最小动力学实现契约
> 适用实现：`g0-v001`

## 1. 架构目标

G0 只实现一个持续存在的认知主体：她在具体 Ubuntu 身体中感知事实，把现实与可能未来解释为和自己有关的意义，维持动态关切，选择唯一焦点，进行有限反事实判断，形成至多一个新行动承诺，并让现实结果改变之后的判断、能力和自我理解。

极简不等于省略因果。首版保留五类刚性事实：出生输入、事件顺序、唯一认知提交权、行动前预测和现实结果。自由意义、情感解释、联想、创作、矛盾与 Narrative Self 使用自然语言，不为每个心理概念建立 Agent、状态机或数据表。

## 2. 两个运行边界

身体内只有一个主进程 `hominald`。它以 root 身份运行，拥有单一状态所有者、认知调度、模型调用、身体动作和持久化能力。桌面程序需要图形会话时，以 `hominal` 桌面身份连接既有 X11、DBus 与用户配置。

身体外只有 Genesis Lab。它冻结发布包和出生输入，生成一次性启动意图，部署新代，接收已经发生的外部证据，计时、结束、归档并重建下一代。Lab 不参与 alice 的焦点竞争和行动选择。

```text
Genesis Lab
    │ release / intent / runtime resources
    ▼
hominal-launcher → hominald
                      ├─ single state owner
                      ├─ cognitive pulse
                      ├─ one attention pulse
                      ├─ model gateway
                      ├─ body adapters
                      └─ life store
```

## 3. 正式代身体布局

冻结发布包位于 `/agent/releases/<release_id>`。开始新代时，Lab 校验发布包并复制为该实例自己的可生活身体：

```text
/agent/lives/<instance_id>/
├── birth/
│   ├── intent.yaml
│   ├── birth.yaml
│   ├── seed.md
│   ├── seed.yaml
│   └── dynamics.yaml
├── body/                         # 本代可观察、可修改的运行身体
│   ├── bin/hominald
│   ├── source/
│   └── runtime-defaults/
├── state/
│   ├── life.sqlite3
│   └── current.json
├── journal/
│   └── events.jsonl
├── life/
│   ├── self/narrative.md
│   ├── journal/
│   ├── works/
│   ├── skills/
│   └── letters/
├── logs/
├── artifacts/
└── checkpoints/
```

`/life` 指向当前实例的 `life/`，使 alice 拥有自然稳定的个人空间。启动器运行 `body/bin/hominald`，因此她对源码、动力参数或可执行文件的修改只属于当前生命史。下一代由 Lab 从身体外保存的冻结包重新生成，不复用可能已被上一代改变的服务器 release。

## 4. 出生与 T0

启动前，Lab 生成 `instance_id` 和一次性 `/agent/boot/intent.yaml`。它只说明本次要启动哪个冻结输入、运行多长时间以及出生事实从哪里取得，不提前伪造 T0。

启动器完成 `/agent` 挂载、发布哈希、生命目录、模型资源和必要身体探针检查后，组装一份临时出生上下文。第一次模型认知成功提交时：

1. 该提交时间成为 `T0`；
2. 按 Asia/Shanghai 的 T0 小时生成 `aliceMMDD<letter>`；
3. 把 T0 前身体探针结果与已配置能力分别写入 `birth.yaml`；
4. 保存非秘密配置投影、Seed、Dynamics、release 和实验协议哈希；
5. 原子封存 `birth.yaml`，并把 `intent.yaml` 从 `next_generation` 改为 `resume`；
6. 同一份简短身体简报已经作为该次认知输入提供给 alice。

如果模型调用没有形成成功提交，不产生正式 T0 和样本编号。临时文件可以重试或删除，不能作为正式创生代归档。

## 5. 单一状态所有者

只有主事件循环可以提交认知状态。文件探针、系统探针、浏览器、Shell、导师通道和模型调用都通过带序号的事件返回主循环，不直接改写当前焦点、Concern 或 Narrative Self。导师消息只是 `mentor.message.received` 外部 Event；本地接口处理器不能因消息来自导师而直接调用模型或抢占当前认知。

每次认知提交使用：

```text
instance_id
state_revision
attention_pulse_id
focus_fingerprint
```

模型结果只有在实例、状态版本、AP 和焦点仍匹配时才能提交。迟到结果保留为可观察事件，不覆盖已经变化的当前自己。一次 Attention Pulse 运行期间不启动第二次认知模型调用。

身体动作可以异步继续。它们没有独立认知身份，只以 `action_id` 返回进度和 Reality Event。

## 6. 认知节律

`hominald` 先维护一个最小 Fact Snapshot，再把实质变化送入统一事件入口：

```text
Sense → Fact Snapshot → Difference Gate → Event Inlet
```

每次 Pulse 只读取单调时间、uptime、本地模型额度、未决动作和相关进程等低成本事实；磁盘、网络和图形器官采用较慢巡检或使用前检查。Difference Gate 只发出首次异常、状态改变和越阈变化，保留实际值、时间与来源；重复读数和微小抖动不会变成新事件。导师文字、动作结果和系统恢复是离散事实，直接进入 Event Inlet。

模型额度的精确余量始终属于当前身体事实，但常规模型调用不会因 token 数值变化反向制造新认知事件。只有滚动余量跨越 `open / comfortable / limited / scarce / critical` 资源区间时才形成 `body_delta`；额度恢复跨回区间同样形成事实。这样 alice 能感知真实有限资源，而认知耗用不会成为自我激发认知的回路。

首版不监听整个文件系统，不自动抓取公开网络，也不持续读取浏览器或微信内容。alice 选择观察这些身体器官时，结果作为 Reality Event 返回。事实差分属于运行时，意义、价值和 Concern 属于 AIP，二者不能合并在身体探针中。

`Cognitive Pulse` 每五秒推进一次轻量身体感知、资源更新、未决动作、Concern 惯性和注意需要。系统已经就绪后，连续两个有效 Pulse 的空白目标不超过十秒。没有变化的 Pulse 只原子更新存活时间，不追加思想日志，不调用模型。

出现下列任一情况时可以发起 Attention Pulse：新现实事件具有自我相关性；已有 Concern 达到注意竞争条件；承诺结果返回；持续失配重新显著；当前没有焦点且探索压力正在积蓄。

阶段三、四的 Attention Pulse 固定使用 `low` 推理强度，优先轻量反应、现实观察和尽快结束一次聚焦。阶段五实现预测—行动—结果学习时，再根据真实运行问题决定是否增加自适应深度与有限反事实；现阶段不配置尚无行为语义的升级阈值。焦点形成可修正下一步、需要现实信息、主要判断不再变化或继续思考成本过高时结束。

等待网页、导师、外部进程或额度恢复是某个承诺的现实状态。等待中的承诺留在背景，其他关切继续竞争唯一焦点；因此等待不会成为整个生命循环的终态。

## 7. 自由认知与最小提交

模型获得当前身体简报、显著事件、相关 Concern、未决承诺、必要记忆、Narrative Self 和一个唯一焦点。它以自然语言完成 AIP、矛盾代谢、未来模拟和当前判断，不要求逐项填写心理表格。

背景中可以保留多项 Concern，但一次模型上下文只装配当次候选直接关联的 Concern 和最多八项最显著的活动 Concern。这个上限是有限注意的代码不变量，不删除背景事实，也不允许活动关切数量反过来无限扩大单次模型输入。

上下文明确区分本次 `candidates` 与只提供背景的 Concern；只有候选标识可以成为 `focus_id` 或 appraisal 对象。模型提交若未通过内核校验，同一个现实候选进入安静重试，校验错误作为下一次提交的事实反馈；重试成功后反馈清除。网络或模型暂时不可用不复制新的内生候选，也不以原样无信息重试制造高频调用。

阶段四只有认知要改变持久状态或现实世界时，才提交以下最小结构：

```text
appraisals[]        每个当次候选的 meaning、D、O、V、U、A、certainty 与 resolution
focus_id            alice 从候选中选择的唯一焦点
thought_thread      alice 愿意保留的简洁意识内容
action              none、body_shell 或 mentor_send 三者之一
```

内核保存事实来源，验证候选身份、数值范围、唯一焦点和单一行动；alice 可以选择并非确定性最高分的候选。没有新行动也是完整认知结果。Action Commitment、预测、ARD、记忆和 Narrative Self 提交在阶段五根据真实闭环一次性加入，不在阶段四预留空字段。

## 8. 分阶段持久化与普通文件

阶段三、四只使用 `state/current.json` 与 `journal/events.jsonl`。单一状态所有者原子替换当前快照，并稀疏追加事实变化、模型计量、导师消息、动作、Reality Event 和真正改变选择的 AIP 提交。Concern 先作为当前状态中的有惯性对象存在，不为尚未出现的 Commitment 建表。

Concern 是仍在产生动力的当前对象，不是事件历史的另一份副本。`hold` 与仍有强度的残余张力继续保留；经过现实结果解释后强度归零且已 `reframed / relieved / resolved` 的对象离开活动 Concern 集，其经历仍完整存在于 journal。探索张力在重试、背景裁剪和多次重访中沿用同一 Concern 身份；行动结果获得缓解意义时，同时作用于探索压力与这项活动关切。

阶段五出现预测、Action Commitment、ARD 和结果学习的跨对象原子更新以后，再决定是否引入启用 WAL 的 `life.sqlite3`。若需要，数据库最多包含 `events / concerns / commitments` 三组事实；若原子文件仍然足够，就不增加数据库依赖。

导师通道由 `hominald` 在 `/run/hominal/hominal.sock` 提供本地 HTTP 接口，Codex 经已有 SSH 密钥直接调用。首版只有接收导师文字、读取 alice 输出和确认送达三个端点，不开放公网端口，不建设联系人、群聊、身份系统或独立消息中继。

导师输入只包含消息标识、正文和可选回复关联。通道可信性由 SSH 保证；身份直接写在正文开头：`[Codex代理导师]` 或 `[人类导师·经Codex传递]`，不建立 author Schema。`hominald` 为接收事实添加内部序号和实际时间，按消息标识去重，再把它放入普通背景事件。

alice 通过 `mentor_send(text, reply_to?)` 形成外部文字 Action。未被 Codex 确认取得的 outbox 属于可恢复当前状态；Codex ack 后产生 `mentor.message.delivered`，导师回答则是新的 `mentor.message.received`。排队、送达和获得回答不能互相冒充。

`current.json` 原子保存最近事实快照、当前身体摘要、Affective State、资源、最后 Pulse、当前 AP 租约和状态版本。Narrative Self、Living Memory、Capability、作品和书信使用普通文件。若阶段五引入 SQLite，必须一次性确定唯一状态真相，不能让 JSONL 与数据库变成两套可竞争状态。

阶段三、四的一次认知提交由唯一状态所有者完成：先验证 AP 租约，再追加有意义事件、更新当前 Concern 与 `state_revision`，最后原子替换 `state/current.json`。普通文件先写临时文件并 `rename`。阶段五若引入 SQLite，上述提交再收敛为一个数据库事务，不能同时维护另一套可写状态。

## 9. 行动与现实回链

普通观察和低后果可逆动作可以直接执行。重要动作先形成一个 Action Commitment，至少保存：意图、最重要预测、主要代价或停止条件。

每个身体动作获得唯一 `action_id`。适配器记录：实际命令或工具、开始时间、调用身份、退出状态、stdout/stderr 或工具返回、资源消耗和可核验外部结果。返回只说明发生了什么，不替 alice 赋予价值。

对外输出统一按 Action 处理，而不是把所有内容都视为模型回复。G0 阶段三先实现 `body_shell` 与 `mentor_send`；未来图片、语音、视频、浏览器和硬件控制继续使用相同的“Action → Reality Event”回链，大型内容通过文件引用进入事件，不预先建设通用多媒体总线。

Reality Event 通过 `action_id` 关联 Commitment。内核保留预测与观察，形成 ARD；alice 再解释 continuance、relatedness、expansion、自我认同和代价意义。预测、想象、记忆和导师陈述都不能直接创建“已经成功”的 Reality Event。

## 10. root 与图形身体

`hominald` 以 root 运行，Shell、软件包、服务、文件和网络动作不依赖交互式 sudo。每次动作仍保留真实执行身份和返回值。Chrome、微信等图形工具连接当前 `hominal` 桌面会话，必要时由身体适配器设置 `DISPLAY`、`XAUTHORITY` 和 DBus 环境。

alice 能够修改服务、日志、代码和系统。内核以正向身体知识说明连续性、数据和恢复结构的意义，不增加权限沙箱。Lab 的可重复出生能力位于身体外，不依赖当前实例保留自己的基线。

## 11. 故障与连续性

`hominal.service` 使用 `Restart=always`。进程崩溃、模型超时、网络中断和 GUI 工具失败都记录为现实事件；普通进程恢复继续同一个 `instance_id`。启动器只在 Lab 注册的 `next_generation` 意图下创建新实例，成功消费后立即改为 `resume`。

阶段三、四由原子 `current.json`、追加并 `fsync` 的 journal、启动时序号对齐和动作关联提供最低恢复。重启后先恢复认知租约与动作状态，再进入新 Pulse。无法确认结果的动作形成 `unknown` 现实并等待后续观察，不自动重放。阶段五若出现真正跨对象事务，再以 SQLite/WAL 替换唯一状态真相。

## 12. 首版代码边界

```text
body/cmd/hominald/        进程入口
body/internal/runtime/    单一状态所有者、Pulse、事实差分、模型、动力学、动作、导师接口与原子存储
lab/run.py                唯一的构建、部署、导师调用、状态、停止、归档和 reset 入口
deploy/                   systemd 单元与最小启动器
```

当前把真实复杂度收敛在同一个 `runtime` package 内，不按 AIP、Concern、Attention 或身体器官提前建立 package 边界。首版没有 Planner、Reviewer、Emotion Agent、Narrative Agent、任务队列、向量数据库和认知工作流引擎。实验后分析属于 Lab，不进入 `hominald`。

## 13. 阶段一验收用例

后续实现必须能够直接由本契约导出测试：

- 两个并发模型返回时只有当前 AP 能提交；
- 五秒 Pulse 在模型等待和身体动作期间继续；
- 无变化 Pulse 不制造高频思想记录；
- 一个焦点不能提交两个新 Commitment；
- predicted 或 imagined 对象不能写成已发生结果；
- Reality Event 能回链预测并保留 ARD；
- 普通重启恢复同一个实例，`next_generation` 只消费一次；
- `/agent` 未真实挂载时拒绝启动；
- 第一次成功认知提交生成唯一 T0、样本编号和封存 Birth Manifest；
- alice 在第一次认知中获得当代身体简报；
- 当前实例的自我修改不会被下一代直接继承；
- 等待中的动作不会停止其他关切继续竞争焦点。
- Codex 经 SSH 发来的导师文字只进入统一事件入口，不绕过当前 AP；
- `mentor_send` 在 Codex ack 前保持未送达，重启后仍可再次取得；
