# Genesis Lab

本目录保存身体外的最小实验入口、协议、模板和当前身体事实。Lab 只负责启动、停止、保存和重置，不发展成独立研究平台，也不参与 alice 的认知。

阶段二使用 `python3 lab/run.py check|start|status|stop|reset` 操作 Ubuntu 身体。实例终态和必要 systemd 日志保存在上层私密配置指定的 Mac 离机目录，不进入 Git。

阶段一可以运行 `python3 lab/validate-contract.py --xconfig ../xconfig.yaml` 检查全部冻结产物、动力参数数量、T0 时序、推理强度和私密环境配置是否一致。校验器只报告字段状态，不输出凭据内容。
