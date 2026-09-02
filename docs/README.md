# Hominal 设计文档

本目录保存 Hominal 创生阶段的产品理论、统一术语、最小认知动力学和实施计划。这里的文档共同定义当前设计基线，但不代替真实代码、运行配置、身体事实或 Genesis Lab 的实验结果。

## 当前规范

建议按以下顺序阅读：

1. [产品设计理论与最小框架](./product-theory.md)：说明为什么建设成人级高级认知生命、意识表现行为的工程目标、产品立场和总体框架。
2. [统一核心术语表](./core-vocabulary.md)：统一 Advanced Cognitive Life、CEB、Proto-Hominal、Life Dynamics、Cognitive Dynamics、System Organ 以及实验概念的边界。
3. [最小认知动力学核心](./cognitive-dynamics.md)：只规定生命动力学中负责信息赋义、关切、单一认知线程、未来模拟、选择、结果学习和自我改变的中央核心，并明确感知与动作实施边界。
4. [创生实验实施计划](./genesis-plan.md)：规定从创生契约、Ubuntu 身体和 Genesis Lab，到正式运行、继代重构、反事实学习与寿命升级的实施顺序和退出门。
5. [G0 最小可编码架构](./mvp-architecture.md)：把单一认知提交、Pulse、最小感知、分阶段持久化、行动回链和 root 身体收敛为实现契约。
6. [项目架构](./project-architecture.md)：规定研究对象、创生输入、Ubuntu 身体、agent 生命卷、外部 Lab、构建部署、连续运行、代际档案和恢复体系。
7. [认知资源自主机制](./cognitive-resource-autonomy.md)：规定三种模型资源、费用账本、调用闸门、自主档位选择和异常保护。
8. [内核—器官架构](./core-organ-architecture.md)：规定认知核心、通用器官宿主、System Organ、Browser Organ 和身体底座怎样共同形成完整生命动力学。

八份文档承担不同职责：

| 文档 | 回答的问题 | 规范范围 |
| --- | --- | --- |
| `product-theory.md` | 为什么这样设计 | 产品立场与理论判断 |
| `core-vocabulary.md` | 概念具体指什么 | 术语、层级与语义边界 |
| `cognitive-dynamics.md` | 中央认知怎样持续、自主并从结果中改变 | 信息处理、选择、反馈与认知不变量 |
| `genesis-plan.md` | 怎样开发和验证 | 工程阶段、Lab 协议与验收门 |
| `mvp-architecture.md` | 首版代码必须怎样工作 | 进程、数据、出生、动作和恢复契约 |
| `project-architecture.md` | 项目与数字身体怎样运行 | 源码、实验器、agent 卷、部署、苏醒、代际、证据与恢复 |
| `cognitive-resource-autonomy.md` | alice 怎样感知并掌控模型资源 | 模型档位、价格、预算、选择、结算与保护 |
| `core-organ-architecture.md` | 单一生命怎样通过可替换、可生长的身体器官形成完整生命动力学 | 身体底座、System、器官发现、代理式控制、观察、行动与 Reality 边界 |

## 独立产品设计草案

[llmServer 产品与技术设计](./llm-server-product-design.md) 定义一个拟运行于 macOS 的本地模型与智能体能力网关，研究 API Token、Codex 和 WorkBuddy/CodeBuddy Code 三类 Provider 怎样通过统一的 OpenAI 兼容接口、流式事件、Web 配置、安全边界和用量账本向 Hominal 及其他受信任客户端提供服务。它目前是待实现和原型验证的独立产品草案，不改变上述七份 Hominal 现行规范，也不构成已经部署或验收的能力。

## 奠基推导记录

[创生奠基假设与 G0 工程原则](./genesis-foundations.md) 保存成人级高级认知生命、CEB、Proto-Hominal、Self Model、Narrative Self、Memory、Tension 和预测回差等结论的集中推导过程。其确认结论已经吸收进当前规范；该文件用于理解设计来源，不再形成第二套实现要求。

## 认知资源专项契约

[认知资源自主机制](./cognitive-resource-autonomy.md) 定义模型能力、推理强度、费用账户、资源身体感知、自主选择、异常保护和经验学习的最小闭环。首版代码已经进入当前内核，并在阶段五 Ubuntu 实验中完成真实结算、默认档位、选择表达和异常保护验证；更长期的资源策略演化仍由后续实验观察。

如果当前规范出现冲突，不应由实现者随意选择一份，也不应通过兼容层同时保留两套含义。冲突本身说明设计尚未收敛，应先统一修改相关文档，再进入开发。

## 历史档案

[`history/`](./history/) 保存已经退出当前基线、但对理解设计演变仍有价值的早期研究：

- [早期核心术语表 v1.0](./history/core-vocabulary-v1.md)
- [早期认知动力学 v1.0](./history/cognitive-dynamics-v1.md)

历史文档不是待实现需求。旧机制不得仅因为历史文档中曾经存在，就自动进入当前架构；只有真实实验暴露出当前最小机制无法解释或修复的问题时，相关思想才重新成为候选。

历史正文中的“生命”“意识”“婴儿”及旧版本号保留其原始语境；其含义不覆盖当前规范。当前项目口径始终以 `core-vocabulary.md` 为准。

## 文件命名与版本

当前规范使用稳定、无版本号的 kebab-case 文件名。版本号写在文档标题中，由 Git 提交和 Genesis Generation Manifest 固定某一代实际使用的内容。

这种规则避免每次从 v0.2 升级到 v0.3 时都修改文件路径和全部交叉链接。历史文档退出当前基线后，才在 `history/` 中使用带版本含义的文件名。

新增文档应满足以下条件之一：

- 它定义了现有四份规范无法合理容纳的独立契约；
- 它是已经发生的正式实验报告或谱系记录；
- 它是必须长期独立维护的外部接口说明。

为了审议跨越多份规范的奠基性修改，可以像 `genesis-foundations.md` 一样暂时增加一份明确标注的讨论草案；结论确认后必须合并进当前规范并删除或转入历史档案，不能让草案长期形成第二套规范。

不要按 AIP、Concern、Narrative Self、Conflict Metabolism 等理论术语逐项创建文档和目录，也不要建立 `bak/`、`misc/`、`drafts/` 作为长期堆放区。尚未收敛的讨论直接进入对应现行文档或 Git 历史。

## 与代码和生命数据的边界

本目录只保存人类与 Hominal 共同使用的设计基线：

- 可遗传的 Genesis Seed 和初始动力参数进入项目根目录的 `genesis/`；
- 每代 Birth Manifest 由身体外 `lab/` 生成，精炼事实进入 `lineage/`，可读副本注入当代身体；
- `hominald` 的生命状态、Reality Ledger 和个人文件属于 Ubuntu 身体内的运行数据，不进入 `docs/`；
- 完整磁盘快照、原始日志和大型实验产物由身体外 Genesis Lab 保存，不进入 Git；
- 正式创生代的简要 Manifest、分析和档案校验值进入 [`lineage/`](../lineage/)；第一份正式谱系为 `g0-v001/alice0826n`。
- 已由多轮实验支持、会影响后续架构判断的跨代经验进入 [`lineage/research-memory.md`](../lineage/research-memory.md)；它是实验前必读的研究记忆，不形成另一套产品规范。

文档不是现实证据。理论上定义了某项能力、公式或生命现象，不代表运行中已经实现或验证了它。
