#!/usr/bin/env python3
"""Minimal external runner for the G0 stage-two Ubuntu body rehearsal."""

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

import yaml


ROOT = Path(__file__).resolve().parents[1]
CONFIG_PATH = ROOT.parent / "xconfig.yaml"
INSTANCE_RE = re.compile(r"^[A-Za-z0-9._-]+$")


def load_settings() -> dict[str, object]:
    data = yaml.safe_load(CONFIG_PATH.read_text(encoding="utf-8"))
    system = data["system"]
    return {
        "host": system["body"]["ssh_host_alias"],
        "sudo_password": system["remote_server"]["password"],
        "archive": Path(system["genesis_lab"]["archive_path"]) / "rehearsals",
        "live": Path(system["genesis_lab"]["live_stream_path"]),
    }


SETTINGS = load_settings()
HOST = str(SETTINGS["host"])
ARCHIVE_ROOT = Path(SETTINGS["archive"])
LIVE_ROOT = Path(SETTINGS["live"])
CURRENT_PATH = LIVE_ROOT / "stage2-current.json"
SSH_BASE = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", HOST]


def run(
    args: list[str],
    *,
    check: bool = True,
    input_text: str | None = None,
    capture: bool = False,
    timeout: int | None = None,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        args,
        check=False,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
        timeout=timeout,
    )
    if check and result.returncode:
        detail = (result.stderr or result.stdout or "command failed").strip()
        raise RuntimeError(detail)
    return result


def ssh(command: str, *, check: bool = True, capture: bool = False, timeout: int = 30) -> subprocess.CompletedProcess[str]:
    remote = "bash -lc " + shlex.quote(command)
    return run([*SSH_BASE, remote], check=check, capture=capture, timeout=timeout)


def sudo(command: str, *, check: bool = True, capture: bool = False, timeout: int = 30) -> subprocess.CompletedProcess[str]:
    remote = "sudo -S -p '' -- bash -lc " + shlex.quote(command)
    sudo_input = str(SETTINGS["sudo_password"]) + "\n"
    return run([*SSH_BASE, remote], check=check, capture=capture, input_text=sudo_input, timeout=timeout)


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
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def ensure_external_dirs() -> None:
    for path in (ARCHIVE_ROOT, LIVE_ROOT):
        path.mkdir(parents=True, exist_ok=True)
        path.chmod(0o700)


def load_current(required: bool = True) -> dict[str, object] | None:
    if not CURRENT_PATH.is_file():
        if required:
            raise RuntimeError("no active stage-two rehearsal is registered")
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


