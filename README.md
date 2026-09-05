# Hominal Stage 20.0 · A1X

这是独立的A1X实验线。10.3源码在分叉时冻结为起点，此后的代码、配置、个体、作品和实验档案独立保存；来源见[分叉记录](lineage/stage20-fork.json)。本仓库没有连接原仓库的Git remote，也不使用旧线路的同步部署。旧`lab/run.py`在这里拒绝执行。

当前身体是Bazzite KDE Wayland中的普通用户进程，使用RTX 5060 Ti 16GB运行常驻GUI-Owl视觉服务，通过真实屏幕截图、定位和输入形成操作闭环。主脑仍是唯一认知和行动意愿所有者；视觉模型只提供可核验的画面解释和定位候选。当前实验空间为独立本地工作室。

[开发计划](plans/stage-20.0-development.md)与[实验计划](plans/stage-20.0-experiments.md)说明目标和验收边界；[实验记录](plans/stage-20.0-results.md)区分工程通过项、失败样本和个人实际经历。`lineage`、旧阶段计划与文档作为历史参考，不代表20.0当前运行状态。

云端模型使用llmserver的terra/none主脑、luna/none低阶协助、sol/low高阶协助。滚动一小时5美元、24小时50美元的费用账本跨个体保留；未知账单采用持久化保守预留。模型资格和凭据存放在仓库外，不提交到Git。

独立Lab入口：

```bash
python3 lab/stage20.py prepare
python3 lab/stage20.py start --minutes 10
python3 lab/stage20.py status
python3 lab/stage20.py extend --total-minutes 30
python3 lab/stage20.py stop --reason completed_observation
```

`prepare`编译并上传冻结发布；运行中的个体必须先停止归档。`start`启动临时用户服务、独立浏览器与空白作品空间，封存身份并设置设备本地截止。`stop`停止生命、保存离机档案，再释放实验服务。它们不会添加开机启动，不修改系统驱动。导师通道见`mentor`与`outbox`子命令；所有人工提示与环境变更应写入干预记录。

代码在本目录；私密配置在`../xconfigs/hominal20`；设备数据在`~/.local/share/hominal20`；Mac离机档案在`~/HominalStage20Lab`。`xconfig.yaml`记录A1X固定地址、SSH用户、专用密钥路径与双方指纹，`gateway.yaml`、`models.yaml`、`runtime.yaml`和`input-scope.yaml`保存带逐字段注释的20.0配置。Lab直接读取这些YAML，并只在向现有Go内核和设备器官交付时生成权限受限的JSON。私钥和网关令牌只留在该仓库外目录，明文SSH密码不写入配置。

实验截图、模型权重、账号状态和个体历史不得提交。工程测试使用Lab的DOM/文件真值核对结果，视觉器官不会得到这些答案。

协议与内核验证使用`go test -race ./body/...`，桌面边界单测使用`python3 body/tools/test_desktop.py`，实验控制时间兼容测试使用`PYTHONPATH=lab python3 -m unittest lab.test_stage20`。实机基准、布局干预和窗口真值工具位于`lab/stage20_*.py`。工程任务和人工修复不算自主成长证据。

MIT License，见[LICENSE](LICENSE)。
