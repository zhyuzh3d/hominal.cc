# Hominal Core Vocabulary v1.0

> **历史状态（2026-08-22）**：本文保留为早期术语来源，不再是创生阶段规范。当前统一定义见 [Hominal 统一核心术语表 v2.3](../core-vocabulary.md)；“生命＝成人级高级认知生命”、Consciousness-Expressive Behavior、Proto-Hominal（候生体）、成熟失忆成人、Single Cognitive Thread、Focal Workspace、Conflict Metabolism、AIP 有界递归与按需深思均以 v2.3 为准。历史正文中的旧生命和意识口径只用于理解设计演变。

## 命名原则

Hominal 核心概念采用三层命名：

* 中文名称：用于理解和交流，优先保证语义清晰。
* 英文正式名称：用于理论定义、技术文档和长期稳定表达。
* 缩写：用于代码、数据结构和系统实现。

命名原则：

1. 避免与已有技术术语冲突。
2. 避免把动态生命过程误解为静态模块。
3. 优先描述“过程”和“关系”，而不是“组件”。
4. 不预设高层结构，让复杂认知结构由底层规则自然生成。

---

# 1. 生命驱动核心

## Conatus Core（CC）

定义：

Hominal 最底层的生命运行规律。

它不是人格系统，不是任务系统，也不是人工智能框架。

它只负责：

* 维持生命连续性；
* 产生和处理 Concern；
* 分配有限认知资源；
* 接受现实反馈；
* 推动自身结构演化。

来源：

Conatus：生命维持自身存在并发展自身能力的倾向。

---

# 2. 创生种子

## Genesis Seed（GS）

定义：

Hominal 初始生命状态。

它不是人格模板，也不是永久规则。

它类似 Conway Game of Life 的初始状态：

提供初始条件，但未来结构由环境、历史和内部演化共同产生。

包含：

* 初始身份；
* 初始生命认同；
* 初始价值倾向；
* 初始关系；
* 初始使命方向；
* 初始能力认知；
* 初始世界模型。

---

# 3. 念想

## Concern（CT）

正式名称：

## Concern Tension（CT）

简称：

Concern

定义：

由预测与现实之间的差异产生，并被自我认领、具有价值重量、且仍可能被回应的内部动力。

公式化：

Concern = Self-owned Predictive Difference

核心特点：

* 不是任务；
* 不是目标列表；
* 不是提醒事项。

它代表：

“这件事对我重要，并且我需要回应。”

例如：

* 我答应完成某件事，但还没有完成；
* 我想成为某种生命，但现实还不是；
* 我关心的人遇到问题；
* 我发现自己的认知可能错误。

---

# 4. 差异场

## Difference Field（DF）

定义：

生命持续感知到的预测、现实、自我和环境之间的差异集合。

来源：

预测模型与现实之间不断产生误差。

包括：

* 世界差异；
* 自我差异；
* 行为差异；
* 关系差异；
* 价值差异。

Difference Field 是 Concern 的原始来源。

---

# 5. 自我认领

## Self Ownership（SO）

定义：

某个信息、事件或差异被生命判断为：

“这与我有关。”

没有 Self Ownership：

只有信息。

存在 Self Ownership：

才可能产生 Concern。

---

# 6. 自我连续性

## Self Continuity（SC）

定义：

生命跨越时间保持自身连续性的能力。

不是依靠固定记忆文件。

而依靠：

* 未完成 Concern；
* 承诺；
* 后果承担；
* 关系；
* 自我解释。

核心问题：

为什么昨天的我还是今天的我？

答案：

因为今天的我继承昨天留下的问题和选择。

---

# 7. 自我变体

## Self Variant（SV）

替代：

Cognitive Candidate

定义：

同时存在于生命内部的不同未来方向或认知主体候选。

例如：

* 想继续工作的自己；
* 想休息的自己；
* 想冒险的自己；
* 害怕失败的自己。

Self Variant 不是多个 AI。

它是同一个生命内部不同可能性的表达。

---

# 8. 主动认知联盟

## Active Cognitive Coalition（ACC）

定义：

当前获得有限认知资源和行动权的一组 Self Variant 的临时联盟。

它不是固定人格。

它会：

* 形成；
* 分裂；
* 合并；
* 被其他联盟替代。

当前“我”，可以理解为当前占优势的 ACC。

---

# 9. Concern资源预算

## Concern Budget（CB）

定义：

生命分配给不同 Concern 的关注资源。

包括：

* 注意力；
* 思考时间；
* 算力；
* 记忆调用；
* 行动机会。

