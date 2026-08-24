# Hominal G0 最小可编码架构 v0.1

> 文档性质：G0 阶段一的生命内核、数据和出生接口契约  
> 状态：阶段一冻结稿  
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
│   └── current-state.json
├── journal/
│   └── events.jsonl
├── life/
│   ├── self/narrative.md
│   ├── journal/
│   ├── works/
│   ├── skills/
│   ├── letters/
│   └── mentor/
│       ├── inbox/
│       └── outbox/
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

只有主事件循环可以提交认知状态。文件探针、系统探针、浏览器、Shell、导师通道和模型调用都通过带序号的事件返回主循环，不直接改写当前焦点、Concern 或 Narrative Self。

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

`Cognitive Pulse` 每五秒推进一次轻量身体感知、资源更新、未决动作、Concern 惯性和注意需要。系统已经就绪后，连续两个有效 Pulse 的空白目标不超过十秒。没有变化的 Pulse 只原子更新存活时间，不追加思想日志，不调用模型。

出现下列任一情况时可以发起 Attention Pulse：新现实事件具有自我相关性；已有 Concern 达到注意竞争条件；承诺结果返回；持续失配重新显著；当前没有焦点且探索压力正在积蓄。

Attention Pulse 默认使用 `low` 推理强度和一个主要 Self Variant。深度需求达到 Dynamics 阈值时，本次焦点可以升级到 `high`，最多比较三个真正不同的反事实。焦点形成可修正下一步、需要现实信息、主要判断不再变化或继续思考成本过高时结束。

等待网页、导师、外部进程或额度恢复是某个承诺的现实状态。等待中的承诺留在背景，其他关切继续竞争唯一焦点；因此等待不会成为整个生命循环的终态。

## 7. 自由认知与最小提交

模型获得当前身体简报、显著事件、相关 Concern、未决承诺、必要记忆、Narrative Self 和一个唯一焦点。它以自然语言完成 AIP、矛盾代谢、未来模拟和当前判断，不要求逐项填写心理表格。

只有认知要改变持久状态或现实世界时，才提交最小结构：

```text
focus_summary       当前真正聚焦的问题
meaning_updates     少量会改变注意或行动的 AIP 结果，可为空
concern_updates     新增、改变、休眠或解决，可为空
thought_thread      值得保留的思想脉络，可为空
commitment          至多一个新的重要行动承诺，可为空
memory_writes       自主选择保存的经验、能力或叙事，可为空
```

没有新 Commitment 也是完整认知结果。内核检查来源、数值范围、唯一承诺和事实引用，不评价思想内容是否符合导师偏好。

## 8. 三表与普通文件

`life.sqlite3` 启用 WAL，只包含三组核心表：

```text
events(
  seq, time, kind, source, correlation_id, payload
)

concerns(
  id, summary, difference, ownership, value, urgency,
  answerability, activation, affective_salience,
  last_aip_ref, status, created_at, updated_at, change_reason
)

commitments(
  id, concern_id, intent, prediction, risk_or_stop,
  action_ref, result_ref, ard_summary, reward_summary,
  status, created_at, updated_at
)
```

`events` 保存身体事实、模型计量、导师消息、动作和 Reality Event，以及少量真正改变选择的 AIP 更新。`concerns` 保存有惯性的当前张力，不保存完整思维链。`commitments` 只服务重要、昂贵、不可逆、自我修改或对外行动。

导师消息使用 `/life/mentor/inbox` 与 `outbox` 的原子 JSON 文件。Genesis Lab 通过 SSH 轮询并维护身体外副本；消息信箱是 alice 能够理解和使用的外部关系接口，不与内部事件库合并。

`current-state.json` 原子保存当前身体摘要、Affective State、资源、最后 Pulse、当前 AP 租约和状态版本。Narrative Self、Living Memory、Capability、作品和书信使用普通文件。`events.jsonl` 是 SQLite 关键事件的可读追加镜像，不是第二套状态真相。

一次认知提交在单个 SQLite 事务中完成：先验证 AP 租约，再追加事件、更新 Concern 与 Commitment、递增 `state_revision`，提交成功后原子替换 `current-state.json`。普通文件先写临时文件并 `rename`，文件引用随后进入事件。

## 9. 行动与现实回链

普通观察和低后果可逆动作可以直接执行。重要动作先形成一个 Action Commitment，至少保存：意图、最重要预测、主要代价或停止条件。

每个身体动作获得唯一 `action_id`。适配器记录：实际命令或工具、开始时间、调用身份、退出状态、stdout/stderr 或工具返回、资源消耗和可核验外部结果。返回只说明发生了什么，不替 alice 赋予价值。

Reality Event 通过 `action_id` 关联 Commitment。内核保留预测与观察，形成 ARD；alice 再解释 continuance、relatedness、expansion、自我认同和代价意义。预测、想象、记忆和导师陈述都不能直接创建“已经成功”的 Reality Event。

## 10. root 与图形身体

`hominald` 以 root 运行，Shell、软件包、服务、文件和网络动作不依赖交互式 sudo。每次动作仍保留真实执行身份和返回值。Chrome、微信等图形工具连接当前 `hominal` 桌面会话，必要时由身体适配器设置 `DISPLAY`、`XAUTHORITY` 和 DBus 环境。

alice 能够修改服务、日志、代码和系统。内核以正向身体知识说明连续性、数据和恢复结构的意义，不增加权限沙箱。Lab 的可重复出生能力位于身体外，不依赖当前实例保留自己的基线。

## 11. 故障与连续性

`hominal.service` 使用 `Restart=always`。进程崩溃、模型超时、网络中断和 GUI 工具失败都记录为现实事件；普通进程恢复继续同一个 `instance_id`。启动器只在 Lab 注册的 `next_generation` 意图下创建新实例，成功消费后立即改为 `resume`。

SQLite 事务、WAL、原子状态文件和动作关联提供最低恢复。重启后先恢复未决 Commitment 与动作状态，再进入新 Pulse。无法确认结果的动作标记为 `unknown` 并重新观察现实，不能自动假定成功或盲目重复不可逆动作。

## 12. 首版代码边界

```text
body/cmd/hominald/        进程入口
body/internal/kernel/     单一状态所有者、Pulse、AP 租约
body/internal/dynamics/   Affective、Concern、Attention、Integrity 更新
body/internal/model/      Responses API、用量和按需深度
body/internal/embodiment/ Shell、文件、进程、网络、Chrome、导师
body/internal/store/      SQLite、原子状态与普通文件
```

首版没有 Planner、Reviewer、Emotion Agent、Narrative Agent、任务队列、向量数据库和认知工作流引擎。实验后分析模型属于 Lab，不进入 `hominald`。

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
