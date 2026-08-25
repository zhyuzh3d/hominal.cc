#!/usr/bin/env python3
"""Validate the frozen G0 stage-one contract without displaying secrets."""

from __future__ import annotations

import argparse
from pathlib import Path
import sys

import yaml


ROOT = Path(__file__).resolve().parents[1]


def load_yaml(path: Path):
    with path.open("r", encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def numeric_leaf_count(value) -> int:
    if isinstance(value, dict):
        return sum(numeric_leaf_count(item) for item in value.values())
    if isinstance(value, list):
        return 0
    return int(isinstance(value, (int, float)) and not isinstance(value, bool))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--xconfig", type=Path, default=ROOT.parent / "xconfig.yaml")
    parser.add_argument("--allow-pending-quota", action="store_true")
    args = parser.parse_args()

    required = [
        ROOT / "genesis/seed.md",
        ROOT / "genesis/seed.yaml",
        ROOT / "genesis/dynamics.yaml",
        ROOT / "lab/body/current-profile.md",
        ROOT / "lab/templates/birth.yaml",
        ROOT / "lab/protocol/mentor.md",
        ROOT / "lab/protocol/experiment.yaml",
        ROOT / "docs/mvp-architecture.md",
    ]
    errors: list[str] = []
    pending: list[str] = []

    for path in required:
        if not path.is_file():
            errors.append(f"missing required contract: {path.relative_to(ROOT)}")

    if errors:
        for message in errors:
            print(f"ERROR: {message}")
        return 1

    seed = load_yaml(ROOT / "genesis/seed.yaml")
    dynamics = load_yaml(ROOT / "genesis/dynamics.yaml")
    birth = load_yaml(ROOT / "lab/templates/birth.yaml")
    experiment = load_yaml(ROOT / "lab/protocol/experiment.yaml")

    if seed.get("identity", {}).get("name") != "alice":
        errors.append("seed identity name must be alice")
    if seed.get("identity", {}).get("age") != 21:
        errors.append("seed identity age must be 21")
    if seed.get("identity", {}).get("genesis_stage") != "proto_hominal":
        errors.append("seed genesis stage must be proto_hominal")

    adjustable = {key: value for key, value in dynamics.items() if key not in {"schema", "dynamics_id", "time_unit", "bounds"}}
    if numeric_leaf_count(adjustable) != 15:
        errors.append("dynamics must contain exactly 15 adjustable numeric parameters")

    pulse_seconds = dynamics.get("pulse", {}).get("interval_seconds")
    if experiment.get("runtime", {}).get("cognitive_pulse_interval_seconds") != pulse_seconds:
        errors.append("experiment pulse interval must match dynamics")
    if birth.get("window", {}).get("duration_minutes") != experiment.get("generation", {}).get("duration_minutes"):
        errors.append("birth and experiment generation duration must match")
    if experiment.get("generation", {}).get("t0_event") != "first_successful_cognitive_commit":
        errors.append("T0 event contract has drifted")

    if not args.xconfig.is_file():
        errors.append("xconfig is missing")
    else:
        config = load_yaml(args.xconfig)
        body = config.get("system", {}).get("body", {})
        lab = config.get("system", {}).get("genesis_lab", {})
        runtime = config.get("llm", {}).get("runtime", {})
        for key in ("agent_mount", "release_root", "life_root", "boot_root", "service_name"):
            if not body.get(key):
                errors.append(f"xconfig system.body.{key} is missing")
        for key in ("archive_path", "live_stream_path", "agent_birth_baseline", "mentor"):
            if not lab.get(key):
                errors.append(f"xconfig system.genesis_lab.{key} is missing")
        mentor = lab.get("mentor", {})
        if (
            mentor.get("transport") != "ssh_unix_socket"
            or mentor.get("socket_path") != "/run/hominal/hominal.sock"
            or mentor.get("context_mode") != "isolated"
        ):
            errors.append("xconfig mentor transport and context isolation have drifted")
        if runtime.get("reasoning_effort") != "low":
            errors.append("xconfig default reasoning effort must remain low during G0")
        quota = config.get("llm", {}).get("quota", {})
        for key in ("hourly_limit", "unit", "refresh_rule", "usage_query"):
            if not quota.get(key):
                pending.append(f"xconfig llm.quota.{key} needs mentor confirmation")

    for message in errors:
        print(f"ERROR: {message}")
    for message in pending:
        print(f"PENDING: {message}")
    if errors:
        return 1
    if pending and not args.allow_pending_quota:
        return 2
    print("G0 stage-one contract validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