def build_fixture(output: Path) -> str:
    env = os.environ.copy()
    env.update({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"})
    result = subprocess.run(
        [
            "go",
            "build",
            "-trimpath",
            "-buildvcs=false",
            "-ldflags=-s -w -buildid=",
            "-o",
            str(output),
            "./cmd/stage2-fixture",
        ],
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or "fixture build failed")
    output.chmod(0o755)
    return sha256(output)


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
printf 'go='; go version
printf 'service='; systemctl is-enabled hominal.service 2>/dev/null || true
"""
    result = ssh(probe, capture=True)
    print(result.stdout, end="")
    print(f"archive={ARCHIVE_ROOT}")
    print("check=passed")


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


def wait_for_reboot(previous_boot_id: str) -> None:
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            result = ssh("cat /proc/sys/kernel/random/boot_id", check=False, capture=True, timeout=10)
        except subprocess.TimeoutExpired:
            print("waiting for Ubuntu reboot...", flush=True)
            time.sleep(5)
            continue
        current = (result.stdout or "").strip()
        if result.returncode == 0 and current and current != previous_boot_id:
            return
        print("waiting for Ubuntu reboot...", flush=True)
        time.sleep(5)
    raise RuntimeError("Ubuntu did not return with a new boot id within 180 seconds")


def wait_for_fixture(instance_id: str) -> None:
    deadline = time.monotonic() + 45
    heartbeat = f"/agent/lives/{instance_id}/state/heartbeat.json"
    while time.monotonic() < deadline:
        result = ssh(
            f"systemctl is-active --quiet hominal.service && test -s {shlex.quote(heartbeat)}",
            check=False,
            timeout=10,
        )
        if result.returncode == 0:
            return
        time.sleep(2)
    raise RuntimeError("hominal.service did not produce a heartbeat within 45 seconds")


def cmd_start() -> None:
    ensure_external_dirs()
    if load_current(required=False) is not None:
        raise RuntimeError("a rehearsal is already registered; run stop and reset first")
    cmd_check()
    previous_boot_id = ssh("cat /proc/sys/kernel/random/boot_id", capture=True).stdout.strip()

    with tempfile.TemporaryDirectory(prefix="hominal-stage2-") as directory:
        binary = Path(directory) / "hominald"
        binary_hash = build_fixture(binary)
        release_id = f"stage2-{binary_hash[:12]}"
        instance_id = f"stage2-{utcstamp().lower()}-{binary_hash[:6]}"
        if not INSTANCE_RE.fullmatch(instance_id):
            raise RuntimeError("generated invalid instance id")

        ssh("install -d -m 0755 /agent/boot /agent/releases /agent/lives /agent/staging /agent/tmp")
        upload = f"/agent/staging/{release_id}.partial"
        scp(binary, upload)
        remote_hash = ssh(f"sha256sum {shlex.quote(upload)} | awk '{{print $1}}'", capture=True).stdout.strip()
        if remote_hash != binary_hash:
            raise RuntimeError("uploaded fixture hash mismatch")

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
chmod 0755 {shlex.quote(release_root)}/bin/hominald
install -d -m 0755 {shlex.quote(instance_root)}/birth {shlex.quote(instance_root)}/body/bin {shlex.quote(instance_root)}/state {shlex.quote(instance_root)}/journal {shlex.quote(instance_root)}/life/mentor/inbox {shlex.quote(instance_root)}/life/mentor/outbox {shlex.quote(instance_root)}/logs
cp {shlex.quote(release_root)}/bin/hominald {shlex.quote(instance_root)}/body/bin/hominald
printf '%s\n' {shlex.quote(release_id)} > /agent/boot/active-release.partial
printf '%s\n' {shlex.quote(instance_id)} > /agent/boot/active-instance.partial
mv /agent/boot/active-release.partial /agent/boot/active-release
mv /agent/boot/active-instance.partial /agent/boot/active-instance
"""
        ssh(remote_prepare)

        manifest = {
            "kind": "stage2-rehearsal",
            "instance_id": instance_id,
            "release_id": release_id,
            "release_sha256": binary_hash,
            "prepared_at": datetime.now(timezone.utc).isoformat(),
            "git_commit": run(["git", "rev-parse", "HEAD"], capture=True).stdout.strip(),
        }
        manifest_path = Path(directory) / "manifest.json"
        manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        scp(manifest_path, f"{instance_root}/birth/manifest.json")
        install_host_files()
        sudo(
            f"if [ -e /life ] && [ ! -L /life ]; then exit 23; fi; "
            f"ln -sfn {shlex.quote(instance_root + '/life')} /life"
        )
        save_current(manifest)

    print(f"instance={instance_id}")
    print("rebooting Ubuntu...", flush=True)
    sudo("systemctl reboot", check=False)
    wait_for_reboot(previous_boot_id)
    wait_for_fixture(instance_id)
    print("start=passed")


def cmd_status() -> None:
    current = load_current(required=False)
    registered = str(current["instance_id"]) if current else "none"
    probe = r"""
printf 'active_instance='; cat /agent/boot/active-instance 2>/dev/null || echo none
printf 'service='; systemctl is-active hominal.service 2>/dev/null || true
printf 'pid='; systemctl show -p MainPID --value hominal.service 2>/dev/null || true
active=$(cat /agent/boot/active-instance 2>/dev/null || true)
if [ -n "$active" ] && [ -s "/agent/lives/$active/state/heartbeat.json" ]; then
  printf 'heartbeat='; cat "/agent/lives/$active/state/heartbeat.json"
else
  printf 'heartbeat=none\n'
fi
"""
    result = ssh(probe, check=False, capture=True)
    print(f"registered_instance={registered}")
    print(result.stdout, end="")


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
    log = ssh("journalctl -u hominal.service --no-pager -n 300", check=False, capture=True).stdout
    (destination / "systemd.log").write_text(log or "", encoding="utf-8")
    hashes = []
    for name in ("agent-final.tar.gz", "manifest.json", "systemd.log"):
        hashes.append(f"{sha256(destination / name)}  {name}")
    (destination / "hashes.sha256").write_text("\n".join(hashes) + "\n", encoding="utf-8")
    return destination


def cmd_stop() -> None:
    current = load_current()
    sudo("systemctl stop hominal.service")
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
        raise RuntimeError("rehearsal is not archived; run stop first")
    sudo("systemctl stop hominal.service")
    sudo(
        f"rm -rf -- /agent/lives/{shlex.quote(instance_id)} && "
        "rm -f -- /agent/boot/active-instance /agent/boot/active-release && "
        "if [ -L /life ]; then rm -f /life; fi"
    )
    CURRENT_PATH.unlink()
    print(f"reset_instance={instance_id}")
    print("reset=passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("check", "start", "status", "stop", "reset"))
    args = parser.parse_args()
    try:
        {
            "check": cmd_check,
            "start": cmd_start,
            "status": cmd_status,
            "stop": cmd_stop,
            "reset": cmd_reset,
        }[args.command]()
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
