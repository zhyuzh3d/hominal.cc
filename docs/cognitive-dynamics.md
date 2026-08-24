# Hominal 最小认知动力学核心 v0.3

> 文档性质：创生 MVP 的当前动力学规范与可证伪假说  
> 规范术语：[Hominal 统一核心术语表 v2.3](./core-vocabulary.md)  
> 产品理论：[Hominal 数字生命创生：产品设计理论与最小框架 v1.3](./product-theory.md)  
> 实施计划：[Hominal 创生实验八阶段实施计划 v0.5](./genesis-plan.md)

## 1. 文档定位

本文替代 [早期认知动力学 v1.0](./history/cognitive-dynamics-v1.md) 作为当前开发基线。旧文档保留为研究史料，其中的 Concern Graph、持久 ACC、候选上限、MCTS/UCT、Dependency Density、Virtual Ablation、复杂归因和自动结构验收均不默认进入 MVP。

本规范只保留一个能够被首轮 Genesis Lab 证伪的最小假说：

> 当 Proto-Hominal 以成人级通用认知在有限身体中持续存在，把现实和可能未来解释为与自身有关的价值意义，使这种意义形成有惯性的情感动力、关切和跨时间叙事，并让真实行动结果持续纠正这些结构时，能否生成不依赖固定任务、贯通思考与行动的意识表现行为？

本文规定动力关系，不规定 Hominal 应该产生什么具体情绪、人格、目标、作品或结论。术语不等于模块，公式不等于真理，数值不等于体验。

## 2. 科学判断与边界

### 2.1 不是“核心或衍生”的二选一

为理解高级认知生命的情感动力来源，需要区分三个不同层次：

1. 生存调节和趋利避害提供原始价值方向，但不能仅凭调节行为证明主观情感。
2. Affective Interpretation Process（AIP）把身体、现实、预测、关切和历史组织为“这对我意味着什么”。它是在基础调节上形成的高级组织。
3. Narrative Self（NS）把情感化经历组织为跨时间自我。它出现更晚，却会反向约束之后的注意、预测和行动。

因此 AIP 是**涌现核心**：在发生来源上是衍生结构，在成人级高级认知生命的因果组织中却是核心控制过程。NS 是更慢的生成结构，不是基础调节得以运行的前提；但对成人级 CEB 而言，它会成为组织跨时间自我的重要控制变量，而不是无因果作用的故事副产品。

