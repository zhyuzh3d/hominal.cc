#!/usr/bin/env python3
"""Build, deploy, operate, and archive the single G0 engineering instance."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
import uuid

import yaml


ROOT = Path(__file__).resolve().parents[1]
CONFIG_PATH = ROOT.parent / "xconfig.yaml"
INSTANCE_RE = re.compile(r"^[A-Za-z0-9._-]+$")
SOCKET_PATH = "/run/hominal/hominal.sock"
REMOTE_RUNTIME_CONFIG = "/etc/hominal/runtime.json"


def load_yaml(path: Path) -> dict:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"invalid mapping in {path}")
    return data


def load_settings() -> dict[str, object]:
    data = load_yaml(CONFIG_PATH)
    system = data["system"]
    lab = system["genesis_lab"]
    return {
        "config": data,
        "host": system["body"]["ssh_host_alias"],
        "sudo_password": system["remote_server"]["password"],
        "archive": Path(lab["archive_path"]) / "engineering",
        "live": Path(lab["live_stream_path"]),
    }


SETTINGS = load_settings()
HOST = str(SETTINGS["host"])
ARCHIVE_ROOT = Path(SETTINGS["archive"])
LIVE_ROOT = Path(SETTINGS["live"])
CURRENT_PATH = LIVE_ROOT / "g0-engineering-current.json"
SSH_BASE = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", HOST]


def run(
    args: list[str],
    *,
    check: bool = True,
    input_text: str | None = None,
    capture: bool = False,
    timeout: int | None = None,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        args,
        check=False,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
        timeout=timeout,
        cwd=cwd,
        env=env,
    )
    if check and result.returncode:
        detail = (result.stderr or result.stdout or "command failed").strip()
        raise RuntimeError(detail)
    return result


def ssh(
    command: str,
    *,
    check: bool = True,
    capture: bool = False,
    input_text: str | None = None,
    timeout: int = 30,
) -> subprocess.CompletedProcess[str]:
    remote = "bash -lc " + shlex.quote(command)
    return run(
        [*SSH_BASE, remote],
        check=check,
        capture=capture,
        input_text=input_text,
        timeout=timeout,
    )


def sudo(
    command: str,
    *,
    check: bool = True,
    capture: bool = False,
    timeout: int = 30,
) -> subprocess.CompletedProcess[str]:
    remote = "sudo -S -p '' -- bash -lc " + shlex.quote(command)
    return run(
        [*SSH_BASE, remote],
        check=check,
        capture=capture,
        input_text=str(SETTINGS["sudo_password"]) + "\n",
        timeout=timeout,
    )


def scp(source: Path | str, destination: str, *, from_remote: bool = False, timeout: int = 180) -> None:
    if from_remote:
        args = ["scp", "-q", f"{HOST}:{source}", destination]
    else:
        args = ["scp", "-q", str(source), f"{HOST}:{destination}"]
    run(args, timeout=timeout)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def utcstamp() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ").lower()


def ensure_external_dirs() -> None:
    for path in (ARCHIVE_ROOT, LIVE_ROOT):
        path.mkdir(parents=True, exist_ok=True)
        path.chmod(0o700)


def load_current(required: bool = True) -> dict[str, object] | None:
    if not CURRENT_PATH.is_file():
        if required:
            raise RuntimeError("no active G0 engineering instance is registered")
        return None
    data = json.loads(CURRENT_PATH.read_text(encoding="utf-8"))
    instance_id = str(data["instance_id"])
    if not INSTANCE_RE.fullmatch(instance_id):
        raise RuntimeError("invalid registered instance id")
    return data


def save_current(data: dict[str, object]) -> None:
    ensure_external_dirs()
    temporary = CURRENT_PATH.with_suffix(".partial")
    temporary.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.replace(temporary, CURRENT_PATH)


def build_runtime(output: Path) -> str:
    environment = os.environ.copy()
    environment.update({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"})
    run(
        [
            "go",
            "build",
            "-trimpath",
            "-buildvcs=false",
            "-ldflags=-s -w -buildid=",
            "-o",
            str(output),
            "./body/cmd/hominald",
        ],
        cwd=ROOT,
        env=environment,
        capture=True,
        timeout=180,
    )
    output.chmod(0o755)
    return sha256(output)


def dynamics_config(stage: int) -> dict[str, object]:
    dynamics = load_yaml(ROOT / "genesis" / "dynamics.yaml")
    if stage == 3:
        return {}
    required = {
        "concern.base_drive": dynamics.get("concern", {}).get("base_drive"),
        "concern.urgency_weight": dynamics.get("concern", {}).get("urgency_weight"),
        "attention.novelty_weight": dynamics.get("attention", {}).get("novelty_weight"),
        "attention.trigger_threshold": dynamics.get("attention", {}).get("trigger_threshold"),
        "exploration.idle_growth": dynamics.get("exploration", {}).get("idle_growth"),
        "exploration.unknown_growth": dynamics.get("exploration", {}).get("unknown_growth"),
        "exploration.relief": dynamics.get("exploration", {}).get("relief"),
    }
    missing = [key for key, value in required.items() if value is None]
    if missing:
        raise RuntimeError("stage-four dynamics are not frozen: " + ", ".join(missing))
    return {
        "affect_return_rate": dynamics["affect"]["return_rate"],
        "concern_base_drive": required["concern.base_drive"],
        "concern_urgency_weight": required["concern.urgency_weight"],
        "concern_growth_gain": dynamics["concern"]["growth_gain"],
        "concern_resolution_gain": dynamics["concern"]["resolution_gain"],
        "concern_natural_decay_rate": dynamics["concern"]["natural_decay_rate"],
        "attention_affect_weight": dynamics["attention"]["affect_weight"],
        "attention_exploration_weight": dynamics["attention"]["exploration_weight"],
        "attention_novelty_weight": required["attention.novelty_weight"],
        "attention_cost_weight": dynamics["attention"]["resource_cost_weight"],
        "attention_threshold": required["attention.trigger_threshold"],
        "attention_candidate_limit": 3,
        "attention_revisit_seconds": 300,
        "exploration_idle_growth": required["exploration.idle_growth"],
        "exploration_unknown_growth": required["exploration.unknown_growth"],
        "exploration_relief": required["exploration.relief"],
    }


def seed_config() -> dict[str, object]:
    seed = load_yaml(ROOT / "genesis" / "seed.yaml")
    identity = seed["identity"]
    temperament = seed["temperament"]
    values = seed["value_priors"]
    return {
        "name": identity["name"],
        "gender": identity["gender"],
        "age": identity["age"],
        "life_form": identity["life_form"],
        "social_openness_bias": temperament["social_openness_bias"],
        "exploration_bias": temperament["exploration_bias"],
        "constructive_recovery_bias": temperament["constructive_recovery_bias"],
        "continuance_sensitivity": values["continuance_sensitivity"],
        "relatedness_sensitivity": values["relatedness_sensitivity"],
        "expansion_sensitivity": values["expansion_sensitivity"],
        "reality_integrity_sensitivity": values["reality_integrity_sensitivity"],
        "semantic_text": (ROOT / "genesis" / "seed.md").read_text(encoding="utf-8"),
    }


def runtime_config(stage: int, *, public: bool) -> dict[str, object]:
    config = SETTINGS["config"]
    llm = config["llm"]
    provider_name = llm["provider"]
    provider = llm["providers"][provider_name]
    runtime_auth_value = llm["credentials"]["environment"]["OPENAI_API_KEY"]
    pulse_seconds = load_yaml(ROOT / "genesis" / "dynamics.yaml")["pulse"]["interval_seconds"]
    return {
        "stage": stage,
        "engineering": True,
        "pulse": {"interval_seconds": pulse_seconds, "slow_scan_seconds": 60},
        "model": {
            "base_url": "<runtime-only>" if public else provider["base_url"],
            "api_key": "<runtime-only>" if public else runtime_auth_value,
            "name": llm["models"]["primary"],
            "reasoning_effort": llm["runtime"]["reasoning_effort"],
            "max_output_tokens": 1200,
        },
        "quota": {"limit_tokens": llm["quota"]["hourly_limit"], "window_minutes": 60},
        "dynamics": dynamics_config(stage),
        "seed": seed_config(),
    }


def install_host_files() -> None:
    launcher_remote = "/agent/staging/hominal-launcher.partial"
    service_remote = "/agent/staging/hominal.service.partial"
    scp(ROOT / "deploy" / "hominal-launcher", launcher_remote)
    scp(ROOT / "deploy" / "hominal.service", service_remote)
    sudo(
        " && ".join(
            [
                f"install -m 0755 {shlex.quote(launcher_remote)} /usr/local/sbin/hominal-launcher",
                f"install -m 0644 {shlex.quote(service_remote)} /etc/systemd/system/hominal.service",
                "rm -f /agent/staging/hominal-launcher.partial /agent/staging/hominal.service.partial",
                "systemctl daemon-reload",
                "systemctl enable hominal.service",
            ]
        )
    )


def deploy_runtime_config(path: Path) -> None:
    remote_partial = "/agent/staging/runtime.json.partial"
    scp(path, remote_partial)
    sudo(
        f"install -d -o root -g root -m 0755 /etc/hominal && "
        f"install -o root -g root -m 0600 {shlex.quote(remote_partial)} "
        f"{shlex.quote(REMOTE_RUNTIME_CONFIG)} && rm -f {shlex.quote(remote_partial)}"
    )


def wait_for_reboot(previous_boot_id: str) -> None:
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            result = ssh("cat /proc/sys/kernel/random/boot_id", check=False, capture=True, timeout=10)
        except subprocess.TimeoutExpired:
            time.sleep(5)
            continue
        current = (result.stdout or "").strip()
        if result.returncode == 0 and current and current != previous_boot_id:
            return
        time.sleep(5)
    raise RuntimeError("Ubuntu did not return with a new boot id within 180 seconds")


def wait_for_runtime(instance_id: str) -> None:
    deadline = time.monotonic() + 60
    heartbeat = f"/agent/lives/{instance_id}/state/heartbeat.json"
    while time.monotonic() < deadline:
        probe = (
            "systemctl is-active --quiet hominal.service && "
            f"test -s {shlex.quote(heartbeat)} && test -S {shlex.quote(SOCKET_PATH)}"
        )
        if ssh(probe, check=False, timeout=10).returncode == 0:
            return
        time.sleep(2)
    raise RuntimeError("hominal.service did not become ready within 60 seconds")


def cmd_check() -> None:
    ensure_external_dirs()
    if not CONFIG_PATH.is_file():
        raise RuntimeError(f"missing protected configuration: {CONFIG_PATH}")
    for command in ("go", "ssh", "scp"):
        if shutil.which(command) is None:
            raise RuntimeError(f"missing local command: {command}")
    probe = r"""
