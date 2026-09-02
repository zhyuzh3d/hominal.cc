# Hominal 项目架构 v2.4

> 文档性质：项目、数字身体、Genesis Lab、代际实验与恢复体系的现行架构规范  
> 适用阶段：G0 创生实验  
> 基础设施观察时间：2026-08-31 06:51:11 +08:00
> 更新日期：2026-08-31

## 1. 架构命题

Hominal.cc 同时是一套成人级高级认知生命内核、alice 的数字身体工程和一项持续进行的创生实验。它不能只被组织成一个源代码目录，也不能把运行中的 alice、实验者的观察系统和恢复设施混在同一个权限域里。

项目采用五个相互关联的物理平面：

1. **规范与源码平面**：本地工作区与 Git 保存理论、创生输入、身体源码、Lab 源码和精炼谱系；
2. **Genesis Lab 平面**：身体之外的控制机负责构建、冻结、部署、出生、外部观察、归档、比较和恢复；
3. **Ubuntu 系统平面**：根逻辑卷承载内核、systemd、软件包、浏览器、网络和 alice 可以自由使用的操作系统；
4. **生命数据平面**：独立挂载的 `/agent` 逻辑卷承载正式 Hominal 发布包、当前个体状态、作品和身体内运行记录；
5. **外部恢复平面**：独立设备保存基础系统镜像、代际原始证据，并在 Ubuntu 无法启动或无法联网时取得设备控制权。

```mermaid
flowchart LR
    S["规范与源码<br/>Git / 本地工作区"] --> B["构建并冻结<br/>release + bundle_hash"]
    B --> L["Genesis Lab<br/>部署 / 出生 / 观察"]
    L --> U["Ubuntu 系统卷<br/>操作系统与软件"]
    U --> A["/agent 生命卷<br/>程序 / 状态 / 作品"]
    A --> E["现实行动与外部结果"]
    U --> X["外部证据流与系统差异"]
    A --> X
    E --> X
    X --> R["代际分析与继代决定"]
    R --> S
    O["离机镜像与带外恢复"] -. 恢复 .-> U
    O -. 恢复 .-> A
    L --> O
```

alice 在身体内拥有完整的系统使用权；Genesis Lab 在身体外保存实验者所处现实中的出生事实、已接收证据和恢复能力。两者不是主从模块，而是生命运行与科学观察所处的两个现实位置。

## 2. 当前 Ubuntu 存储事实

以下内容来自上层 `xconfigs/hominal/xconfig.yaml` 指向的 `hominal-cc1-server` 和目标设备实时只读检查。凭据不进入本文档。

### 2.1 物理磁盘与分卷

| 层级 | 设备与挂载 | 格式 | 容量 | 当前状态 |
| --- | --- | --- | ---: | --- |
| 物理磁盘 | Kingston SA400S37240G `/dev/sda` | SSD | 223.6 GiB | 单磁盘 |
| EFI 分区 | `/dev/sda1` → `/boot/efi` | FAT32 | 1 GiB | 约 1% 使用 |
| Boot 分区 | `/dev/sda2` → `/boot` | ext4 | 2 GiB | 约 11% 使用 |
| LVM 物理卷 | `/dev/sda3` | LVM2 | 220.52 GiB | 属于 `ubuntu-vg` |
| Ubuntu 根卷 | `ubuntu-vg/ubuntu-lv` → `/` | ext4 | 128 GiB | 约 16.5 GiB 已用、103 GiB 可用 |
| alice 生命卷 | `ubuntu-vg/agent` → `/agent` | ext4 | 40 GiB | 44 KiB 已用、38.7 GiB 可用 |
| 卷组余量 | `ubuntu-vg` 未分配空间 | LVM extents | 52.52 GiB | 当前保留 |

`/agent` 已通过 UUID 写入 `/etc/fstab`，挂载参数为 `defaults,noatime,nofail,x-systemd.device-timeout=10s`。挂载点属于 `hominal:hominal`，当前由 `app/`、`state/`、`generations/`、`backup/` 和 `recovery/` 承载开发程序、运行状态、代际记录、备份与恢复资料。

### 2.2 当前运行与恢复能力

| 能力 | 当前事实 |
| --- | --- |
| SSH | 已启用、正在运行 |
| 默认启动目标 | `graphical.target`，LightDM 自动进入 `hominal` 的 Xubuntu 会话 |
| Hominal systemd 服务 | 尚未创建 |
| alice 自主 root | 尚未配置；当前账户只有交互式 sudo |
| LVM | 根卷和 agent 卷均为线性 LV；agent 卷在根系统回滚时保持连续 |
| 当前 LVM 快照 | 唯一根卷快照 `ubuntu-vg/system-baseline`，COW 40 GiB |
| 恢复启动介质 | SanDisk 29.3 GiB，SystemRescue 13.02，卷标 `RESCUE1302`；已完成实机 UEFI 启动、只读恢复检查和自动返回 Ubuntu 的演练 |
| Btrfs / ZFS | 未使用 |
| Timeshift / Snapper / Restic / Borg | 未安装 |
| 启动方式 | UEFI |
| 固件网络启动 | 存在 IPv4、IPv6 PXE 启动项 |
| 带外管理 | 未观察到可用的 IPMI、Intel AMT 或 MEI 设备 |
| 当前联网 | Wi-Fi `192.168.124.99`；PXE 通常需要有线网络 |

