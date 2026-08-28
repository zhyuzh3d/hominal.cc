# Ubuntu XFCE 智能体运行环境执行计划

## 目标

在现有 Ubuntu 24.04 物理系统上建立一个真实、无沙箱的智能体运行环境。智能体可以直接操作系统、Google Chrome 和微信；Mac 通过 SSH、Mutagen 和同桌面 VNC 进行部署、查看与恢复。

系统根卷 `/` 继续作为可被智能体完整修改的系统卷；`/agent` 作为不随根卷回滚的持久卷。最终稳定系统创建 40 GiB LVM 基线快照，极端情况下由 Mac 远程触发合并并重启。

## 已确认的初始状态

- Ubuntu 24.04，当前默认启动目标为 `multi-user.target`。
- 根逻辑卷 `ubuntu-vg/ubuntu-lv` 为 128 GiB，约 104 GiB 可用。
- 持久逻辑卷 `ubuntu-vg/agent` 为 40 GiB，挂载到 `/agent`。
- 卷组在创建 Agent 卷后仍有约 52.52 GiB 空闲。
- 当前 HDMI 桌面输出为 1280×720。
- Mutagen 会话 `hominal-cc1` 已同步到 `/agent/app/hominal.cc`。

## 执行原则

1. 始终先验证 SSH 可用，避免桌面安装破坏远程管理。
2. 使用 `xubuntu-desktop-minimal --no-install-recommends`，只补齐中文、音频、授权代理、托盘、终端和 X11 自动化能力。
3. LightDM 自动登录 `hominal`，所有 GUI 程序运行在本地 Xorg `:0` 会话。
4. 使用只监听 `127.0.0.1` 的 x11vnc 查看同一桌面；外部访问必须经过 SSH 隧道，不开放 5900。
5. Chrome、微信及智能体状态落在 `/agent/state`；项目落在 `/agent/app`。
6. 不触碰已发现的额外 `/dev/sdb` 设备。
7. 只有桌面、GUI 应用、持久化路径、自动启动与恢复脚本全部验收后，才创建系统基线快照。

## 执行步骤

### 1. 桌面与工具安装

- 更新 APT 索引。
- 安装 Xubuntu Minimal、中文字体与输入法、PipeWire 控制、PolicyKit、XFCE 托盘与终端。
- 安装 x11vnc、xdotool、wmctrl、xclip、scrot、rsync 等智能体观察和操作工具。
- 将系统默认启动目标设为 `graphical.target`，启用 LightDM。

验收：软件包配置完成，`dpkg --audit` 无输出，LightDM 已启用。

### 2. 持久图形会话与远程观看

- 配置 LightDM 自动登录 `hominal` 和 Xubuntu Session。
- 安装 XFCE 会话启动器，关闭屏幕休眠，并尝试启动 `/agent/app/current/start`。
- 安装 x11vnc systemd 服务，只监听本机回环地址并连接 Xorg `:0`。
- 重启后验证 Xorg、XFCE、LightDM、D-Bus 用户会话和 x11vnc。

验收：重启后无需人工登录即可出现桌面；5900只监听 `127.0.0.1`；Mac 可经 SSH 隧道查看同一桌面。

### 3. Chrome 与微信

- 安装 Google 官方稳定版 amd64 Chrome。
- 安装腾讯官方 `WeChatLinux_x86_64.deb`。
- 将 Chrome 用户配置链接到 `/agent/state/profiles/chrome`。
- 首次运行微信后识别其实际数据目录，再迁移到 `/agent/state/profiles/wechat`。
- 在 Xorg `:0` 中启动两个程序并截屏验证。

验收：Chrome 进程、调试端口和页面渲染正常；微信进程与登录界面正常；两者均可在 VNC 同桌面中看到。

### 4. 项目部署路径迁移

- 刷新并暂停现有 Mutagen 会话。
- 将旧远端目录保存到 `/agent/backup/migrations`，不直接删除。
- 重新创建 `hominal-cc1` 会话，目标改为 `/agent/app/hominal.cc`。
- 建立 `/agent/app/current -> /agent/app/hominal.cc`。
- 将兼容路径 `/home/hominal/hominal.cc` 指向 `/agent/app/hominal.cc`。
- 更新项目上层 `xconfig.yaml` 中的远端项目路径和同步端点。
- 配置 SSH 在 `agent.mount` 完成挂载尝试后再接受连接，避免重启期间 Mutagen 将未挂载路径误判为根删除；挂载失败不阻止 SSH 修复。

验收：Mutagen 为 Watching、无冲突；本地与新远端关键文件哈希一致。

### 5. Agent 代际备份与系统恢复

