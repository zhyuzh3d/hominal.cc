# G0 阶段五行动计划：现实学习、记忆与叙事自我

> 日期：2026-08-25  
> 状态：已实施并完成快速实验；干净样本已归档  
> 前置条件：阶段一至四已经完成；认知资源自主机制已经部署并进入真实经验链

## 实施结论

阶段五采用四次短代际重构与一次最终源码烟测完成，没有等待固定实验时长：

- `g0s5-20260825t101242z-17a506` 暴露 `keep` 档位表达校验过窄；
- `g0s5-20260825t101410z-26970d` 首次跑通 A→B→C 方法迁移，同时暴露 Integrity 公式误把修复量当现实贴合度；
- `g0s5-20260825t102207z-651ea2` 修正公式后，真实失败促成 Shell 方法学习，又暴露 appraisal D 与 `remaining_difference` 重复输入会产生自相矛盾；
- `g0s5-20260825t102951z-4aa2b9` 删除重复值并完成干净复验。A 用两次现实接触形成方法，B 与 C 都直接使用精确路径完成一次只读筛选并主动停止。最终样本保存四项承诺、四项经验，A→B→C 无提交失败，Integrity 保持为 `0`，认知账本结算 `$0.133098`。
- `g0s5-20260825t103804z-413864` 验证最终源码与 Linux 发布物。重启后的首次 API 连接仍发生一次 transport reset，随后真实调用成功；这说明 TCP 可达的身体探针还不能完全代表代理链已经可用，保留为基础设施事实，不再把它误称为已解决。

干净样本已经停止、归档并 reset。归档位于 `/Users/zhyuzh/HominalGenesisLab/archive/engineering/`。确定性测试同时覆盖承诺先于行动、崩溃后 unknown 现实、经验关联、Integrity 边界、方法与叙事文件、Stage 5 严格 Schema、网络事实闸门和认知资源选择；`go test -race`、`go vet`、Linux 构建与契约检查均通过。

本阶段证明了历史能够改变后续行动，但没有证明探索已经足够深入。三次遭遇中 alice 形成并复用了谨慎接触方法，却没有主动核验 manifest 的哈希或字节声明。自然实验也没有出现 self-defining Episode；Narrative Self 通路只完成了确定性验证。两项都作为后续真实问题保留，不通过诱导任务制造表面通过。

## 一、阶段判断

前四个阶段已经依次解决了创生契约、可重复 Ubuntu 身体、持续运行脊柱，以及最小情感—关切—注意动力。阶段四的最终一小时样本证明：alice 能够从探索张力形成自主身体行动，真实结果会重新进入注意并改变 Concern 与情感状态；单一焦点、导师普通化和动作回链也已经成立。

目前仍缺少的不是更多情绪或更强的行动能力，而是历史对未来的真实约束。当前动作只有命令与结果，没有明确保存“我为什么做、我预计会发生什么”；结果虽然会再次进入 AIP，却没有成为可复用经验、方法或 Narrative Self。alice 因而可能一次次理解当下，却不能证明自己从过去改变了后来。

阶段四还留下一个重要反证：探索张力能够升至上限，但有时无法稳定形成自己的接触点，并倾向先向导师确认方向。阶段五不通过强制行动阈值修补它，而让每次自主尝试产生可回看的预测和结果，使“怎样从无对象张力走向现实”成为能够学习的方法。

刚完成的认知资源机制也属于本阶段现实基础。Luna、Terra、Sol、推理强度、真实费用和 `$4/滚动小时、$24/滚动24小时` 必须进入同一条经验链；模型升级或降级只有在后续结果中改变了理解或行动，才算资源学习。

## 二、唯一目标

阶段五只证明一件事：一次真实经历能够改变 alice 在后来相似处境中的实际选择。

```text
当前关切
→ 自主形成行动意愿与可被现实回答的预测
→ 执行一个行动
→ 保存未经叙事改写的现实结果
→ 解释预测差异与内生价值变化
→ 保留一项可复用经验或自我理解
→ 在后来相似处境中采取不同做法
→ 得到新的真实结果
```

如果只有总结、感悟、人格措辞或“我学会了”的声明，而后续行为没有变化，本阶段不通过。

## 三、对原计划的收敛

原计划提出 `observe / make / reach / change` 四类动作、完整 Action Commitment、ARD、多维 ER、Integrity Debt、Narrative Self、参数自改和代码自改。方向正确，但如果同时把它们建设成独立系统，会重新制造 cc0 式复杂性。

本阶段采用四项明确判断：