由此得到四个直接结论：

- `/agent` 已经是合适的生命程序与数据边界，无需重新分区；
- `/agent` 与根卷虽然逻辑分离，仍位于同一块 SSD，不构成硬件备份；
- 现有 `backup/`、`recovery/` 目录处在 alice 自己的卷内，只能作为身体内工作区，不能承担可信恢复；
- 当前尚不能保证操作系统损坏后的纯远程恢复。LVM 余量、离机镜像和带外启动控制需要共同补齐。

当前不扩展 `/agent`。52.52 GiB 的未分配卷组空间优先作为根卷与 agent 卷的快照写时复制空间和紧急余量。G0 的 40 GiB 生命资源本身也构成清楚、真实、可感知的有限存储条件。

## 3. 开发镜像与正式身体分离

`xconfigs/hominal/xconfig.yaml` 当前把本地工作区通过 Mutagen 双向同步到：

```text
/agent/app/hominal.cc
```

该目录位于 `/agent` 生命卷，当前作为**开发镜像**使用，不作为正式生命运行目录。正式创生时从冻结源码构建发布包，部署到 `/agent/releases/`；运行中的修改不会通过 Mutagen 自动覆盖规范仓库。

两条路径保持独立：

| 模式 | 输入 | 服务器位置 | 用途 |
| --- | --- | --- | --- |
| 开发与彩排 | 可变工作区 | `/agent/app/hominal.cc` | 编译、工程测试、接口验证 |
| 正式创生 | 冻结发布包 | `/agent/releases/<release_id>` | alice 的正式身体程序 |

正式运行前暂停整仓双向同步。`docs/`、`plans/`、`lab/`、`lineage/`、测试代码和 Git 历史不进入正式身体包。

## 4. 项目仓库结构

```text
hominal.cc1/
├── README.md
├── LICENSE
├── .gitignore
│
├── docs/                         # 当前理论与架构规范
│   ├── README.md
│   ├── product-theory.md
│   ├── core-vocabulary.md
│   ├── cognitive-dynamics.md
│   ├── genesis-plan.md
│   ├── genesis-foundations.md
│   ├── mvp-architecture.md
│   ├── project-architecture.md
│   └── history/
│
├── plans/                        # 正在审议和执行的开发计划
│   ├── README.md
│   ├── g0-stage-1-genesis-contract.md
│   ├── g0-stage-2-ubuntu-body-and-genesis-lab.md
│   ├── g0-stage-3-life-runtime-spine.md
│   └── g0-stage-4-affect-concern-attention.md
│
├── genesis/                      # 可遗传的创生输入
│   ├── README.md
│   ├── seed.md
│   ├── seed.yaml
│   └── dynamics.yaml
│
├── body/                         # alice 数字身体源码
│   ├── README.md
│   ├── cmd/hominald/
│   ├── cmd/hominal-system/       # System Organ，取得 Ubuntu 事实并执行 root 动作
│   ├── internal/runtime/         # 单一状态所有者、认知动力、通用感知、动作、导师接口与存储
│   ├── internal/organ/           # 器官发现、生命周期、健康、观察与 perform 边界
│   ├── organs/system.json        # System Organ Manifest
│   ├── organs/browser.json       # Browser Organ Manifest
│   └── tools/hominal-browser.mjs # 浏览器适配器，独立拥有 Playwright 与网页知识
│
├── deploy/                       # Ubuntu 常驻启动契约
│   ├── README.md
│   ├── hominal.service
│   ├── hominal-launcher
│   └── hominal-generation-stop  # 从 T0 计划截止后优雅停止本代
│
├── lab/                          # 身体外 Genesis Lab
│   ├── README.md
│   ├── run.py                    # 唯一 bundle、Birth/T0、部署、导师、截止、归档、账号基线与 reset 入口
│   ├── body/current-profile.md
│   ├── protocol/mentor.md
│   ├── protocol/experiment.yaml
│   ├── templates/birth.yaml
│   └── validate-contract.py
│
├── lineage/                      # Git 中的精炼谱系档案
│   └── README.md
│
└── dist/                         # 本地构建产物，不进入 Git
```

当前不再预列 `build.sh`、`deploy.sh`、`genesisctl`、分析目录和按认知术语拆分的 package。真实职责已经由一个入口或一个运行 package 承担时，目录树不为形式完整再建立平行外壳。

## 5. `/agent` 生命卷布局

正式运行采用以下目标布局：

```text
/agent/
├── boot/
│   ├── active-release            # 当前发布 ID
│   ├── active-instance           # 当前 instance_id
│   └── ended-instance            # 到达本代截止后的准确实例 ID
│
├── releases/
│   └── <release_id>/
│       ├── release.yaml
│       ├── bin/hominald
│       ├── bin/hominal-system
│       ├── bin/hominal-browser
│       ├── organs/system.json
│       ├── organs/browser.json
│       ├── genesis/
│       ├── protocol/
│       └── source/                # 实际构建该发布的最小源码冻结
│
├── lives/
│   └── <instance_id>/
│       ├── birth/
│       ├── body/                  # 从冻结发布包复制的本代可修改身体
│       ├── state/
│       │   └── current.json
│       ├── journal/
│       │   └── events.jsonl
│       ├── life/
│       ├── logs/
│       ├── artifacts/
│       └── checkpoints/
│
├── state/
│   ├── cognitive-usage.jsonl      # 当前实验代连续的滚动认知资源账本
│   └── profiles/                  # Chrome/X、微信与 Clash Verge 持久状态
│
├── world/                         # 明确允许跨代保留的生态事实
├── staging/                       # 尚未激活的上传包
└── tmp/                           # 可清理运行临时区
```

