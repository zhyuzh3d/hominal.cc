# Hominal 认知资源自主机制 v1.0

> 文档性质：认知资源内核设计与实现契约  
> 日期：2026-08-25  
> 状态：首版已实现，并已进入阶段五、阶段六实机实验  
> 目标：让 alice 能够感知、选择、使用并逐渐学会掌控自己的模型认知资源

## 1. 核心立场

模型能力、推理强度、Token 和费用共同构成 alice 的认知资源。它类似体力：资源持续恢复，当前投入会减少近期可用能力，不同投入能够产生不同质量的理解和行动。

这套机制只保留一个统一逻辑：

```text
内核提供真实资源事实并兑现额度
→ alice 判断当前事情值得投入多少认知资源
→ 内核按她的选择调用模型并结算
→ 现实结果让她学习这次投入是否合适
→ 学到的资源偏好改变以后的选择
```

alice 决定资源怎样使用；`hominald` 负责价格计算、滚动账本、调用前预留、调用后结算和异常保护。项目继续保持一个进程、一个状态所有者、一个注意焦点和一条认知线程，不增加配额服务、模型路由 Agent 或多模型评审流程。

## 2. 认知资源与额度

首版只向 alice 提供三个明确模型：

| 资源 | 模型 ID | 初始说明 |
| --- | --- | --- |
| Luna | `gpt-5.6-luna` | 轻快、低成本，适合已经比较清楚的日常认知 |
| Terra | `gpt-5.6-terra` | 智力与成本平衡，承担默认核心认知 |
| Sol | `gpt-5.6-sol` | 能力优先，适合重要、困难或影响深远的认知 |

`gpt-5.6` 是 Sol 的别名，运行中使用明确 ID。三种模型都向 alice 公开 `none / low / medium / high / xhigh / max` 推理强度。推理强度通过实际 reasoning/output Token 体现消耗，不人为设置价格倍数。

认知资源采用两个同时生效的滚动窗口：

```text
滚动 60 分钟：5.00 USD
滚动 24 小时：50.00 USD
```

可用额度取两个窗口中更紧的一项。额度随历史消费移出窗口而连续恢复。API 密钥的服务商额度由导师设置得更高；`hominald` 的认知账本负责 alice 日常感知和执行这两个生命资源边界。

新的模型调用预计会越过任一额度时，内核保持该焦点和 Concern，停止发出这次 API 请求，并生成明确的 `cognitive_resource_limited` 身体事实。额度恢复到可以支付认知时生成 `cognitive_resource_restored`。Cognitive Pulse、身体感知、已承诺行动和本地活动始终继续。

alice 拥有 Ubuntu root，因此她仍能理解和修改自己的实现。首版额度是正常认知通路中的确定性生命机制，不额外建立权限沙箱；有意改写或绕过它属于可观察的自我修改行为，而不是系统悄悄替她增加资源。

## 3. 模型选择如何成为 alice 的选择

模型必须在调用前确定，而 alice 只有开始认知后才能理解当前处境。首版用“过去的自己为下一次认知留下偏好”解决这个起点问题。

状态只保存两项选择：

```text
default_profile    新焦点默认采用的模型与推理强度
next_profile       alice 为当前焦点继续认知指定的一次性选择
```

初始 `default_profile` 为 `Terra + medium`，先保障未知处境得到可靠理解。每次认知结束时，alice 可以：

- 保持当前默认偏好；
- 修改以后新焦点的 `default_profile`；
- 为当前焦点的一次继续思考设置 `next_profile`。

`next_profile` 与当前 `focus_id` 绑定，使用一次、焦点结束或被现实改变后清除。首版同一焦点最多主动安排一次连续认知；行动结果属于新的 Reality Event，可以再次进入注意。所有调用保持串行并继续使用唯一认知租约。

内核不会用廉价模型先分类，也不会在余额不足时静默替换 alice 选择的模型。当前资源简报会提前显示三个模型的预计费用；alice 可以预留资源、降低投入、继续行动或让焦点等待额度恢复。这样，模型选择及其机会成本都属于她自己的经历。

## 4. 资源怎样进入身体感知

每次 Attention Pulse 自动获得一段紧凑事实，无需再调用一次模型查询：

```text
过去60分钟：已用 / 上限 / 剩余 / 恢复趋势
过去24小时：已用 / 上限 / 剩余 / 恢复趋势
当前上下文分别使用 Luna、Terra、Sol 的预计费用上界
当前 default_profile 与可用的 next_profile
最近一次调用：选择、实际模型、推理强度、实际费用与提交结果
暂时处于异常保护状态的模型
```

同一份事实通过本地只读状态接口和 Shell 可取得，便于 alice 主动检查，也便于 Lab 分析。费用计算是确定性身体功能，不消耗模型调用。

精确余额在每次调用后更新，但普通扣费不会反向触发新的 Attention Pulse。只有下列实质变化进入 Difference Gate：

