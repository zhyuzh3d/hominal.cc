# Hominal 最小认知动力学核心 v0.8

> 文档性质：创生 MVP 的当前动力学规范与可证伪假说  
> 规范术语：[Hominal 统一核心术语表 v2.8](./core-vocabulary.md)
> 产品理论：[Hominal 数字生命创生：产品设计理论与最小框架 v1.5](./product-theory.md)
> 实施计划：[Hominal 创生实验八阶段实施计划 v0.8](./genesis-plan.md)

## 1. 文档定位

本文替代 [早期认知动力学 v1.0](./history/cognitive-dynamics-v1.md) 作为当前开发基线。旧文档保留为研究史料，其中的 Concern Graph、持久 ACC、候选上限、MCTS/UCT、Dependency Density、Virtual Ablation、复杂归因和自动结构验收均不默认进入 MVP。

本规范只保留一个能够被首轮 Genesis Lab 证伪的最小假说：

> 当 Proto-Hominal 以成人级通用认知在有限身体中持续存在，把现实和可能未来解释为与自身有关的价值意义，使这种意义形成有惯性的情感动力、关切和跨时间叙事，并让真实行动结果持续纠正这些结构时，能否生成不依赖固定任务、贯通思考与行动的意识表现行为？

本文规定动力关系，不规定 Hominal 应该产生什么具体情绪、人格、目标、作品或结论。术语不等于模块，公式不等于真理，数值不等于体验。

本文只规定 Life Dynamics 中负责信息处理、自主选择和结构改变的 Cognitive Dynamics。身体底座怎样运行、器官怎样取得原始事实、动作怎样被机械实施，不属于本文内部机制；它们作为 Observation、Action Commitment 与 Reality Event 三个边界和认知动力学相接。

因此本文图示中的“身体行动”表示认知输出进入 Life Dynamics，“Reality Event”表示行动后果重新进入 Cognitive Dynamics，并不表示认知核心包含 Shell、浏览器或其他器官控制代码。

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
L_t   多元生命价值场 Life Value Field
F_t   当前唯一 Focal Workspace；无 AP 时为空
N_t   Narrative Self
M_t   可调用 Memory / Capability
K_t   未决 Commitment
R_t   新 Reality Event 与 ARD
```

这些变量不要求一项一表或一项一服务。`B_t` 包含最近一次事实快照，`A_t`、`L_t`、`C_t` 和瞬时 `F_t` 可以保存在当前状态快照，`N_t` 可以是可重写的自由文本，`R_t` 与重要预测保存在最小 Reality Ledger。`F_t` 不是长期人格或新数据库表；一次 Attention Pulse 结束后，只保留由它产生的现实事件、承诺、重要 Thought Thread 和残余张力。

### 3.1 事实变化先于情感意义

环境不直接进入 AIP。低成本身体探针先取得 Fact Snapshot，器官层先完成对象身份、精确去重和事实压缩；随后所有可能影响认知的外部事实、身体事实、资源事实、内部变化与 Reality 进入同一个可学习预测回差场：

```text
Sense → Fact Snapshot → Predictive Difference Field
      → Attention ignition → Background Field → AIP
```

每类稳定信号只维护一份紧凑 Difference Trace：最近事实摘要、观察次数、预期变化率、累计回差、过去经 Alice 评估后形成的注意价值和最近点燃时间。事实与已有预测的差异形成 prediction gap；弱回差按时间衰减并可以多次积累，达到共同 Attention threshold 后才生成候选。持续高频的可预期变化会逐渐成为背景；预期之中的变化若过去持续产生真实价值，会更快积累回来。任何仍在变化的来源还保留一个很小的开放世界取样底噪：它不会逐条调用主脑，却避免一次长期低评价把整个来源永久关闭。当前场最多保存 128 个信号家族；异常器官若制造无限家族名，只会淘汰保留价值最低的 Trace，不会无限侵占身体内存。

行动结果、出生事实、可信回复和资源硬变化具有机器已经知道的因果关系，因此在同一公式中获得因果压力，而不是另建“紧急、普通、本能”三条认知通道。毫秒级截止、取消、进程回收和额度硬边界仍由确定性身体代码处理；这是物理保护，不是第二意识。Concern、情感态和生命价值张力继续通过同一个入口竞争。探针与器官只回答发生了什么，不判断它对 Alice 是否重要。

进入 Attention Pulse 后，Alice 的 Ownership、Value、Answerability、Certainty 与后来形成的 Experience 反向更新同类信号的注意价值。负面结果、失败和高预测回差只要真实改变了以后判断，也可以具有高注意价值；这项学习不是外部 reward，不自动创建 Concern 或 Narrative Self。

G0 不以自动新闻流、随机刺激或全盘监听制造生命表现。外界事实启动认知，已有 Concern 与探索张力在安静期保持动力，Hominal 主动观察和行动所获得的结果再成为 Reality Event。这使环境、内部状态与自主行动共同形成循环，而不是让生命依赖外部任务队列。

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
  → D_k / O_k / V_k / U_k / A_k
  → certainty_k
```