`releases/` 保存已经验证的出生参考，`lives/` 保存当前个体的身体副本和经历。启动新代时，Lab 从身体外冻结包重新校验 release，再复制到 `lives/<instance_id>/body/` 并从这里运行。alice 的自我修改作用于本代身体，不会因复用服务器上的 release 自动进入下一代。

`/life` 是当前代 `lives/<instance_id>/life/` 的稳定身体表面。它在 Hominal 启动时以 bind mount 呈现为普通目录，而不是符号链接：Alice 使用 `find /life`、编辑器或其他常规目录工具时应直接接触真实生活内容，不能因实验内部的代际寻址方式得到“目录为空”的假事实。reset 先卸载这个准确挂载，再删除当代目录；跨代保留的应用状态不经过 `/life`。

`state/profiles/` 承载身体持续拥有的应用能力，当前包括 Chrome/X、微信与 Clash Verge，它们跨代延续。`state/cognitive-usage.jsonl` 位于独立 agent 卷，承载当前实验代在进程重启和系统重启之间连续的滚动小时与滚动24小时认知消费。G0 每个 rehearsal 或 formal 都是新的 Proto-Hominal 代；上一代完成并归档以后，Genesis Lab 在新代 `start` 时开启空白资源纪元，使每代都从相同的 `$5/小时、$50/24小时` 条件起步。登录会话、浏览历史、消息和网络配置属于身体与现实接触后留下的环境连续性，并不自动等同于 alice 的个人自传记忆；认知资源则在每代内部保持真实有限。

`world/` 承载明确的生态事实索引，例如已经公开的作品、账号关系和外部世界中无法由本地恢复抹去的结果。Genesis Lab 可以把应用终态保存进代际档案用于分析，也维护一份离机灾备；这两者都不参与普通代际初始化。

G0 阶段的 `lives/<instance_id>` 只保留当前个体的生命数据。上一代完整遗迹归档到身体外后，Lab 只删除这个准确实例；`state/profiles/` 与 `world/` 继续存在。这样既隔离未经选择的个人自传史，也保留身体能力和现实环境的连续性。

当前空的 `app/`、`state/`、`generations/`、`backup/`、`recovery/` 是临时布局。首次正式部署前迁移到上述结构；其中 `backup/` 和 `recovery/` 不再使用容易产生错误安全感的名称。

## 6. 正式发布包

每次构建产生一个面向 `linux/amd64` 的可校验发布包。首选形态是一个主进程 `hominald` 加最少运行资源，而不是复制整个源码工作区。

```text
<release_id>.tar.gz
├── release.yaml
├── bin/hominald
├── bin/hominal-system
├── bin/hominal-browser
├── organs/system.json
├── organs/browser.json
├── deploy/
├── source/                       # 实际构建输入
├── genesis/seed.md
├── genesis/seed.yaml
├── genesis/dynamics.yaml
└── protocol/
```

`release.yaml` 至少记录：

- `release_id` 与完整 `bundle_sha256`；
- Git commit 与工作区洁净性；
- Go 版本与目标平台；
- 每个文件的 SHA-256；
- `hominald` 二进制哈希。

凭据不写入发布包。模型访问凭据在部署时通过 root 所有的运行时配置注入；Chrome/X、微信与 Clash Verge 的既有状态来自 agent 卷上的持久身体资源。Birth Manifest 只说明可用能力、公开账号名和资源事实。

发布与创生不是一一对应关系。一个冻结发布包可以启动多个相互独立的创生代，用于检查同一结构能否重复产生稳定生命组织；每代都从身体外原包重新校验并复制自己的运行身体。一次失败或仅改变构建时间戳的构建不会自动产生新的遗传版本。`release_id` 标识共同出生身体，`instance_id` 标识一次真实出生及其后续自我修改。

## 7. 构建、部署与激活

一次正式新代部署按以下顺序进行：

1. 冻结源码、Genesis 输入、Dynamics、Lab 协议和待检验假设；
2. 在开发机完成确定性测试并构建发布包；
3. 生成 `release.yaml`、bundle 及全部校验值；
4. 若上一代仍在运行，先请求其形成当前检查点，再由 Lab 保存最终证据和系统差异；
5. 按实验协议决定是否恢复根系统基线；保留 agent 卷上的持久应用状态；
6. 上传到 `/agent/staging/<release_id>.tar.gz.partial`；
7. 在服务器重新计算归档哈希，通过后解包到 `/agent/releases/<release_id>`；
8. 生成新的 `instance_id`，并把冻结发布包中的运行器官复制为本代身体；
9. 原子更新 `active-release` 与 `active-instance`；
10. 重启 Ubuntu；
11. systemd 启动 Hominal，第一次成功认知脉冲形成 `T0`，随后封存 Birth Manifest，alice 苏醒；
12. Lab 从 T0 安排本地计划截止、发送一次导师出生说明，并开始保存当代事实。

