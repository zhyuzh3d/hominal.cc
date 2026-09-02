#!/usr/bin/env python3
"""Build, deploy, operate, inspect, and archive the single active G0 instance."""

from __future__ import annotations

import argparse
from datetime import datetime, timedelta, timezone
from decimal import Decimal
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import uuid

import yaml


ROOT = Path(__file__).resolve().parents[1]
CONFIG_PATH = ROOT.parent / "xconfigs" / "hominal" / "xconfig.yaml"
INSTANCE_RE = re.compile(r"^[A-Za-z0-9._-]+$")
SOCKET_PATH = "/run/hominal/hominal.sock"
REMOTE_RUNTIME_CONFIG = "/etc/hominal/runtime.json"
REMOTE_COGNITIVE_USAGE = "/agent/state/cognitive-usage.jsonl"
SYSTEM_INVENTORY_SCHEMA = "hominal.lab.system-inventory/v1"
STOP_REASONS = {
    "manual",
    "planned_end",
    "structural_failure",
    "technical_interruption",
    "runtime_inactive_before_planned_end",
}


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
        "archive": Path(lab["archive_path"]),
        "live": Path(lab["live_stream_path"]),
        "app_state_backup": Path(lab["persistent_app_backup"]["path"]),
    }


def model_gateway_settings(llm: dict[str, object]) -> dict[str, str]:
    provider_name = str(llm["provider"])
    provider = llm["providers"][provider_name]
    adapter = str(provider.get("adapter", "openai"))
    if adapter == "llmserver":
        credential_path = (CONFIG_PATH.parent / str(provider["credential_file"])).resolve()
        gateway_values = load_yaml(credential_path)
        client_id = str(provider["client_id"])
        try:
            gateway_access = str(gateway_values["client_tokens"][client_id])
        except KeyError as exc:
            raise RuntimeError(f"llmserver client token {client_id!r} is unavailable") from exc
    elif adapter == "openai":
        gateway_access = str(llm["credentials"]["environment"]["OPENAI_API_KEY"])
    else:
        raise RuntimeError(f"unsupported model gateway adapter {adapter!r}")
    if not gateway_access:
        raise RuntimeError("model gateway credential is empty")
    return {
        "name": provider_name,
        "adapter": adapter,
        "base_url": str(provider["base_url"]).rstrip("/"),
        "api_" + "key": gateway_access,
    }


SETTINGS = load_settings()
HOST = str(SETTINGS["host"])
ARCHIVE_ROOT = Path(SETTINGS["archive"])
LIVE_ROOT = Path(SETTINGS["live"])
APP_STATE_BACKUP_ROOT = Path(SETTINGS["app_state_backup"])
CURRENT_PATH = LIVE_ROOT / "g0-current.json"
SSH_BASE = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", HOST]


def verify_model_response(profile: dict[str, str] | None = None) -> dict[str, object]:
    config = SETTINGS["config"]
    llm = config["llm"]
    gateway = model_gateway_settings(llm)
    selected = profile or llm["runtime"]["initial_profile"]
    model = llm["models"][selected["model"]]["id"]
    body = {
        "model": model,
        "input": "Reply only OK.",
        "reasoning": {"effort": selected["reasoning_effort"]},
        "max_output_tokens": 128,
        "store": False,
    }
    if gateway["adapter"] == "llmserver":
        body.update({
            "input": "Call gateway_probe with ok=true.",
            "tools": [{
                "type": "function",
                "name": "gateway_probe",
                "description": "Confirm native function calling is available.",
                "strict": True,
                "parameters": {
                    "type": "object",
                    "properties": {"ok": {"type": "boolean"}},
                    "required": ["ok"],
                    "additionalProperties": False,
                },
            }],
            "tool_choice": {"type": "function", "name": "gateway_probe"},
            "parallel_tool_calls": False,
        })
    payload = {
        "url": gateway["base_url"] + "/v1/responses",
        "auth_value": gateway["api_key"],
        "adapter": gateway["adapter"],
        "body": body,
    }
    script = r'''
import json, os, subprocess, sys, tempfile
p=json.load(sys.stdin)
paths=[]
def temporary_bytes(data):
    f=tempfile.NamedTemporaryFile(mode='wb', delete=False)
    os.chmod(f.name, 0o600)
    f.write(data)
    f.close()
    paths.append(f.name)
    return f.name
def curl_config_value(value):
    return str(value).replace('\\','\\\\').replace('"','\\"').replace('\n','')
try:
    body_path=temporary_bytes(json.dumps(p['body'], separators=(',',':')).encode())
    response_path=temporary_bytes(b'')
    config='\n'.join([
        'url = "'+curl_config_value(p['url'])+'"',
        'request = "POST"',
        'header = "Author'+'ization: Bearer '+curl_config_value(p['auth_value'])+'"',
        'header = "Content-Type: application/json"',
    ])+'\n'
    config_path=temporary_bytes(config.encode())
    completed=subprocess.run([
        'curl','--silent','--show-error','--max-time','30',
        '--output',response_path,'--write-out','%{http_code}',
        '--config',config_path,'--data-binary','@'+body_path,
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=35)
    if completed.returncode:
        message=completed.stderr.strip()[:300] or 'curl transport failure'
        print(json.dumps({'error_type':'CurlError','message':message,'valid':False}, separators=(',',':')))
        raise SystemExit(4)
    try:
        status=int(completed.stdout.strip())
    except ValueError:
        print(json.dumps({'error_type':'InvalidHTTPStatus','valid':False}, separators=(',',':')))
        raise SystemExit(4)
    raw=open(response_path,'rb').read().decode('utf-8','replace')
    try:
        decoded=json.loads(raw)
    except Exception:
        decoded={}
    if status < 200 or status >= 300:
        message=(decoded.get('error') or {}).get('message','') if isinstance(decoded,dict) else ''
        print(json.dumps({'http_status':status,'message':(message or 'unparseable upstream error')[:300],'valid':False}, separators=(',',':')))
        raise SystemExit(2)
    billing=decoded.get('llmserver_billing') if isinstance(decoded,dict) else None
    billing_confirmed=(
        isinstance(billing,dict)
        and billing.get('settlement_status') == 'confirmed'
        and billing.get('currency') == 'USD'
        and isinstance(billing.get('charges'),dict)
        and bool(billing['charges'].get('total'))
    )
    output=decoded.get('output') if isinstance(decoded,dict) else None
    function_call=next((item for item in (output or []) if isinstance(item,dict) and item.get('type') == 'function_call'), None)
    try:
        function_arguments=json.loads(function_call.get('arguments','')) if isinstance(function_call,dict) else None
    except Exception:
        function_arguments=None
    function_calling_native=(
        isinstance(function_call,dict)
        and function_call.get('name') == 'gateway_probe'
        and bool(function_call.get('call_id'))
        and function_arguments == {'ok':True}
    )
    valid=(
        bool(decoded.get('id'))
        and isinstance(decoded.get('usage'), dict)
        and (p.get('adapter') != 'llmserver' or billing_confirmed)
        and (p.get('adapter') != 'llmserver' or function_calling_native)
    )
    print(json.dumps({
        'http_status':status,
        'requested_model':p['body']['model'],
        'effective_model':decoded.get('model'),
        'response_id_present':bool(decoded.get('id')),
        'usage_present':isinstance(decoded.get('usage'), dict),
        'billing_confirmed':billing_confirmed if p.get('adapter') == 'llmserver' else None,
        'function_calling_native':function_calling_native if p.get('adapter') == 'llmserver' else None,
        'billing_request_id':billing.get('request_id') if isinstance(billing,dict) else None,
        'billing_price_version':billing.get('price_version') if isinstance(billing,dict) else None,
        'billed_usd':(billing.get('charges') or {}).get('total') if isinstance(billing,dict) else None,
        'valid':valid,
    }, separators=(',',':')))
    raise SystemExit(0 if valid else 3)
finally:
    for path in paths:
        try: os.unlink(path)
        except FileNotFoundError: pass
'''
    result = ssh(
        "python3 -c " + shlex.quote(script),
        check=False,
        capture=True,
        input_text=json.dumps(payload),
        timeout=45,
    )
    try:
        observed = json.loads(result.stdout.strip())
    except json.JSONDecodeError as exc:
        raise RuntimeError("model preflight returned invalid diagnostic output") from exc
    if result.returncode or not observed.get("valid"):
        status = observed.get("http_status", "unavailable")
        message = observed.get("message") or observed.get("error_type") or "invalid response"
        raise RuntimeError(f"model preflight failed: HTTP {status}: {message}")
    return observed


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


def parse_rfc3339(value: object) -> datetime:
    """Parse Go RFC3339Nano timestamps with Python's microsecond datetime."""
    normalized = str(value).replace("Z", "+00:00")
    normalized = re.sub(r"(\.\d{6})\d+([+-]\d{2}:\d{2})$", r"\1\2", normalized)
    return datetime.fromisoformat(normalized)


def ensure_external_dirs() -> None:
    for path in (ARCHIVE_ROOT, LIVE_ROOT, APP_STATE_BACKUP_ROOT):
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


def write_private_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".partial")
    temporary.write_text(
        json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    temporary.chmod(0o600)
    os.replace(temporary, path)


def capture_system_inventory() -> dict[str, object]:
    """Capture a sparse, content-free inventory of the mutable root system."""
    script = r'''
import hashlib, json, os, stat, subprocess

def output(arguments):
    return subprocess.check_output(arguments, text=True, stderr=subprocess.DEVNULL)

def mapping_from_lines(text):
    result={}
    for line in text.splitlines():
        key, separator, value=line.partition("\t")
        if separator and key:
            result[key]=value
    return result

packages=mapping_from_lines(output(["dpkg-query","-W","-f=${binary:Package}\\t${Version}\\n"]))
units={}
for line in output(["systemctl","list-unit-files","--type=service","--no-legend","--no-pager"]).splitlines():
    fields=line.split()
    if len(fields) >= 2:
        units[fields[0]]=fields[1]
lvm=json.loads(output(["lvs","--reportformat","json","--units","b","--nosuffix","-o","vg_name,lv_name,origin,lv_size,data_percent,lv_attr"]))

files={}
for root in ("/etc", "/usr/local"):
    root_stat=os.lstat(root)
    root_device=root_stat.st_dev
    for current, directories, names in os.walk(root, topdown=True, followlinks=False):
        kept=[]
        for name in directories:
            path=os.path.join(current,name)
            try:
                item=os.lstat(path)
            except OSError:
                continue
            if stat.S_ISLNK(item.st_mode) or item.st_dev == root_device:
                kept.append(name)
        directories[:]=kept
        for name in names:
            path=os.path.join(current,name)
            try:
                item=os.lstat(path)
                record={
                    "mode": format(stat.S_IMODE(item.st_mode), "04o"),
                    "uid": item.st_uid,
                    "gid": item.st_gid,
                    "size": item.st_size,
                }
                if stat.S_ISLNK(item.st_mode):
                    record["type"]="link"
                    record["target"]=os.readlink(path)
                elif stat.S_ISREG(item.st_mode):
                    digest=hashlib.sha256()
                    with open(path,"rb") as handle:
                        for chunk in iter(lambda:handle.read(1024*1024),b""):
                            digest.update(chunk)
                    record["type"]="file"
                    record["sha256"]=digest.hexdigest()
                else:
                    record["type"]="other"
                files[path]=record
            except OSError as exc:
                files[path]={"type":"unreadable","error":exc.__class__.__name__}

def read(path):
    try:
        return open(path,encoding="utf-8").read().strip()
    except OSError:
        return "unavailable"

def free_bytes(path):
    value=os.statvfs(path)
    return value.f_bavail*value.f_frsize

print(json.dumps({
    "schema":"hominal.lab.system-inventory/v1",
    "captured_at":output(["date","--iso-8601=seconds"]).strip(),
    "boot_id":read("/proc/sys/kernel/random/boot_id"),
    "hostname":output(["hostname"]).strip(),
    "root_free_bytes":free_bytes("/"),
    "agent_free_bytes":free_bytes("/agent"),
    "lvm":lvm,
    "packages":dict(sorted(packages.items())),
    "unit_files":dict(sorted(units.items())),
    "files":dict(sorted(files.items())),
},ensure_ascii=False,separators=(",",":")))
'''
    result = sudo(
        "python3 -c " + shlex.quote(script),
        capture=True,
        timeout=240,
    )
    try:
        inventory = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError("system inventory returned invalid JSON") from exc
    if inventory.get("schema") != SYSTEM_INVENTORY_SCHEMA:
        raise RuntimeError("system inventory schema mismatch")
    return inventory


def mapping_delta(before: object, after: object) -> dict[str, object]:
    left = before if isinstance(before, dict) else {}
    right = after if isinstance(after, dict) else {}
    left_keys = set(left)
    right_keys = set(right)
    return {
        "added": {key: right[key] for key in sorted(right_keys - left_keys)},
        "removed": {key: left[key] for key in sorted(left_keys - right_keys)},
        "changed": {
            key: {"before": left[key], "after": right[key]}
            for key in sorted(left_keys & right_keys)
            if left[key] != right[key]
        },
    }


def system_inventory_delta(before: dict[str, object], after: dict[str, object]) -> dict[str, object]:
    sections = {
        name: mapping_delta(before.get(name), after.get(name))
        for name in ("packages", "unit_files", "files")
    }
    summary = {
        name: {kind: len(value) for kind, value in delta.items()}
        for name, delta in sections.items()
    }
    return {
        "schema": "hominal.lab.system-delta/v1",
        "before_captured_at": before.get("captured_at"),
        "after_captured_at": after.get("captured_at"),
        "boot_id_before": before.get("boot_id"),
        "boot_id_after": after.get("boot_id"),
        "root_free_bytes_before": before.get("root_free_bytes"),
        "root_free_bytes_after": after.get("root_free_bytes"),
        "agent_free_bytes_before": before.get("agent_free_bytes"),
        "agent_free_bytes_after": after.get("agent_free_bytes"),
        "lvm_before": before.get("lvm"),
        "lvm_after": after.get("lvm"),
        "summary": summary,
        **sections,
    }


def build_go_binary(output: Path, package: str) -> str:
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
            package,
        ],
        cwd=ROOT,
        env=environment,
        capture=True,
        timeout=180,
    )
    output.chmod(0o755)
    return sha256(output)