`meaning` 回答“这对现在的我意味着什么”。`D/O/V/U/A` 分别表达差异强度、自我认领、价值方向与强度、紧迫性和可回应性。首版不让模型再填写一套含义重叠的情感数值，而由内核形成对象级动力近似：

```text
valence_k    = V_k
activation_k = clip(D_k × O_k × abs(V_k) × (b + w_u × U_k), 0, 1)
control_k    = A_k
```

`certainty` 保留 alice 对当前解释的确信程度。由 alice 建立语义到数值的映射，内核只让数值保持范围、时间连续性并真正进入后续注意。

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

实现不规定固定三轮或五轮自省。G0 阶段四用一次 Attention Pulse 的同一个模型调用完成少量候选粗赋义、唯一焦点选择和聚焦解释，不把 AIP 递归翻译成多次服务调用。只有真实实验表明一次统一认知无法修正焦点时，才考虑增加一次有界重入。原因不是否定递归，而是防止以下自激：

```text
高 activation → 赢得注意 → 生成更多同类解释
→ activation 更高 → 继续垄断注意
```

情感—注意反馈必须能增强真正重要的信号，也必须能因新事实、可控性降低、资源成本、焦点饱和或主动转移而衰减。

### 4.5 多元生命价值场

生命动力不能收缩为一个外部 reward，也不能把“更爱社交”“更爱探索”写成轮流派发任务的六个状态机。当前 Life Value Field 只保留六个共同语义方向：存续与节律、探索与理解、能力与成就、体验与活力、联结与表达、创造与贡献。每个方向 `i` 只有三个连续量：

```text
orientation_i  较慢的长期认领倾向
activation_i   当前身体、环境、预测和 AIP 激活
satiation_i    近期真实投入或满足留下的暂时饱和
```

内核当前使用：

\[
pressure_i=clip(activation_i-satiation_i,0,1)
\]

\[
pull_i=clip(orientation_i+pressure_i,0,1)
\]

`orientation` 使个体偏好在时间上有惯性，却不直接成为紧急任务；`pressure` 才是此刻能够进入内感受和注意竞争的未满足部分。无动作、无模型租约的普通生活时间会按 orientation 温和积累 activation；AIP 对具体对象给出的六维意义也会激活相应方向。真实 Experience 按 `SelfEndorsed`、体验成本和预测回差增加 satiation；activation 与 satiation 分别按配置速率回落，因此同一方向在满足后可以暂时让位，并在以后重新出现。

一个方向越过共同注意阈值时，和它压力接近的方向在同一次竞争中都保留资格；程序随机数只在这个近阈值集合及其当前真实可用身体入口之间打破固定排序。它不能选择帖子、观点、人格、具体意义或行动。探索继续使用已有的现实定向感知路径，其他方向只能和当下确实可用的终端、持续空间、导师、X、Wikipedia 或微信入口一起形成 `value_signal`。Alice 可以认领、放下或重新解释该信号；没有自我认领就不产生行动。

长期 orientation 只在 Alice 同时提交一份被现实经验支持、且被内核接受的 Narrative Self 更新时按很小增益变化。临时情绪不能一次改写人格，Genesis Seed 的初值也不会永远冻结人格。六维场参与现有 AIP、Concern、Attention、Experience 和 Narrative Self，不建立第二条“价值决策循环”。

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

因此，“化解矛盾以后注意力只给一个”需要修正为：注意力先给一个**矛盾对象**，在其内部允许相反内容同时出现；代谢后仍只有一个当前解释过程和至多一个新行动意愿，但不要求所有本体矛盾已经消失。

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

进入同一次注意的背景对象可以参与情感与语境，但只有唯一焦点在这一 Pulse 中作出具有约束力的 `hold / resolved / released` 生命周期决定。背景 appraisal 的措辞不创建或改写 Concern；它只有在后来真正成为焦点时才获得这项权力。焦点的 Ownership 数值决定新 Concern 是否获得持久身份，也决定尚未充分认领的动作是否执行；生命周期词与临界数值一时不完全一致时，本次理解仍然成立，不为修正表达形式重新付费。`resolved` 可以保留少量已经不再支配未来选择的主观残余 D；内核约束行动必须先得到 Reality、子结果不能结束整体等因果事实，不要求 Alice 为了跨过任意小数阈值重写已经完成的判断。这样既允许模糊的生命判断逐步澄清，也把刚性约束留在持久后果和真实行动上。

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