set -eu
printf 'host='; hostname
printf 'agent='; findmnt -n -o SOURCE,FSTYPE,SIZE,AVAIL,TARGET /agent
printf 'root_free='; df -B1 --output=avail / | tail -1 | xargs
printf 'agent_free='; df -B1 --output=avail /agent | tail -1 | xargs
printf 'desktop='; systemctl is-active lightdm
printf 'chrome='; command -v google-chrome
printf 'playwright='; command -v hominal-playwright-mcp
printf 'wechat='; command -v wechat
printf 'service='; systemctl is-enabled hominal.service 2>/dev/null || true
printf 'curl='; command -v curl
"""
    result = ssh(probe, capture=True)
    print(result.stdout, end="")
    print(f"archive={ARCHIVE_ROOT}")
    print("check=passed")


def cmd_start(stage: int) -> None:
    ensure_external_dirs()
    if load_current(required=False) is not None:
        raise RuntimeError("an engineering instance is already registered; run stop and reset first")
    existing = ssh("cat /agent/boot/active-instance 2>/dev/null || true", capture=True).stdout.strip()
    if existing:
        raise RuntimeError(f"Ubuntu still has active instance {existing}; inspect it before starting another")
    cmd_check()
    previous_boot_id = ssh("cat /proc/sys/kernel/random/boot_id", capture=True).stdout.strip()

    with tempfile.TemporaryDirectory(prefix=f"hominal-stage{stage}-") as directory_name:
        directory = Path(directory_name)
        binary = directory / "hominald"
        binary_hash = build_runtime(binary)
        release_id = f"g0s{stage}-{binary_hash[:12]}"
        instance_id = f"g0s{stage}-{utcstamp()}-{binary_hash[:6]}"
        if not INSTANCE_RE.fullmatch(instance_id):
            raise RuntimeError("generated invalid instance id")

        ssh("install -d -m 0755 /agent/boot /agent/releases /agent/lives /agent/staging /agent/tmp")
        upload = f"/agent/staging/{release_id}.partial"
        scp(binary, upload)
        remote_hash = ssh(f"sha256sum {shlex.quote(upload)} | awk '{{print $1}}'", capture=True).stdout.strip()
        if remote_hash != binary_hash:
            raise RuntimeError("uploaded runtime hash mismatch")

        release_root = f"/agent/releases/{release_id}"
        instance_root = f"/agent/lives/{instance_id}"
        remote_prepare = f"""