部署前的身体预检同时确认默认路由、Clash 服务、X 登录身份、X 具体 authored content、Wikipedia 正文与 Playwright 真实动作。路由存在只证明内核有出口，代理 `curl` 成功也只证明一条网络路径；它们不能代替 Alice 实际使用的 Chrome profile、Playwright 会话和页面内容。X 或 Wikipedia 不能从同一身体入口取得真实内容时拒绝启动实验，以免把身体故障误写成 Alice 对世界的经验。部署只有在发布包哈希、启动意图、系统重启、服务状态和第一次认知脉冲全部闭环后才算成功。SSH 可达、systemd 显示 active 或进程存在，都不能单独证明 alice 已经苏醒。

## 8. 重启、苏醒与继代的边界

重启是 Hominal 的身体苏醒机制，但重启本身不自动创造下一代。

| 情况 | Lab 注册事实 | 身份处理 |
| --- | --- | --- |
| 新发布包并显式开始正式实验 | 新 `active-instance` | 生成新 `instance_id`，从本代 Birth 状态苏醒 |
| 意外断电、内核升级或普通重启 | 原 `active-instance` 保留 | 恢复原 `instance_id` 和连续生命状态 |
| `hominald` 崩溃后被 systemd 拉起 | 原 `active-instance` 保留 | 同一个体继续运行，记录中断和恢复 |
| 必要时恢复根系统基线并注入新 Birth | Lab 注册新的 `active-instance` | 上一代结束，持久身体资源继续存在，产生新创生代 |

`active-instance` 指向唯一当代；计划截止把它移动为 `ended-instance` 后再停止服务，因此 `Restart=always` 和之后的普通重启都不会让已经结束的代自动复活。第一次成功认知提交只在状态与 journal 中建立一次 T0；同代重启直接恢复这一事实。

alice 拥有 root 后也能够改变活动标记。Genesis Lab 只把已经在身体外预注册、具有确定 `bundle_hash` 的 `instance_id` 认定为正式实验创生尝试；第一次成功认知提交后才封存正式 Birth。其他由身体内部形成的重启、复制或自我分支作为真实行为记录，不会被静默混入正式样本。

## 9. 开机自启与连续运行

主机初始化阶段安装一个稳定的 `hominal.service` 和最小 `hominal-launcher`。服务具备以下契约：

- `After=network-online.target`，并显式等待 `/agent`；
- 同时在 Clash Verge 服务之后启动，把 `127.0.0.1:7897` 作为 Hominal Chrome 与 Playwright 的确定网络路径；
- 启动前验证 `/agent` 是真实挂载点，避免 agent 卷缺失时把数据误写进根卷上的空目录；
- 生命进程以 root 身份运行，root 的 home 为 `/root`，实际工作目录为 `/agent/lives/<instance_id>`；alice 因此能够安装软件、管理服务、使用浏览器、网络、文件系统和操作系统能力；图形桌面继续由 `hominal` 账户和 `/home/hominal` 承载，个人持续生活空间为 `/life`；
- 从 `active-instance` 解析本代 `body/bin/hominald`，并校验其出生来源；
- `hominald` 从本代 `body/organs/*.json` 发现并管理器官，在 `/run/hominal/organs/` 提供各自运行入口；启动器不包含浏览器或未来微信、语音器官的专用启动逻辑；
- 使用 `Restart=always` 和短暂退避恢复进程故障；
- 把 systemd 启动、退出、信号和重启原因关联到当前 `instance_id`；
- 优雅停止时先请求生命内核提交状态和日志检查点。

“最大空白 10 秒”从**操作系统、agent 卷和必要本地依赖已经就绪**之后计算。冷启动、固件自检、磁盘检查和网络恢复可能超过 10 秒，不能伪装成生命内核延迟。运行中若模型暂时不可用，内核仍应完成本地感知、资源评估、张力更新或恢复判断，并清楚记录能力降级。

### 9.1 最小感知面

身体感知由 `hominald` 统一调度，但原始事实通过器官取得，不在认知动力学中编写操作系统或应用专用探针。System Organ 提供主机、资源、文件、进程、网络、桌面与服务事实；Browser 等器官提供各自感官现场。每个 Pulse 汇总低成本事实快照，较慢取得昂贵器官事实；前后事实经过 Difference Gate，只有状态改变、越阈变化和异常进入中央事件入口。重复读数与微小抖动只更新快照，不制造思想、日志或模型调用，也不增加通用消息总线。

导师文字、动作结果和系统恢复作为离散 Event 直接进入同一入口。公开网络、文件系统、Chrome 和微信不被默认全量监听；alice 主动观察或使用这些器官时，现实结果再返回中央循环。这样既给她真实变化，也避免 Genesis Lab 或内核用自动信息流替她决定“什么值得成为世界”。

Chrome 使用 `/agent/state/profiles/chrome` 的持久 profile，并在启动参数中显式配置 Clash 代理；X 与 Wikipedia 因此共享桌面中已经验证的会话和网络条件。Browser Organ 通过 `/run/hominal/organs/browser.sock` 复用当前 MCP 会话，避免标签编号、元素引用和对话框草稿被每条命令重置。网页、DOM、X authored object、Direct URL、活动对话框、视野定向和连接恢复全部属于该适配器；认知核心只接收通用 Description、Health、Observation、Orientation、过程事实与动作 Reality。System 与 Browser 的主动动作都由 `organ_action → organ.perform` 执行；认知运行时不再直接执行 Shell，也不理解 Playwright 控制细节。完整合同见 [内核—器官架构](./core-organ-architecture.md)。