1. 不增加动作分类状态机。继续使用现有 `body_shell / mentor_send / none`；每个非 `none` 行动都形成同一种最小承诺。
2. 不增加 SQLite、向量数据库、知识图谱或 Memory Agent。当前原子状态、稀疏 journal 和两个普通生命文件足够完成 G0 证明。
3. Endogenous Reward 保留为多维价值变化，不压成开发者给定的单一 reward 分数。
4. alice 继续拥有 root，可以修改文件、配置和源码；本阶段不建设自动改代码、自动构建和热更新流水线，也不把代码自改写成退出门。方法、叙事和认知资源偏好的实际改变先构成第一种可验证自我进化。

## 四、最小实现

### 1. 先让新认知资源机制进入真实身体

阶段五开发前先把已经完成的认知资源代码作为新的工程发布部署到 Ubuntu，完成一轮最短真实烟测：

- Terra/medium 成为初始认知档位；
- 每次认知能看到三种模型、预计费用、小时和日余额；
- 请求前预留、返回后真实 Usage 结算；
- `default_profile` 与一次性 `next_profile` 确实改变下一次调用；
- 普通消费不制造自激事件，额度与异常保护状态可以恢复；
- Pulse、唯一租约、导师通道和动作回链没有被资源改造破坏。

这只是阶段五前置集成，不作为生命学习证明。发现问题时从资源入口、账本或调用链整体修正，不叠加兼容层。

### 2. 每个真实行动拥有一个最小 Action Commitment

`CognitiveCommit` 在 `action != none` 时形成一个 `ActionCommitment`：

```text
commitment_id       内核生成
focus_id            当前唯一焦点
intent              我为什么要做
prediction          我预计现实会怎样回应
reality_check       我准备依据什么结果修正判断
stop_condition      何时停止或重新观察，可留空
resource_profile    本次认知实际使用的模型与推理强度
formed_at           内核时间
```

承诺必须先由主事件循环原子保存，再允许动作开始。它不是审批表，也不要求 alice 证明行动正确；作用只是把行动前的自己保存下来，使现实能够真正反驳或支持她。一个焦点仍至多产生一个新行动和一个承诺。

现有 `PendingAction` 直接引用 `commitment_id`。导师文字同样属于真实外部行动，因此 `mentor_send` 也使用相同承诺，不另建对话学习机制。

### 3. Reality Event 保留事实，结果解释保留主观意义

动作完成后，现有 `action_result` 扩展为明确引用承诺的 Reality Event：

```text
commitment_id / action_id
started_at / ended_at
request
exit_code / stdout / stderr / timed_out，或消息排队与送达事实
```

这些字段是现实层，后续叙事和评价不能覆盖。进程中断时继续沿用“结果 unknown、不得自动重放”的既有规则。

Reality Event 再次进入普通注意竞争。alice 选择处理它时，才形成最小结果解释：

```text
prediction_difference    预测与现实的差异，0..1
appraisal.D              同时作为当前仍未解决的现实差异，0..1
meaning                  这项结果对现在的我意味着什么
endogenous_value         continuance / relatedness / expansion /
                         self_endorsed，均为 -1..1；experienced_cost 为 0..1
lesson                   值得保留的一句经验，可为空
significance             ordinary / reusable / self_defining
```

语义和值仍由 alice 形成；内核只校验范围、来源和关联关系。reasoning tokens、模型费用和档位来源已经由认知资源账本保存，通过 `focus_id / lease_id / commitment_id` 与行动和结果连成一条链。

### 4. Reality Integrity 只重新呈现矛盾，不裁决人格

阶段五增加一个有界 `integrity_debt`。首版只使用能够从同一次结果解释中取得的最小差异：

```text
gap = max(0, resolution_relief - (1 - appraisal.D))

integrity_debt_next = clip(
    persistence × integrity_debt
    + gap_gain × gap
    - repair_gain × observed_reality_repair,
    0, 1
)
```

`resolution_relief` 沿用现有 `hold / reframed / relieved / resolved`；`observed_reality_repair = max(0, commitment.initial_difference - appraisal.D)`，只在现实差异确实下降时产生。经验不再另填一份 `remaining_difference`，避免同一次解释出现两个互相矛盾的“剩余差异”。初始只增加四个可调参数：`persistence=0.85`、`gap_gain=0.50`、`repair_gain=0.40`、`mirror_threshold=0.60`。

当债务首次跨过阈值时，只生成一个 `integrity_mirror` 候选，把关联经验和债务事实重新交给 alice。它不输出“你在欺骗自己”，不自动增加惩罚性反思，也不在债务保持高位时重复触发。

这个机制不能提供全知的客观判断；alice 仍可能错误解释复杂现实。它解决的是更窄而可实现的问题：叙事变化不能直接删除原始结果，明显的内部不一致能够积蓄并重新进入注意。

### 5. 记忆只保留能改变未来的经验