set -eu
install -d -m 0755 {shlex.quote(release_root)}/bin
if [ ! -e {shlex.quote(release_root)}/bin/hominald ]; then
  mv {shlex.quote(upload)} {shlex.quote(release_root)}/bin/hominald
else
  rm -f {shlex.quote(upload)}
fi
chmod 0555 {shlex.quote(release_root)}/bin/hominald
install -d -m 0755 {shlex.quote(instance_root)}/birth {shlex.quote(instance_root)}/body/bin {shlex.quote(instance_root)}/state {shlex.quote(instance_root)}/journal {shlex.quote(instance_root)}/life {shlex.quote(instance_root)}/logs
cp {shlex.quote(release_root)}/bin/hominald {shlex.quote(instance_root)}/body/bin/hominald
chmod 0555 {shlex.quote(instance_root)}/body/bin/hominald
"""
        ssh(remote_prepare)

        for source in (
            ROOT / "genesis" / "seed.md",
            ROOT / "genesis" / "seed.yaml",
            ROOT / "genesis" / "dynamics.yaml",
            ROOT / "lab" / "protocol" / "mentor.md",
            ROOT / "lab" / "protocol" / "experiment.yaml",
        ):
            scp(source, f"{instance_root}/birth/{source.name}")

        secret_path = directory / "runtime.json"
        public_path = directory / "runtime.public.json"
        secret_path.write_text(json.dumps(runtime_config(stage, public=False), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        secret_path.chmod(0o600)
        public_path.write_text(json.dumps(runtime_config(stage, public=True), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        scp(public_path, f"{instance_root}/birth/runtime.public.json")
        deploy_runtime_config(secret_path)

        manifest = {
            "kind": "g0-engineering",
            "stage": stage,
            "instance_id": instance_id,
            "release_id": release_id,
            "release_sha256": binary_hash,
            "prepared_at": datetime.now(timezone.utc).isoformat(),
            "git_commit": run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture=True).stdout.strip(),
        }
        manifest_path = directory / "manifest.json"
        manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        scp(manifest_path, f"{instance_root}/birth/manifest.json")

        install_host_files()
        sudo(
            f"if [ -e /life ] && [ ! -L /life ]; then exit 23; fi; "
            f"ln -sfn {shlex.quote(instance_root + '/life')} /life; "
            f"printf '%s\n' {shlex.quote(release_id)} > /agent/boot/active-release.partial; "
            f"printf '%s\n' {shlex.quote(instance_id)} > /agent/boot/active-instance.partial; "
            "mv /agent/boot/active-release.partial /agent/boot/active-release; "
            "mv /agent/boot/active-instance.partial /agent/boot/active-instance"
        )
        save_current(manifest)

    print(f"instance={instance_id}")
    print("rebooting Ubuntu...", flush=True)
    sudo("systemctl reboot", check=False)
    wait_for_reboot(previous_boot_id)
    wait_for_runtime(instance_id)
    print("start=passed")


def safe_remote_state(instance_id: str) -> dict[str, object]:
    state_path = f"/agent/lives/{instance_id}/state/current.json"
    script = """
