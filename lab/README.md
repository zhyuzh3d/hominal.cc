# Genesis Lab

本目录保存身体外的创生实验器源码、协议、模板、身体探针和分析工具。Lab 负责冻结实验输入、生成纯净身体、创建 Birth Manifest、记录时间边界、保存外部证据、归档最终快照并产生下一遗传版本。

建议的实现分区：

```text
body/       目标身体事实探针与当前开发快照
protocol/   导师与实验协议
templates/  Birth、Outcome 和实验包模板
probes/     外部设备、资源和结果探针
analysis/   时间线重建与代际比较工具
genesisctl  Lab 控制入口
```

Lab 代码与档案位于 alice 身体之外。正式运行时只把身体客户端和当代 Birth Manifest 注入 Ubuntu 设备。

阶段一可以运行 `python3 lab/validate-contract.py --xconfig ../xconfig.yaml` 检查全部冻结产物、动力参数数量、T0 时序、推理强度和私密环境配置是否一致。校验器只报告字段状态，不输出凭据内容。