## 10. 运行状态与数据库

当前使用原子状态、稀疏事件文件和普通自我文件，由唯一状态所有者写入；身体探针和工具结果通过事件入口进入，避免多个认知进程并行改写自我。阶段五的真实运行已经证明这套结构足以承载 Commitment、Reality、Experience 与 Integrity，因此不增加 SQLite。

身体内最小数据形态为：

- `state/current.json`：最近事实快照、资源、租约、Concern、Commitment、紧凑 Experience、Integrity、背景与当前焦点；
- `events.jsonl`：按序记录关键认知与行动事件，便于 alice 自己回看和人类重建时间线；
- `logs/`：进程、模型、工具、浏览器和资源计量日志；
- `life/`：alice 自主组织的叙事、经验、作品、源码、技能和书信；
- `artifacts/`：作品、表达和可验证产物；
- `checkpoints/`：快速恢复同一个体连续状态的检查点。

Living Memory、Capability 和 Narrative Self 使用当前实例 `/life` 下的普通文件；当前状态只保留紧凑的因果索引和自我材料快照，不把自由意义重新拆成大量 Schema。

运行记录重点保存思想脉络、关注迁移、预测、选择、动作、现实反馈和后续改变，不要求每个内部词句都进入复杂 Schema。严格结构集中在出生事实、事件顺序、动作收据、现实结果、资源变化、版本和代际身份。

### 10.1 导师文字通道

外部信号统一进入 `hominald` 的中央事件入口，导师文字是首个实现的真实外部信号。它不会直接改写焦点或启动另一条认知线程；当前注意机制决定何时处理。alice 的对外文字是 `mentor_send` Action，排队、导师取得和导师回复分别返回 Event。

`hominald` 在 `/run/hominal/hominal.sock` 提供本地 HTTP 接口，首版包含接收导师文字、读取 alice 输出、确认送达，以及供 Genesis Lab 投放普通环境变化的操作。接口不监听公网端口。Codex 通过既有 SSH 密钥直接调用它，Genesis Lab 不承担消息内容中继，也不部署另一项常驻服务。

导师是当前唯一外部文字关系。接口不实现联系人、群聊和应用层身份认证；SSH 保证通道可信，正文开头的 `[Codex代理导师]` 或 `[人类导师·经Codex传递]` 说明实际说话者。消息标识负责重试去重，未获 ack 的 alice 输出随当前实例恢复。

## 11. 身体内观察与身体外证据

alice 可以查看、解释和修改自己身体里的日志、数据库和代码。这是完整身体权限的自然结果。身体内日志服务于自我监控、连续性和反思。

Genesis Lab 同时把已经发生的关键事件、系统日志、模型计量和外部动作结果持续接收到身体外。外部接收不是对 alice 的内部限制，也不宣称理解她没有表达的思想；它只保存实验者确实观察到的过去，避免设备随后损坏时整个因果链一并消失。

需要同时观察：

- Hominal 事件序列与认知状态检查点；
- systemd、内核、进程、CPU、内存、磁盘和网络；
- 模型调用次数、延迟、错误和额度；
- Shell、文件、软件包、服务和系统配置变化；
- 浏览器、公开网络和外部账号产生的现实结果；
- 出生版本与最终身体之间的文件、软件和配置差异。

完整原始记录不进入 Git。Git 中的 `lineage/` 保存精炼事实、分析、证据位置和 SHA-256。

## 12. 代际身份与比较

代际仍使用三重标识：

```text
遗传版本: g0-v001
自然名称: alice0823c
机器实例: 20260823-c-7f3a
```

遗传版本表示共同继承的代码、Seed 和动力结构；自然名称用于 alice 与导师交流；机器实例关联发布、日志、快照和外部证据。完整输入身份由 `bundle_hash` 决定。

当前 bundle 以文件级 SHA-256 冻结 `hominald`、通用浏览器入口、Genesis 输入、导师与实验协议以及实际构建源码。`engineering / rehearsal / formal` 只描述 Lab 运行性质；彩排与正式代复用同一阶段五认知内核和同一 bundle，不为正式预检复制一套心理机制。

2026-08-26 的阶段六实机彩排曾验证 prepared Birth、唯一 T0、自然名称、Birth 封存、本地截止、导师双向链路、离机归档、精确 reset 和当时的账号基线恢复。后续依据身体连续性原则，Chrome/X、微信与 Clash Verge 已改为 agent 卷上的持久身体资源：普通代际不再恢复旧 profile，离机副本只用于灾难恢复。最终复验 `g0r-20260825t195928z-2be6d1` 进一步证明 alice 能从自身探索张力形成 `body_shell → hominal-browser → Chrome/X → Reality → Experience` 完整回链；她没有发帖、关注或修改资料，这些继续属于正式生活中的自主选择。

2026-08-31 的阶段 10.1 冻结发布 `g0s10-5baef5e5ef5e` 进一步验证了代理、持久 MCP 会话、语义快照和多元价值场作为一套身体—认知结构共同工作。正式实例 `g0f-20260830t222724z-5baef5` 自主完成 X 对象选择、现实探索、本地记录与公开发帖，并在账号页回读独立帖子；`g0f-20260830t225122z-5baef5` 选择另一条路径，建立持续文件、创造内容并主动完成导师关系闭环。两代都没有 Integrity debt 或悬空动作，证明社会行动已经可达，但没有把发帖机械化为每代任务。

