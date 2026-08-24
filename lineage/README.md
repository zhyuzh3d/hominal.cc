# Lineage Archive

本目录保存可进入 Git 的精炼谱系档案，包括遗传版本、冻结实验包、出生与结果 Manifest、证据校验值、整体诊断和继代决定。

```text
g0-v001/
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

完整磁盘快照、原始日志、模型调用、浏览器遗迹和大型作品保存在仓库之外的 Lab Archive。谱系文件通过位置引用与 SHA-256 校验值连接这些原始证据。

正式第一代发生后再创建对应的 `g0-vNNN/` 目录。本文件只定义归档契约，不预造实验结果。