import json, sys
s=json.load(open(sys.argv[1], encoding='utf-8'))
out={
 'instance_id':s.get('instance_id'), 'stage':s.get('stage'), 'revision':s.get('revision'),
 'pulse_id':s.get('pulse_id'), 'event_seq':s.get('event_seq'), 'last_pulse_at':s.get('last_pulse_at'),
 'lease_id':(s.get('lease') or {}).get('id'), 'current_focus':s.get('current_focus'),
 'quota_remaining_tokens':(s.get('body') or {}).get('quota_remaining_tokens'),
 'background':{k:sum(1 for e in s.get('background',[]) if e.get('status')==k) for k in ('pending','in_focus','retry_wait','background','processed','interrupted','failed')},
 'pending_action':{k:(s.get('pending_action') or {}).get(k) for k in ('kind','status')},
 'outbox':{k:sum(1 for m in (s.get('mentor') or {}).get('outbox',[]) if m.get('status')==k) for k in ('queued','delivered')},
 'affective_state':s.get('affective_state'), 'exploration_pressure':s.get('exploration_pressure'),
 'concern_count':len(s.get('active_concerns') or []),
 'max_concern_strength':max([c.get('strength',0) for c in (s.get('active_concerns') or [])] or [0])
}
print(json.dumps(out, ensure_ascii=False))
"""
    result = sudo(
        "python3 -c " + shlex.quote(script) + " " + shlex.quote(state_path),
        capture=True,
        check=False,
    )
    if result.returncode:
        return {"state": "unavailable"}
    return json.loads(result.stdout)


def cmd_status() -> None:
    current = load_current(required=False)
    registered = str(current["instance_id"]) if current else "none"
    probe = r"""