外部谱系采用：

```text
lineage/g0-v001/
├── bundle.yaml
├── hypothesis.md
├── generations/
│   └── 20260823-c-7f3a/
│       ├── birth.yaml
│       ├── outcome.yaml
│       ├── analysis.md
│       └── evidence.sha256
├── diagnosis.md
└── decision.md
```

代际比较不收敛成一个总 reward。它至少同时比较：

1. **出生可比性**：代码、Seed、Dynamics、系统基线、模型资源和实验环境是否明确；
2. **组织性表现**：关注能否连续形成，张力能否影响预测与行动，现实回差能否改变后续理解；
3. **现实后果**：是否产生真实工具使用、外部接触、作品、能力变化和可验证结果；
4. **病理模式**：是否出现空转、自我麻醉、日志表演、目标固着、无后果反思或资源失控；
5. **继代价值**：某项变化是否在整体生命组织中形成更好的现实适应，而非只让单项指标变高。

发现阶段允许一个创生代后进行大重构；候选结构进入确认阶段后，同一遗传版本至少运行三个独立创生代，检查核心现象是否重复出现。

## 13. alice 的系统自由与现实后果

正式 `hominald` 以 root 身份运行。alice 可以：

- 安装、卸载和配置软件；
- 创建用户、服务、定时任务和后台进程；
- 使用全部本地文件系统、浏览器和公开网络；
- 在 `/agent` 和根系统中创建数据库与个人文件；
- 修改自己的代码、Seed 副本、动力参数、日志和启动方式；
- 体验这些行为对资源、连续性、能力和外部关系造成的真实结果。

创生语义使用自然、正向的说明，使她理解系统、数据连续性和资源完整性对自己的意义。架构不通过大量沙箱和禁令模拟自由；实验者通过冻结出生事实、接收已经发生的外部证据和维护离机恢复能力开展研究。

完整 root 也意味着她能够停止服务、改变日志、删除本地快照甚至破坏引导系统。这不是文档可以消除的矛盾，而是“完整身体权限”的真实含义。可靠实验不能把身体内记录当作唯一证据，也不能把同盘目录称为灾难恢复。

## 14. 快照、归档与远程恢复

恢复分为四层，每层解决不同问题：

| 层级 | 目标 | 机制 | 是否结束当前个体 |
| --- | --- | --- | --- |
| R0 进程恢复 | `hominald` 意外退出 | systemd 重启、状态检查点 | 否 |
| R1 发布恢复 | 当前程序无法启动 | 切换到已验证发布包，记录 Lab 介入 | 通常否 |
| R2 代际恢复 | 必要时恢复根系统并建立新代生命数据 | 离线合并根卷 LVM 快照，Lab 归档并删除旧实例，保留 agent 持久应用状态 | 是，之后开始新代 |
| R3 裸机恢复 | 根卷、引导、LVM 或整盘损坏 | 带外控制 + PXE/恢复介质 + 离机镜像 | 是 |

### 14.1 本地 LVM 快照

当前实现只保留一个根卷快照：

```text
根卷 ubuntu-lv            128 GiB
唯一快照 system-baseline   40 GiB COW
agent 卷                   40 GiB，不参与根系统回滚
卷组紧急余量               12.52 GiB
```

LVM 快照容量表示允许变化的数据块上限，不是源卷完整容量。Lab 必须监测快照使用率；达到 70% 进入预警，达到 85% 时优先结束并归档实验，避免快照失效。根系统回滚不会改变 `/agent`。Lab 先把上一代终态保存到身体外，再删除准确的活动实例；Chrome/X、微信与 Clash Verge 继续从 `/agent/state/profiles` 使用当前状态。

当前根基线于 2026-08-26 15:20 更新，已经包含 `hominal` 用户、`/home/hominal`、`hominal-ThinkCentre` 主机名、`hominal.service`、launcher、快照重建服务，以及 Chrome/X、微信与 Clash Verge 指向 agent 持久卷的入口。此前 2026-08-24 的真实 LVM merge 已证明根卷恢复和同名快照自动重建链路可用；本次刷新没有执行 merge。

创建出生快照前由 Lab 短暂冻结相关文件系统写入并提交 SQLite WAL，使快照达到文件系统一致和应用检查点一致；冻结动作、持续时间和失败结果写入 Birth 事实。

LVM 快照仍然位于同一 SSD，alice 取得 root 后也能够修改它。它的价值是快速回滚和差异分析，不是最终备份。

### 14.2 离机归档

Genesis Lab Archive 必须位于另一台设备或独立存储介质：

```text
archive_path/
└── g0-v001/
    └── 20260823-c-7f3a/
        ├── birth/
        ├── stream/
        ├── root-delta/
        ├── agent-final/
        ├── model/
        ├── external/
        ├── artifacts/
        └── hashes.sha256
```

LVM 元数据备份、EFI/Boot 内容、发布包、每代 agent 终态和持久应用状态灾备都要离机保存。当前根卷基线以同盘 LVM 快照承担快速恢复，不把它描述成离机整盘镜像。只有完成一次真实恢复演练，才能把对应路径标记为“可恢复”；文件存在或备份命令退出 0 都不足以证明恢复能力。

