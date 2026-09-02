# Hominal llmserver 认知网关迁移计划

> 日期：2026-09-02  
> 状态：已完成  
> 范围：模型传输适配、账务事实、Ubuntu 部署与三模型速度验收

## 判断

llmserver 的 `/v1/responses`、Bearer 鉴权、`instructions`、结构化 `input`、`reasoning.effort`、`store=false`、标准 `usage` 和原生 function tools 能承载 Hominal 的无状态认知请求。服务继续不承担器官执行与生命历史保存，并增加幂等、确认账单与请求 ID。

Hominal 仍由自身状态保存认知连续性，不依赖服务端会话。`cognitive_commit` 是主脑每次单线程认知的唯一结构化出口：llmserver 负责原生传输并校验 Schema，Hominal 负责再次验证候选身份、现实约束和器官权限。工具调用改变传输可靠性，不改变认知动力学与身体执行权。

## 必要开发

1. ModelGateway 明确区分 `openai` 与 `llmserver` 适配器；动力学与认知 Prompt 不感知供应商。
2. 正式认知只发送一个严格 `cognitive_commit`，强制选择该函数并设置 `parallel_tool_calls=false`；删除提示词 JSON 合同和伪函数调用适配。工程 Stage 3 只在确有行动选择时声明两个小工具，并用标准 `function_call_output` 回传实际结果。
3. 每次请求在首次发送前由实例、焦点和完整请求摘要生成稳定幂等键；同一请求的网络重试保持请求体不变。
4. 保存 `x-llmserver-request-id`、`settlement_status`、`price_version`、币种和服务端确认费用。费用字符串用十进制定点解析到现有 microUSD 账本，不用浮点重算替换服务端账单。
5. xconfig 激活 `llmserver`，凭据继续只从上层私密 `xconfigs/llmserver/xconfig.yaml` 读取；运行包只得到本次部署需要的 Token，不复制凭据文件。
6. 模型公开 ID 改为 `codex-luna`、`codex-terra`、`codex-sol`；本轮部署后的速度实验统一使用 `reasoning.effort=none`。

## 验收

本地必须证明：llmserver 请求只含一个强制的严格认知工具并关闭并行；真实 `function_call` 能还原为现有 `CognitiveCommit`；多调用、缺失 `call_id` 和非法参数不会越过本地边界；确认账单成为真实消耗；未确认账单不伪装成零费用；幂等键稳定且不同请求不冲突；OpenAI 旧适配仍通过已有测试。

Ubuntu 必须证明：设备能访问 `healthz`、`readyz` 和授权模型列表；新发布包和私密运行配置成功激活；`hominal.service` 能启动并维持 Pulse；从 Ubuntu 分别调用 `codex-luna`、`codex-terra`、`codex-sol` 的 `none` 并取得原生函数调用与确认账单；完整 Stage 10 认知记录从开始到结算的时间、Token 与费用。

速度测试只比较相同输入下的传输和生成表现，不据一次调用判断长期主力模型。若任一模型不能稳定返回符合 Schema 的原生 `function_call`、确认账单或在 Hominal 时限内完成，即判定当前 llmserver 契约不足并停止把它作为正式认知网关。

本轮部署是工程验收，不启动新的 Alice 生命样本，也不把服务启动或单次模型回复称作自主生命实验结果。

## 实施结果

2026-09-02 发布 `g0s10-87e0a93adc42` 到 Ubuntu，并用工程实例 `g0s10-20260902t085908z-87e0a9` 完成端到端验收。`hominal.service` 重启后正常进入 Pulse，默认档位为 Terra/none。工程运行完成 11 次 llmserver 认知，11 次都返回合法 `CognitiveCommit` 和确认账单；第 12 次在人工结束工程验收时被中止，不记为网关失败。完整认知输入约 1.0 万至 1.45 万 Token，11 次从 `cognition_started` 到确认结算平均 8.230 秒，最短 6.922 秒，最长 9.514 秒，总确认费用 `$0.345914000`。实际账本保存了 `codex-terra`、request ID、confirmed、USD 和价格版本。

同一 Ubuntu 设备以相同短输入对三种 `none` 模型交错测试三次，九次均成功并得到确认账单：

| 模型 | 平均首个可见字符 | 平均完成 | 首字符后平均可见字符/秒 | 三次总费用 |
| --- | ---: | ---: | ---: | ---: |
| `codex-luna` | 1.297 秒 | 6.622 秒 | 81.467 | `$0.001557600` |
| `codex-terra` | 1.522 秒 | 7.545 秒 | 69.300 | `$0.014460000` |
| `codex-sol` | 2.538 秒 | 14.280 秒 | 37.733 | `$0.025400000` |

这些数值证明当前局域网路径与三模型 `none` 可用，不证明主生命模型质量排序。Sol 的第三次样本首字符 4.402 秒、完成 20.679 秒，显示上游生成速度仍有波动；长期模型选择仍需按阶段 10.2 的生命行为结果判断。

以上是原纯 JSON 过渡适配的历史验收记录。2026-09-02 llmserver 开放 Codex Luna、Terra、Sol 原生 function calling 后，Hominal 已删除该过渡层：运行时不再要求模型手写 JSON 或伪造 `call_id`，并保留全部本地事实校验。新的发布与实机结果记录在本计划后续实施结果中。

原生工具版本发布为 `g0s10-9078569a0aaa`，工程实例 `g0s10-20260902t144808z-907856` 已部署、重启、验收并归档。Ubuntu 先分别调用 Luna、Terra、Sol 的 `none`，三者都返回强制 `gateway_probe`、有效 `call_id`、Schema 合法参数和确认账单。随后 Terra/none 以完整 Stage 10 `cognitive_commit` 连续完成 7 次认知，没有工具合同、解析、账务或器官派发失败；输入为 10022–13818 Token，完成时间平均 10.040 秒、最短 7.755 秒、最长 11.802 秒。

另用 Luna/none 完成一次独立的两轮协议验收：第一轮返回 `read_probe`，本地结果通过同一 `call_id` 作为 `function_call_output` 进入第二轮，第二轮准确吸收 `alpha=observed`，两轮账单均为 confirmed。这验证了工程 Stage 3 的标准结果回传路径；正式生命循环仍让行动 Reality 在下一次自然注意中被吸收，不把身体行动压缩成服务器内的代理循环。

这 7 次认知实际形成了 System 身体核验、Browser 页面导航和 Browser 快照读取，并把行动结果带回 Experience 与 Narrative Self，证明原生函数调用没有绕开原有生命动力学。与旧纯 JSON 工程样本平均 8.230 秒相比，本轮小样本慢约 1.8 秒，但仍处于同一可用量级；样本、上下文和输出长度不同，因此不能据此断言原生工具本身造成固定延迟。当前 llmserver 不提供函数参数增量事件，Hominal 保持非流式调用，避免为没有提前可用结果的 SSE 增加复杂度。

工程实例已经归档并 reset，避免无界消耗；发布包、systemd 单元和局域网 `NO_PROXY` 配置保留在 Ubuntu。下一次正式或 rehearsal 启动会直接使用这一冻结发布路径。