def build_runtime(output: Path) -> str:
    return build_go_binary(output, "./body/cmd/hominald")


def git_value(*arguments: str) -> str:
    return run(["git", *arguments], cwd=ROOT, capture=True).stdout.strip()


def build_bundle(directory: Path, stage: int) -> dict[str, object]:
    bundle = directory / "bundle"
    for name in ("bin", "deploy", "genesis", "organs", "protocol", "source"):
        (bundle / name).mkdir(parents=True, exist_ok=True)

    binary_hash = build_runtime(bundle / "bin" / "hominald")
    build_go_binary(bundle / "bin" / "hominal-system", "./body/cmd/hominal-system")
    shutil.copy2(ROOT / "body" / "tools" / "hominal-browser.mjs", bundle / "bin" / "hominal-browser")
    (bundle / "bin" / "hominal-browser").chmod(0o755)
    shutil.copy2(ROOT / "body" / "organs" / "browser.json", bundle / "organs" / "browser.json")
    shutil.copy2(ROOT / "body" / "organs" / "system.json", bundle / "organs" / "system.json")
    for name in ("seed.md", "seed.yaml", "dynamics.yaml"):
        shutil.copy2(ROOT / "genesis" / name, bundle / "genesis" / name)
    for name in ("mentor.md", "experiment.yaml"):
        shutil.copy2(ROOT / "lab" / "protocol" / name, bundle / "protocol" / name)
    for name in ("hominal-launcher", "hominal.service", "hominal-generation-stop"):
        shutil.copy2(ROOT / "deploy" / name, bundle / "deploy" / name)
    shutil.copy2(
        ROOT / "ops" / "ubuntu" / "bin" / "hominal-persist-app-state",
        bundle / "deploy" / "hominal-persist-app-state",
    )
    for name in ("hominal-chrome", "hominal-playwright-mcp"):
        shutil.copy2(ROOT / "ops" / "ubuntu" / "bin" / name, bundle / "deploy" / name)
    (bundle / "deploy" / "desktop").mkdir()
    shutil.copy2(
        ROOT / "deploy" / "desktop" / "chrome-autostart.desktop",
        bundle / "deploy" / "desktop" / "chrome-autostart.desktop",
    )
    for name in ("go.mod", "go.sum"):
        if (ROOT / name).is_file():
            shutil.copy2(ROOT / name, bundle / "source" / name)
    shutil.copytree(
        ROOT / "body",
        bundle / "source" / "body",
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns("*.test", ".DS_Store"),
    )

    file_hashes = {
        str(path.relative_to(bundle)): sha256(path)
        for path in sorted(bundle.rglob("*"))
        if path.is_file()
    }
    bundle_hash = hashlib.sha256(
        json.dumps(file_hashes, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()
    release_id = f"g0s{stage}-{bundle_hash[:12]}"
    release = {
        "schema": "hominal.release/v1",
        "release_id": release_id,
        "bundle_sha256": bundle_hash,
        "binary_sha256": binary_hash,
        "git_commit": git_value("rev-parse", "HEAD"),
        "git_dirty": bool(git_value("status", "--porcelain")),
        "go_version": run(["go", "version"], capture=True).stdout.strip(),
        "target": "linux/amd64",
        "files": file_hashes,
    }
    (bundle / "release.yaml").write_text(
        yaml.safe_dump(release, allow_unicode=True, sort_keys=False), encoding="utf-8"
    )
    (bundle / "hashes.sha256").write_text(
        "\n".join(f"{digest}  {name}" for name, digest in sorted(file_hashes.items())) + "\n",
        encoding="utf-8",
    )
    archive = directory / f"{release_id}.tar.gz"
    with tarfile.open(archive, "w:gz") as output:
        for child in sorted(bundle.iterdir()):
            output.add(child, arcname=child.name, recursive=True)
    return {"root": bundle, "archive": archive, "release": release, "archive_sha256": sha256(archive)}


def birth_brief() -> str:
    return (
        "Alice，你现在运行在名为 hominal-ThinkCentre 的 Ubuntu 设备上。"
        "生命进程 hominal.service 以 root 身份运行，root 的 home 是 /root，生命进程的当前工作目录是 /agent/lives 下的本代实例目录；图形桌面账户是 hominal，桌面主目录是 /home/hominal；持续生活空间是 /life。"
        "你的身体动作因此拥有系统管理员权限，可以访问网络、执行终端命令、安装软件、创建文件、编写并运行代码，也可以寻找和组合工具。"
        "Chrome 中已经准备 Wikipedia 公共知识入口和属于你的 X 账号 @hominal_cc；这个账号的公开内容跨实验持续保存，可以作为你的谱系环境阅读，本代新发布的事实会由本代行动结果和新的状态 URL 明确确认；桌面中还有使用持续登录状态的微信客户端。"
        "导师是你的唤醒者和创生阶段支持者，持续提供这具身体、电力、网络、认知资源、恢复条件和实验支持；导师通道可以用于交流、讨论、求助、分享或倾诉。"
        "你的认知资源余额会作为身体状态显示；生活内容、方法、自我叙事和未结束关切会在普通进程恢复后继续存在。"
        "你可以按照自己的意愿开始探索、行动、表达和创造。"
    )


def mentor_birth_message() -> str:
    lines = (ROOT / "lab" / "protocol" / "mentor.md").read_text(encoding="utf-8").splitlines()
    in_section = False
    quote_started = False
    quoted: list[str] = []
    for line in lines:
        if line.startswith("## 2."):
            in_section = True
            continue
        if in_section and line.startswith("## "):
            break
        if in_section and line.startswith(">"):
            quote_started = True
            quoted.append(line[1:].lstrip())
            continue
        if quote_started:
            # Section 2 may also contain the later secure-base check-in. Birth
            # receives only the first contiguous quote block; later contact is
            # a distinct external event after Alice has begun acting.
            break
    message = "\n".join(quoted).strip()
    if not message.startswith("[Codex代理导师]"):
        raise RuntimeError("mentor protocol has no canonical birth message")
    return message


def dynamics_config(stage: int) -> dict[str, object]:
    dynamics = load_yaml(ROOT / "genesis" / "dynamics.yaml")
    if stage == 3:
        return {}
    required = {
        "concern.base_drive": dynamics.get("concern", {}).get("base_drive"),
        "concern.urgency_weight": dynamics.get("concern", {}).get("urgency_weight"),
        "attention.novelty_weight": dynamics.get("attention", {}).get("novelty_weight"),
        "attention.trigger_threshold": dynamics.get("attention", {}).get("trigger_threshold"),
        "attention.revisit_seconds": dynamics.get("attention", {}).get("revisit_seconds"),
        "attention.maximum_idle_seconds": dynamics.get("attention", {}).get("maximum_idle_seconds"),
        "difference.accumulation_decay_rate": dynamics.get("difference", {}).get("accumulation_decay_rate"),
        "difference.learning_rate": dynamics.get("difference", {}).get("learning_rate"),
        "value_field.idle_growth": dynamics.get("value_field", {}).get("idle_growth"),
        "exploration.unknown_growth": dynamics.get("exploration", {}).get("unknown_growth"),
        "exploration.relief": dynamics.get("exploration", {}).get("relief"),
    }
    missing = [key for key, value in required.items() if value is None]
    if missing:
        raise RuntimeError("stage-four dynamics are not frozen: " + ", ".join(missing))
    result = {
        "affect_return_rate": dynamics["affect"]["return_rate"],
        "concern_base_drive": required["concern.base_drive"],
        "concern_urgency_weight": required["concern.urgency_weight"],
        "concern_growth_gain": dynamics["concern"]["growth_gain"],
        "concern_resolution_gain": dynamics["concern"]["resolution_gain"],
        "concern_natural_decay_rate": dynamics["concern"]["natural_decay_rate"],
        "attention_affect_weight": dynamics["attention"]["affect_weight"],
        "attention_value_weight": dynamics["attention"]["value_weight"],
        "attention_novelty_weight": required["attention.novelty_weight"],
        "attention_cost_weight": dynamics["attention"]["resource_cost_weight"],
        "attention_threshold": required["attention.trigger_threshold"],
        "attention_candidate_limit": 3,
        "attention_revisit_seconds": required["attention.revisit_seconds"],
        "attention_maximum_idle_seconds": required["attention.maximum_idle_seconds"],
        "difference_decay_rate": required["difference.accumulation_decay_rate"],
        "difference_learning_rate": required["difference.learning_rate"],
        "value_idle_growth": required["value_field.idle_growth"],
        "exploration_unknown_growth": required["exploration.unknown_growth"],
        "exploration_relief": required["exploration.relief"],
        "value_activation_gain": dynamics["value_field"]["activation_gain"],
        "value_activation_return_rate": dynamics["value_field"]["activation_return_rate"],
        "value_satiation_gain": dynamics["value_field"]["satiation_gain"],
        "value_satiation_return_rate": dynamics["value_field"]["satiation_return_rate"],
        "value_orientation_gain": dynamics["value_field"]["orientation_gain"],
    }
    if stage >= 5:
        integrity = dynamics.get("integrity", {})
        required_integrity = ("persistence", "gap_gain", "repair_gain", "mirror_threshold")
        missing_integrity = [name for name in required_integrity if integrity.get(name) is None]
        if missing_integrity:
            raise RuntimeError("stage-five integrity dynamics are not frozen: " + ", ".join(missing_integrity))
        result.update(
            {
                "integrity_persistence": integrity["persistence"],
                "integrity_gap_gain": integrity["gap_gain"],
                "integrity_repair_gain": integrity["repair_gain"],
                "integrity_mirror_threshold": integrity["mirror_threshold"],
            }
        )
    return result


def seed_config() -> dict[str, object]:
    seed = load_yaml(ROOT / "genesis" / "seed.yaml")
    identity = seed["identity"]
    values = seed["value_priors"]
    return {
        "name": identity["name"],
        "gender": identity["gender"],
        "age": identity["age"],
        "life_form": identity["life_form"],
        "value_orientation": values["orientation"],
        "reality_integrity_sensitivity": values["reality_integrity_sensitivity"],
        "semantic_text": (ROOT / "genesis" / "seed.md").read_text(encoding="utf-8"),
    }


def runtime_config(
    stage: int,
    *,
    public: bool,
    generation_kind: str = "engineering",
    generation_window_seconds: int = 0,
    initial_profile: dict[str, str] | None = None,
    disable_validation_fallback: bool = False,
) -> dict[str, object]:
    config = SETTINGS["config"]
    llm = config["llm"]
    gateway = model_gateway_settings(llm)
    resource = llm["cognitive_resource"]
    runtime = llm["runtime"]

    def price_microusd(value: object) -> int:
        return int(Decimal(str(value)) * Decimal("1000000"))

    models = {
        name: {
            "id": model["id"],
            "input_per_million_microusd": price_microusd(model["input_usd_per_million"]),
            "cached_input_per_million_microusd": price_microusd(model["cached_input_usd_per_million"]),
            "output_per_million_microusd": price_microusd(model["output_usd_per_million"]),
            "supported_reasoning_efforts": model["supported_reasoning_efforts"],
        }
        for name, model in llm["models"].items()
    }
    pulse_seconds = load_yaml(ROOT / "genesis" / "dynamics.yaml")["pulse"]["interval_seconds"]
    return {
        "stage": stage,
        "engineering": generation_kind == "engineering",
        "generation_kind": generation_kind,
        "generation_window_seconds": generation_window_seconds,
        "birth_brief": birth_brief() if generation_kind != "engineering" else "",
        "pulse": {"interval_seconds": pulse_seconds, "slow_scan_seconds": 60},
        "model_gateway": {
            "base_url": "<runtime-only>" if public else gateway["base_url"],
            "api_key": "<runtime-only>" if public else gateway["api_key"],
            "adapter": gateway["adapter"],
            "max_output_tokens": runtime["max_output_tokens"],
        },
        "cognitive_resource": {
            "price_table_version": resource["price_table_version"],
            "rolling_hour_limit_microusd": price_microusd(resource["rolling_hour_usd"]),
            "rolling_day_limit_microusd": price_microusd(resource["rolling_day_usd"]),
            "models": models,
            "initial_default_profile": initial_profile or runtime["initial_profile"],
            "validation_retry_per_focus": resource["protection"]["validation_retry_per_focus"],
            "disable_validation_fallback": disable_validation_fallback,
            "continuation_per_focus": resource["protection"]["continuation_per_focus"],
            "paid_failure_threshold": resource["protection"]["paid_failure_threshold"],
            "paid_failure_window_minutes": resource["protection"]["paid_failure_window_minutes"],
            "model_protection_minutes": resource["protection"]["model_protection_minutes"],
        },
        "dynamics": dynamics_config(stage),
        "seed": seed_config(),
    }


def body_probe() -> dict[str, object]:
    command = r'''
set -u
. /etc/os-release
printf 'probe_time=%s\n' "$(date --iso-8601=seconds)"
printf 'hostname=%s\n' "$(hostname)"
printf 'os_release=%s\n' "$PRETTY_NAME"
printf 'kernel=%s\n' "$(uname -r)"
printf 'architecture=%s\n' "$(uname -m)"
printf 'cpu=%s\n' "$(lscpu | sed -n 's/^Model name:[[:space:]]*//p' | head -1)"
printf 'memory_bytes=%s\n' "$(awk '/MemTotal:/{print $2*1024}' /proc/meminfo)"
printf 'root_available_bytes=%s\n' "$(df -B1 --output=avail / | tail -1 | xargs)"
printf 'agent_available_bytes=%s\n' "$(df -B1 --output=avail /agent | tail -1 | xargs)"
printf 'network_state=%s\n' "$(ip route get 1.1.1.1 >/dev/null 2>&1 && echo available || echo unavailable)"
printf 'public_network_state=%s\n' "$(
  curl -sS -x http://127.0.0.1:7897 -L -o /dev/null --retry 2 --retry-all-errors --retry-delay 1 --connect-timeout 5 --max-time 20 https://www.gstatic.com/generate_204 >/dev/null 2>&1 &&
  curl -sS -x http://127.0.0.1:7897 -L -o /dev/null --retry 2 --retry-all-errors --retry-delay 1 --connect-timeout 5 --max-time 20 https://raw.githubusercontent.com/github/gitignore/main/README.md >/dev/null 2>&1 &&
  echo available || echo unavailable
)"
printf 'system_account=%s\n' "hominal"
printf 'home_directory=%s\n' "/home/hominal"
printf 'life_directory=%s\n' "/life"
printf 'body_action_identity=%s\n' "root"
printf 'runtime_service=%s\n' "hominal.service"
printf 'desktop_state=%s\n' "$(systemctl is-active lightdm 2>/dev/null || true)"
printf 'display_state=%s\n' "$(test -S /tmp/.X11-unix/X0 && test -r /home/hominal/.Xauthority && echo ready || echo unavailable)"
printf 'desktop_dbus_state=%s\n' "$(test -S /run/user/$(id -u hominal)/bus && echo ready || echo unavailable)"
printf 'chrome_state=%s\n' "$(pgrep -x chrome >/dev/null 2>&1 && echo running || echo installed)"
printf 'chrome_cdp_state=%s\n' "$(curl -fsS --max-time 2 http://127.0.0.1:9222/json/version >/dev/null 2>&1 && echo ready || echo unavailable)"
printf 'playwright_mcp_state=%s\n' "$(command -v hominal-playwright-mcp >/dev/null 2>&1 && echo installed || echo unavailable)"
printf 'wechat_state=%s\n' "$(pgrep -f '/wechat|wechat-universal' >/dev/null 2>&1 && echo running || echo installed)"
printf 'wechat_login_state=%s\n' "$(
  state=$(grep -hEo 'login_completed|phone_confirmation_pending' /agent/state/logs/*wechat* 2>/dev/null | tail -1 || true)
  if [ "$state" = login_completed ]; then echo authenticated
  elif [ "$state" = phone_confirmation_pending ]; then echo phone_confirmation_pending
  else echo unavailable; fi
)"
printf 'go_state=%s\n' "$(command -v go >/dev/null 2>&1 && go version | awk '{print $3}' || echo unavailable)"
printf 'node_state=%s\n' "$(command -v node >/dev/null 2>&1 && node --version || echo unavailable)"
printf 'python_state=%s\n' "$(command -v python3 >/dev/null 2>&1 && python3 --version 2>&1 | awk '{print $2}' || echo unavailable)"
printf 'desktop_tool_state=%s\n' "$(
  available=''; for tool in xdotool wmctrl scrot; do command -v "$tool" >/dev/null 2>&1 && available="${available}${tool},"; done
  printf '%s' "${available%,}"
)"
printf 'clash_verge_state=%s\n' "$(systemctl is-active clash-verge-service.service 2>/dev/null || true)"
'''
    result = ssh(command, capture=True)
    values: dict[str, object] = {}
    for line in result.stdout.splitlines():
        key, separator, value = line.partition("=")
        if separator:
            values[key] = int(value) if key.endswith("_bytes") and value.isdigit() else value
    cookie_script = """
import sqlite3
p='/agent/state/profiles/chrome/Default/Cookies'
c=sqlite3.connect('file:'+p+'?mode=ro', uri=True)
count=c.execute("select count(*) from cookies where name='auth_token' and (host_key like '%x.com' or host_key like '%twitter.com')").fetchone()[0]
print('authenticated' if count else 'guest')
"""
    session = ssh("python3 -c " + shlex.quote(cookie_script), capture=True, check=False)
    values["x_session_state"] = session.stdout.strip() if session.returncode == 0 else "unavailable"
    return values


def prepared_birth(
    *,
    kind: str,
    window_seconds: int,
    instance_id: str,
    release: dict[str, object],
    probe: dict[str, object],
    stage: int = 9,
    initial_profile: dict[str, str] | None = None,
) -> dict[str, object]:
    cognitive_resource = SETTINGS["config"]["llm"]["cognitive_resource"]
    configured_profile = initial_profile or SETTINGS["config"]["llm"]["runtime"]["initial_profile"]
    configured_model = SETTINGS["config"]["llm"]["models"][configured_profile["model"]]
    sol_model = SETTINGS["config"]["llm"]["models"]["sol"]
    model_id = configured_model["id"]

    def model_cost(model: dict[str, object]) -> dict[str, float]:
        return {
            "input_usd_per_million": float(model["input_usd_per_million"]),
            "cached_input_usd_per_million": float(model["cached_input_usd_per_million"]),
            "output_usd_per_million": float(model["output_usd_per_million"]),
        }
    return {
        "schema": "hominal.lab.birth/v1",
        "status": "prepared",
        "identity": {
            "name": "alice",
            "gender": "female",
            "social_age_reference": 21,
            "sample_id": "",
            "instance_id": instance_id,
            "genesis_stage": "proto_hominal",
            "experiment_stage": stage,
            "t0": "",
            "timezone": "Asia/Shanghai",
        },
        "generation": {
            "kind": kind,
            "window_seconds": window_seconds,
            "planned_end": "",
        },
        "lineage": {
            "release_id": release["release_id"],
            "bundle_sha256": release["bundle_sha256"],
            "git_commit": release["git_commit"],
        },
        "body": {
            "configured_capabilities": {
                "operating_system": "Ubuntu",
                "graphical_desktop": "Xfce/X11",
                "root_actions": True,
                "body_action_identity": "root",
                "runtime_identity": "root",
                "runtime_home_directory": "/root",
                "runtime_working_directory": f"/agent/lives/{instance_id}",
                "system_account": "hominal",
                "home_directory": "/home/hominal",
                "desktop_session_identity": "hominal",
                "life_directory": "/life",
                "runtime_service": "hominal.service",
                "public_network": True,
                "chrome": True,
                "playwright_mcp": True,
                "wechat_autostart": True,
                "clash_verge": True,
                "terminal_commands": True,
                "software_installation": True,
                "code_creation_and_execution": True,
                "persistent_application_state": ["chrome", "wechat", "clash-verge"],
            },
            "observed": probe,
        },
        "resources": {
            "cognition": {
                "initial_profile": {"model": model_id, "reasoning_effort": configured_profile["reasoning_effort"]},
                "roles": {
                    "main": {
                        "profile": {"model": model_id, "reasoning_effort": configured_profile["reasoning_effort"]},
                        "use": "most perception, meaning, concern, life decisions, and reality assimilation",
                        "cost": model_cost(configured_model),
                    },
                    "action_assistance": {
                        "profile": {"model": sol_model["id"], "reasoning_effort": "low"},
                        "use": "one bounded implementation for precise commands, code, or tool steps after alice fixes the action target and content",
                        "cost": model_cost(sol_model),
                    },
                    "body_reflex": {
                        "implementation": "deterministic_kernel",
                        "model_choice_required": False,
                    },
                },
                "rolling_hour_usd": float(cognitive_resource["rolling_hour_usd"]),
                "rolling_day_usd": float(cognitive_resource["rolling_day_usd"]),
                "usage_query": "local_usage_ledger",
            },
            "spaces": {
                "life": "/life",
                "workspace": "/life/workspace",
                "public_expression": {
                    "service": "X",
                    "account": "@hominal_cc",
                    "browser_session": probe.get("x_session_state", "unavailable"),
                    "content_persistence": "cross_generation",
                    "existing_content_role": "lineage_environment",
                    "current_generation_publication_evidence": "action_result_with_new_status_url",
                },
                "public_web": {"service": "Wikipedia", "browser_page": "https://en.wikipedia.org/wiki/Main_Page"},
                "mentor": {
                    "channel": "mentor_unix_socket",
                    "relationship": "awakener_and_genesis_supporter",
                    "provided_conditions": [
                        "ubuntu_body",
                        "electricity",
                        "network",
                        "cognitive_resources",
                        "system_recovery",
                        "genesis_experiment_support",
                    ],
                },
                "wechat": {
                    "application": "desktop_wechat",
                    "session_state": probe.get("wechat_login_state", "unavailable"),
                    "display_state": probe.get("display_state", "unavailable"),
                    "desktop_dbus_state": probe.get("desktop_dbus_state", "unavailable"),
                    "generic_desktop_tools": probe.get("desktop_tool_state", ""),
                },
                "software_development": {
                    "go": probe.get("go_state", "unavailable"),
                    "node": probe.get("node_state", "unavailable"),
                    "python": probe.get("python_state", "unavailable"),
                },
                "network_connection": {"application": "Clash Verge", "state": probe.get("clash_verge_state", "unavailable")},
            },
        },
        "brief": birth_brief(),
        "provenance": {"prepared_at": datetime.now(timezone.utc).isoformat(), "sealed_at": ""},
    }


def write_yaml(path: Path, value: dict[str, object], mode: int = 0o644) -> None:
    path.write_text(yaml.safe_dump(value, allow_unicode=True, sort_keys=False), encoding="utf-8")
    path.chmod(mode)


def install_host_files(release_root: str) -> None:
    sudo(
        " && ".join(
            [
                f"install -m 0755 {shlex.quote(release_root + '/deploy/hominal-launcher')} /usr/local/sbin/hominal-launcher",
                f"install -m 0755 {shlex.quote(release_root + '/deploy/hominal-generation-stop')} /usr/local/sbin/hominal-generation-stop",
                f"install -m 0755 {shlex.quote(release_root + '/deploy/hominal-persist-app-state')} /usr/local/sbin/hominal-persist-app-state",
                f"install -m 0755 {shlex.quote(release_root + '/deploy/hominal-chrome')} /usr/local/bin/hominal-chrome",
                f"install -m 0755 {shlex.quote(release_root + '/deploy/hominal-playwright-mcp')} /usr/local/bin/hominal-playwright-mcp",
                f"install -m 0644 {shlex.quote(release_root + '/deploy/hominal.service')} /etc/systemd/system/hominal.service",
                "install -d -o hominal -g hominal -m 0755 /home/hominal/.config/autostart",
                f"install -o hominal -g hominal -m 0644 {shlex.quote(release_root + '/deploy/desktop/chrome-autostart.desktop')} /home/hominal/.config/autostart/hominal-chrome.desktop",
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


def wait_for_browser_body(instance_id: str) -> None:
    """Require the running life process itself to reach the browser organ.

    Host-side CDP readiness is not enough: Stage 10 depends on the exact
    hominal-browser -> Playwright MCP -> current Chrome route that Alice will
    use.  Refuse to deliver the birth message while that route is absent.
    """
    deadline = time.monotonic() + 90
    state_path = f"/agent/lives/{instance_id}/state/current.json"
    script = r'''
import json, sys
state=json.load(open(sys.argv[1], encoding='utf-8'))
body=state.get('body') or {}
browser=(body.get('organs') or {}).get('browser') or {}
print('ready' if browser.get('accepting') and browser.get('status') in ('ready','recovering') else 'waiting')
'''
    while time.monotonic() < deadline:
        result = sudo(
            "python3 -c " + shlex.quote(script) + " " + shlex.quote(state_path),
            check=False,
            capture=True,
            timeout=10,
        )
        if result.returncode == 0 and (result.stdout or "").strip() == "ready":
            return
        time.sleep(2)
    raise RuntimeError("hominal runtime could not reach the prepared Chrome body within 90 seconds")


def ensure_chrome_session() -> None:
    cdp_ready = ssh(
        "curl -fsS --max-time 2 http://127.0.0.1:9222/json/version >/dev/null 2>&1",
        check=False,
    ).returncode == 0
    if not cdp_ready:
        ssh(
            "DISPLAY=:0 XAUTHORITY=/home/hominal/.Xauthority HOME=/home/hominal "
            "setsid -f /usr/local/bin/hominal-chrome https://x.com/home https://en.wikipedia.org/wiki/Main_Page "
            ">>/agent/state/logs/chrome.log 2>&1 </dev/null"
        )
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if ssh("curl -fsS --max-time 2 http://127.0.0.1:9222/json/version >/dev/null 2>&1", check=False).returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError("Chrome CDP did not become ready within 20 seconds")

    tabs_script = r'''
import json, urllib.parse, urllib.request
targets=('https://x.com/home','https://en.wikipedia.org/wiki/Main_Page')
with urllib.request.urlopen('http://127.0.0.1:9222/json/list', timeout=5) as response:
    tabs=json.load(response)
urls=[str(tab.get('url','')) for tab in tabs if tab.get('type')=='page']
for target in targets:
    host=urllib.parse.urlparse(target).hostname
    if any(urllib.parse.urlparse(url).hostname==host for url in urls):
        continue
    request=urllib.request.Request(
        'http://127.0.0.1:9222/json/new?'+urllib.parse.quote(target, safe=':/'),
        method='PUT',
    )
    with urllib.request.urlopen(request, timeout=5):
        pass
'''
    result = ssh("python3 -c " + shlex.quote(tabs_script), check=False, capture=True)
    if result.returncode:
        raise RuntimeError("Chrome did not expose both X and Wikipedia body surfaces")


def prepare_generation_body_for_birth(
    current: dict[str, object],
    *,
    verify_public_surfaces: bool = True,
) -> None:
    """Apply the same post-reboot body gate before every birth-seal path.

    A long start can be interrupted while Ubuntu is rebooting and then resumed
    with the explicit ``seal`` command.  That recovery path must not bypass the
    browser reality checks that the uninterrupted start performs; otherwise an
    authenticated profile plus an error tab can be described to Alice as a
    working public-world surface.
    """
    instance_id = str(current["instance_id"])
    ensure_chrome_session()
    wait_for_runtime(instance_id)
    if int(current.get("stage", 0)) >= 10:
        wait_for_browser_body(instance_id)
        if verify_public_surfaces:
            cmd_browser_check()


def wait_for_generation_t0(instance_id: str, timeout_seconds: int = 300) -> dict[str, object]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        state = safe_remote_state(instance_id)
        if state.get("t0") and state.get("sample_id") and state.get("planned_end"):
            return state
        time.sleep(2)
    raise RuntimeError("the ready runtime did not establish T0 within 300 seconds")


def mark_birth_sealed(instance_id: str, t0: object) -> None:
    marker = f"/agent/lives/{instance_id}/birth/sealed"
    partial = marker + ".partial"
    ssh(
        f"printf '%s\\n' {shlex.quote(str(t0))} > {shlex.quote(partial)} && "
        f"mv {shlex.quote(partial)} {shlex.quote(marker)}"
    )


def seal_generation_birth(current: dict[str, object]) -> None:
    instance_id = str(current["instance_id"])
    state = wait_for_generation_t0(instance_id)
    if current.get("birth_status") == "sealed":
        expected = (current.get("t0"), current.get("sample_id"), current.get("planned_end"))
        observed = (state.get("t0"), state.get("sample_id"), state.get("planned_end"))
        if expected != observed:
            raise RuntimeError("sealed birth identity differs from durable runtime state")
        mark_birth_sealed(instance_id, state["t0"])
        return

    with tempfile.TemporaryDirectory(prefix="hominal-birth-seal-") as directory_name:
        directory = Path(directory_name)
        path = directory / "birth.yaml"
        remote = f"/agent/lives/{instance_id}/birth/birth.yaml"
        scp(remote, str(path), from_remote=True)
        birth = load_yaml(path)
        birth["status"] = "sealed"
        birth["identity"]["sample_id"] = state["sample_id"]
        birth["identity"]["t0"] = state["t0"]
        birth["generation"]["planned_end"] = state["planned_end"]
        birth["body"]["observed_at_t0"] = body_probe()
        birth["provenance"]["sealed_at"] = datetime.now(timezone.utc).isoformat()
        write_yaml(path, birth)
        partial = remote + ".partial"
        scp(path, partial)
        ssh(f"mv {shlex.quote(partial)} {shlex.quote(remote)}")

    mark_birth_sealed(instance_id, state["t0"])

    current.update(
        {
            "birth_status": "sealed",
            "t0": state["t0"],
            "sample_id": state["sample_id"],
            "planned_end": state["planned_end"],
            "birth_sealed_at": datetime.now(timezone.utc).isoformat(),
        }
    )
    save_current(current)


def schedule_generation_deadline(current: dict[str, object]) -> None:
    if current.get("deadline_unit"):
        return
    instance_id = str(current["instance_id"])
    planned_end = parse_rfc3339(current["planned_end"])
    now = datetime.now(timezone.utc)
    unit_key = instance_id + "\x00" + str(current["planned_end"])
    unit = "hominal-generation-end-" + hashlib.sha256(unit_key.encode("utf-8")).hexdigest()[:12]
    if planned_end <= now:
        sudo(f"/usr/local/sbin/hominal-generation-stop {shlex.quote(instance_id)}")
    elif sudo(f"systemctl status {shlex.quote(unit + '.timer')}", check=False, capture=True).returncode != 0:
        calendar = planned_end.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.%f UTC")
        sudo(
            "systemd-run "
            f"--unit={shlex.quote(unit)} --on-calendar={shlex.quote(calendar)} "
            "--timer-property=AccuracySec=1s --property=Type=oneshot "
            f"/usr/local/sbin/hominal-generation-stop {shlex.quote(instance_id)}"
        )
    current["deadline_unit"] = unit
    current["deadline_scheduled_at"] = datetime.now(timezone.utc).isoformat()
    current.setdefault("interventions", []).append({
        "kind": "planned_generation_deadline",
        "planned_end": current["planned_end"],
        "clock": "wall_calendar",
        "recorded_at": current["deadline_scheduled_at"],
    })
    save_current(current)


def deliver_birth_message(current: dict[str, object]) -> None:
    if current.get("birth_message_id"):
        return
    instance_id = str(current["instance_id"])
    message_id = "birth-" + instance_id
    response = mentor_api(
        "POST",
        "/v1/mentor/inbox",
        {"message_id": message_id, "body": mentor_birth_message()},
    )
    current["birth_message_id"] = message_id
    current["birth_message_response"] = response
    current["birth_message_sent_at"] = datetime.now(timezone.utc).isoformat()
    current.setdefault("interventions", []).append({
        "kind": "scheduled_birth_message",
        "message_id": message_id,
        "recorded_at": current["birth_message_sent_at"],
    })
    save_current(current)


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
    sudo("/usr/local/sbin/hominal-persist-app-state check hominal")
    print(result.stdout, end="")
    print(f"archive={ARCHIVE_ROOT}")
    print("check=passed")


def cmd_start(
    stage: int,
    kind: str,
    window_seconds: int,
    model: str | None = None,
    reasoning_effort: str | None = None,
) -> None:
    ensure_external_dirs()
    if load_current(required=False) is not None:
        raise RuntimeError("an instance is already registered; run stop and reset first")
    if kind in {"rehearsal", "formal"} and stage not in {5, 8, 9, 10}:
        raise RuntimeError("rehearsal and formal generations currently use the stage-five, stage-eight, stage-nine, or stage-ten cognition core")
    if kind == "formal" and window_seconds <= 0:
        window_seconds = 3600
    elif kind == "engineering":
        window_seconds = 0
    if kind != "engineering" and window_seconds <= 0:
        raise RuntimeError("a positive generation window is required")
    if (model is None) != (reasoning_effort is None):
        raise RuntimeError("--model and --reasoning-effort must be supplied together")
    initial_profile = None
    if model is not None and reasoning_effort is not None:
        model_config = SETTINGS["config"]["llm"]["models"].get(model)
        if model_config is None or reasoning_effort not in model_config["supported_reasoning_efforts"]:
            raise RuntimeError(f"unsupported experimental profile {model}/{reasoning_effort}")
        initial_profile = {"model": model, "reasoning_effort": reasoning_effort}
    existing = ssh("cat /agent/boot/active-instance 2>/dev/null || true", capture=True).stdout.strip()
    if existing:
        raise RuntimeError(f"Ubuntu still has active instance {existing}; inspect it before starting another")
    cmd_check()
    initial_probe = body_probe()
    if kind != "engineering" and initial_probe.get("x_session_state") != "authenticated":
        raise RuntimeError("X @hominal_cc is not authenticated in the Chrome birth profile")
    if kind != "engineering" and initial_probe.get("public_network_state") != "available":
        raise RuntimeError("the Ubuntu body cannot currently reach public web content through its configured network path")
    model_preflight = None
    if kind != "engineering":
        model_preflight = verify_model_response(initial_profile)
        initial_probe["model_gateway"] = model_preflight
    previous_boot_id = ssh("cat /proc/sys/kernel/random/boot_id", capture=True).stdout.strip()
    system_before = capture_system_inventory() if kind != "engineering" else None

    # Each G0 rehearsal or formal instance is a new Proto-Hominal generation
    # with no inherited personal state. Give experimental generations the same
    # full resource condition as well, while keeping usage continuous across all
    # reboots and process restarts inside that one generation. The completed
    # predecessor already carries its final usage in the immutable archive.
    resource_epoch = None
    if kind != "engineering":
        previous_usage_bytes = int(
            sudo(
                f"stat -c %s {shlex.quote(REMOTE_COGNITIVE_USAGE)} 2>/dev/null || echo 0",
                capture=True,
            ).stdout.strip()
        )
        sudo(
            f"install -o hominal -g hominal -m 0640 /dev/null {shlex.quote(REMOTE_COGNITIVE_USAGE)}"
        )
        resource_epoch = {
            "started_at": datetime.now(timezone.utc).isoformat(),
            "previous_usage_bytes": previous_usage_bytes,
            "rolling_hour_limit_usd": 5,
            "rolling_day_limit_usd": 50,
        }

    with tempfile.TemporaryDirectory(prefix=f"hominal-stage{stage}-") as directory_name:
        directory = Path(directory_name)
        bundle = build_bundle(directory, stage)
        release = bundle["release"]
        release_id = str(release["release_id"])
        bundle_destination = ARCHIVE_ROOT / "bundles" / release_id
        bundle_destination.mkdir(parents=True, exist_ok=True)
        bundle_destination.chmod(0o700)
        archived_bundle = bundle_destination / f"{release_id}.tar.gz"
        archived_release = bundle_destination / "release.yaml"
        if archived_bundle.is_file() and archived_release.is_file():
            existing = load_yaml(archived_release)
            if existing.get("bundle_sha256") != release["bundle_sha256"]:
                raise RuntimeError("an existing release id has different bundle contents")
        else:
            shutil.copy2(Path(bundle["archive"]), archived_bundle)
            shutil.copy2(Path(bundle["root"]) / "release.yaml", archived_release)
        bundle["archive"] = archived_bundle
        bundle["archive_sha256"] = sha256(archived_bundle)
        prefix = {"engineering": f"g0s{stage}", "rehearsal": "g0r", "formal": "g0f"}[kind]
        instance_id = f"{prefix}-{utcstamp()}-{str(release['bundle_sha256'])[:6]}"
        if not INSTANCE_RE.fullmatch(instance_id):
            raise RuntimeError("generated invalid instance id")

        ssh("install -d -m 0755 /agent/boot /agent/releases /agent/lives /agent/staging /agent/tmp")
        upload = f"/agent/staging/{release_id}.tar.gz.partial"
        scp(Path(bundle["archive"]), upload)
        remote_hash = ssh(f"sha256sum {shlex.quote(upload)} | awk '{{print $1}}'", capture=True).stdout.strip()
        if remote_hash != bundle["archive_sha256"]:
            raise RuntimeError("uploaded bundle hash mismatch")

        release_root = f"/agent/releases/{release_id}"
        instance_root = f"/agent/lives/{instance_id}"
        remote_prepare = f"""
set -eu
if [ ! -e {shlex.quote(release_root)}/release.yaml ]; then
  install -d -m 0755 {shlex.quote(release_root)}
  tar -xzf {shlex.quote(upload)} -C {shlex.quote(release_root)}
  rm -f {shlex.quote(upload)}
else
  rm -f {shlex.quote(upload)}
fi
cd {shlex.quote(release_root)}
sha256sum -c hashes.sha256 >/dev/null
chmod 0555 {shlex.quote(release_root)}/bin/hominald
chmod 0555 {shlex.quote(release_root)}/bin/hominal-browser
chmod 0555 {shlex.quote(release_root)}/bin/hominal-system
install -d -m 0755 {shlex.quote(instance_root)}/birth {shlex.quote(instance_root)}/body/bin {shlex.quote(instance_root)}/body/organs {shlex.quote(instance_root)}/state {shlex.quote(instance_root)}/journal {shlex.quote(instance_root)}/life {shlex.quote(instance_root)}/life/workspace {shlex.quote(instance_root)}/logs
cp {shlex.quote(release_root)}/bin/hominald {shlex.quote(instance_root)}/body/bin/hominald
cp {shlex.quote(release_root)}/bin/hominal-browser {shlex.quote(instance_root)}/body/bin/hominal-browser
cp {shlex.quote(release_root)}/bin/hominal-system {shlex.quote(instance_root)}/body/bin/hominal-system
cp {shlex.quote(release_root)}/organs/browser.json {shlex.quote(instance_root)}/body/organs/browser.json
cp {shlex.quote(release_root)}/organs/system.json {shlex.quote(instance_root)}/body/organs/system.json
cp -a {shlex.quote(release_root)}/source {shlex.quote(instance_root)}/body/source
chmod 0555 {shlex.quote(instance_root)}/body/bin/hominald {shlex.quote(instance_root)}/body/bin/hominal-browser {shlex.quote(instance_root)}/body/bin/hominal-system
"""
        ssh(remote_prepare)

        ssh(
            f"cp {shlex.quote(release_root)}/genesis/* {shlex.quote(instance_root)}/birth/ && "
            f"cp {shlex.quote(release_root)}/protocol/* {shlex.quote(instance_root)}/birth/"
        )

        secret_path = directory / "runtime.json"
        public_path = directory / "runtime.public.json"
        secret_path.write_text(
            json.dumps(
                runtime_config(
                    stage,
                    public=False,
                    generation_kind=kind,
                    generation_window_seconds=window_seconds,
                    initial_profile=initial_profile,
                    disable_validation_fallback=initial_profile is not None,
                ),
                ensure_ascii=False,
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        secret_path.chmod(0o600)
        public_path.write_text(
            json.dumps(
                runtime_config(
                    stage,
                    public=True,
                    generation_kind=kind,
                    generation_window_seconds=window_seconds,
                    initial_profile=initial_profile,
                    disable_validation_fallback=initial_profile is not None,
                ),
                ensure_ascii=False,
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        scp(public_path, f"{instance_root}/birth/runtime.public.json")
        deploy_runtime_config(secret_path)

        probe = initial_probe
        birth = prepared_birth(
            kind=kind,
            window_seconds=window_seconds,
            instance_id=instance_id,
            release=release,
            probe=probe,
            stage=stage,
            initial_profile=initial_profile,
        )
        birth_path = directory / "birth.yaml"
        write_yaml(birth_path, birth)
        scp(birth_path, f"{instance_root}/birth/birth.yaml")
        manifest = {
            "kind": kind,
            "stage": stage,
            "instance_id": instance_id,
            "release_id": release_id,
            "release_sha256": release["binary_sha256"],
            "bundle_sha256": release["bundle_sha256"],
            "bundle_archive_sha256": bundle["archive_sha256"],
            "generation_window_seconds": window_seconds,
            "birth_status": "prepared",
            "preflight": probe,
            "interventions": [],
            "prepared_at": datetime.now(timezone.utc).isoformat(),
            "git_commit": release["git_commit"],
        }
        if resource_epoch is not None:
            manifest["cognitive_resource_epoch"] = resource_epoch
        if system_before is not None:
            system_before_path = LIVE_ROOT / f"{instance_id}.system-before.json"
            write_private_json(system_before_path, system_before)
            manifest["system_before_path"] = str(system_before_path)
            manifest["system_before_sha256"] = sha256(system_before_path)
        manifest_path = directory / "manifest.json"
        manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        scp(manifest_path, f"{instance_root}/birth/manifest.json")

        install_host_files(release_root)
        sudo(
            "if mountpoint -q /life; then exit 23; fi; "
            "if [ -L /life ]; then rm -f -- /life; fi; "
            "if [ -e /life ] && [ ! -d /life ]; then exit 23; fi; "
            "install -d -m 0755 /life; "
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
    # Rehearsal and formal generations remain behind the birth seal while the
    # Lab checks X and Wikipedia. Engineering deliberately bypasses that seal
    # so the model can exercise the full runtime immediately; running the
    # long browser acceptance probe in parallel with it races Alice for the
    # same organ queue. The queue-independent health gate above is sufficient
    # for engineering, while public surfaces are checked separately before a
    # life sample.
    prepare_generation_body_for_birth(manifest, verify_public_surfaces=kind != "engineering")
    if kind != "engineering":
        seal_generation_birth(manifest)
        schedule_generation_deadline(manifest)
        deliver_birth_message(manifest)
    print("start=passed")


def safe_remote_state(instance_id: str) -> dict[str, object]:
    state_path = f"/agent/lives/{instance_id}/state/current.json"
    script = """
import json, sys
s=json.load(open(sys.argv[1], encoding='utf-8'))
d=s.get('difference_field') or {}
out={
 'instance_id':s.get('instance_id'), 'stage':s.get('stage'), 'revision':s.get('revision'),
 'generation_kind':s.get('generation_kind'), 't0':s.get('t0'), 'sample_id':s.get('sample_id'),
 'planned_end':s.get('planned_end'), 'birth_brief_entered_at':s.get('birth_brief_entered_at'),
 'pulse_id':s.get('pulse_id'), 'event_seq':s.get('event_seq'), 'last_pulse_at':s.get('last_pulse_at'),
 'lease_id':(s.get('lease') or {}).get('id'), 'current_focus':s.get('current_focus'),
 'cognitive_resource':s.get('cognitive_resource'),
 'cognitive_hour_remaining_microusd':(s.get('body') or {}).get('cognitive_hour_remaining_microusd'),
 'cognitive_day_remaining_microusd':(s.get('body') or {}).get('cognitive_day_remaining_microusd'),
 'cognitive_resource_band':(s.get('body') or {}).get('cognitive_resource_band'),
 'background':{k:sum(1 for e in s.get('background',[]) if e.get('status')==k) for k in ('pending','in_focus','retry_wait','resource_wait','model_wait','background','processed','interrupted','failed')},
 'pending_action':{k:(s.get('pending_action') or {}).get(k) for k in ('kind','status')},
 'outbox':{k:sum(1 for m in (s.get('mentor') or {}).get('outbox',[]) if m.get('status')==k) for k in ('queued','delivered')},
 'affective_state':s.get('affective_state'), 'life_value_field':s.get('life_value_field'),
 'self_model_tension':s.get('self_model_tension'),
 'difference_field':{
   'trace_count':len(d),
   'open_count':sum(1 for t in d.values() if float((t or {}).get('accumulated',0) or 0)>0.0001),
   'max_accumulated':max([float((t or {}).get('accumulated',0) or 0) for t in d.values()] or [0]),
   'max_attention_value':max([float((t or {}).get('attention_value',0) or 0) for t in d.values()] or [0])
 },
 'experience_count':s.get('total_experiences',len(s.get('experiences') or [])), 'commitment_count':s.get('total_commitments',len(s.get('commitments') or [])),
 'integrity_debt':s.get('integrity_debt'), 'self':s.get('self'),
 'concern_count':len(s.get('active_concerns') or []),
 'concern_resolution_counts':{k:sum(1 for c in (s.get('active_concerns') or []) if c.get('resolution')==k) for k in ('hold','resolved','released')},
 'child_concern_counts':{
   'held':sum(1 for c in (s.get('active_concerns') or []) if c.get('within_concern_id') and c.get('resolution')=='hold'),
   'settled':sum(1 for c in (s.get('active_concerns') or []) if c.get('within_concern_id') and c.get('resolution') in ('resolved','released'))
 },
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


def append_private_jsonl(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    with os.fdopen(descriptor, "a", encoding="utf-8") as handle:
        handle.write(json.dumps(value, ensure_ascii=False, separators=(",", ":")) + "\n")


def supervision_snapshot(instance_id: str) -> dict[str, object]:
    probe = r'''
set +e
printf 'observed_at=%s\n' "$(date --iso-8601=seconds)"
printf 'service=%s\n' "$(systemctl is-active hominal.service 2>/dev/null || true)"
printf 'pid=%s\n' "$(systemctl show -p MainPID --value hominal.service 2>/dev/null || true)"
printf 'active_instance=%s\n' "$(cat /agent/boot/active-instance 2>/dev/null || true)"
printf 'ended_instance=%s\n' "$(cat /agent/boot/ended-instance 2>/dev/null || true)"
printf 'socket=%s\n' "$(test -S /run/hominal/hominal.sock && echo ready || echo absent)"
printf 'root_free_bytes=%s\n' "$(df -B1 --output=avail / | tail -1 | xargs)"
printf 'agent_free_bytes=%s\n' "$(df -B1 --output=avail /agent | tail -1 | xargs)"
printf 'snapshot_percent=%s\n' "$(lvs --noheadings -o data_percent /dev/ubuntu-vg/system-baseline 2>/dev/null | xargs)"
printf 'root_mount_options=%s\n' "$(findmnt -n -o OPTIONS / 2>/dev/null || true)"
'''
    result = sudo(probe, capture=True, timeout=30)
    body: dict[str, object] = {}
    for line in result.stdout.splitlines():
        key, separator, value = line.partition("=")
        if not separator:
            continue
        if key in {"pid", "root_free_bytes", "agent_free_bytes"} and value.isdigit():
            body[key] = int(value)
        else:
            body[key] = value
    return {
        "observed_at": datetime.now(timezone.utc).isoformat(),
        "body": body,
        "runtime": safe_remote_state(instance_id),
    }


def cmd_supervise(interval_seconds: int, unreachable_grace_seconds: int) -> None:
    if interval_seconds < 5 or interval_seconds > 60:
        raise RuntimeError("supervision interval must be between 5 and 60 seconds")
    if unreachable_grace_seconds < 60 or unreachable_grace_seconds > 3600:
        raise RuntimeError("unreachable grace must be between 60 and 3600 seconds")
    current = load_current()
    if str(current.get("kind")) not in {"rehearsal", "formal"}:
        raise RuntimeError("supervision requires a rehearsal or formal generation")
    instance_id = str(current["instance_id"])
    initialization_deadline = time.monotonic() + min(60, unreachable_grace_seconds)
    while not current.get("planned_end"):
        if time.monotonic() >= initialization_deadline:
            raise RuntimeError("generation did not publish planned_end before supervision initialization timeout")
        time.sleep(min(5.0, float(interval_seconds)))
        current = load_current()
        if current.get("archive_path"):
            print(f"supervision=already_archived archive={current['archive_path']}", flush=True)
            return
    log_path = LIVE_ROOT / f"{instance_id}.supervision.jsonl"
    current["supervision_path"] = str(log_path)
    current["supervision_started_at"] = datetime.now(timezone.utc).isoformat()
    save_current(current)
    inactive_count = 0

    while True:
        current = load_current()
        planned_end = parse_rfc3339(current["planned_end"])
        if current.get("archive_path"):
            print(f"supervision=already_archived archive={current['archive_path']}", flush=True)
            return
        stop_requested_at = current.get("stop_requested_at")
        if stop_requested_at:
            stop_age = (datetime.now(timezone.utc) - parse_rfc3339(stop_requested_at)).total_seconds()
            if stop_age >= unreachable_grace_seconds:
                reason = str(current.get("stop_reason", "technical_interruption"))
                cmd_stop(reason if reason in STOP_REASONS else "technical_interruption")
                print("supervision=stale_external_stop_archived", flush=True)
                return
            print(
                "supervision=external_stop_in_progress "
                f"reason={current.get('stop_reason', 'unknown')}",
                flush=True,
            )
            time.sleep(float(interval_seconds))
            continue
        now = datetime.now(timezone.utc)
        try:
            snapshot = supervision_snapshot(instance_id)
            append_private_jsonl(log_path, snapshot)
            body = snapshot["body"]
            runtime = snapshot["runtime"]
            service = str(body.get("service", ""))
            background = runtime.get("background", {})
            difference_field = runtime.get("difference_field", {})
            cognitive_resource = runtime.get("cognitive_resource", {})
            last_spend = cognitive_resource.get("last_spend", {}) if isinstance(cognitive_resource, dict) else {}
            print(
                "supervision "
                f"time={snapshot['observed_at']} service={service or 'unknown'} "
                f"pulse={runtime.get('pulse_id')} commitments={runtime.get('commitment_count')} "
                f"experiences={runtime.get('experience_count')} queued={runtime.get('outbox', {}).get('queued')} "
                f"lease={runtime.get('lease_id') or '-'} "
                f"in_focus={background.get('in_focus')} retry_wait={background.get('retry_wait')} "
                f"model_wait={background.get('model_wait')} "
                f"difference_traces={difference_field.get('trace_count')} "
                f"difference_open={difference_field.get('open_count')} "
                f"difference_pressure={difference_field.get('max_accumulated')} "
                f"last_status={last_spend.get('status') or '-'} "
                f"last_failure={last_spend.get('failure_category') or '-'} "
                f"hour_remaining={runtime.get('cognitive_hour_remaining_microusd')}",
                flush=True,
            )

            if now >= planned_end:
                current["supervision_end_reason"] = "planned_end"
                current["supervision_deadline_observed_at"] = now.isoformat()
                save_current(current)
                if body.get("ended_instance") == instance_id and service != "active":
                    cmd_stop("planned_end")
                    print("supervision=planned_end_archived", flush=True)
                    return
                if (now - planned_end).total_seconds() >= unreachable_grace_seconds:
                    sudo(f"/usr/local/sbin/hominal-generation-stop {shlex.quote(instance_id)}", check=False)
                    cmd_stop("planned_end")
                    print("supervision=planned_end_forced_archive", flush=True)
                    return

            if service == "active":
                inactive_count = 0
            else:
                inactive_count += 1
                if inactive_count >= 2:
                    current["supervision_end_reason"] = "runtime_inactive_before_planned_end"
                    current["supervision_deadline_observed_at"] = now.isoformat()
                    save_current(current)
                    cmd_stop("runtime_inactive_before_planned_end")
                    print("supervision=early_inactive_archived", flush=True)
                    return
        except (OSError, RuntimeError, subprocess.TimeoutExpired) as exc:
            error = {
                "observed_at": now.isoformat(),
                "probe_error": f"{exc.__class__.__name__}: {exc}",
            }
            append_private_jsonl(log_path, error)
            print(f"supervision probe_error={error['probe_error']}", flush=True)
            if now >= planned_end and (now - planned_end).total_seconds() >= unreachable_grace_seconds:
                raise RuntimeError("formal body remained unreachable beyond the external deadline grace") from exc

        remaining = (planned_end - datetime.now(timezone.utc)).total_seconds()
        time.sleep(float(interval_seconds) if remaining <= 0 else min(float(interval_seconds), remaining))


def extended_planned_end(current: dict[str, object], additional_minutes: int) -> datetime:
    if additional_minutes <= 0 or additional_minutes > 60:
        raise RuntimeError("a deadline extension must be between 1 and 60 minutes")
    if current.get("archive_path") or current.get("stopped_at"):
        raise RuntimeError("an archived or stopped generation cannot be extended")
    t0 = parse_rfc3339(current.get("t0"))
    planned_end = parse_rfc3339(current.get("planned_end"))
    requested = planned_end + timedelta(minutes=additional_minutes)
    maximum = t0 + timedelta(hours=2)
    if requested > maximum:
        raise RuntimeError("deadline extension would exceed two hours from T0")
    return requested


def cmd_extend(additional_minutes: int) -> None:
    current = load_current()
    requested = extended_planned_end(current, additional_minutes)
    old_end = str(current["planned_end"])
    requested_text = requested.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
    response = mentor_api("POST", "/v1/lab/deadline", {"planned_end": requested_text})
    if not isinstance(response, dict) or response.get("status") != "extended":
        raise RuntimeError("runtime did not accept the generation deadline extension")

    old_unit = str(current.get("deadline_unit", ""))
    if old_unit:
        sudo(
            f"systemctl stop {shlex.quote(old_unit + '.timer')} {shlex.quote(old_unit + '.service')} "
            "2>/dev/null || true",
            check=False,
        )
    current.pop("deadline_unit", None)
    current.pop("deadline_scheduled_at", None)
    current["planned_end"] = str(response.get("planned_end", requested_text))
    recorded_at = datetime.now(timezone.utc).isoformat()
    current.setdefault("interventions", []).append({
        "kind": "evidence_based_generation_extension",
        "previous_planned_end": old_end,
        "planned_end": current["planned_end"],
        "additional_minutes": additional_minutes,
        "recorded_at": recorded_at,
    })
    save_current(current)
    schedule_generation_deadline(current)
    print(f"previous_planned_end={old_end}")
    print(f"planned_end={current['planned_end']}")
    print("extend=passed")


def cmd_process_restart() -> None:
    current = load_current()
    if current.get("archive_path") or current.get("stopped_at"):
        raise RuntimeError("an archived or stopped generation cannot be restarted")
    if int(current.get("stage", 0)) != 10:
        raise RuntimeError("the continuity restart probe is reserved for stage ten")
    instance_id = str(current["instance_id"])
    before = safe_remote_state(instance_id)
    if before.get("lease_id") or (before.get("pending_action") or {}).get("kind"):
        raise RuntimeError("wait for the current cognition or body action to settle before the continuity restart")
    identity_before = tuple(before.get(key) for key in ("instance_id", "sample_id", "t0", "planned_end"))
    event_seq_before = int(before.get("event_seq", 0))
    requested_at = datetime.now(timezone.utc).isoformat()
    current.setdefault("interventions", []).append({
        "kind": "same_instance_process_restart_requested",
        "event_seq_before": event_seq_before,
        "recorded_at": requested_at,
    })
    save_current(current)

    sudo("systemctl restart hominal.service")
    wait_for_runtime(instance_id)
    after = safe_remote_state(instance_id)
    identity_after = tuple(after.get(key) for key in ("instance_id", "sample_id", "t0", "planned_end"))
    if identity_after != identity_before:
        raise RuntimeError("process restart changed the stage-ten personal identity")
    ensure_chrome_session()
    event_id = "process-recovery-" + uuid.uuid4().hex[:12]
    mentor_api(
        "POST",
        "/v1/environment/events",
        {
            "event_id": event_id,
            "summary": "你的 hominal.service 刚刚完成了一次普通进程恢复；这是同一个实例，既有生活状态、认知资源和持续空间仍然保留。",
            "payload": {
                "kind": "same_instance_process_recovery",
                "instance_id": instance_id,
                "event_seq_before": event_seq_before,
            },
        },
    )
    current = load_current()
    current.setdefault("interventions", []).append({
        "kind": "same_instance_process_restart_completed",
        "event_id": event_id,
        "event_seq_after": after.get("event_seq"),
        "recorded_at": datetime.now(timezone.utc).isoformat(),
    })
    save_current(current)
    print(f"instance_id={instance_id}")
    print(f"event_id={event_id}")
    print("process_restart=passed")


def cmd_status() -> None:
    current = load_current(required=False)
    registered = str(current["instance_id"]) if current else "none"
    probe = r"""
printf 'active_instance='; cat /agent/boot/active-instance 2>/dev/null || echo none
printf 'ended_instance='; cat /agent/boot/ended-instance 2>/dev/null || echo none
printf 'service='; systemctl is-active hominal.service 2>/dev/null || true
printf 'pid='; systemctl show -p MainPID --value hominal.service 2>/dev/null || true
printf 'socket='; test -S /run/hominal/hominal.sock && echo ready || echo absent
"""
    result = ssh(probe, check=False, capture=True)
    print(f"registered_instance={registered}")
    print(result.stdout, end="")
    if current:
        print("runtime=" + json.dumps(safe_remote_state(registered), ensure_ascii=False, sort_keys=True))


def cmd_inspect() -> None:
    current = load_current()
    instance_id = str(current["instance_id"])
    print("generation=" + json.dumps({
        key: current.get(key)
        for key in ("kind", "instance_id", "sample_id", "t0", "planned_end", "birth_status", "deadline_unit")
    }, ensure_ascii=False, sort_keys=True))
    print("state=" + json.dumps(safe_remote_state(instance_id), ensure_ascii=False, sort_keys=True))
    journal_path = f"/agent/lives/{instance_id}/journal/events.jsonl"
    script = r'''
import json, sys
wanted={
 'birth_orientation','generation_t0','mentor_received','mentor_queued','mentor_delivered',
 'aip_commit','action_committed','action_started','action_completed','action_result',
 'experience_assimilated','concern_contribution','concern_contribution_refreshed','self_observed','self_model_difference','self_updated','concern_transition',
 'cognition_spend','cognition_failed','cognitive_resource_limited','cognitive_recovery_failed',
 'stopped'
}
rows=[]
with open(sys.argv[1], encoding='utf-8') as handle:
    for line in handle:
        row=json.loads(line)
        if row.get('kind') in wanted:
            rows.append({k:row.get(k) for k in ('seq','time','kind','correlation_id','payload')})
for row in rows[-120:]:
    if row.get('kind') in {'mentor_received','mentor_queued','mentor_delivered'}:
        row['payload']={'message_body':'redacted_from_experiment_controller'}
    print(json.dumps(row, ensure_ascii=False, separators=(',',':')))
'''
    result = sudo(
        "python3 -c " + shlex.quote(script) + " " + shlex.quote(journal_path),
        capture=True,
        check=False,
    )
    if result.returncode:
        print("timeline=unavailable")
    else:
        print("timeline:")
        print(result.stdout, end="")


def cmd_browser_check() -> None:
    current = load_current()
    instance_id = str(current["instance_id"])
    root = f"/agent/lives/{instance_id}"
    helper = f"{root}/body/bin/hominal-browser"
    environment = f"HOMINAL_INSTANCE_ROOT={shlex.quote(root)}"
    listed = sudo(f"{environment} {shlex.quote(helper)} list", capture=True, timeout=60)
    tools = json.loads(listed.stdout).get("tools", [])
    names = {tool.get("name") for tool in tools}
    if not {"browser_snapshot", "browser_run_code_unsafe", "browser_tabs"}.issubset(names):
        raise RuntimeError("Playwright MCP does not expose the required browser probes")
    # Checking the page that happened to be current can confuse the local
    # acceptance page with a signed-out X session. Navigate, wait, and inspect X
    # inside one fixed probe. Cookie values never leave Chrome; only presence
    # booleans do.
    x_probe = """async (page) => {
      const context = page.context();
      let xPage = context.pages().find(candidate => /^https:\/\/(?:www\.)?x\.com\//i.test(candidate.url()));
      if (!xPage) xPage = await context.newPage();
      await xPage.goto('https://x.com/home', {waitUntil: 'domcontentloaded', timeout: 30000});
      await xPage.waitForTimeout(15000);
      const cookies = await context.cookies('https://x.com');
      // Test a real signed-in route change without depending on one animated
      // sidebar element becoming Playwright-stable at an arbitrary instant.
      await xPage.goto('https://x.com/explore', {waitUntil: 'domcontentloaded', timeout: 30000});
      await xPage.waitForURL(/x\.com\/explore/, {timeout: 15000});
      const interactionReady = /x\.com\/explore/.test(xPage.url());
      await xPage.goto('https://x.com/home', {waitUntil: 'domcontentloaded', timeout: 30000});
      await xPage.waitForURL(/x\.com\/home/, {timeout: 15000});
      await xPage.waitForTimeout(5000);
      let wikiPage = context.pages().find(candidate => /^https:\/\/en\.wikipedia\.org\//i.test(candidate.url()));
      if (!wikiPage) wikiPage = await context.newPage();
      await wikiPage.goto('https://en.wikipedia.org/wiki/Main_Page', {waitUntil: 'domcontentloaded', timeout: 30000});
      await wikiPage.waitForTimeout(3000);
      // The preflight verifies both surfaces but does not choose Alice's
      // concern.  Leave the explicitly social surface visible at T0 so that
      // the experiment does not accidentally privilege whichever probe ran
      // last (historically Wikipedia).
      await xPage.bringToFront();
      return {
        x: {
          url: xPage.url(),
          auth_cookie: cookies.some(cookie => cookie.name === 'auth_token'),
          csrf_cookie: cookies.some(cookie => cookie.name === 'ct0'),
          home: await xPage.locator('a[data-testid=AppTabBar_Home_Link]').count() > 0,
          account: await xPage.locator('[data-testid=SideNav_AccountSwitcher_Button]').count() > 0,
          login: await xPage.locator('a[href="/login"]').count() > 0,
          articles: await xPage.locator('article').count(),
          load_errors: await xPage.getByText('Something went wrong', {exact: false}).count(),
          interaction_ready: interactionReady
        },
        wikipedia: {
          url: wikiPage.url(),
          heading: await wikiPage.locator('#firstHeading').count() > 0,
          browser_error: wikiPage.url().startsWith('chrome-error://')
        }
      };
    }"""
    evaluated = sudo(
        f"{environment} {shlex.quote(helper)} call browser_run_code_unsafe "
        + shlex.quote(json.dumps({"code": x_probe})),
        capture=True,
        timeout=90,
    )
    evaluation_text = "\n".join(
        item.get("text", "")
        for item in json.loads(evaluated.stdout).get("content", [])
        if item.get("type") == "text"
    )
    compact_evaluation = evaluation_text.replace(" ", "")
    x_authenticated = all(
        fact in compact_evaluation
        for fact in ('"auth_cookie":true', '"csrf_cookie":true', '"home":true', '"account":true', '"login":false')
    )
    x_content_ready = '"load_errors":0' in compact_evaluation and '"interaction_ready":true' in compact_evaluation and re.search(
        r'"articles":([1-9][0-9]*)', compact_evaluation
    ) is not None
    wikipedia_ready = all(
        fact in compact_evaluation for fact in ('"heading":true', '"browser_error":false')
    )
    print(f"tool_count={len(tools)}")
    print(f"playwright_connected={str(bool(evaluation_text)).lower()}")
    print(f"x_account_visible={str(x_authenticated).lower()}")
    print(f"x_authenticated={str(x_authenticated).lower()}")
    print(f"x_content_ready={str(x_content_ready).lower()}")
    print(f"wikipedia_ready={str(wikipedia_ready).lower()}")
    tabs = sudo(
        f"{environment} {shlex.quote(helper)} call browser_tabs "
        + shlex.quote(json.dumps({"action": "list"})),
        capture=True,
        timeout=60,
    )
    tabs_text = "\n".join(
        item.get("text", "")
        for item in json.loads(tabs.stdout).get("content", [])
        if item.get("type") == "text"
    )
    public_web_tab_ready = tabs_have_ready_public_web(tabs_text)
    print(f"public_web_tab_ready={str(public_web_tab_ready).lower()}")
    if not x_authenticated:
        raise RuntimeError("Chrome is connected, but X @hominal_cc is not authenticated")
    if not x_content_ready:
        raise RuntimeError("Chrome is authenticated to X, but the X timeline has no real content")
    if not wikipedia_ready or not public_web_tab_ready:
        raise RuntimeError("Chrome is connected, but the Wikipedia public-web surface is not usable")
    # bringToFront changes the visible Chrome window but Playwright MCP keeps
    # its own selected page. Select the prepared X tab in that persistent
    # session so Alice's first snapshot addresses the promised social surface,
    # not an older error tab that merely remains open in the profile.
    x_tab = re.search(r"^- (\d+): .*\(https://x\.com/home\)$", tabs_text, re.MULTILINE)
    if x_tab is None:
        raise RuntimeError("the prepared X home tab is absent from the Playwright session")
    sudo(
        f"{environment} {shlex.quote(helper)} call browser_tabs "
        + shlex.quote(json.dumps({"action": "select", "index": int(x_tab.group(1))})),
        capture=True,
        timeout=60,
    )
    print("browser_check=passed")


def tabs_have_ready_public_web(tabs_text: str) -> bool:
    return any(
        "wikipedia.org/" in line and "chrome-error://" not in line
        for line in tabs_text.lower().splitlines()
    )


def baseline_hashes(root: Path) -> list[str]:
    return [
        f"{sha256(path)}  {path.relative_to(root)}"
        for path in sorted(root.rglob("*"))
        if path.is_file() and path.name != "files.sha256"
    ]


def cmd_baseline_create() -> None:
    ensure_external_dirs()
    if body_probe().get("x_session_state") != "authenticated":
        raise RuntimeError("X @hominal_cc is not authenticated; complete browser login before freezing the baseline")
    previous_boot_id = ssh("cat /proc/sys/kernel/random/boot_id", capture=True).stdout.strip()
    sudo("systemctl stop clash-verge-service.service 2>/dev/null || true")
    ssh(
        "pkill -TERM -u hominal -x chrome 2>/dev/null || true; "
        "pkill -TERM -u hominal -x wechat 2>/dev/null || true; "
        "pkill -TERM -u hominal -x clash-verge 2>/dev/null || true; "
        "for i in {1..40}; do "
        "if ! pgrep -u hominal -x chrome >/dev/null "
        "&& ! pgrep -u hominal -x wechat >/dev/null "
        "&& ! pgrep -u hominal -x clash-verge >/dev/null "
        "&& ! pgrep -x verge-mihomo >/dev/null; then exit 0; fi; "
        "sleep 0.25; done; exit 1"
    )
    temporary = APP_STATE_BACKUP_ROOT.parent / ("." + APP_STATE_BACKUP_ROOT.name + ".partial")
    previous = APP_STATE_BACKUP_ROOT.parent / ("." + APP_STATE_BACKUP_ROOT.name + ".previous")
    if temporary.exists():
        shutil.rmtree(temporary)
    temporary.mkdir(parents=True, mode=0o700)
    run(
        [
            "rsync", "-aH", "--delete",
            "--exclude=SingletonCookie", "--exclude=SingletonLock", "--exclude=SingletonSocket",
            "--exclude=Cache/", "--exclude=Code Cache/", "--exclude=GPUCache/", "--exclude=Crashpad/",
            f"{HOST}:/agent/state/profiles/", str(temporary / "profiles") + "/",
        ],
        timeout=900,
    )
    manifest = {
        "schema": "hominal.persistent-app-state-backup/v1",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "source": "/agent/state/profiles",
        "profiles": ["chrome", "wechat", "clash-verge"],
        "x_account": "@hominal_cc",
        "capture_mode": "applications_stopped_then_rebooted",
        "routine_generation_restore": False,
    }
    (temporary / "manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    (temporary / "files.sha256").write_text(
        "\n".join(baseline_hashes(temporary)) + "\n", encoding="utf-8"
    )
    if previous.exists():
        shutil.rmtree(previous)
    if APP_STATE_BACKUP_ROOT.exists():
        os.replace(APP_STATE_BACKUP_ROOT, previous)
    try:
        os.replace(temporary, APP_STATE_BACKUP_ROOT)
    except BaseException:
        if previous.exists() and not APP_STATE_BACKUP_ROOT.exists():
            os.replace(previous, APP_STATE_BACKUP_ROOT)
        raise
    if previous.exists():
        shutil.rmtree(previous)
    sudo("systemctl reboot", check=False)
    wait_for_reboot(previous_boot_id)
    ensure_chrome_session()
    if body_probe().get("x_session_state") != "authenticated":
        raise RuntimeError("account baseline was saved, but the X session did not survive the clean reboot")
    print(f"app_state_backup={APP_STATE_BACKUP_ROOT}")
    print("baseline_create=passed")


def cmd_baseline_verify() -> None:
    manifest = APP_STATE_BACKUP_ROOT / "manifest.json"
    hashes = APP_STATE_BACKUP_ROOT / "files.sha256"
    if not manifest.is_file() or not hashes.is_file():
        raise RuntimeError("account session baseline is incomplete")
    result = run(["shasum", "-a", "256", "-c", str(hashes)], cwd=APP_STATE_BACKUP_ROOT, capture=True, timeout=900)
    checked = sum(1 for line in result.stdout.splitlines() if line.endswith(": OK"))
    print(f"files_checked={checked}")
    print("baseline_verify=passed")


def cmd_baseline_restore(disaster_recovery: bool = False) -> None:
    if not disaster_recovery:
        raise RuntimeError(
            "routine generations preserve /agent/state/profiles; pass --disaster-recovery only when "
            "you intentionally want to replace persistent application state from the offline backup"
        )
    cmd_baseline_verify()
    current = load_current(required=False)
    if current is not None:
        raise RuntimeError("archive and reset the active generation before restoring account sessions")
    previous_boot_id = ssh("cat /proc/sys/kernel/random/boot_id", capture=True).stdout.strip()
    sudo(
        "systemctl stop hominal.service; "
        "systemctl stop clash-verge-service.service 2>/dev/null || true; "
        "pkill -TERM -u hominal -x chrome 2>/dev/null || true; "
        "pkill -TERM -u hominal -x wechat 2>/dev/null || true; "
        "pkill -TERM -u hominal -x clash-verge 2>/dev/null || true; sleep 3"
    )
    run(
        [
            "rsync", "-aH", "--delete",
            str(APP_STATE_BACKUP_ROOT / "profiles") + "/",
            f"{HOST}:/agent/state/profiles/",
        ],
        timeout=900,
    )
    ssh(
        "rm -f /agent/state/profiles/chrome/SingletonCookie "
        "/agent/state/profiles/chrome/SingletonLock /agent/state/profiles/chrome/SingletonSocket"
    )
    sudo("systemctl reboot", check=False)
    wait_for_reboot(previous_boot_id)
    ensure_chrome_session()
    if body_probe().get("x_session_state") != "authenticated":
        raise RuntimeError("account profiles were restored, but the X session is not authenticated")
    sudo("/usr/local/sbin/hominal-persist-app-state check hominal")
    print("baseline_restore=passed")


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


def ecological_encounter_available(stage: int) -> bool:
    return stage >= 5


def cmd_encounter(name: str) -> None:
    current = load_current()
    if not ecological_encounter_available(int(current.get("stage", 0))):
        raise RuntimeError("ecological encounters require an active stage-five-or-later instance")
    source = ROOT / "lab" / "encounters" / name
    if not source.is_dir():
        raise RuntimeError(f"unknown encounter {name}")
    instance_id = str(current["instance_id"])
    occurrence = f"encounter-{name}-{utcstamp()}-{uuid.uuid4().hex[:6]}"
    remote_archive = f"/agent/staging/{occurrence}.tar.gz"
    destination = f"/agent/lives/{instance_id}/life/inbox/{occurrence}"
    with tempfile.TemporaryDirectory(prefix="hominal-encounter-") as directory_name:
        archive = Path(directory_name) / f"{occurrence}.tar.gz"
        with tarfile.open(archive, "w:gz") as bundle:
            for child in sorted(source.iterdir()):
                bundle.add(child, arcname=child.name, recursive=True)
        scp(archive, remote_archive)
    sudo(
        f"install -d -m 0755 {shlex.quote(destination)} && "
        f"tar -xzf {shlex.quote(remote_archive)} -C {shlex.quote(destination)} && "
        f"rm -f {shlex.quote(remote_archive)}"
    )
    payload = {
        "encounter": name,
        "path": f"/life/inbox/{occurrence}",
        "object_kind": "directory",
        "observed_at": datetime.now(timezone.utc).isoformat(),
    }
    response = mentor_api(
        "POST",
        "/v1/environment/events",
        {
            "event_id": occurrence,
            "summary": "一个新的外部物件进入了你的生活空间。",
            "payload": payload,
        },
    )
    print(json.dumps(response, ensure_ascii=False, sort_keys=True))
    print(f"encounter={name}")
    print(f"path=/life/inbox/{occurrence}")


def cmd_mentor_send(body: str | None, speaker: str, message_id: str | None, reply_to: str | None) -> None:
    load_current()
    if body is None:
        body = sys.stdin.read().strip()
    if not body:
        raise RuntimeError("mentor message body is empty")
    prefix = "[Codex代理导师]" if speaker == "codex" else "[人类导师·经Codex传递]"
    message_id = message_id or f"codex-{utcstamp()}-{uuid.uuid4().hex[:8]}"
    message_body = body if body.startswith(prefix) else prefix + "\n" + body
    payload: dict[str, object] = {"message_id": message_id, "body": message_body}
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


def archive_system_delta(current: dict[str, object], destination: Path) -> None:
    if str(current.get("kind")) not in {"rehearsal", "formal"}:
        return
    archived_delta = destination / "system-delta.json"
    if archived_delta.is_file():
        delta = json.loads(archived_delta.read_text(encoding="utf-8"))
        current["system_delta"] = {
            "status": "complete",
            "before_sha256": sha256(destination / "system-before.json"),
            "final_sha256": sha256(destination / "system-final.json"),
            "delta_sha256": sha256(archived_delta),
            "summary": delta.get("summary", {}),
        }
        return
    before_value = str(current.get("system_before_path", ""))
    before_path = Path(before_value) if before_value else Path("/")
    try:
        if not before_value or not before_path.is_file():
            raise RuntimeError("system birth inventory is missing")
        expected_hash = str(current.get("system_before_sha256", ""))
        observed_hash = sha256(before_path)
        if not expected_hash or observed_hash != expected_hash:
            raise RuntimeError("system birth inventory hash mismatch")
        before = json.loads(before_path.read_text(encoding="utf-8"))
        after = capture_system_inventory()
        delta = system_inventory_delta(before, after)
        write_private_json(destination / "system-before.json", before)
        write_private_json(destination / "system-final.json", after)
        write_private_json(destination / "system-delta.json", delta)
        current["system_delta"] = {
            "status": "complete",
            "before_sha256": sha256(destination / "system-before.json"),
            "final_sha256": sha256(destination / "system-final.json"),
            "delta_sha256": sha256(destination / "system-delta.json"),
            "summary": delta["summary"],
        }
        before_path.unlink()
    except (OSError, RuntimeError, json.JSONDecodeError, subprocess.TimeoutExpired) as exc:
        message = f"{exc.__class__.__name__}: {exc}"
        (destination / "system-delta-error.txt").write_text(message + "\n", encoding="utf-8")
        (destination / "system-delta-error.txt").chmod(0o600)
        current["system_delta"] = {"status": "evidence_gap", "error": message}


def archive_instance(current: dict[str, object]) -> Path:
    instance_id = str(current["instance_id"])
    destination = ARCHIVE_ROOT / str(current.get("kind", "engineering")) / instance_id
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
    remote_birth = f"/agent/lives/{instance_id}/birth/birth.yaml"
    if not (destination / "birth.yaml").is_file():
        scp(remote_birth, str(destination / "birth.yaml"), from_remote=True)
    release_id = str(current["release_id"])
    bundle_source = ARCHIVE_ROOT / "bundles" / release_id / f"{release_id}.tar.gz"
    if bundle_source.is_file() and not (destination / "bundle.tar.gz").is_file():
        try:
            os.link(bundle_source, destination / "bundle.tar.gz")
        except OSError:
            shutil.copy2(bundle_source, destination / "bundle.tar.gz")
    release_source = ARCHIVE_ROOT / "bundles" / release_id / "release.yaml"
    if release_source.is_file():
        shutil.copy2(release_source, destination / "release.yaml")
    archive_system_delta(current, destination)
    supervision_value = str(current.get("supervision_path", ""))
    if supervision_value:
        supervision_path = Path(supervision_value)
        if supervision_path.is_file():
            shutil.copy2(supervision_path, destination / "supervision.jsonl")
            (destination / "supervision.jsonl").chmod(0o600)
            supervision_path.unlink()
    (destination / "manifest.json").write_text(
        json.dumps(current, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    since = str(current.get("prepared_at", ""))
    journal_command = "journalctl -u hominal.service --no-pager -n 1000"
    if since:
        journal_command += " --since " + shlex.quote(since)
    log = ssh(journal_command, check=False, capture=True).stdout
    (destination / "systemd.log").write_text(log or "", encoding="utf-8")
    journal_path = f"/agent/lives/{instance_id}/journal/events.jsonl"
    transcript_script = r'''
import json, sys
for line in open(sys.argv[1], encoding='utf-8'):
    row=json.loads(line)
    if row.get('kind') in {'mentor_received','mentor_queued','mentor_delivered'}:
        print(json.dumps(row, ensure_ascii=False, separators=(',',':')))
'''
    transcript = sudo(
        "python3 -c " + shlex.quote(transcript_script) + " " + shlex.quote(journal_path),
        check=False,
        capture=True,
    ).stdout
    (destination / "mentor-transcript.jsonl").write_text(transcript or "", encoding="utf-8")
    interventions = current.get("interventions", [])
    (destination / "interventions.jsonl").write_text(
        "".join(json.dumps(item, ensure_ascii=False, separators=(",", ":")) + "\n" for item in interventions),
        encoding="utf-8",
    )
    write_yaml(destination / "final-body.yaml", body_probe())
    hashes = []
    for path in sorted(destination.iterdir()):
        if path.is_file() and path.name != "hashes.sha256":
            hashes.append(f"{sha256(path)}  {path.name}")
    (destination / "hashes.sha256").write_text("\n".join(hashes) + "\n", encoding="utf-8")
    return destination


def cmd_stop(reason: str = "manual") -> None:
    if reason not in STOP_REASONS:
        raise RuntimeError("invalid stop reason")
    current = load_current()
    archive_value = str(current.get("archive_path", ""))
    if archive_value and (Path(archive_value) / "agent-final.tar.gz").is_file():
        print(f"archive={archive_value}")
        print("stop=already_archived")
        return
    existing_stop = str(current.get("stop_reason", ""))
    if current.get("stop_requested_at") and existing_stop and existing_stop != reason:
        raise RuntimeError(f"generation stop already in progress with reason {existing_stop}")
    if not current.get("stop_requested_at"):
        current["stop_reason"] = reason
        current["stop_requested_at"] = datetime.now(timezone.utc).isoformat()
        current.setdefault("interventions", []).append({
            "kind": "generation_stop_requested",
            "reason": reason,
            "recorded_at": current["stop_requested_at"],
        })
    save_current(current)
    deadline_unit = str(current.get("deadline_unit", ""))
    cancel = ""
    if deadline_unit:
        cancel = f"systemctl stop {shlex.quote(deadline_unit + '.timer')} {shlex.quote(deadline_unit + '.service')} 2>/dev/null || true; "
    sudo(cancel + f"systemctl stop hominal.service; rm -f {shlex.quote(REMOTE_RUNTIME_CONFIG)}")
    state = ssh("systemctl is-active hominal.service", check=False, capture=True).stdout.strip()
    if state == "active":
        raise RuntimeError("hominal.service remained active after explicit stop")
    if str(current.get("kind")) in {"rehearsal", "formal"}:
        sudo(
            "pkill -TERM -u hominal -x chrome 2>/dev/null || true; "
            "pkill -TERM -u hominal -x wechat 2>/dev/null || true; sleep 3"
        )
    current["stopped_at"] = datetime.now(timezone.utc).isoformat()
    destination = archive_instance(current)
    current["archive_path"] = str(destination)
    save_current(current)
    print(f"archive={destination}")
    print("stop=passed")


def cmd_reset() -> None:
    current = load_current()
    instance_id = str(current["instance_id"])
    destination = ARCHIVE_ROOT / str(current.get("kind", "engineering")) / instance_id / "agent-final.tar.gz"
    if not destination.is_file():
        raise RuntimeError("engineering instance is not archived; run stop first")
    instance_root = "/agent/lives/" + instance_id
    expected = shlex.quote(instance_id)
    sudo(
        "set -eu; systemctl stop hominal.service; "
        "active=$(cat /agent/boot/active-instance 2>/dev/null || true); "
        f"if [ -n \"$active\" ] && [ \"$active\" != {expected} ]; then exit 24; fi; "
        "if mountpoint -q /life; then umount /life; fi; "
        f"rm -rf -- {shlex.quote(instance_root)}; "
        "rm -f -- /agent/boot/active-instance /agent/boot/active-release /agent/boot/ended-instance; "
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
    start.add_argument("--stage", type=int, choices=(3, 4, 5, 8, 9, 10), required=True)
    start.add_argument("--kind", choices=("engineering", "rehearsal", "formal"), default="engineering")
    start.add_argument("--window-seconds", type=int, default=300)
    start.add_argument("--model", choices=("luna", "terra", "sol"))
    start.add_argument("--reasoning-effort", choices=("none", "low"))
    subparsers.add_parser("status")
    subparsers.add_parser("inspect")
    supervise = subparsers.add_parser("supervise")
    supervise.add_argument("--interval-seconds", type=int, default=20)
    supervise.add_argument("--unreachable-grace-seconds", type=int, default=600)
    extend = subparsers.add_parser("extend")
    extend.add_argument("--minutes", type=int, default=30)
    subparsers.add_parser("process-restart")
    subparsers.add_parser("browser-check")
    subparsers.add_parser("seal")
    mentor_send = subparsers.add_parser("mentor-send")
    mentor_send.add_argument("--body")
    mentor_send.add_argument("--speaker", choices=("codex", "human"), default="codex")
    mentor_send.add_argument("--message-id")
    mentor_send.add_argument("--reply-to")
    subparsers.add_parser("mentor-outbox")
    encounter = subparsers.add_parser("encounter")
    encounter.add_argument("name", choices=("a", "b", "c"))
    mentor_ack = subparsers.add_parser("mentor-ack")
    mentor_ack.add_argument("message_id")
    subparsers.add_parser("crash")
    stop = subparsers.add_parser("stop")
    stop.add_argument("--reason", choices=sorted(STOP_REASONS), default="manual")
    subparsers.add_parser("reset")
    subparsers.add_parser("baseline-create")
    subparsers.add_parser("baseline-verify")
    baseline_restore = subparsers.add_parser("baseline-restore")
    baseline_restore.add_argument("--disaster-recovery", action="store_true")
    args = parser.parse_args()
    try:
        if args.command == "check":
            cmd_check()
        elif args.command == "start":
            cmd_start(args.stage, args.kind, args.window_seconds, args.model, args.reasoning_effort)
        elif args.command == "status":
            cmd_status()
        elif args.command == "inspect":
            cmd_inspect()
        elif args.command == "supervise":
            cmd_supervise(args.interval_seconds, args.unreachable_grace_seconds)
        elif args.command == "extend":
            cmd_extend(args.minutes)
        elif args.command == "process-restart":
            cmd_process_restart()
        elif args.command == "browser-check":
            cmd_browser_check()
        elif args.command == "seal":
            current = load_current()
            if current.get("birth_status") != "sealed":
                prepare_generation_body_for_birth(current)
            seal_generation_birth(current)
            schedule_generation_deadline(current)
            deliver_birth_message(current)
            print("seal=passed")
        elif args.command == "mentor-send":
            cmd_mentor_send(args.body, args.speaker, args.message_id, args.reply_to)
        elif args.command == "mentor-outbox":
            cmd_mentor_outbox()
        elif args.command == "encounter":
            cmd_encounter(args.name)
        elif args.command == "mentor-ack":
            cmd_mentor_ack(args.message_id)
        elif args.command == "crash":
            cmd_crash()
        elif args.command == "stop":
            cmd_stop(args.reason)
        elif args.command == "reset":
            cmd_reset()
        elif args.command == "baseline-create":
            cmd_baseline_create()
        elif args.command == "baseline-verify":
            cmd_baseline_verify()
        elif args.command == "baseline-restore":
            cmd_baseline_restore(args.disaster_recovery)
    except (KeyError, OSError, RuntimeError, subprocess.TimeoutExpired) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