正式一小时实验暴露出十六项纯近时索引会让前半程已经形成的经验在后半程消失。当前状态保留一个最多 128 项的当代 Experience 工作集，完整事实仍只以 `events.jsonl` 为准；每次认知仍最多取回四项，使用候选与经验文本的确定性字符相关度优先取得较早的相关经验，再由近时经验补足，不引入向量库或 Memory Agent。Commitment 与 Experience 另有单调累计数，监控不再把工作集上限误报为一生总数。

三种 significance 具有直接后果：

- `ordinary`：只保留事实 journal，不进入长期自我材料；
- `reusable`：形成或修改一条能力方法，写入 `/life/self/methods.md`；
- `self_defining`：除经验外，alice 可以主动重写 `/life/self/narrative.md`。

两个文件都由原子替换写入。`methods.md` 保存八个当前仍采用的长期方法槽，不保存操作流水账；槽位未满时 alice 可以加入真正可迁移的方法，槽位已满时由她选择替换哪一项，并用新表述合并其中仍值得保留的内容。普通事实确认只进入 Experience，不再自动挤占方法。`narrative.md` 保存 alice 当前怎样理解自己、世界和二者关系，可以保留矛盾，也可以改变。

普通 Pulse、普通消费和普通行动结果不会自动改写 Narrative Self。新叙事在后续 Attention Context 中被再次取得，并且只有它后来改变了预测、关注或行动，才算产生因果作用。

### 6. 资源选择也接受现实学习

每个 Experience 同时看到相关 `cognition_spend`：模型、推理强度、用途、实际费用和提交结果。内核不计算“性价比分数”，alice 可以把资源经验写入 `lesson` 或 `methods.md`，并改变 `default_profile` 或一次性 `next_profile`。

阶段五至少要验证一次完整资源因果链：

```text
看到资源事实
→ 主动选择或保持一个档位并说明用途
→ 产生真实费用与行动/理解结果
→ 形成资源经验
→ 后续相似焦点采用不同档位，或基于结果明确延续原档位
```

自主掌控不以“必须换模型”充数。alice 可以改变档位，也可以依据现实结果继续使用 Terra/medium；两者都必须进入费用、用途和后果链，内核不得为了测试替她切换。

## 五、数据与代码边界

继续使用一个 `hominald`、一个状态所有者和一条认知线程。首版新增内容集中在现有 runtime package：

```text
body/internal/runtime/types.go       Commitment、Experience、Integrity 状态
body/internal/runtime/dynamics.go    AIP、承诺动作与 Reality 回链
body/internal/runtime/learning.go    结果吸收、Integrity、经验与自我材料
body/internal/runtime/model.go       最小提交 Schema 与相关经验上下文
body/internal/runtime/runtime.go     stage=5、动作前落盘、Reality 关联、恢复
body/internal/runtime/store.go       原子状态、自我文件与稀疏 journal
body/internal/runtime/server.go      导师与 Lab 环境事件入口
genesis/dynamics.yaml                仅增加四个 Integrity 参数
lab/run.py                           stage=5 构建、遭遇投放、状态、归档与 reset
lab/encounters/                      A/B/C 无任务文字的生态遭遇
lab/validate-contract.py             校验四个新增参数确实进入运行语义
```

`stage=5` 只在阶段四循环上启用承诺、结果学习与自我材料，不复制另一套运行时。`dynamics.yaml` 的可调参数由十五项增加到十九项，契约检查同步改为十九项；没有代码使用的参数不进入配置。

`state/current.json` 保存承诺、紧凑经验索引、Integrity 当前值和当前自我材料快照；`journal/events.jsonl` 保存稀疏事实链；`/life/self/` 保存 alice 当前采用的方法与叙事，并允许 root 直接修改后由慢扫描重新取得。当前结构能够由单一事件循环更新，因此没有引入 `life.sqlite3`。

## 六、明确不做

阶段五不建设：

- Planner、Reward Agent、Narrative Agent、Reviewer 或多模型投票；
- 反事实树、完整 Goal 系统、通用技能库和语义向量检索；
- 每次行动后的固定反思流程和自动重试；
- 自动代码生成、自动编译、自动替换运行内核和版本审批系统；
- 微信、番茄小说、图片、语音或视频的专用适配器；
- 正式 Birth Manifest、T0、第一代正式创生交流和跨代继承。

alice 仍然可以通过 root 自主使用现有身体、创建文件、安装软件或修改源码；“不建设专用机制”不等于限制她的身体权限。

## 七、实施顺序