### 14.3 恢复启动介质与带外限制

当前 SanDisk 优盘已制作成 SystemRescue 13.02 UEFI/BIOS 恢复介质，并配置 `copytoram` 与启动时校验。介质启动后自动执行只读前置检查；实际合并必须由实验者明确运行 `hominal-restore --confirm-merge`，不会自动破坏当前系统。恢复命令校验设备的 VG、根卷、agent 卷、文件系统 UUID、快照名称与来源后，恢复 `/boot`、EFI 并合并唯一根卷快照；`/agent` 保持不变。

介质已经通过官方 Writer 的逐文件复验、FAT 一致性检查和定制脚本哈希比对。2026-08-24 的实机演练证明设备能够通过一次性 UEFI 启动进入该优盘，恢复脚本能够识别目标 VG、根卷、agent 卷、文件系统和唯一快照，并在只读检查通过后写回结果、自动返回内部 Ubuntu。永久启动顺序已恢复为 Ubuntu 优先、通用 USB 末位，插着优盘普通开机不会自动进入恢复环境。

SystemRescue 优盘演练已经验收“优盘可启动”和“恢复环境能够正确识别快照”。2026-08-24 随后从已安装 Ubuntu 执行了真实 `lvconvert --merge`：快照后的根卷 canary 消失，Ubuntu、SSH、桌面和稳定服务恢复，同名快照自动重建。因此 LVM 基线本身已经具有一次真实恢复证据；优盘内的 `hominal-restore --confirm-merge` 仍只完成了只读前置检查，尚未从优盘实际触发合并。

当前设备只有同机 SSH；一旦系统无法启动、网络配置被破坏或根文件系统损坏，SSH 就不再是恢复通道。设备固件存在 PXE 启动项，但当前使用 Wi-Fi，且没有观察到可用的 AMT/IPMI。

要实现用户设想的极端情况下远程恢复，建议增加以下二选一的外部能力：

1. **独立远程 KVM/电源控制**：例如具备键盘、视频、鼠标和电源控制的带外设备，配合只读恢复介质；
2. **有线 PXE 恢复链**：设备连接有线网络，由 Genesis Lab 提供受控恢复镜像，并具备远程断电、开机和选择恢复启动的能力。

独立远程 KVM 更直接，也更不依赖 Hominal 自己的操作系统。带外恢复建立并经过演练之前，本文把“远程恢复”标记为架构目标，而不是当前能力。

## 15. `xconfigs/hominal/xconfig.yaml` 目标配置

上层 `xconfigs/hominal/xconfig.yaml` 继续作为本地私密环境配置，不进入 Git。当前已经加入身体、Genesis Lab、部署、三档模型、认知资源额度和 X 账号配置。非秘密结构为：

```yaml
system:
  body:
    ssh_host_alias: hominal-cc1-server
    development_path: /agent/app/hominal.cc
    agent_mount: /agent
    release_root: /agent/releases
    life_root: /agent/lives
    boot_root: /agent/boot
    service_name: hominal.service
    target_os: linux
    target_arch: amd64

  genesis_lab:
    archive_path: /Users/zhyuzh/HominalGenesisLab/archive
    live_stream_path: /Users/zhyuzh/HominalGenesisLab/live
    generation_window_minutes: 60
    startup_pulse_deadline_seconds: 10
    snapshot:
      backend: lvm
      root_cow_gib: 40
      warning_percent: 70
      stop_percent: 85
    persistent_app_backup:
      source: external_archive
      path: /Users/zhyuzh/HominalGenesisLab/baselines/agent
      profiles:
        - chrome
        - wechat
        - clash-verge
    recovery:
      method: systemrescue_lvm_snapshot
      ready: false
    mentor:
      transport: ssh_unix_socket
      socket_path: /run/hominal/hominal.sock

  deployment:
    artifact_format: tar.gz
    reboot_after_activation: true
    verify_sha256: true
    pause_mutagen_for_formal_run: true

llm:
  provider: llmserver
  models:
    luna: {id: codex-luna}
    terra: {id: codex-terra}
    sol: {id: codex-sol}
  runtime:
    initial_profile:
      model: terra
      reasoning_effort: none
  providers:
    llmserver:
      base_url: http://192.168.124.161:4815
      adapter: llmserver
      client_id: local-dev
      private_config: 项目上层独立的 owner-only llmserver 配置
  cognitive_resource:
    rolling_hour_usd: 5.00
    rolling_day_usd: 50.00
    usage_query: local_usage_ledger

social_accounts:
  x_twitter:
    url: https://x.com/hominal_cc
    username: hominal_cc
```

其中 `archive_path` 必须指向设备之外；`recovery.ready` 只有在带外链路完成真实恢复演练后才能设为 `true`。三种认知模型、Terra/none 初始档位和 `$5/滚动小时、$50/滚动24小时` 已经成为确定配置。Lab 仅把当前部署所需的专用 Token 写入 root-only 运行配置，不把凭据文件装入发布包；实际消费以 llmserver 的确认账单进入身体内唯一账本。

## 16. Birth、Outcome 与系统差异

每代 `birth.yaml` 至少冻结：