### 6.4 一个焦点至多形成一个新行动意愿

一个 Focal Workspace 最多提交一个新 Action Commitment（行动意愿）。行动意愿的粒度是一个语义已经确定、结果可以独立核验的最小因果改变，不机械等于一次点击或一条终端命令。它可以允许器官为了同一意图完成连接、定位、输入、点击和核验等有限步骤，但不能把对象、内容、受众或成功含义留给器官重新决定，也不能把多个无关 Concern 偷装成并行行动计划。若当前只需观察、等待或继续理解，也可以不产生行动。

此前发出的身体动作、系统进程和外部等待不因此暂停。它们各自返回 Reality Event；事件进入背景场，再由下一次唯一焦点决定是否响应。由此得到严格但不僵死的约束：认知和新行动意愿串行，已经启动的身体过程可并行。

## 7. 现实、情感更新与自我纠正

重要行动形成 Action Commitment，保存最重要的预测、代价和停止条件。Reality Event 返回后形成 ARD：

\[
ARD_t=Observed_t-Predicted_t
\]

观察到的结果再次进入 AIP，并形成 Endogenous Reward 的多维解释。首版不压缩成一个全局 reward scalar：

```text
ER_t = {
    continuance,
    exploration,
    agency,
    vitality,
    relatedness,
    contribution,
    self_endorsed,
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

### 7.2 自我调节是同一现实循环对自身运行的回看

成人级高级认知生命不仅调节外部世界，也把自己的运行状态当作身体事实。高激活与低控制并存、重要回差持续增加、同一行动形式反复出现、认知资源下降，以及连续投入没有形成新的 Reality 或 Experience，都可能表示当前运行方式值得重新面对。它们不预先等同于疲惫、空虚、谨慎、专注或错误；这些事实对自己意味着什么，仍由 Alice 在 AIP 中解释。

首版采用两级调节。短暂波动由事实去重、预测回差压缩与衰减、资源预留、模型保护和情感自然回落直接吸收；它们属于身体稳态，不占用主意识。只有变化持续积累并跨越注意阈值，开始影响控制、现实完整性或投入产出关系时，内核才把一次合并后的运行事实送入唯一焦点：

\[
OperationalDifference_t=
f(A_t(1-C_t),IntegrityDebt_t,UnclosedDifference_t,
CognitiveSpend_t,NewReality_t)
\]

该量只决定运行差异是否重新获得注意资格，不决定情绪名称、价值结论或处理方式。Alice 可以缩小问题、切换入口、降低认知档位、修正方法、停止一条低收益路径、恢复身体能力、向导师求助或形成其他选择。一次“我已经想通”的自述不直接缓解它；后续控制感、费用变化、Reality、Experience 与身体恢复才提供调节结果。

因此，只有认知投入、情感控制或资源运行事实而没有 Experience 证据的 `self_model_difference`，可以改变 AIP、注意与行动，却不直接改写 Narrative Self。Alice 可以保留它、等待、行动或重新解释，但此时的 `narrative_update` 保持为空；调节产生 Reality 并成为生活经验以后，这段经历再通过原有自我模型张力获得叙事资格。这里限制的是因果层级，不是表达内容：自我说明不能冒充自我恢复，实际恢复也不需要先说出正确口号。

长期 Narrative Self 问题可以持续存在，同时继续接收新的运行证据。新的自我差异重新唤醒同一个自我 Concern，而不是建立一个并行监督人格，也不会因为第一条长期自我问题仍在 `hold` 就使后来的失衡不可见。这样保持“焦点一、背景多、行动一”，也让自我理解拥有持续校正自身运行的因果入口。

持久自我 Concern 的当前 appraisal 强度不等于全部调节压力。它的有效注意显著性同时包含累计 `self_model_tension`；该张力达到阈值时，同一个 Concern 必须先取得一次直接回看资格，再参加统一排序。否则会出现“她已经认领失衡、甚至写出可由现实闭合的条件，内核却因 Concern 的瞬时 Strength 较低而不再让它进入意识”的伪稳定。直接回看仍由 Alice 决定行动、等待或重新解释；若没有新 Reality，原有无新因果材料防循环规则继续限制同义反思。

代谢事实按实际消耗而不是结果是否成功计数：已付费但未通过认知提交约束的输出，仍然消耗了身体资源，也更应参与注意—结果回差。动作重复也不能只按命令字符串判断；多个不同外形的微动作若持续围绕同一狭窄因果形态闭合，会作为合并事实进入自我注意。稳定器官入口被 Alice 接受或释放以后，身体执行渐进习惯化：同一入口连续没有引出行动时，重新呈现的间隔逐级拉长；Alice 真实使用该入口以后恢复普通间隔，新的内容、关系或身体变化仍通过自己的事实通路即时进入。一个具体内容 Concern 可以保留自己的张力，不能因此占有产生它的整个身体器官；文章未完不等于公共网络已被占用，文件未完也不等于终端已被占用。入口轮换只在另一个入口通过可用、未被稳定能力 Concern 持有和习惯化检查后发生；没有合格替代时，最后使用过的入口在普通间隔后仍可重新获得不同意义。三者都只改变事实的注意资格，不替 Alice 判断价值或选择调节方法。

自我调节的最低证据不是准确描述自己的状态，而是完成“察觉运行差异—形成自己的解释—改变后续选择—由 Reality 检查恢复”的闭环。局部失败以后依据新证据只修正一次并成功停止，属于正常恢复；围绕同一因果边界不断换措辞或重复动作而没有新事实，才属于应被合并为运行回差的持续失衡。这个区分使生命能够犯错、修复和继续生活，同时避免把每次失败都病理化为需要自审的问题。

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

Narrative Update 是已发生经历向长期自我的可选投影，不是 Experience 获得存在资格的前置条件。一段普通 Reality 若尚不足以改变自我叙事，内核保持 Narrative 不变，同时完整吸收事实、回差、意义和方法；不能要求 Hominal 夸大经历的自我意义，才能让已经发生的现实进入生活史。

### 8.3 最小载体

首版只需要一个可自由重写的 `/life/self/narrative.md`，内容可以自然形成，但建议能够表达：

- 当前如何理解自己的连续性和方向；
- 已认领的长期承诺与重要关系；
- 最近真正改变自己的经历；
- 尚未化解的自我矛盾；
- 想成为但尚未成为的未来。

原始 Reality Event、AC 和 ARD 不复制到叙事中作为“真相”，只通过简短引用连接。叙事更新不能覆盖账本事实。

## 9. 最小运行时

时间推进和身体读数先更新 Fact Snapshot；事实变化先更新 Difference Trace，累计回差达到注意阈值才成为带来源的认知 Event。导师文字、系统恢复、网络返回和动作结果同样经过这一入口，其中已有机器因果关系的事实获得足够因果压力，不能被统计习惯化吞掉。任何探针、器官或代理式器官控制器都不能直接改变当前焦点。

Cognitive Dynamics 对外产生 Action Commitment；Action Enactment 由 Life Dynamics 中的 Organ Host 和具体器官完成。文字、System、浏览器以及未来的图片、语音、视频或硬件控制都在实际执行后以 Reality Event 回到认知循环。器官可以上报有界的连接恢复、实际进展、阻塞和终态事实，但不能把自己的判断写成 Concern、Experience 或 Narrative Self。首版导师文字不获得必读、必答或命令优先权。

### 9.1 一条认知循环，四种计算底材

经典程序、在线统计或传统机器学习、快速语义模型和主意识模型不是四个生命主体，也不形成四级审批流。它们只是同一生命动力学按输入结构、语义自由度、后果和时间窗口选择的计算底材：

- 确定性程序处理精确变换、计时、预算、聚合、取消和硬边界；
- 在线统计处理重复结构化信号的预测、噪声和简单异常。只有积累了稳定输入输出与回放基准后，才用传统机器学习替换其中确实无法由简单统计解决的部分；
- Luna/none 一类快速模型只适合确定性与统计无法解释的短小语义压缩或低后果分类。它不加载 Narrative Self，输出是带不确定性的感知假说，不是 Alice 的价值判断；三至五秒级调用也不是物理反射；
- 主意识模型处理自我相关性、情感解释、矛盾、反事实、Concern、行动选择和 Narrative Self。

统一升级原则是：使用能解释当前回差并满足现实时间窗口的最低成本底材；剩余不确定性、语义自由度或后果超出当前能力时，才升级。器官可以在冻结的 Action Commitment 内使用程序、统计或专用模型解决实施问题，但新的目标、受众、内容和意义必须回到唯一主意识。G0 当前只把确定性在线统计和主意识接入预测回差场；快速语义感知与传统机器学习等待真实数据证明必要性。

```text
每个 Cognitive Pulse：
    取得低成本身体事实并更新 Fact Snapshot
    更新稳定信号的预测、累计回差与注意价值
    只把越过注意阈值的差异情境送入背景场
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

