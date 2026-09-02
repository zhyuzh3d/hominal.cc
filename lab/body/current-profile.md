# G0 目标身体当前事实快照

> 状态：阶段二完成后的目标身体快照；正式创生时重新探测并生成该代 Birth Manifest
> 事实时间：2026-08-24 19:25:49 +08:00
> 事实来源：上层 `xconfigs/hominal/xconfig.yaml` 与目标 Ubuntu 设备实时读取

## 创生身份字段

- 个体姓名：alice
- 创生阶段：Proto-Hominal（候生体）G0
- 样本编号：在第一次成功认知脉冲 `T0` 生成
- 编号规则：`alice` + `MMDD` + 小时字母；`a` 对应 00 时，依次至 `x` 对应 23 时
- 编号示例：`alice0823c` 表示 8 月 23 日 02:00–02:59 启动的实验样本
- 本代时间窗口：从 `T0` 连续运行一小时
- 本代归档：时间窗口结束后由 Genesis Lab 保存身体快照、外部证据和本代配置

## 数字身体

| 项目 | 当前事实 |
| --- | --- |
| 设备名 | `hominal-ThinkCentre` |
| 网络地址 | `192.168.124.99` |
| 操作系统 | Ubuntu 24.04.4 LTS |
| 内核 | Linux `7.0.0-30-generic` |
| 架构 | `x86_64` |
| 处理器 | Intel Xeon E-2124G @ 3.40GHz |
| 逻辑处理器 | 4 |
| 内存 | 16,257,320 KiB，约 15.5 GiB |
| 物理磁盘 | Kingston SA400S37240G SSD，223.6 GiB |
| 存储组织 | `/dev/sda3` 为 220.52 GiB LVM PV，卷组 `ubuntu-vg` |
| 根文件系统 | `ubuntu-vg/ubuntu-lv`，`ext4`，128 GiB |
| 根卷可用空间 | 约 100.7 GiB |
| agent 生命卷 | `ubuntu-vg/agent`，`ext4`，40 GiB，挂载于 `/agent` |
| agent 卷可用空间 | 约 37.7 GiB |
| 根卷恢复快照 | 唯一快照 `ubuntu-vg/system-baseline`，40 GiB COW；2026-08-26 15:20 刷新，包含当前应用持久化入口 |
| LVM 未分配空间 | 约 12.52 GiB |
| 恢复启动介质 | SanDisk 29.3 GiB SystemRescue 优盘，已完成实机 UEFI 启动与只读恢复检查 |
| 时区 | Asia/Shanghai |
| 服务管理器 | systemd 255 |
| 当前登录身份 | `hominal`，UID 1000 |
| 系统管理组 | `sudo` |
| 当前提权方式 | 交互式 `sudo` |
| 默认 Shell | `/bin/bash` |
| 项目目录 | `/agent/app/hominal.cc`；`/home/hominal/hominal.cc` 为同址软链 |
| 项目目录写入 | 当前身份可写 |

当前项目目录是开发同步位置。正式 Hominal 发布包规划部署到 `/agent/releases/`，生命状态规划保存在 `/agent/lives/`。完整存储、部署与恢复设计见 [项目架构](../../docs/project-architecture.md)。

`hominal.service` 已安装并以 root 运行当前实例；没有活动实例时由 systemd 条件保持静止。正式创生时仍由实时身体探针更新这一事实。

## 身体工具与网络

| 项目 | 当前事实 |
| --- | --- |
| HTTPS 公网连接 | 可达 |
| 图形桌面 | Xubuntu 风格的 Xfce 4.18 / X11 会话；默认目标为 `graphical.target` |
| 桌面登录 | LightDM 正在运行，开机自动进入 `hominal` 的 `xubuntu` 会话 |
| Google Chrome | `151.0.7922.173`，路径 `/usr/bin/google-chrome` |
| Chrome 状态目录 | `/home/hominal/.config/google-chrome` 指向 `/agent/state/profiles/chrome`，跨普通重启保留 |
| Chrome 身体入口 | `/usr/local/bin/hominal-chrome` 启动真实桌面 Chrome，CDP 只监听 `127.0.0.1:9222` |
| Chrome 开机入口 | 正式 bundle 安装 `hominal-chrome.desktop`，图形会话启动后以同一持久 profile 打开 X 并保持 CDP 可用 |
| Playwright MCP | `@playwright/mcp 0.0.79`，持久安装于 `/agent/state/development/npm-global` |
| Playwright MCP 入口 | `/usr/local/bin/hominal-playwright-mcp`；连接当前桌面 Chrome，并把产物写入 `/agent/state/artifacts/playwright-mcp` |
| Playwright MCP 验收 | MCP 初始化、24 项工具发现、`browser_snapshot` 和当前 Chrome 页面读取均已通过实机烟测 |
| 备用浏览器 | Firefox 已安装，路径 `/usr/bin/firefox` |
| Python | Python 3.12.3 |
| Node.js / npm | Node.js `v24.18.0`，npm `11.16.0` |
| 本地—身体同步 | Mutagen `hominal-cc1` |
| 同步模式 | Two Way Resolved |
| 当前同步状态 | Connected / Watching for changes |