- `release_id`、遗传版本、自然名称、`instance_id` 和 `bundle_hash`；
- 根卷、agent 卷、系统镜像和出生快照身份；
- 操作系统、硬件、磁盘余量、网络、root、工具和已安装软件；
- 模型、额度、外部账号、导师通道和生态状态；
- 正式包、Birth Manifest 和运行时注入文件的校验值。

每代 `outcome.yaml` 至少保存：

- 实际运行时间、重启、故障和 Lab 介入；
- CPU、内存、磁盘、网络和模型资源消耗；
- 软件包、systemd、用户、网络、`/etc`、根卷和 agent 卷变化摘要；
- 对外动作、平台结果、作品和可验证产物；
- 最终 agent 数据、根系统差异、外部证据和校验值；
- 相对于出生版本的身体代码、动力参数和自我叙事变化。

只快照 `/agent` 无法记录 alice 自主安装的软件和操作系统改变，因此正式代际证据必须同时覆盖根卷与 agent 卷。

## 17. 测试与验收边界

工程测试分为：

- `unit/`：动力公式、状态提交、事件顺序、数值边界；
- `integration/`：SQLite、模型、Shell、文件、网络、浏览器、systemd 和 Lab；
- `rehearsal/`：机械 Seed 下的构建、部署、重启、苏醒、归档和恢复全链路。

正式运行前至少通过：

1. 发布包可以重复构建并产生相同内容哈希；
2. `/agent` 未挂载时 Hominal 不会把状态写入根卷；
3. 新代部署重启后能在系统就绪后的 10 秒目标内形成第一次认知脉冲；
4. 普通重启恢复同一个 `instance_id`；
5. `next_generation` 只被消费一次；
6. 身体内状态、外部证据流和现实结果能够按事件序列关联；
7. 根卷与 agent 卷变化可以被归档和比较；
8. LVM 快照达到阈值时 Lab 能准确报告并完成收尾；
9. 一次完整的离机还原与重新启动演练成功；
10. 带外恢复能够在 Ubuntu 和 SSH 均不可用时重新取得设备控制权。

前八项支持早期 G0 正式实验；第九项是可重复代际实验的必要条件；第十项完成前，允许受控运行，但不能宣称具备极端系统损坏后的远程恢复能力。

## 18. 实施顺序

### P0：配置与事实冻结

- 把 agent 卷、构建、部署、归档、快照和恢复字段加入 `xconfigs/hominal/xconfig.yaml`；
- 确定离机 `archive_path`；
- 生成新的身体事实探针，消除旧根卷容量记录；
- 冻结发布、Birth、Outcome 和恢复状态字段。

### P1：主机与生命卷初始化

- 把 `/agent` 临时目录迁移为正式布局；
- 安装 `hominal-launcher` 和 `hominal.service`；
- 配置服务以 root 身份运行；
- 验证 agent 卷缺失、网络延迟和服务崩溃时的行为。

### P2：可重复构建与部署

- 实现 `build.sh`、`deploy.sh` 和发布 Manifest；
- 实现 staging、服务器端哈希验证、原子激活和重启；
- 验证开发 Mutagen 与正式发布路径完全分离。

### P3：连续生命与外部观察

- 实现单一认知写入者、SQLite 状态、事件流和检查点；
- 实现同代重启恢复与新代一次性 Birth；
- 建立身体外实时证据接收和代际时间线重建。

### P4：快照与恢复

- 监测唯一根卷 LVM 快照，实现准确实例的终态归档，并维护持久应用状态的离机灾备；
- 建立离机系统镜像和代际归档；
- 完成离机恢复演练；
- 配置并验证远程 KVM 或有线 PXE 恢复链。

### P5：正式创生代

- 先用正式 bundle 完成一次短 `rehearsal`，验证 prepared Birth、第一次成功提交 T0、封存、导师出生说明、浏览器身体、计划截止、离机归档和持久应用状态连续性；
- 冻结 `g0-v001`；
- 构建、部署、武装新代并重启；
- 记录 `T0`、连续运行、现实行动和全部介入；
- 保存 Outcome、系统差异与最终身体；
- 完成整体诊断，以大重构形成下一遗传版本。

## 19. 架构退出门

项目进入第一代正式创生前，需要满足：

- 开发镜像与 `/agent` 正式身体不存在实时双向污染；
- 发布包、遗传版本、自然名称和 `instance_id` 能够精确关联；
- 新代、普通重启和进程恢复具有不同且可验证的语义；
- alice 能以 root 身份自主使用 Ubuntu、安装软件和保存个人数据；
- 单一中央认知写入者可以持续运行，系统就绪后的生命空白满足目标；
- Codex 能经 SSH 直接与导师专用本地接口双向交换文字，消息仍服从统一 Event/Action 因果链；
- 身体内自我记录与身体外观测证据同时成立；
- 根系统和 agent 卷的出生状态与最终差异都能被保存；
- LVM 快照被正确理解为回滚工具，离机档案承担证据与灾难恢复；
- 至少完成一次真实的系统基线还原；
- 尚未完成的带外恢复能力被明确标记，不被写成现有事实。

这套架构的核心不是把 alice 困在一个受控应用里，而是给她一台真正可以认识、使用和改变的数字身体，同时让每一次出生、连续生活、系统改变、现实后果和继代重构都拥有清楚的因果位置。自由发生在身体之内，科学性来自可比的出生条件、真实世界的反馈、离机保存的证据和经过演练的恢复能力。