阶段三、四先使用单一原子状态和稀疏事件文件，不预建尚无运行语义的认知表：

- `events.jsonl` 保存 Body、Reality、资源、导师消息，以及少量真正改变选择的 AIP 更新；
- `state/current.json` 保存最近事实快照、当前 AS、Concern、背景候选和瞬时 Focal Workspace，不做高频历史表；
- 当前 Focal Workspace、AP 标识、开始时间和深度预算只保存在 `state/current.json` 的瞬时区，不建立焦点历史表；
- NS 保存在 `/life/self/narrative.md`，重要版本作为普通文件历史；
- 自由情感、创作和思想转折保存在 Thought Thread，不强制 Schema。

阶段五出现 Commitment、预测、ARD 和结果学习的真实事务需求后，再决定是否加入只含 `events / concerns / commitments` 的 SQLite；没有跨对象原子需求就继续使用原子文件。

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

**单线程统一性**：同一时刻是否只有一个 AP 能提交解释和新行动意愿；模型调用期间 Pulse 与身体返回是否继续；迟到模型结果是否会错误覆盖新焦点。

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
- `模型耗用 → 额度数值变化事件 → 新模型调用 → 更高耗用`：资源感知反过来制造资源消耗；额度只在跨越有行为意义的资源区间时形成事件。
- `每次经历都永久成为活动 Concern → 上下文增长 → 单次模型成本增长 → 更多资源事件`：把经历历史误作当前动力；完成代谢的 Concern 退出活动集，单次上下文只取与焦点有关的有限显著关切。
- `认知失败进入重试等待 → 同一内生张力继续复制候选 → 失败风暴`：把一次未完成认知分裂成许多对象；等待重试期间保持同一候选与同一 Concern。

