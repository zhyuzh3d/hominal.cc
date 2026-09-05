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
    parser.add_argument(
        "--xconfig",
        type=Path,
        default=ROOT.parent / "xconfigs" / "hominal" / "xconfig.yaml",
    )
    args = parser.parse_args()

    required = [
        ROOT / "genesis/seed.md",
        ROOT / "genesis/seed.yaml",
        ROOT / "genesis/dynamics.yaml",
        ROOT / "lab/body/current-profile.md",
        ROOT / "lab/templates/birth.yaml",
        ROOT / "lab/protocol/mentor.md",
        ROOT / "lab/protocol/experiment.yaml",
        ROOT / "body/tools/hominal-browser.mjs",
        ROOT / "body/organs/browser.json",
        ROOT / "body/cmd/hominal-system/main.go",
        ROOT / "body/organs/system.json",
        ROOT / "deploy/hominal-generation-stop",
        ROOT / "deploy/desktop/chrome-autostart.desktop",
        ROOT / "docs/mvp-architecture.md",
    ]
    errors: list[str] = []

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
    if numeric_leaf_count(adjustable) != 28:
        errors.append("dynamics must contain exactly 28 adjustable numeric parameters")
    difference = dynamics.get("difference", {})
    for key in ("accumulation_decay_rate", "learning_rate"):
        value = difference.get(key)
        if not isinstance(value, (int, float)) or isinstance(value, bool) or not 0 < value <= 1:
            errors.append(f"difference.{key} must remain within (0, 1]")
    if dynamics.get("attention", {}).get("revisit_seconds") != 10:
        errors.append("idle exploration must return to attention within 10 seconds")

    pulse_seconds = dynamics.get("pulse", {}).get("interval_seconds")
    if experiment.get("runtime", {}).get("cognitive_pulse_interval_seconds") != pulse_seconds:
        errors.append("experiment pulse interval must match dynamics")
    if birth.get("generation", {}).get("window_seconds") != 60 * experiment.get("generation", {}).get("duration_minutes", 0):
        errors.append("birth and experiment generation duration must match")
    if experiment.get("generation", {}).get("t0_event") != "runtime_ready":
        errors.append("T0 event contract has drifted")
    if experiment.get("birth", {}).get("deliver_birth_message_once_after_t0") is not True:
        errors.append("birth message must be delivered once after T0")
    if experiment.get("outcome", {}).get("archive_system_delta") is not True:
        errors.append("formal archive must retain a sparse root-system delta")
    expression = birth.get("resources", {}).get("spaces", {}).get("public_expression", {})
    if expression.get("service") != "X" or expression.get("account") != "@hominal_cc":
        errors.append("Birth Manifest must expose alice's X window")
    if "番茄" in str(birth.get("brief", "")):
        errors.append("Birth Manifest brief must leave Fanqie for alice to discover")

    if not args.xconfig.is_file():
        errors.append("xconfig is missing")
    else:
        config = load_yaml(args.xconfig)
        body = config.get("system", {}).get("body", {})
        lab = config.get("system", {}).get("genesis_lab", {})
        llm = config.get("llm", {})
        runtime = llm.get("runtime", {})
        for key in ("agent_mount", "release_root", "life_root", "boot_root", "service_name"):
            if not body.get(key):
                errors.append(f"xconfig system.body.{key} is missing")
        for key in ("archive_path", "live_stream_path", "persistent_app_backup", "mentor"):
            if not lab.get(key):
                errors.append(f"xconfig system.genesis_lab.{key} is missing")
        mentor = lab.get("mentor", {})
        if (
            mentor.get("transport") != "ssh_unix_socket"
            or mentor.get("socket_path") != "/run/hominal/hominal.sock"
            or mentor.get("context_mode") != "isolated"
        ):
            errors.append("xconfig mentor transport and context isolation have drifted")
        if runtime.get("initial_profile") != {"model": "terra", "reasoning_effort": "none"}:
            errors.append("xconfig initial cognitive profile must be terra/none")
        models = llm.get("models", {})
        if set(models) != {"luna", "terra", "sol"} or any(not value.get("id") for value in models.values()):
            errors.append("xconfig requires luna, terra and sol roles with actual model IDs")
        for role, effort in (("luna", "none"), ("terra", "none"), ("sol", "low")):
            if effort not in models.get(role, {}).get("supported_reasoning_efforts", []):
                errors.append(f"xconfig {role} role must support {effort}")
        provider_name = llm.get("provider")
        provider = llm.get("providers", {}).get(provider_name, {})
        if provider_name != "llmserver" or provider.get("adapter") != "llmserver":
            errors.append("xconfig active cognitive provider must be the llmserver adapter")
        if provider.get("base_url") != "http://192.168.124.161:4815":
            errors.append("xconfig llmserver must use the fixed LAN endpoint")
        credential_file = provider.get("credential_file")
        if not credential_file or not (args.xconfig.parent / credential_file).resolve().is_file():
            errors.append("xconfig llmserver credential_file is unavailable")
        resource = llm.get("cognitive_resource", {})
        if resource.get("rolling_hour_usd") != 5.0 or resource.get("rolling_day_usd") != 50.0:
            errors.append("xconfig cognitive resource limits must be 5 USD/hour and 50 USD/day")
        if resource.get("usage_query") != "local_usage_ledger":
            errors.append("xconfig cognitive resource usage must come from the local ledger")
        x_account = config.get("social_accounts", {}).get("x_twitter", {})
        if x_account.get("username") != "hominal_cc" or not x_account.get("url"):
            errors.append("xconfig X account window is incomplete")

    for message in errors:
        print(f"ERROR: {message}")
    if errors:
        return 1
    print("G0 stage-one contract validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
