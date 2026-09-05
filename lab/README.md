# Genesis Lab

本目录是身体外的最小创生入口。Lab 负责冻结发布、准备出生、注册当代、封存 T0、安排截止、保存结果和准确 reset；它不参与 alice 的认知，也不发展成观察产品。

统一入口：

```bash
python3 lab/run.py check
python3 lab/run.py start --stage 5 --kind rehearsal --window-seconds 300
python3 lab/run.py status
python3 lab/run.py inspect
python3 lab/run.py supervise
python3 lab/run.py browser-check
python3 lab/run.py mentor-send --body '...'
python3 lab/run.py mentor-outbox
python3 lab/run.py mentor-ack <message_id>
python3 lab/run.py stop
python3 lab/run.py reset
```

`browser-check` 先确认 Playwright MCP 与 Chrome 的真实连接，再用一次固定的短探针进入 X 并只判断登录 cookie 是否存在、Home 导航与账号切换入口是否可见、登录入口是否消失；它不读取或输出 cookie 值，也不以当前恰好打开的标签页判断会话状态。

`lab/body/browser-recovery-probe.mjs` 是独立的器官恢复诊断，须复制到 Ubuntu 并在生命服务停止、Chrome 就绪时运行：`node browser-recovery-probe.mjs /path/to/candidate/hominal-browser.mjs`。它拒绝在活跃生命中启动，只在本地回环地址创建两张同网址测试页，核对三次感知抢占后的实际操作对象和时延；最终关闭自己的页面、会话和临时文件。不操作公共账号、不调用模型，结果不能算成 Alice 的自主行为。Playwright 库可由 `HOMINAL_PLAYWRIGHT_MODULE` 指定，默认沿用身体已安装的库。

`lab/body/browser-reading-probe.mjs` 使用同样的停止前提和调用方式，以本地页面检验三次单字符完整读取、长文本末尾保留、当前内容与加载标志并存、后续真实变化及 HTTP 404 报告。它不按内容丰富程度验收，也不将这些隔离动作算入生命样本。

正式出生前的公网检查使用两个中立 HTTPS 端点并允许短暂传输重试，用来判断身体是否具有公共网络路径。X、微信或任一具体服务的瞬时可达性属于运行中的真实环境状态，不再被误当成整个公网是否存在。

`start` 为 `rehearsal` 与 `formal` 构建同一种完整 bundle。Ubuntu 重启后，运行时进入 ready 即原子形成唯一 T0、`aliceMMDD<字母>` 和计划截止；Lab 随后幂等封存 `birth.yaml`、以同一个绝对 `planned_end` 安排本地 systemd wall-calendar 截止，并核验 timer 确实处于 active，再发送一次导师出生说明。清单里已有 unit 名不等于截止仍被 systemd 持有。模型第一次成功回答不决定出生时刻。`formal` 的窗口固定为 3600 秒；彩排使用更短上限，链路完成后可以立即停止。相对 timer 与绝对生命时间不得并存，计划终点后的无认知空转不计入耐久性。

`supervise` 是身体外的前台正式代监督。它每二十秒只读取服务、Pulse、资源和导师队列数量，在身体外保存 T0 与截止事实；本机截止失效、身体仍可达时，它会在 `planned_end` 立即触发同一个幂等停止与有界排空流程，而不会等待用于断网的宽限时间。它不读取导师消息正文，不向 alice 发送事件，也不评价认知内容。正式代运行期间保持该命令持续运行。

System 与 Browser 的可执行文件和 Manifest 随 bundle 分别进入本代 `body/bin` 与 `body/organs`，由生命进程内的通用 Organ Host 发现。alice 形成 `organ_action` 后，由同一个 `organ.perform` 把已确定的操作交给对应器官；System 承接 Ubuntu 命令与代码，Browser 承接 Playwright 操作。结果继续回到同一条 Commitment—Reality—Experience 链；Lab 只负责发布与实验外壳，不管理器官的生活意义。

持久应用状态的离机灾备使用以下命令管理：

```bash
python3 lab/run.py baseline-create
python3 lab/run.py baseline-verify
python3 lab/run.py baseline-restore --disaster-recovery
```

Chrome/X、微信与 Clash Verge 的状态统一保存在 `/agent/state/profiles`，作为身体长期拥有的能力和现实环境跨代延续。普通 `stop`、归档、`reset` 和新代启动都不改写这些状态。`baseline-create` 把三类状态备份到上层私密配置指定的 Mac 离机目录；`baseline-restore` 默认拒绝运行，只有明确给出 `--disaster-recovery` 才会用离机副本替换当前状态并重启 Ubuntu。

认知费用账本保存在 `/agent/state/cognitive-usage.jsonl`。同一代内的进程重启、系统重启和自主运行都继续使用这一份账本，因此 `$5/滚动小时、$50/滚动24小时` 始终是 Alice 能够感知和管理的真实有限资源。G0 的 rehearsal 与 formal 每次代表一个新的 Proto-Hominal 实验代；上一代完整归档并 reset 后，`start` 为新代原子开启空白资源纪元，使不同实验代都从相同的完整额度起步。应用 profile 和外部生态事实仍按原规则跨代延续。

`stop` 明确停止并归档，`reset` 只删除已经归档的准确实例。正式代归档同时保存出生与终态的 LVM、软件包、服务和 `/etc`、`/usr/local` 文件清单差异；Chrome、微信与 Clash Verge 的实际状态留在持续身体中，不在每一代重复复制，独立灾备基线负责极端恢复。完整 bundle、Birth Manifest、实例终态、导师转录、监督记录、系统差异、最终身体探针与哈希保存在私密离机目录，不进入 Git。阶段一契约仍可用 `python3 lab/validate-contract.py --xconfig ../xconfigs/hominal/xconfig.yaml` 校验；校验器不输出凭据。

## 隔离认知契约诊断

`TestExportStoppedCognitiveContract`可用`HOMINAL_CONTRACT_ARCHIVE`、`HOMINAL_CONTRACT_FOCUS`与`HOMINAL_CONTRACT_OUTPUT`从封存个体的最终状态重建一份待处理认知请求；它只调用本地假网关，不执行动作。此导出是末态复现而非原请求字节录像。

`python3 lab/contract_probe.py <请求JSON> --output <新的私有结果文件>`使用当前xconfig所选llmserver，每次仅一次真实付费调用，返回动作永不执行或导入个体。依赖本机`jsonschema`对模型原样参数做本地校验；llmserver只传输调用意图，不替Hominal修改、批准或执行参数。正常调用失败、无效JSON、未声明函数、Schema违规或账单未确认都会返回非零退出码。结果文件包含个人上下文/输出，应保存在Lab私有目录，不提交仓库。`--inspect-schema`仅用于改变生成约束后的诊断，不能作为生产合同或历史失败原文。

生产适配收敛于 `runtime/model_contract.go`。所有llmserver模型使用同一个Responses路径、同一个`cognitive_commit`函数、`strict:false`与同一份本地校验；模型ID只改变能力、速度和价格，不改变Alice的因果规则。原契约中已经由当前事实唯一决定的顶层元数据由内核绑定，意义、焦点、行动、记忆、经验和叙事选择仍由模型生成。导出时可设 `HOMINAL_CONTRACT_ROLE=assistance` 获取高阶小型实现助手请求；同样不执行动作、不修改个体。Lab启动预检不推测供应商能力，只核对配置模型存在，并用一次真实函数调用、参数校验和确认账单证明当前接口可用。
