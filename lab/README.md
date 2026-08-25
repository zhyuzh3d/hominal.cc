# Genesis Lab

本目录保存身体外的最小实验入口、协议、模板和当前身体事实。Lab 只负责启动、停止、保存和重置，不发展成独立研究平台，也不参与 alice 的认知。

阶段三、四继续复用唯一入口：

```bash
python3 lab/run.py check
python3 lab/run.py start --stage 3
python3 lab/run.py start --stage 4
python3 lab/run.py status
python3 lab/run.py mentor-send --body '...'
python3 lab/run.py mentor-outbox
python3 lab/run.py mentor-ack <message_id>
python3 lab/run.py crash
python3 lab/run.py stop
python3 lab/run.py reset
```

`start` 构建并冻结当前 `hominald`，部署工程实例、安装实例外运行配置、激活 systemd 并重启 Ubuntu。`mentor-*` 经 SSH 调用目标机 Unix Socket；`stop` 明确停止并归档，`reset` 只删除已经归档的准确实例。实例终态和必要 systemd 日志保存在上层私密配置指定的 Mac 离机目录，不进入 Git。

阶段一可以运行 `python3 lab/validate-contract.py --xconfig ../xconfig.yaml` 检查全部冻结产物、动力参数数量、T0 时序、推理强度和私密环境配置是否一致。校验器只报告字段状态，不输出凭据内容。