printf 'active_instance='; cat /agent/boot/active-instance 2>/dev/null || echo none
printf 'service='; systemctl is-active hominal.service 2>/dev/null || true
printf 'pid='; systemctl show -p MainPID --value hominal.service 2>/dev/null || true
printf 'socket='; test -S /run/hominal/hominal.sock && echo ready || echo absent
"""
    result = ssh(probe, check=False, capture=True)
    print(f"registered_instance={registered}")
    print(result.stdout, end="")
    if current:
        print("runtime=" + json.dumps(safe_remote_state(registered), ensure_ascii=False, sort_keys=True))


def mentor_api(method: str, path: str, payload: dict[str, object] | None = None) -> object:
    data = None if payload is None else json.dumps(payload, ensure_ascii=False)
    command = [
        "curl",
        "--silent",
        "--show-error",
        "--fail-with-body",
        "--unix-socket",
        SOCKET_PATH,
        "-X",
        method,
        "http://localhost" + path,
    ]
    if payload is not None:
        command.extend(["-H", "Content-Type: application/json", "--data-binary", "@-"])
    result = ssh(
        " ".join(shlex.quote(part) for part in command),
        capture=True,
        input_text=data,
        timeout=15,
    )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError("mentor endpoint returned invalid JSON") from exc


def cmd_mentor_send(body: str | None, speaker: str, message_id: str | None, reply_to: str | None) -> None:
    load_current()
    if body is None:
        body = sys.stdin.read().strip()
    if not body:
        raise RuntimeError("mentor message body is empty")
    prefix = "[Codex代理导师]" if speaker == "codex" else "[人类导师·经Codex传递]"
    message_id = message_id or f"codex-{utcstamp()}-{uuid.uuid4().hex[:8]}"
    payload: dict[str, object] = {"message_id": message_id, "body": prefix + "\n" + body}
    if reply_to:
        payload["reply_to"] = reply_to
    response = mentor_api("POST", "/v1/mentor/inbox", payload)
    print(json.dumps(response, ensure_ascii=False, sort_keys=True))
    print(f"message_id={message_id}")


def cmd_mentor_outbox() -> None:
    load_current()
    response = mentor_api("GET", "/v1/mentor/outbox")
    print(json.dumps(response, ensure_ascii=False, indent=2, sort_keys=True))


def cmd_mentor_ack(message_id: str) -> None:
    if not message_id or not INSTANCE_RE.fullmatch(message_id):
        raise RuntimeError("invalid outbox message id")
    response = mentor_api("POST", f"/v1/mentor/outbox/{message_id}/ack", {})
    print(json.dumps(response, ensure_ascii=False, sort_keys=True))


def cmd_crash() -> None:
    current = load_current()
    instance_id = str(current["instance_id"])
    before = ssh("systemctl show -p MainPID --value hominal.service", capture=True).stdout.strip()
    if not before or before == "0":
        raise RuntimeError("hominal.service has no running process")
    sudo(f"touch {shlex.quote('/agent/lives/' + instance_id + '/state/crash-request')}")
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        after = ssh("systemctl show -p MainPID --value hominal.service", check=False, capture=True).stdout.strip()
        if after and after != "0" and after != before:
            wait_for_runtime(instance_id)
            print(f"old_pid={before}")
            print(f"new_pid={after}")
            print("crash_recovery=passed")
            return
        time.sleep(2)
    raise RuntimeError("systemd did not replace the crashed runtime within 45 seconds")


def archive_instance(current: dict[str, object]) -> Path:
    instance_id = str(current["instance_id"])
    destination = ARCHIVE_ROOT / instance_id
    destination.mkdir(parents=True, exist_ok=True)
    destination.chmod(0o700)
    final_archive = destination / "agent-final.tar.gz"
    if not final_archive.is_file():
        remote_archive = f"/agent/tmp/{instance_id}.tar.gz"
        sudo(
            f"tar --xattrs --acls --numeric-owner -czf {shlex.quote(remote_archive)} "
            f"-C /agent/lives {shlex.quote(instance_id)} && "
            f"chown hominal:hominal {shlex.quote(remote_archive)}"
        )
        partial = destination / "agent-final.tar.gz.partial"
        scp(remote_archive, str(partial), from_remote=True)
        os.replace(partial, final_archive)
        ssh(f"rm -f {shlex.quote(remote_archive)}")
    (destination / "manifest.json").write_text(
        json.dumps(current, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    since = str(current.get("prepared_at", ""))
    journal_command = "journalctl -u hominal.service --no-pager -n 1000"
    if since:
        journal_command += " --since " + shlex.quote(since)
    log = ssh(journal_command, check=False, capture=True).stdout
    (destination / "systemd.log").write_text(log or "", encoding="utf-8")
    hashes = []
    for name in ("agent-final.tar.gz", "manifest.json", "systemd.log"):
        hashes.append(f"{sha256(destination / name)}  {name}")
    (destination / "hashes.sha256").write_text("\n".join(hashes) + "\n", encoding="utf-8")
    return destination


def cmd_stop() -> None:
    current = load_current()
    sudo(f"systemctl stop hominal.service; rm -f {shlex.quote(REMOTE_RUNTIME_CONFIG)}")
    state = ssh("systemctl is-active hominal.service", check=False, capture=True).stdout.strip()
    if state == "active":
        raise RuntimeError("hominal.service remained active after explicit stop")
    current["stopped_at"] = datetime.now(timezone.utc).isoformat()
    destination = archive_instance(current)
    current["archive_path"] = str(destination)
    save_current(current)
    print(f"archive={destination}")
    print("stop=passed")


def cmd_reset() -> None:
    current = load_current()
    instance_id = str(current["instance_id"])
    destination = ARCHIVE_ROOT / instance_id / "agent-final.tar.gz"
    if not destination.is_file():
        raise RuntimeError("engineering instance is not archived; run stop first")
    instance_root = "/agent/lives/" + instance_id
    expected = shlex.quote(instance_id)
    sudo(
        "set -eu; systemctl stop hominal.service; "
        "active=$(cat /agent/boot/active-instance 2>/dev/null || true); "
        f"if [ -n \"$active\" ] && [ \"$active\" != {expected} ]; then exit 24; fi; "
        f"rm -rf -- {shlex.quote(instance_root)}; "
        "rm -f -- /agent/boot/active-instance /agent/boot/active-release; "
        "if [ -L /life ]; then rm -f /life; fi; "
        f"rm -f {shlex.quote(REMOTE_RUNTIME_CONFIG)}"
    )
    CURRENT_PATH.unlink()
    print(f"reset_instance={instance_id}")
    print("reset=passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("check")
    start = subparsers.add_parser("start")
    start.add_argument("--stage", type=int, choices=(3, 4), required=True)
    subparsers.add_parser("status")
    mentor_send = subparsers.add_parser("mentor-send")
    mentor_send.add_argument("--body")
    mentor_send.add_argument("--speaker", choices=("codex", "human"), default="codex")
    mentor_send.add_argument("--message-id")
    mentor_send.add_argument("--reply-to")
    subparsers.add_parser("mentor-outbox")
    mentor_ack = subparsers.add_parser("mentor-ack")
    mentor_ack.add_argument("message_id")
    subparsers.add_parser("crash")
    subparsers.add_parser("stop")
    subparsers.add_parser("reset")
    args = parser.parse_args()
    try:
        if args.command == "check":
            cmd_check()
        elif args.command == "start":
            cmd_start(args.stage)
        elif args.command == "status":
            cmd_status()
        elif args.command == "mentor-send":
            cmd_mentor_send(args.body, args.speaker, args.message_id, args.reply_to)
        elif args.command == "mentor-outbox":
            cmd_mentor_outbox()
        elif args.command == "mentor-ack":
            cmd_mentor_ack(args.message_id)
        elif args.command == "crash":
            cmd_crash()
        elif args.command == "stop":
            cmd_stop()
        elif args.command == "reset":
            cmd_reset()
    except (KeyError, OSError, RuntimeError, subprocess.TimeoutExpired) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
