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

跨代际、会影响以后架构判断的已确认经验进入 [`research-memory.md`](./research-memory.md)。制定新实验、解释异常或重构认知内核以前先阅读它；它只保存可复用因果结论，不复制每代时间线，也不替代当前设计规范。

首个正式谱系为 [`g0-v001/`](./g0-v001/)。它保存正式实例 `alice0826n` 的冻结遗传版本、精炼出生与结果、证据校验值、整体诊断和继代决定；大型原始档案仍留在身体外 Genesis Lab。