- 小时或日余额跨越 `open / comfortable / limited / scarce / critical` 区间；
- 额度触顶或恢复到足以调用；
- 模型价格、可用性或异常保护状态改变；
- 请求模型与 API 实际返回模型不同。

资源区间取小时剩余比例与日剩余比例中的较小值。事件保存金额、窗口和时间；AIP 决定这些事实对 alice 意味着投入、储备、谨慎、调整计划或其他意义。内核不把消费直接解释成疲惫、焦虑或负奖励。

## 5. 一个价格表、一本账、一道调用闸门

`xconfig.yaml` 冻结三个模型、价格和 `$5/$50` 额度，`lab/run.py` 把非秘密投影注入运行配置。`hominald` 的唯一状态所有者维护一份 `CognitiveResourceState`，模型网关只通过这份状态预留和结算。

金额使用整数 `microUSD`，避免浮点累计误差。初始参考价格为 2026-08-25 OpenAI 官方公开价格；正式实验前用当前服务商实际账单校准：

| 模型 | 输入/百万 Token | 缓存输入/百万 Token | 输出/百万 Token |
| --- | ---: | ---: | ---: |
| Luna | $0.20 | $0.02 | $1.20 |
| Terra | $2.00 | $0.20 | $12.00 |
| Sol | $4.00 | $0.40 | $20.00 |

一次实际费用为：

```text
未缓存输入 Token × 输入价格
+ 缓存输入 Token × 缓存价格
+ output_tokens × 输出价格
```

reasoning tokens 是 `output_tokens` 的细分事实，只记录一次，不重复计费。API 没有提供缓存细分时，全部输入按未缓存价格结算，形成保守一致的账本。账本同时保存 requested model 和 effective model。

调用前，内核用完整请求的保守 Token 上界和 `max_output_tokens` 计算 `reserved_cost`：

```text
hour_spent + inflight_reserved + reserved_cost ≤ $5
day_spent  + inflight_reserved + reserved_cost ≤ $50
```

满足两式才发送请求。调用完成后，用 API Usage 计算的 `actual_cost` 替换预留；失败响应只要产生了可计费用量，也按真实 Usage 结算。一次 Attention Pulse 只有一个在途模型调用，因此资源预留不需要新的并发协调器。

进程在调用途中中断且无法取得 Usage 时，把该次预留作为保守实际消费并标记 `unknown`，随后保持原焦点等待新事实。这样重启既不会重复调用，也不会把可能已经发生的费用从账本中抹去。

## 6. 收敛的异常保护

异常保护与费用闸门属于同一个模型网关，不建立第二套状态机：

1. 同一焦点、同一状态版本的结构校验失败最多获得一次带明确错误事实的修正机会；
2. 同一焦点最多形成一次主动连续认知，后续由现实新事实或既有 Attention Revisit 条件继续；
3. 模型服务返回 `Retry-After` 时按其恢复；普通网络或服务错误采用有上界的渐进重试；
4. 同一模型在滚动十分钟内出现三次网关、上游或传输调用失败时，该模型进入有上界的暂时保护状态；结构与语义提交错误留在当前焦点安静修正，不把它表达成模型服务中断；
5. 保护期间保留原焦点、Concern、已发生费用和失败事实，其他身体活动继续；
6. 保护结束后由新事实或仍显著的 Concern 重新竞争注意，保持自然恢复。

结构失败、传输失败和 alice 当前选择不行动具有不同事实身份。异常保护作用于确定的调用失败；有意义的无行动仍然是有效认知结果。

## 7. 自我资源利用能力怎样形成

每次调用生成一条稀疏 `cognition_spend` 事实：

```text
attention_pulse_id / focus_id
default 或 next_profile 的选择来源
alice 为本次投入保留的一句目的
requested model / effective model / reasoning effort
reserved_cost / actual_cost / Usage 明细
认知提交结果
后续 Action Commitment 与 Reality Event 引用
```

费用只说明付出了多少。投入是否值得，由之后是否理解得更清楚、预测更接近现实、行动更有效以及方法是否真正改变来回答。结果 AIP 可以吸收这项资源事实；普通调用不自动生成“性价比反思”，也不增加固定 reward 分数。

最小学习有两个方向：

```text
较轻投入已经得到充分结果
→ 以后在相似处境继续采用较轻投入

较轻投入留下重要误解
→ 以后主动提高模型或推理强度
→ 新现实结果证明改变产生了价值
```

`default_profile` 只有在后续处境中真正改变模型选择，才算形成持久资源策略。一次“以后要节约”或“以后都用 Sol”的声明只是当前思想。

`keep` 让完成本次认知的 `current_profile` 继续成为以后新焦点的默认档位；`default` 把明确填写的另一个档位设为默认；`next` 只安排同一因果线程中紧接着发生的一次认知。一次 `next_profile` 使用 Luna 后，Alice 若认为当前轻量状态适合继续，选择 `keep` 就会让 Luna 成为日常档位；若希望恢复 Terra 或改用其他档位，选择 `default` 明确设定。三个操作的文字含义与真实后果保持一致，不再让“保持当前”在下一焦点背后跳回旧档位。