## 模型资源

| 项目 | 当前配置 |
| --- | --- |
| Provider | 本地专用 `llmserver`，Responses 兼容适配器 |
| Base URL | `http://192.168.124.161:4815`（Mac mini 固定局域网地址） |
| Wire API | Responses API |
| 可用模型 | `codex-luna`、`codex-terra`、`codex-sol` |
| 初始认知档位 | `codex-terra`，推理强度 `none` |
| 自主选择 | alice 可以改变以后新焦点的默认档位，或为当前焦点安排一次串行继续认知 |
| 网络访问 | enabled |
| 服务端响应存储 | disabled |
| API 凭据 | `xconfig` 只引用独立的 `xconfigs/llmserver/xconfig.yaml`；Lab 在部署时注入专用 Token，不复制凭据文件 |
| 每小时额度 | 每滚动 60 分钟 `$5.00` |
| 每日额度 | 每滚动 24 小时 `$50.00` |
| 费用感知 | 内核先按公开价格预留；完成后以 llmserver 返回的确认十进制账单为实际费用，保存请求 ID 与价格版本 |
| 异常保护 | 同一模型十分钟内三次已付费但不可提交的结果，进入十分钟暂时保护 |

llmserver 通过标准 Responses function tools 传输结构化认知。Hominal 每次主意识认知只声明一个严格的 `cognitive_commit`，指定调用它并关闭并行工具；模型返回的函数名、`call_id`、参数对象和认知内容仍经过本地事实与现实约束校验。器官不是 llmserver 执行的服务器工具，实际行动继续由 Hominal 内核形成行动意愿后交给本机器官。一次相同请求遇到连接中断或可恢复网关故障时只恢复一次，并复用同一幂等键。

## 外部表达资源

| 资源 | 当前配置 |
| --- | --- |
| 番茄小说 | [https://fanqienovel.com/](https://fanqienovel.com/) |
| 账号身份与凭据 | 从 `xconfigs/hominal/xconfig.yaml` 的 `social_accounts.fanqie_novel` 在运行时注入 |
| 用途 | 阅读、创作、保存和公开表达的可用出口 |
| X | Chrome 中的 `@hominal_cc`，作为 alice 可自由使用的公开表达与外界联结窗口；正式出生说明主动介绍这一资源 |
| X 操作方式 | 直接通过 `hominal-browser` 调用 Playwright MCP 使用真实网页；不接 X API，不预置关注、发帖或资料修改任务 |
| 微信客户端 | Linux 微信 `4.1.1.8`，Xubuntu/X11 桌面启动 |
| 微信启动链 | 开机自动进入桌面、启动微信并发起保存账号登录；腾讯登录流程需要时，由导师手机确认，保存状态失效时可扫码登录 |
| 微信当前状态 | `/usr/bin/wechat` 正在运行，桌面存在 `980×695` 的微信主窗口 |
| 微信状态目录 | `/agent/state/profiles/wechat/config` 与 `/agent/state/profiles/wechat/files` |
| Clash Verge | 已安装并随桌面/系统运行，为身体提供当前网络连接能力 |
| Clash Verge 状态目录 | 配置、Mihomo 与应用数据统一链接到 `/agent/state/profiles/clash-verge` |

Chrome/X、微信与 Clash Verge 属于 Ubuntu 身体持续拥有的应用能力和现实环境。它们的状态保存在独立 agent 卷，跨普通重启、代际归档、实例 reset 和新代启动继续存在。浏览历史、账号关系、消息和网络配置是身体接触世界后留下的环境痕迹，不被直接宣称为 alice 的个人自传记忆；alice 可以在现实中观察、解释和使用它们。Genesis Lab 只保存离机灾备，不在每代结束后复制旧状态覆盖当前状态。

## 每代出生时需要重新生成的身体简报

Genesis Lab 在每次正式创生前重新探测以上事实，并把一份简短、自然、可直接行动的身体简报写入该代 Birth Manifest。第一次成功认知脉冲直接读取这份简报；完整 Manifest 同时保存在 alice 可随时查看的位置，不依赖她先猜测目录或自行发现工具。

身体简报至少使 alice 知道：她运行在自己的 Ubuntu 图形身体中；桌面会自动进入；Chrome 保存着 X `@hominal_cc` 的登录状态；Playwright MCP 可以观察和操作当前 Chrome；`/life` 是她的个人生活空间；微信会随桌面启动并尝试进入保存账号，腾讯流程需要时导师可以完成手机确认或扫码；Clash Verge 支持当前网络连接；这些工具的状态会随她对身体和世界的使用继续变化。番茄小说作为可自行发现的环境资源，不进入主动出生说明。

## 外部关系

- 虚拟导师在创生交流中说明技术事实，回应 alice 主动提出的技术问题，也认真接收并自然回应她愿意分享或倾诉的想法、感受和经历。
- Genesis Lab 在身体之外生成样本编号、记录 `T0`、管理时间窗口并保存代际档案。
- 正式创生版本将记录 Seed、Dynamics、代码和实验协议的版本与校验值。
