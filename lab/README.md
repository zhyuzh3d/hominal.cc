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

正式出生前的公网检查使用两个中立 HTTPS 端点并允许短暂传输重试，用来判断身体是否具有公共网络路径。X、微信或任一具体服务的瞬时可达性属于运行中的真实环境状态，不再被误当成整个公网是否存在。

`start` 为 `rehearsal` 与 `formal` 构建同一种完整 bundle。Ubuntu 重启后，运行时进入 ready 即原子形成唯一 T0、`aliceMMDD<字母>` 和计划截止；Lab 随后幂等封存 `birth.yaml`、以同一个绝对 `planned_end` 安排本地 systemd wall-calendar 截止，并发送一次导师出生说明。模型第一次成功回答不决定出生时刻。`formal` 的窗口固定为 3600 秒；彩排使用更短上限，链路完成后可以立即停止。相对 timer 与绝对生命时间不得并存，计划终点后的无认知空转不计入耐久性。

`supervise` 是身体外的前台正式代监督。它每二十秒只读取服务、Pulse、资源和导师队列数量，在身体外保存 T0 与截止事实；本机截止失效时仍会在 `planned_end` 请求停止并调用既有归档链。它不读取导师消息正文，不向 alice 发送事件，也不评价认知内容。正式代运行期间保持该命令持续运行。

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