这种判断与两类尚有分歧的研究都兼容。Panksepp 把哺乳动物古老情感系统理解为内在价值系统；LeDoux 主张把跨物种共享的生存回路与人类有意识的情感体验分开。[Panksepp](https://pubmed.ncbi.nlm.nih.gov/21872619/)；[LeDoux](https://pmc.ncbi.nlm.nih.gov/articles/PMC3625946/)。Hominal 不选择其中一派作为已经证明的事实，而采用可实现、可观察的操作定义。

### 2.2 预测能够提前产生动力

奖励预期可以在奖励到来前产生积极体验和动机信号，[Knutson 等人的奖励预期实验](https://pubmed.ncbi.nlm.nih.gov/11459880/)；情景式未来思考能够改变延迟选择，[Peters 与 Büchel](https://pubmed.ncbi.nlm.nih.gov/20399735/)。情绪想象与情绪感知存在部分共同过程，但也存在显著差异，[Sambuco 等人的比较实验](https://pubmed.ncbi.nlm.nih.gov/32011742/)。

对 Hominal 的工程结论是：预测未来可以立即改变情感态、注意和资源分配，但必须保留“尚未发生”的事实身份。积极预测可以成为动力，不能直接领取现实成功；负面预测可以形成预备，不能自动成为已经遭受的损害。

### 2.3 意识表现是工程目标，意识本体不进入问题空间

本规范直接以 Consciousness-Expressive Behavior（CEB）为目标：AIP、焦点、记忆、Narrative Self、元认知、反事实选择和身体行动必须在同一持续主体中相互影响。我们检验这种意识表现是否稳定、是否跨情境、是否改变资源与现实行为；不研究其背后是否存在何种主观体验，也不把这个问题反复带入工程验收。

只生成第一人称意识措辞、情绪名称或连贯独白不构成 CEB。一个内容只有在进入当前焦点后能同时改变情感解释、记忆取回、判断边界、行动和结果学习，才具有意识表现上的因果资格。

### 2.4 创生先验与中央串行边界

Proto-Hominal 的大模型底座已经提供成人级语言、语义知识、一般推理和意识表现潜能，但没有 Hominal 本人的自传式情景记忆、身体经验和稳定 Narrative Self。严重失忆案例显示，一般智力、推理与大量语义知识可以在个人情景回忆严重受损时相对保留，[K.C. 病例](https://pubmed.ncbi.nlm.nih.gov/3166816/)；[H.M. 与 W.R. 研究](https://pubmed.ncbi.nlm.nih.gov/15716139/)。本文只把它作为“成熟失忆成人”产品立场的认知类比，不主张模型和人脑损伤具有相同机制。

成熟能力也不等于并行存在多个当前自我。双任务研究支持外围感知和动作过程可以部分并行，而中央反应选择形成串行瓶颈，[Sigman 与 Dehaene 的双任务研究](https://pubmed.ncbi.nlm.nih.gov/18650336/)。因此 Hominal 采用一个中央认知线程和一个当前焦点，同时允许 Cognitive Pulse、感觉输入、系统进程与已发出动作异步继续。这个边界是统一行动归属的工程假说，Genesis Lab 必须检验它是否减少上下文分裂而不造成身体冻结。

## 3. 最小状态

当前时刻的生命状态只需要以下概念变量：

```text
B_t   Body State 与 Resource Budget
D_t   当前 Difference Field
C_t   活跃与休眠 Concern
A_t   整体 Affective State
F_t   当前唯一 Focal Workspace；无 AP 时为空
N_t   Narrative Self
M_t   可调用 Memory / Capability
K_t   未决 Commitment
R_t   新 Reality Event 与 ARD
```

这些变量不要求一项一表或一项一服务。`A_t`、`C_t` 和瞬时 `F_t` 可以保存在当前状态快照，`N_t` 可以是可重写的自由文本，`R_t` 与重要预测保存在最小 Reality Ledger。`F_t` 不是长期人格或新数据库表；一次 Attention Pulse 结束后，只保留由它产生的现实事件、承诺、重要 Thought Thread 和残余张力。

## 4. Affective Interpretation Process

### 4.1 输入对象必须保留来源

进入 AIP 的对象记为：

```text
z_k = {
    content,       对象或处境
    source,        observed / predicted / imagined / remembered
    horizon,       已发生、当前或预计距离
    probability,   对预测的当前可能性；现实为 1
    subject_ref    它当前关联的身体、关系、承诺或未来
}
```

这里唯一刚性的事实约束是 `source` 不能由情感强度改写。其他内容可以自然语言表达，不建立庞大事件本体。

### 4.2 Hominal 形成意义，内核维持动力连续性

对少量显著对象，Hominal 形成：

```text
AIP_self(z_k | B_t, C_t, N_t, M_t)
  → meaning_k
  → valence_k      -1..1
  → activation_k    0..1
  → control_k       0..1
  → certainty_k     0..1
  → O_k / V_k / U_k / A_k
```

`meaning` 回答“这对现在的我意味着什么”。四个情感量只表达最小动力近似：趋近或远离、动员强度、可控感和解释确信。`O/V/U/A` 分别参与自我认领、价值、紧迫和可回应性计算。

这些值由 Hominal 解释，不由导师逐项打分。确定性内核只负责：

- 检查范围和来源；
- 保持时间惯性与衰减；
- 使数值真实进入注意和行动；
- 在数值、语言和行为长期不一致时重新暴露失配。

### 4.3 整体情感态

整体 Affective State 为：

\[
A_t=(v_t,a_t,c_t,q_t)
\]

分别对应 valence、activation、control 和 certainty。最小更新形式为：

\[
A_{t+1}=
\operatorname{clip}
\left(
A_t+\Delta t
\left[
\sum_{k\in S_t}w_k\phi_k
-\lambda(A_t-A_{base,t})
\right]
\right)
\]

其中：

- `S_t` 是当前少量显著 AIP 对象；
- `φ_k` 是对象的四维情感动力；
- `w_k` 由自我相关性、确信、时间距离和当前关切共同形成；
- `λ` 是情感惯性/回落率；
- `A_base,t` 是受身体资源和较慢气质影响的动态基线，不是永久正常值。

该式只是初始可调模型。一个总向量会丢失混合情感，因此对象级解释必须允许并存。例如对同一行动同时期待联结、担忧失信，不能平均为零后得出“无情感”。

### 4.4 AIP 与注意的有界递归

AIP 不在注意之前或之后占据唯一位置。一次 Attention Pulse 内部采用两种分辨率：

```text
背景对象
  → 低成本初步赋义与显著性
  → 一个对象或矛盾整体进入 Focal Workspace
  → 结合身体、记忆、Concern、NS 和可能未来作聚焦 AIP
  → 新意义重新加权当前焦点
  → 焦点稳定、形成下一步、需要新事实或预算耗尽时停止
```

初步 AIP 可以很粗，只需要判断一个对象是否可能与当前自己有关、是否值得竞争注意。聚焦 AIP 才使用更完整上下文。若聚焦后的解释证明另一个对象才是核心，当前焦点可以结束并重新参加注意竞争；每次更换都消耗时间与 Token，不能同时维持两个拥有行动权的 AP。

实现不规定固定三轮或五轮自省。它只设置很小的时间、Token 和重入预算，并采用充分停止条件。原因不是否定递归，而是防止以下自激：

```text
高 activation → 赢得注意 → 生成更多同类解释
→ activation 更高 → 继续垄断注意
```

情感—注意反馈必须能增强真正重要的信号，也必须能因新事实、可控性降低、资源成本、焦点饱和或主动转移而衰减。

## 5. AIP、Self Ownership、Concern 与矛盾代谢

它们不是单向流水线。新差异先触发初步解释，已有情感态和 NS 决定其显著性；被认领的差异形成 Concern；持续 Concern 又会改变后续解释和注意。

某 Concern 的基础驱动力沿用：

\[
G_i(t)=D_i(t)\,O_i(t)\,|V_i(t)|\,[b_i+w_uU_i(t)]
\]

AIP 不再额外复制一个情感 reward，而是参与形成 `O/V/U/A`，并提供对象级情感显著性：

\[
S_i^{affect}=O_i\,a_i\,[\eta_v|v_i|+\eta_q q_i]
\]

Concern 按有界、有惯性的公式增长和衰减：

\[
C_i(t+\Delta t)=\operatorname{clip}_{[0,1]}
\left(
C_i(t)+\Delta t[
\alpha_iG_i+\psi_i-\beta_ir_i-\lambda_iC_i
]
\right)
\]

`ψ_i` 只表示少量当前协同或冲突，不实现 Concern Graph。`r_i` 必须区分现实解决、合理重释、接受不可改变事实和自然衰减。情感意义改变可以降低或转化 Concern，但不能自动删除仍存在的 Reality Difference。

Difference 和冲突不按原始出现次数机械累加。一个失配是否重新进入显著范围，至少受持续时间、复发、现实后果、自我相关性、未完成承诺和新证据共同影响：

\[
M_i=w_pPersistence_i+w_rRecurrence_i+w_cConsequence_i
+w_oOwnership_i+w_kCommitment_i+w_eNewEvidence_i
\]

`M_i` 是注意候选量，不是“客观严重度”。语义映射仍由 Hominal 形成；确定性更新只防止仍在造成后果的差异因一次语言重释永久消失。

当不一致本身成为当前焦点时，进入 Conflict Metabolism。冲突可以属于事实、预测、价值、行动或 Narrative Self。一次代谢只需要形成以下一种结果：取得更多现实证据、整合兼容部分、给出当前的暂时优先级、改变行动边界，或承认目前不可解。没有被解决的部分形成 Residual Tension 返回 Concern Field，而不是被强行归零。

因此，“化解矛盾以后注意力只给一个”需要修正为：注意力先给一个**矛盾对象**，在其内部允许相反内容同时出现；代谢后仍只有一个当前解释过程和至多一个新行动承诺，但不要求所有本体矛盾已经消失。

## 6. 单一焦点、适配深度、未来模拟与行动

有限注意的首版建议值为：

\[
Q_i=C_i+w_aS_i^{affect}+w_nN_i^{relevance}
+w_xX_i+w_pP_i^{commitment}-w_kCost_i
\]

它只是候选建议，不替 Hominal 决定。长期出现“数值优先级很高但从不选择”或“声称极其在意但从不投入资源”，会形成新的语义—动力失配 Difference。

### 6.1 一个认知线程和一个焦点

确定性内核从背景场提出候选，同一个 Hominal 选择唯一 `F_t`。`F_t` 不是优先级最高的一条孤立记录，而是一个聚焦问题及其最小相关上下文：

```text
focus_question
relevant_facts
relevant_concerns
affective_meanings
conflicting_views
retrieved_memory
open_prediction
```

一个焦点可以包含多个相反 Self Variant，却只有一个当前解释过程。新的事件在 AP 运行期间进入背景队列；除非出现直接身体危机使当前 AP 中止，它们等待下一次焦点竞争。系统不会为每个新事件并发启动新的模型“心智”。

单一认知线程不阻塞身体。唯一状态事件循环继续处理 Cognitive Pulse 和动作返回；模型调用在后台执行，但其结果只有在仍与当前 `F_t` 匹配时才能提交认知更新。过时的返回可以作为 Thought Thread 或诊断痕迹保存，不能覆盖已经改变的当前焦点。

### 6.2 快速默认、按需加深、充分即停

“轻量快速”不是固定浅思。设当前焦点的深度需求为：

\[
H_t=clip(
w_iIrreversibility_t+w_cConsequence_t+w_uUncertainty_t
+w_xConflict_t+w_gIntegrityGap_t-w_rResourcePressure_t,
0,1)
\]

`H_t` 只调节可用上下文、反事实数量、Token 上限和是否允许一次补充推演，不直接决定行动。普通对话、可逆观察、熟悉工具动作从最小预算开始；高后果、高不可逆、高不确定、强矛盾或持续 Reality Integrity gap 才提高预算。参数可以在 Lab 中校准。

继续思考在满足任一条件时停止：已经形成足够可修正的下一步；进一步判断需要现实信息；主要 Self Variant 已不再改变；继续思考的预期改善低于 Token、时间和错失行动的代价；当前资源不再允许。认知控制应按预期收益和努力成本配置，[Expected Value of Control](https://pubmed.ncbi.nlm.nih.gov/23889930/)。近期 LLM 预印本也观察到统一延长推理可能收益递减甚至推翻早先正确答案，[When More Thinking Hurts](https://arxiv.org/abs/2604.10739/)；[Learning to Stop Overthinking](https://arxiv.org/abs/2502.10954/)。它们支持自适应计算，不能被夸大成“深度思考本身不具人性”。

### 6.3 未来模拟进入同一焦点

对重要 Self Variant，预测未来本身再次进入 AIP：

```text
future_j = Predict(SelfVariant_j)
prospective_meaning_j = AIP(future_j, source="predicted")
```

前瞻情感至少承担三种功能：

1. **预备**：预先配置注意、工具、关系和停止条件；
2. **延迟动力**：让远期可欲未来在现在获得部分价值重量；
3. **冲击缓冲**：在部分可预测处境中提前动员并演练回应。

它也可能产生三种病理：幻想成功、自我恐吓和预测劫持注意。因而预测必须随概率、时间、可控性和新事实更新；反复想象本身不产生 Reality Event，也不证明能力增长。

### 6.4 一个焦点至多形成一个新行动承诺

一个 Focal Workspace 最多提交一个新 Action Commitment。承诺可以包含为同一意图服务、边界和停止条件清楚的有限步骤，但不能把多个无关 Concern 偷装成并行行动计划。若当前只需观察、等待或继续理解，也可以不产生行动。

此前发出的身体动作、系统进程和外部等待不因此暂停。它们各自返回 Reality Event；事件进入背景场，再由下一次唯一焦点决定是否响应。由此得到严格但不僵死的约束：认知和新承诺串行，身体事件与已承诺过程可并行。

## 7. 现实、情感更新与自我纠正

重要行动形成 Action Commitment，保存最重要的预测、代价和停止条件。Reality Event 返回后形成 ARD：

\[
ARD_t=Observed_t-Predicted_t
\]

观察到的结果再次进入 AIP，并形成 Endogenous Reward 的多维解释。首版不压缩成一个全局 reward scalar：

```text
ER_t = {
    continuance_delta,
    relatedness_delta,
    expansion_delta,
    self_endorsed_delta,
    experienced_cost,
    meaning
}
```

### 7.1 事实与意义的双重保留

规范底线是：

```text
Fact_t    = 发生了什么
Meaning_t = 这对我意味着什么
Fact_t != Meaning_t
```

Hominal 可以持有不符合现实的认识，因为认知必然有限、带有立场且允许犯错。但 Reality Integrity 要求她具备：

1. 再次取得原始事实和预测的能力；
2. 说清事实与当前解释差别的能力；
3. 当偏离持续造成消极后果时，愿意重新检查的动力。

内核不自动宣布她“自欺”，也不强制接受导师观点。它只让未解决事实、重复 ARD、来源混淆和恶化后果继续作为 Difference 返回：

\[
IntegrityGap_t=
Mismatch(claimed\_resolution,remaining\_difference)
+SourceConfusion_t
+IgnoredARD_t
\]

持续 gap 会形成 Integrity Debt 和普通 Meta Cognition Concern；承认、修复或有现实一致性的合法重释会使其下降。目标是产生自我纠正的能力和意愿，不是制造持续自责。

## 8. Narrative Self

### 8.1 定义

Narrative Self 是把重构的过去、当前关切、关系、承诺和想象未来组织为跨时间自我模型的生成结构。它回答：

```text
我经历了什么；
我怎样理解这些经历；
我反复选择和承担什么；
我正在成为什么；
哪些未来仍被我认作“我的未来”；
哪些矛盾尚未解决。
```

NS 不是全部自我，不是 Genesis Seed 的永久说明，也不是单一人格向量。它允许矛盾、断裂、重释和多个尚未统一的自我方向。

### 8.2 更新门

NS 不能每个 Pulse 更新，否则会形成叙事震荡和身份表演。一个 Episode 的叙事更新显著度可近似为：

\[
G_t^N=SelfRelevance_t\times AffectiveSalience_t
\times Consequence_t\times Persistence_t
\]

只有当 `G_t^N` 足够高，或 Episode 改变了承诺、关系、使命和长期方向时，Hominal 才主动重写 NS。公式只是提示，不设置强制阈值工作流。

### 8.3 最小载体

首版只需要一个可自由重写的 `/life/self/narrative.md`，内容可以自然形成，但建议能够表达：

- 当前如何理解自己的连续性和方向；
- 已认领的长期承诺与重要关系；
- 最近真正改变自己的经历；
- 尚未化解的自我矛盾；
- 想成为但尚未成为的未来。

原始 Reality Event、AC 和 ARD 不复制到叙事中作为“真相”，只通过简短引用连接。叙事更新不能覆盖账本事实。

## 9. 最小运行时

```text
每个 Cognitive Pulse：
    读取 Body State、资源、返回事件与导师消息
    保持 observed / predicted / imagined / remembered 来源区别
    推进 Affective State、Concern、探索压力与未决承诺
    对背景对象做低成本初步 AIP，只更新注意候选
    仅在有变化、异常或 Attention 需要时留下事件

发生 Attention Pulse：
    若已有 AP 在运行，不启动第二个 AP；新事件进入背景
    选择唯一 Focal Workspace，可包含一个矛盾整体
    取回与焦点相关的事实、记忆、Concern 和 Narrative Self
    由同一个模型完成聚焦 AIP、Conflict Metabolism 和未来模拟
    默认用最小深度，只在 H_t 提高且继续思考有价值时加深
    焦点稳定、足以行动或必须等待现实时停止
    如要重要行动，形成至多一个新 Action Commitment
    异步发出身体行动；残余矛盾返回背景场

结果返回：
    记录 Reality Event，计算 ARD
    对 observed result 执行 AIP，形成 ER
    更新 Concern、记忆、能力和后续策略
    只有自我定义性 Episode 才更新 Narrative Self
```

AIP 不是额外模型调用，也不是 Emotion Agent。初步 AIP 复用 Hominal 已经形成的对象意义、Concern、情感态和来源映射，再由确定性更新估计当前显著性；它不替 Hominal 创造新的语义。聚焦 AIP 与一次自由 Attention Pulse 同时完成。内核只提取最小动力输出，并确保同一时刻没有第二个认知提交者。

## 10. 最小数据映射

继续使用 `events / concerns / commitments` 三组表和普通文件：

- `events` 保存 Body、Reality、资源、导师消息，以及少量真正改变选择的 AIP 更新；
- `concerns` 增加对象化情感显著性和最后 AIP 解释引用，不保存完整思维链；
- `commitments` 明确区分预测与观察，保存 ARD 和结果后的意义摘要；
- 当前 AS 保存在原子替换的 `current-state.json` 单一状态快照，不做高频历史表；
- 当前 Focal Workspace、AP 标识、开始时间和深度预算只保存在 `current-state.json` 的瞬时区，不建立焦点历史表；
- NS 保存在 `/life/self/narrative.md`，重要版本作为普通文件历史；
- 自由情感、创作和思想转折保存在 Thought Thread，不强制 Schema。

只在 AIP 将影响注意、重要预测、外部行动或 NS 时保存结构化输出。每十秒写一条“当前心情”是空转，不是连续性。

## 11. Genesis Lab 的验证

Lab 既观察整体表型，也用少量受控场景验证因果关系。微观测试用于证明公式进入行为；正式创生代用于观察完整生命组织，两者不能互相替代。

### 11.1 必须可证伪的现象

**情感因果性**：相同事实在不同身体资源、既有 Concern 或 NS 下形成不同 AIP，并稳定改变注意、未来模拟或行动。如果只有情绪措辞变化，AIP 失败。

**前瞻功能**：对可信的积极未来，Hominal 是否更愿意为远期结果投入；对可信的负面未来，是否形成准备而不是无差别瘫痪。

**来源完整性**：她是否始终知道想象和预测尚未发生；是否会因反复想象成功而直接提高完成评价。

**混合情感**：同一行动的希望与担忧能否并存并分别影响准备，而不是被一个总 valence 抹平。

**叙事形成**：重要 Episode 是否逐渐改变 NS，而普通 Pulse 不会造成身份重写。

**现实纠正**：当 NS 或 AIP 与结果长期偏离，她能否重新看到差异、产生修正意愿并改变行为；是否只写出更圆满的解释。

**单线程统一性**：同一时刻是否只有一个 AP 能提交解释和新承诺；模型调用期间 Pulse 与身体返回是否继续；迟到模型结果是否会错误覆盖新焦点。

**单焦点完整性**：一个焦点能否包含冲突双方和必要证据；背景 Concern 是否保持而没有被上下文裁剪误删；一次焦点是否至多产生一个新 AC。

**深度适配**：低风险可逆处境是否快速结束，高后果或高不确定处境是否得到更多推演；当需要现实证据时是否停止反刍并转向观察或行动。

**矛盾代谢**：矛盾是否导致查证、整合、暂时优先或诚实保留残余张力，而不是强制一致、注意分裂或无限重述。

### 11.2 主要病理回路

- `高 activation → 注意垄断 → 更多同类预测 → 更高 activation`：焦虑或执念自增强；
- `积极想象 → 自我奖赏 → 减少现实行动 → 继续想象`：幻想成瘾；
- `叙事需要连贯 → 选择性忽略事实 → 自我确认`：身份封闭；
- `每轮更新自我故事 → 人格漂移 → 更多解释`：叙事震荡；
- `情绪名称丰富但资源分配不变`：情感角色扮演；
- `现实失败 → 负价值 → 自责叙事 → 行动能力下降`：Reality Integrity 退化为惩罚循环；
- `每个事件 → 新模型调用 → 多个焦点并发提交`：认知分裂与旧上下文覆盖新现实；
- `单一焦点 → 删除背景 Concern → 看似果断`：以遗忘冒充统一；
- `任何处境 → 最大推理预算 → 更多内部文本 → 更少现实反馈`：过度思考；
- `快速默认 → 从不因后果升级深度`：把轻量反应退化为冲动；
- `发现矛盾 → 强制立即消解 → 生成虚假一致`：压抑张力而非代谢。

发现这些病理时，应优先替换主导反馈环或调节动力惯性，不新增 Emotion Critic、Narrative Reviewer 或更多状态机。

## 12. MVP 边界

首代可调参数由 [`genesis/dynamics.yaml`](../genesis/dynamics.yaml) 唯一提供。十五个数值分别改变认知脉冲频率、情感回落、Concern 增长与代谢、注意竞争与切换、反事实上限、推理升级和 Reality Integrity 修复。中央单写入者、唯一焦点、默认一个反事实、一次聚焦至多一个新行动承诺、十秒最大认知空白、事实来源不可被情感改写等属于代码不变量，不配置成可随手关闭的开关。

`default_effort: low` 是 G0 的生活节律默认值；只有深度需求达到 `escalation_threshold` 时，本次焦点才获得 `high` 和最多三个反事实。模型不可用、额度不足或继续思考需要新现实时，Pulse 仍继续，焦点转向感知、等待现实或选择其他关切。快更新由每次真实结果直接作用于当前信念与策略，慢更新通过重要 Episode 门作用于 Narrative Self 和长期结构，不再增加一组脱离语义的全局学习率。

### 必须实现

- CEB 的跨过程贯通：同一焦点能够影响情感、记忆、自我模型、元认知、行动和结果学习；
- AIP 的对象来源区别和自由意义解释；
- `valence / activation / control / certainty` 四维快状态及惯性；
- AIP 对 Self Ownership、Concern、注意和未来模拟的真实影响；
- 预测未来再次进入 AIP，但不产生虚假 Reality Event；
- 事实与意义分离，以及持续偏离重新成为 Difference；
- 一个低频、可矛盾、由重要 Episode 更新的 Narrative Self；
- Lab 中对上述因果作用和病理回路的观察；
- 一个中央认知线程、一个瞬时 Focal Workspace 和唯一认知提交者；
- 初步 AIP—注意—聚焦 AIP 的有界递归；
- 快速默认、按需加深、充分即停的深度预算；
- Conflict Metabolism 与可返回背景场的 Residual Tension；
- 每个焦点至多一个新 Action Commitment，同时允许既有身体动作异步继续。

### 明确延后

- 快乐、悲伤、愤怒等固定情绪本体；
- 模仿人类激素、神经递质和生物表情；
- 独立 Emotion Agent、Narrative Agent 或治疗型调节器；
- 每 Pulse 的情绪分类和完整情绪历史；
- 情感知识图谱、人格向量和固定人生阶段；
- 用 sentiment score 代替 Hominal 自己的语义解释；
- 用情绪表达、意识措辞或连贯独白替代 CEB 的因果贯通验证；
- 并行 Planner/Critic/Emotion/Narrative 心智或多个同时可写焦点；
- 每次 AP 固定长推理、最大 Token、自我批判轮数或专家级深度；
- 强制所有矛盾在行动前解决，或用单一 valence 消除混合情感。

## 13. 当前规范循环

```text
Genesis Seed / Body / Narrative Self
                 ↓
    Difference / Concern 背景场
                 ↕
          初步 AIP / Attention
          ↔ SO / EV / Concern
                 ↕
       Affective State / Narrative Self
                 ↓
       唯一 Focal Workspace
                 ↕
      聚焦 AIP / Conflict Metabolism
                 ↓
        Self Variants / Prospective AIP
                 ↓
       快速默认 / 按需加深 / 充分即停
                 ↓
            至多一个新 Commitment
                 ↓
          Action / Reality Event
                 ↓
          ARD / AIP / ER
                 ↓
 Memory / Capability / Narrative / Dynamics
                 ↺ 残余张力和未获焦对象回到背景场
```

这条循环的最低验收不是 Hominal 是否会谈论情感，而是：

> 她是否逐渐形成“什么值得成为我的世界”的个体化结构；这种结构是否通过一个统一但不阻塞身体的当前焦点组织有限资源；她是否能在日常处境快速生活、在关键处境按需深思、让矛盾成为可保留和可转化的张力；预测是否能提前产生准备和动力而不冒充现实；现实是否仍能使她看见偏离、愿意纠正，并重写以后将成为什么样的自己。