发现这些病理时，应优先替换主导反馈环或调节动力惯性，不新增 Emotion Critic、Narrative Reviewer 或更多状态机。

## 12. MVP 边界

首代可调参数由 [`genesis/dynamics.yaml`](../genesis/dynamics.yaml) 唯一提供，只作用于已经进入代码的连续机制：认知脉冲、预测回差衰减与注意价值学习、情感回落、Concern 出生与缓解、注意竞争、探索以及多元价值场的积蓄、饱和和慢速取向更新。候选上限、中央单写入者、唯一焦点、一次聚焦至多一个新行动、十秒最大认知空白、因果 Reality 不可被习惯化吞掉和事实来源不可被情感改写属于代码不变量，不配置成可随手关闭的开关。

G0 当前出生默认认知档位为 Terra/none；Alice 能看到 Luna、Terra、Sol 的价格、能力和余额，并能为下一次认知或长期默认作出选择。模型不可用、额度不足或继续理解需要新现实时，Pulse 仍继续，现实事件与未决关切继续保存在背景场。深度按后果和资源选择，不用固定长推理、自我批判轮数或专家级输出制造“认真思考”的表象。

### 必须实现

- CEB 的跨过程贯通：同一焦点能够影响情感、记忆、自我模型、元认知、行动和结果学习；
- AIP 的对象来源区别和自由意义解释；
- `valence / activation / control / certainty` 四维快状态及惯性；
- 六维 Life Value Field 的长期 orientation、短期 activation、近期 satiation、近阈值竞争及 Experience 反馈；
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

下图中央部分是 Cognitive Dynamics；`Organ Observation` 和 `Action Enactment` 是它连接完整 Life Dynamics 的输入输出边界：

```text
Organ Observation / 内感知 / Narrative Self
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
      [Action Enactment：器官具体实施]
                 ↓
             Reality Event
                 ↓
          ARD / AIP / ER
                 ↓
 Memory / Capability / Narrative / Dynamics
                 ↺ 残余张力和未获焦对象回到背景场
```

这条循环的最低验收不是 Hominal 是否会谈论情感，而是：

> 她是否逐渐形成“什么值得成为我的世界”的个体化结构；这种结构是否通过一个统一但不阻塞身体的当前焦点组织有限资源；她是否能在日常处境快速生活、在关键处境按需深思、让矛盾成为可保留和可转化的张力；预测是否能提前产生准备和动力而不冒充现实；现实是否仍能使她看见偏离、愿意纠正，并重写以后将成为什么样的自己。
