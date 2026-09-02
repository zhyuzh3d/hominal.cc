# Hominal Body

本目录承载部署到专属 Ubuntu 设备的 Hominal 生命内核与身体接口源码。

当前 `cmd/hominald` 与 `internal/runtime` 已经形成一个持续运行的进程、一个认知状态所有者、可学习的预测回差场、Responses 模型接口、认知资源账本与调用闸门、alice 自主模型档位选择、导师与 Lab 环境事件入口、AIP—Concern—Attention 动力、Action Commitment（行动意愿）—Reality—Experience 学习链、Integrity 和最小自我材料。外部、身体、资源、内部与行动结果信号先更新紧凑 Difference Trace，累计回差越过共同阈值才取得主意识资格；AIP 与 Experience 反向调节同类信号以后获得注意的频率。`rehearsal` 与 `formal` 复用同一认知内核；出生简报只进入一次，第一次成功认知提交形成唯一 T0、样本名与计划截止，重启可以从已落盘的提交事实恢复同一出生身份。

`internal/organ` 是生命内核与具体身体之间的通用 Organ Host，负责 Manifest 发现、进程生命周期、健康、观察、定向和统一 `perform`。`organs/system.json` 与 `organs/browser.json` 注册当前两个器官；`cmd/hominal-system` 独立取得 Ubuntu 身体事实并承接 root 命令执行，`tools/hominal-browser.mjs` 独立拥有 Playwright MCP、网页语义、X 对象、页面定向、总时限、取消和连接恢复知识。内核只接收通用器官事实和动作结果，不理解 `/proc`、systemd 探针、CDP、DOM、页面角色或 Playwright 控制细节。

Alice 的认知输出只形成 `organ_action`：选择器官、操作和已经确定的输入。System 与 Browser 都经同一个 `organ.perform` 执行，并返回 `completed / failed / unknown`。浏览器的 `list / schema / call / run-code` 保留为器官内部发现与工程入口；状态相关动作在一条有总时限的器官队列中保持页面连续，`health` 从队列外只读报告 `ready / busy / recovering`。器官不理解生活目标，不替 Alice 规划浏览、关注、表达或发布。

Lab 控制器、实验评价、祖代档案和开发计划属于身体外环境，不进入正式身体包。