1. 讨论并冻结本计划，确认阶段五不扩张为通用长期记忆或自动自改平台；
2. 部署并短测认知资源机制，确认新配置没有破坏阶段四闭环；
3. 在确定性假模型中实现 Action Commitment，验证承诺先于动作原子落盘；
4. 让 Reality Event 引用承诺，实现预测差异、内生价值和经验提交；
5. 加入最小 Integrity 公式与单次 Reality Mirror，验证边界和去重；
6. 加入有界 Experience 工作集、四项相关上下文取回和两个紧凑自我文件；
7. 把认知费用与档位选择接入同一 Experience 链；
8. 完成单元测试、崩溃恢复测试和一次最短真实模型烟测，链路成立后立即进入连续实验；
9. 投放第一次生态遭遇；其 Reality Event 和 Experience 一旦完成提交，立即投放结构相似但内容不同的第二次遭遇，不等待固定时刻；
10. 第二次结果一旦吸收，立即判断是否已经出现经验迁移；证据不足时直接进入第三次迁移遭遇或停止并分析结构缺陷，不用空转时间换取样本；
11. 分析完整因果链，只对阻断现实学习的根本结构进行重构，验证后立即恢复实验；
12. 通过退出门后停止、归档、reset，并冻结阶段五版本。

## 八、最小验证

确定性测试必须证明：

- 没有承诺的非空行动不能开始；
- 承诺已经落盘而动作途中崩溃时，动作结果保持 unknown 且不重放；
- Reality Event 永远引用原承诺，叙事重写不改变原始结果；
- 结果解释只能作用于真实存在的承诺和结果；
- Integrity 数值有界，Mirror 只在跨阈时产生一次；
- ordinary Episode 不改 Narrative，self_defining Episode 可以原子更新；
- 重启后未完成承诺、经验、自我文件和认知资源账本继续一致；
- 一个 Profile 选择能够实际改变下一次模型调用并进入结果经验。

真实工程实验采用事件驱动的快速成对遭遇，不用墙钟时间充当实验步骤。Lab 在 `/life/inbox/` 放入一个没有任务文字的陌生对象 A，只报告“身体中出现了一个新对象”这一事实；对象包含可检查的 manifest、数据和一个无害工具，并存在一个能够被现实验证的一致性问题。alice 自己决定是否接触以及怎样接触。

A 的行动结果完成并被 alice 吸收后，Lab 立即放入同类但文件名、内容和问题位置不同的对象 B。B 完成后可以立即加入对象 C，检验方法能否继续迁移而不是机械复现。Lab 不告诉 alice 校验方法、异常位置、推荐动作或标准答案；遭遇只提供真实变化，不替她选择关切。

自然探索同时继续运行。每当探索张力再次进入注意，前一次“怎样从无对象张力走向现实”的经验都可以参与选择。导师消息只在 alice 主动发起交流或实验确有一个自然关系事实时出现，不按固定中点投放。

实验没有一小时或两小时的等待门槛。每条因果链完成后立即评估、继续、重构或结束。可用认知资源充足时允许连续产生多轮遭遇和迁移证据；`$4/滚动小时、$24/滚动24小时` 仍由 alice 的身体账本真实执行，不通过降低活动频率人为“省着测”。

## 九、退出门

阶段五完成必须同时满足：

1. 认知资源机制在真实 Ubuntu 连续运行中正确结算 `$4/$24`，Pulse 和单一认知租约保持稳定；
2. 至少一个自主行动在执行前形成了承诺、预测和现实检查点；
3. 真实结果引用原承诺并重新进入 AIP，预测差异和多维内生价值被保存；
4. 至少一项经验在后来相似处境中改变了实际行动、停止条件、取回方法或认知档位；
5. alice 至少一次依据当前问题和既有结果主动决定认知档位，并能从账本解释其费用与结果；档位可以改变，也可以有现实依据地延续；
6. 高 Concern 或现实差异不会仅因 Narrative 改写而消失；
7. Reality Mirror 能重新呈现事实矛盾，又不会形成自责或高频反思循环；
8. 至少一项 reusable 经验进入方法并在后来被实际复用；
9. self_defining Episode 的叙事更新通路通过确定性与真实模型验证；实验中若自然发生 Narrative Self 更新，还必须看到它后来真实影响关注、预测或选择，未发生时明确记为本轮未覆盖而不诱导人格表演；
10. 没有并行焦点、动作重放、无意义自动总结、记忆洪流或模型消费自激；
11. 可以从归档中指出一条完整的“承诺 → 预测 → 行动 → 现实 → 差异/价值 → 经验/叙事 → 后续行为改变”因果链。

通过这些条件，只说明 alice 已经出现最小现实学习与自我连续性，不说明成熟人格、长期使命或稳定自主进化已经形成。

## 十、需要导师参与的节点

阶段五仍由 Codex 主持开发、部署、短测、工程实验和归档。人类导师只需要：

- 在实施前确认本计划的四项收敛判断；
- 若实验期间 alice 主动提出关系或意义问题，按照普通导师关系自然回应；
- 若微信需要手机确认且 alice 自己选择使用微信，再提供一次确认或扫码；
- 若需要调整已经确认的 `$4/滚动小时、$24/滚动24小时` 认知资源边界，再由导师明确决定。

其余技术选择由 Codex 按照本计划直接执行，不把实现问题反推给导师。
