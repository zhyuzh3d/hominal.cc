# G0 阶段二开发计划：最小可重复 Ubuntu 身体

> 对应总计划：[创生实验八阶段实施计划·阶段二](../docs/genesis-plan.md#阶段二可重复-ubuntu-身体与-genesis-lab)
> 版本：v0.2
> 日期：2026-08-24
> 状态：已完成
> 前置冻结：`g0-stage1-v001`（Git `0af06a5`）

## 一、阶段目的

本阶段只建立一条能够快速反复使用的身体循环：

```text
构建 → 部署 → 重启苏醒 → 持续运行 → 停止保存 → 恢复出生状态
```

它服务于尽快开发出能够持续、自主运行、主动扩展并形成进化能力的 alice。实验记录只保留足以判断失败原因和决定下一次根本重构的内容，不建设独立研究平台。

阶段二使用没有认知能力的机械测试体验证身体循环。测试体不扮演 alice，不产生正式 `T0`、样本编号或生命表现结论。生命运行脊柱从阶段三开始。

## 二、当前起点

目标 Ubuntu 已具备独立 root 卷和 agent 卷、Xfce 桌面、Chrome、Playwright MCP、微信、Go、Node.js、Python、网络和模型访问条件。SystemRescue 优盘已经通过实机启动与只读检查，根卷存在唯一 LVM 基线快照。

当前缺少的只有：

- `/agent/releases`、`/agent/lives` 和 `/agent/boot` 正式目录；
- 开机启动当前实例的 `hominal.service`；
- 代替认知内核验证启动链的极小程序；
- 从 Mac 一键启动、停止、保存和重置的脚本；
- 一次真实 LVM 合并恢复证明。

Mutagen 当前同步到 `/agent/app/hominal.cc`，继续服务开发，不作为正式发布方式。阶段二演练包独立构建并部署到 `/agent/releases`。

## 三、最小实现

### 1. `lab/run.py`

它是身体外的实验按钮，只提供四个主要命令：

```bash
python3 lab/run.py check
python3 lab/run.py start
python3 lab/run.py stop
python3 lab/run.py reset
```

- `check`：检查 SSH、挂载、磁盘、服务和必要工具；
- `start`：构建、上传、创建新实例、安装启动服务并重启 Ubuntu；
- `stop`：明确停止服务，把本代实例和必要 systemd 日志保存到 Mac；
- `reset`：确认本代已保存，删除本代活动状态，准备下一次启动。

另提供只读 `status` 方便查看当前实例。脚本读取上层私密配置，但不会输出或归档凭据。

### 2. `hominal.service` 与 launcher

系统服务以 root 运行，等待 `/agent` 挂载，从 `/agent/boot/active-instance` 找到当前实例并启动其 `body/bin/hominald`。

进程异常退出后自动重启；`systemctl stop` 表示实验明确结束，不触发自动重启。服务不限制未来 alice 对 Ubuntu、网络、文件和软件的使用。

### 3. 机械测试体

一个极小 Go 程序完成：

- 启动后写入 ready；
- 每五秒更新 heartbeat；
- 收到停止信号后记录并正常退出；
- 收到测试用崩溃文件后异常退出，用于验证 systemd 自动恢复。

它使用未来 `hominald` 相同的实例目录和启动入口，阶段三可以直接替换，不扩展成模拟认知系统。

## 四、最小目录

```text
/agent/
├── boot/                         当前实例
├── releases/<release_id>/        冻结发布参考
├── lives/<instance_id>/
│   ├── birth/
│   ├── body/bin/hominald
│   ├── state/
│   ├── journal/
│   ├── life/
│   └── logs/
├── staging/
├── tmp/
└── recovery/                     现有系统恢复材料，保持不动
```

当前 `/agent/app`、`/agent/state` 和既有恢复资料在本阶段不删除。新实验实例只使用正式目录，避免破坏已配置的 Chrome、微信和开发环境。

## 五、最小保存内容

每次停止只保存：

```text
manifest.json       实例、发布哈希和时间
agent-final.tar.gz  本代实例终态
systemd.log         启动、崩溃、恢复和停止记录
hashes.sha256       归档校验值
```

不建设实时观察数据库、复杂实验状态机、全面系统差异和多层 Schema。阶段三之后如果某类缺陷无法凭这些材料判断，再增加直接解决该缺陷的记录。

## 六、执行流程

1. 实现测试体、launcher、systemd service 和 `run.py`；
2. `check` 确认当前身体条件；
3. `start` 构建并部署新实例，重启 Ubuntu；
4. 验证开机自启、五秒心跳和持续运行；
5. 触发一次测试崩溃，验证 systemd 恢复同一实例；
6. 验证 root、网络、模型、Chrome/Playwright MCP 和微信当前可用；
7. `stop` 保存本代实例；
8. `reset` 清除本代活动状态；
9. 再次 `start`，确认能够生成并运行第二个干净实例；
10. 在稳定服务全部安装后更新唯一根系统基线；
11. 经导师确认，执行一次真实 LVM 合并和恢复后启动验收。

Chrome、微信和番茄账号的正式代际会话基线在第一次真实创生前冻结。本阶段只确认当前身体能够使用这些出口，不为机械测试体构造账号历史。

## 七、退出门

阶段二完成只要求：

1. 一条命令能够构建、部署并重启进入新实例；
2. Ubuntu 开机后测试体自动运行并持续产生 heartbeat；
3. 测试体崩溃后恢复，明确停止后保持停止；
4. root、网络、模型、Chrome/Playwright MCP 和微信当前真实可用；
5. 停止后能够保存本代实例和必要日志；
6. reset 后能够再次启动另一个没有前代生命状态的实例；
7. 完成一次真实根卷 LVM 恢复，Ubuntu、SSH、桌面和服务重新可用。

达到这些条件即进入阶段三，不继续扩建 Genesis Lab。

## 八、需要导师配合的节点

通常只在两个节点需要导师：

1. 微信或其他账号重新要求手机确认时；
2. 替换唯一根快照并执行真实 LVM 合并前。

其余实现、部署、重启、测试和归档由 Codex 主持完成。

## 九、执行结果

2026-08-24，阶段二完成：

- `lab/run.py` 已贯通 check、start、status、stop 和 reset；
- 同一机械测试体连续产生三个独立实例，发布 SHA-256 均为 `4a4211c45f09281c1b9bbea5e48b89caaa6ef53f79173862a3b9aef253dde146`；
- 三次 Ubuntu 重启均由 `hominal.service` 自动进入当前实例；
- 测试体异常退出后约两秒恢复同一实例，明确停止后保持停止；
- 三个实例终态和 systemd 日志均已保存到 Mac，归档哈希复验通过；
- root、HTTPS、`gpt-5.5` Responses API、Chrome/CDP、Playwright MCP 和微信完成真实烟测；
- reset 后前代生命目录消失，新实例只包含自己的 ready 与 heartbeat；
- 唯一根基线更新为包含当前 launcher、service 和恢复服务的系统状态；
- 真实 LVM merge 成功，恢复后 canary 消失，Ubuntu、SSH、桌面和服务恢复，同名唯一快照自动重建；
- 恢复后再次完成 start、stop 和 reset，当前没有活动实验实例。

阶段二到此停止扩展，后续工作进入阶段三生命运行脊柱。