- 安装 `/usr/local/sbin/hominal-agent-backup`，以硬链接增量方式保存 `/agent/app` 和 `/agent/state`。
- 配置每日代际备份 timer，并在系统回滚前强制生成一代备份。
- 保存 `/boot`、EFI、已安装软件包清单和 LVM 元数据到 `/agent/recovery/system-baseline`。
- 创建 40 GiB `system-baseline` LVM 根卷快照。
- 安装回滚命令和快照自动重建服务。经典 LVM 根卷合并需要两次卷激活：第一次启动完成数据合并并自动再次重启，第二次启动清理旧映射并重新建立同名基线快照。

验收：快照状态正常，COW 使用率可读，卷组仍保留约12.5 GiB空闲；回滚检查命令全部通过。

### 6. 受控回滚测试

- 在根卷创建一次性探针，在 `/agent/state` 创建持久探针。
- 执行一次真实系统回滚并重启。
- 验证根卷探针消失、Agent 探针保留。
- 验证系统快照自动重建、LightDM/XFCE/x11vnc/Chrome/微信运行环境恢复。

## 回滚和风险控制

- LightDM 配置错误：通过 SSH 删除对应 drop-in 并恢复 `multi-user.target`。
- GUI 包安装失败：执行 `dpkg --configure -a` 和 `apt-get -f install`，不强杀 dpkg。
- VNC 服务失败：不影响 SSH；保持5900不对局域网开放。
- 根卷快照接近75%：立即告警；接近90%时停止高写入实验并选择回滚或重建基线。
- 同盘快照与代际目录不能抵御物理磁盘损坏；后续应由 Mac 定期拉取 `/agent/generations` 和 `/agent/recovery`。

## 最终交付

- 可持续运行的 XFCE/Xorg `:0` 图形环境。
- 可由智能体直接操作的 Chrome 和微信。
- Mac 可通过 SSH 隧道查看同一个桌面并访问开发端口。
- `/agent` 持久运行数据、代际备份和系统恢复资料。
- 可重复执行的根卷快速回滚机制及一次真实回滚验收记录。

## 执行结果（2026-08-24）

本计划已全部执行。服务器运行 Ubuntu 24.04、内核 `7.0.0-30-generic`，默认进入 `graphical.target`；LightDM、Xorg、XFCE、x11vnc 和 Agent 每日备份 timer 均为 active。桌面在重启后自动登录，按照物理显示器的实际分辨率固定为 1280×720，x11vnc 仅监听回环地址。

Google Chrome `151.0.7922.173` 和微信 `4.1.1.8` 已安装。真实 Xorg 桌面验收确认 Chrome 能渲染中文和执行 JavaScript，CDP 仅监听 `127.0.0.1:9222`；微信登录窗口正常出现。Chrome 的异常关机残留锁和恢复气泡已由专用启动器处理。两者的用户状态均持久化到 `/agent/state/profiles`。

Mutagen 会话 `hominal-cc1` 已迁移到 `/agent/app/hominal.cc`，状态为 Watching；`/agent/app/current` 与旧项目路径均指向新目录。项目上层 `xconfig.yaml` 的项目路径和同步端点已同步更新。旧远端目录保存在 `/agent/backup/migrations`，未直接删除。

根卷为 128 GiB，`/agent` 为 40 GiB，系统基线快照为 40 GiB，卷组剩余 12.52 GiB。已建立 10 代 Agent 备份。回滚机制经过两轮真实探针测试，其中完整自动测试确认根卷探针被恢复、`/agent` 探针保留、两阶段自动重启结束后同名基线快照自动重建；随后又以最终配置刷新了基线。快照通过 udev 规则对 XFCE/udisks 隐藏，创建脚本也会强制确认其未被挂载，避免桌面自动挂载污染 COW。当前快照使用率约 0.01%，`dpkg --audit` 为空，待升级软件包为 0。

唯一尚未执行的是“启动实际智能体业务程序”：当前项目没有 `/agent/app/current/start` 可执行入口。桌面启动器已经预留自动调用逻辑，项目提供该入口后即可随 XFCE 会话启动；这不影响本计划中的操作系统、GUI、同步、持久化、备份和回滚环境验收。

后续补充安装官方 Go `1.27.0` 和 Node.js `24.18.0` LTS（含 npm），并分别通过编译运行和 JavaScript 执行测试。Mac 端同桌面连接改用 `ops/macos/hominal-vnc`：脚本使用本地 15900 端口在后台建立 SSH 隧道，检查 RFB 握手成功后再启动系统“屏幕共享”，避免前台 `ssh -N` 阻塞后续命令。x11vnc 使用持久卷中的权限 600 密码文件认证，连接时输入 Ubuntu 系统密码；服务仍只监听回环地址。最终通过 macOS“屏幕共享”完成真实认证，并持续显示 1280×720 XFCE 桌面，服务器端会话保持 ESTABLISHED。
