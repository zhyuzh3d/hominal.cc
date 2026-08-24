# G0 目标身体当前事实快照

> 状态：阶段一目标身体快照；正式创生时由 Genesis Lab 重新探测并生成该代 Birth Manifest  
> 事实时间：2026-08-24 15:03:22 +08:00  
> 事实来源：上层 `xconfig.yaml` 与目标 Ubuntu 设备实时读取

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
| 根卷恢复快照 | 唯一快照 `ubuntu-vg/system-baseline`，40 GiB COW，当前数据占用约 0.14% |
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

阶段一将在正式创生前建立可由 alice 自主使用的 root 通道，并由实时身体探针更新这一事实。

## 身体工具与网络

| 项目 | 当前事实 |
| --- | --- |
| HTTPS 公网连接 | 可达 |
| 图形桌面 | Xubuntu 风格的 Xfce 4.18 / X11 会话；默认目标为 `graphical.target` |
| 桌面登录 | LightDM 正在运行，开机自动进入 `hominal` 的 `xubuntu` 会话 |
| Google Chrome | `151.0.7922.173`，路径 `/usr/bin/google-chrome` |
| Chrome 状态目录 | `/home/hominal/.config/google-chrome` 指向 `/agent/state/profiles/chrome`，跨普通重启保留 |
| Chrome 身体入口 | `/usr/local/bin/hominal-chrome` 启动真实桌面 Chrome，CDP 只监听 `127.0.0.1:9222` |
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
| Provider | OpenAI-compatible |
| Base URL | `https://ai.ai-mesh.cn` |
| Wire API | Responses API |
| 主模型 | `gpt-5.5` |
| Review 模型 | `gpt-5.5` |
| 推理强度 | 默认 `low`；当前焦点达到动力学升级条件时使用 `high` |
| 网络访问 | enabled |
| 服务端响应存储 | disabled |
| API 凭据 | 从 `xconfig.yaml` 的环境配置在运行时注入 |
| 每小时额度 | 每滚动 60 分钟 `1,000,000 total_tokens`；身体内本地用量账本读取，Genesis Lab 独立复核 |

## 外部表达资源

| 资源 | 当前配置 |
| --- | --- |
| 番茄小说 | [https://fanqienovel.com/](https://fanqienovel.com/) |
| 账号身份与凭据 | 从 `xconfig.yaml` 的 `social_accounts.fanqie_novel` 在运行时注入 |
| 用途 | 阅读、创作、保存和公开表达的可用出口 |
| 微信客户端 | Linux 微信 `4.1.1.8`，Xubuntu/X11 桌面启动 |
| 微信启动链 | 开机自动进入桌面、启动微信并发起保存账号登录；腾讯登录流程需要时，由导师手机确认，保存状态失效时可扫码登录 |
| 微信当前状态 | `/usr/bin/wechat` 正在运行，桌面存在 `980×695` 的微信主窗口 |
| 微信状态目录 | `/agent/state/profiles/wechat/config` 与 `/agent/state/profiles/wechat/files` |

当前 Chrome、Playwright MCP 和微信目录属于开发身体的持久配置，不直接作为正式创生代的跨代继承。正式实验由 Genesis Lab 在身体外保存一份干净账号会话基线，每代重新复制；上一代运行后的完整 profile 进入该代档案，避免把浏览历史、标签页、消息和个人文件误作下一代记忆。

## 每代出生时需要重新生成的身体简报

Genesis Lab 在每次正式创生前重新探测以上事实，并把一份简短、自然、可直接行动的身体简报写入该代 Birth Manifest。第一次成功认知脉冲直接读取这份简报；完整 Manifest 同时保存在 alice 可随时查看的位置，不依赖她先猜测目录或自行发现工具。

身体简报至少使 alice 知道：她运行在自己的 Ubuntu 图形身体中；桌面会自动进入；Chrome 保存着连续使用状态；Playwright MCP 可以观察和操作当前 Chrome；微信会随桌面启动并尝试进入保存账号，腾讯流程需要时导师可以完成手机确认或扫码；这些工具的当前运行状态仍由她在现实中继续观察和校准。

## 外部关系

- 虚拟导师在创生交流中说明技术事实并回应 alice 主动提出的技术问题。
- Genesis Lab 在身体之外生成样本编号、记录 `T0`、管理时间窗口并保存代际档案。
- 正式创生版本将记录 Seed、Dynamics、代码和实验协议的版本与校验值。