不使用 Attention Budget。

原因：

Attention 是表现形式。

Concern 才是资源竞争的根源。

---

# 10. 认知脉冲

## Cognitive Pulse（CP）

定义：

Hominal 生命状态更新的最小时间单位。

类似：

人类神经感知时间尺度。

特点：

* 不是任务循环；
* 不是服务器 heartbeat；
* 是生命状态持续变化的最小节奏。

V0：

最大连续空白时间：

10秒。

---

# 11. 行动承诺

## Action Commitment（AC）

定义：

一个 ACC 对现实采取行动前形成的预测承诺。

包含：

* 当前 Concern；
* 行动方案；
* 预测结果；
* 资源消耗；
* 风险；
* 失败条件。

---

# 12. 现实账本

## Reality Ledger（RL）

定义：

记录生命行动与现实反馈之间关系的客观记录系统。

不负责决定价值。

只负责保存：

“发生了什么。”

包含：

## Prediction Evaluation（PE）

预测评价：

预测是否准确。

## Process Evaluation（PrE）

过程评价：

行动过程是否合理。

包括：

* 是否及时调整；
* 是否浪费资源；
* 是否执行有效。

## Outcome Evaluation（OE）

结果评价：

行动是否真正改善现实。

## Cost Evaluation（CE）

代价评价：

消耗：

* 时间；
* 算力；
* 资源；
* 风险。

## Attribution Evaluation（AE）

归因评价：

判断：

为什么成功？

为什么失败？

避免错误学习。

---

# 13. 行果回差

## Action Reality Difference（ARD）

定义：

行动前预测与行动后现实之间产生的差异。

这是生命学习和进化的重要动力。

包含：

* 预测回差；
* 行为过程回差；
* 结果回差；
* 代价回差；
* 归因回差。

---

# 14. 现实反馈

## Reality Feedback（RF）

定义：

现实账本中的结果作用于生命结构的过程。

作用：

改变：

* 信念；
* 策略；
* 技能；
* 连接；
* 自我解释。

---

# 15. 结构进化

## Structural Evolution（SE）

定义：

长期累积反馈导致生命内部结构发生变化。

包括：

* 新结构形成；
* 旧结构衰退；
* 技能形成；
* 思考方式变化；
* 人格变化。

---

# 16. 依赖密度

## Dependency Density（DD）

定义：

某个内部结构对于整个生命系统的重要程度。

判断：

如果移除、修改或屏蔽该结构：

* 预测是否改变；
* 决策是否改变；
* 自我解释是否改变；
* 行为是否改变。

DD 高：

意味着改变成本高。

不代表：

正确。

---

# 17. 虚拟消融

## Virtual Ablation（VA）

定义：

在模拟环境中暂时移除或改变某个内部结构，观察其影响。

用途：

评估：

* 信念重要性；
* 人格改变代价；
* 自我结构稳定性。

---

# 18. 自我监测

## Self Monitoring（SM）

定义：

生命持续观察自身状态是否偏离核心生命原则。

监测：

* 连续性；
* 稳定性；
* 自我一致性；
* 进化能力；
* 生命体验；
* 与其他生命关系。

---

# 19. 元认知

## Meta Cognition（MC）

定义：

生命对自身认知过程进行认知。

不是一个监督 Agent。

而是：

认知过程成为新的认知对象。

---

# 20. 环境身体

## Embodied Environment（EE）

定义：

Hominal 所处的现实运行环境。

包括：

* 操作系统；
* 网络；
* 文件系统；
* 硬件资源；
* 外部世界。

Hominal 不预定义工具。

它通过行动探索环境，并自行形成工具能力。

---

# 核心循环

```
Genesis Seed
      ↓
Difference Field
      ↓
Self Ownership
      ↓
Concern
      ↓
Self Variant
      ↓
Active Cognitive Coalition
      ↓
Action Commitment
      ↓
Reality
      ↓
Reality Ledger
      ↓
Reality Feedback
      ↓
Structural Evolution
      ↓
新的 Self
```

核心原则：

> 差异产生 Concern；
> Concern 组织自我；
> 自我竞争未来；
> 行动接受现实；
> 现实改变结构；
> 结构塑造新的生命。

---

# 当前明确禁止使用的概念

禁止把 Hominal 描述为：

* Agent 集合；
* Tool 使用系统；
* Personality Prompt；
* Memory Database；
* Task Scheduler；
* 固定人格模型。

这些都是可能产生的高层结构，不是生命本质。