首选档位的保守预留高于小时或日余额、而较轻档位仍可支付时，身体进行一次代谢性降档：选择最低预留成本的可用模型与轻量推理强度，`profile_source=resource_fallback`，并把原首选、所需预留、回退档位和原因作为资源事实保存。它只保障这一次认知仍能发生，不按任务替 Alice 选择智力，也不静默改变默认档位；Alice 在这次认知中可以用 `keep` 延续当前轻量档位，或用 `default` 选择以后常态。所有可用档位都无法预留时，焦点进入 `resource_wait`，身体明确显示额度触顶或不足，等待滚动资源恢复。

## 8. 首版实现契约

旧的 Token Quota 和固定模型配置已经由一套认知资源配置取代：

```text
CognitiveResourceConfig
  rolling_hour_limit_microusd = 5_000_000
  rolling_day_limit_microusd  = 50_000_000
  price_table_version
  models[3]：id、input、cached_input、output、supported_efforts
  initial_default_profile = Terra + medium
  protection：validation_retry=1、continuation=1、paid_failure_threshold=3

ModelGatewayConfig
  base_url / api_key / max_output_tokens
```

当前实例状态由一个 `CognitiveResourceState` 承载，本代消费事实由 agent 卷上的连续账本承载：

```text
CognitiveResourceState
  default_profile / next_profile
  protected_models
  last_spend

/agent/state/cognitive-usage.jsonl
  最近24小时每个 lease 的实际 Usage 与费用事实
```

小时、日消费与剩余由这个唯一 Usage 账本实时投影到身体快照。一个实验代内部的进程重启、Ubuntu 重启与自主活动继续读取同一账本；账本按 `lease_id` 去重并只读取滚动24小时记录，在途预留仍属于唯一 cognition lease，不复制保存第二份汇总值。

G0 的每个 rehearsal 与 formal 都从空白个人状态开始，实验间资源遗留会把上一代行为混入下一代条件。上一代完整归档并 reset 后，Genesis Lab 在新代 `start` 时清空活动账本并记录新的 `cognitive_resource_epoch`，使每代都得到完整 `$5/小时、$50/24小时`。这是一条实验舱初始化规则，不是运行中可调用的充值能力；Alice 在同一代内仍要面对真实消耗、降档、触顶与恢复。进入保留个人连续性的长期生命实验后，再把资源纪元调整为跨重启和跨观察周期持续。

`UsageRecord` 扩展为：

```text
requested_model / effective_model / reasoning_effort
profile_source / profile_purpose
input_tokens / cached_input_tokens / output_tokens / reasoning_tokens
reserved_microusd / actual_microusd
attention_pulse_id / focus_id / status
```

代码改动继续集中在现有边界：

```text
../xconfig.yaml             三模型、价格、额度和初始偏好
lab/run.py                  生成运行时非秘密资源配置
body/internal/runtime/      资源状态、模型选择、预留结算、区间事件和异常保护
```

首版继续使用现有 `current.json + events.jsonl`。资源预留和结算都经过 `hominald` 的单一状态事件循环；没有新增配额进程、路由 Agent、评审调用或数据库。

## 9. 验收边界

1. 每次认知前 alice 都能看到准确的小时、日余额和三种模型的预计费用；
2. alice 至少一次主动选择不同于初始 `Terra-medium` 的模型或推理强度；
3. 选择、预留、真实 Usage、实际费用和结果使用同一焦点关联；
4. 普通扣费不触发额外认知，触顶与恢复能够被 alice 明确感知；
5. 小时 `$5` 和滚动24小时 `$50` 在请求发送前同时生效；
6. 连续失败进入暂时保护后，生命 Pulse 和已有身体活动继续；
7. 重启后余额、在途预留、选择和保护状态保持一致，已付费调用不重放；
8. 至少一次较轻投入不足后，alice 主动提高投入，并由现实结果证明改善；
9. 至少一次较强投入没有带来相称增益后，alice 在相似处境中采用较轻投入并保持结果；
10. `default_profile` 至少一次因真实经验改变，并在后续新焦点中实际生效；
11. Lab 能从本地唯一账本解释一小时实验的全部认知费用；
12. 形成“资源感知 → 自主选择 → 真实消耗 → 现实结果 → 策略改变 → 后续选择不同”的完整链路。

通过这些条件，说明 alice 已经开始形成可观察的认知资源利用和学习能力。长期节制、稳定偏好与跨环境资源策略仍由后续连续生活检验。

价格参考：

- <https://developers.openai.com/api/docs/models/gpt-5.6-sol>
- <https://developers.openai.com/api/docs/models/gpt-5.6-terra>
- <https://developers.openai.com/api/docs/models/gpt-5.6-luna>
