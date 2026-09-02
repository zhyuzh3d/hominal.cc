#!/usr/bin/env python3
"""Fail closed when a Git change contains local configuration or credentials.

The scanner never prints a matched value or source-config contents. It compares
candidate Git blobs against secret-like scalar values loaded from the protected
YAML configuration and also applies conservative high-confidence detectors.
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from typing import Iterable


ZERO_OID = "0" * 40
SENSITIVE_KEY = re.compile(
    r"(?:^|[_-])(?:api[_-]?key|access[_-]?key|app[_-]?secret|auth(?:orization)?|"
    r"client[_-]?secret|cookie|credential|pass(?:word)?|phone|mobile|private[_-]?key|secret|token)(?:$|[_-])",
    re.IGNORECASE,
)
FORBIDDEN_NAME = re.compile(
    r"(?:^|/)(?:xconfig(?:\.[^/]*)?\.(?:ya?ml)|\.env(?:\.[^/]*)?|credentials\.json|"
    r"secrets?\.ya?ml|id_(?:rsa|dsa|ecdsa|ed25519)|[^/]+\.(?:pem|key))$",
    re.IGNORECASE,
)
HIGH_CONFIDENCE_PATTERNS = (
    ("private key material", re.compile(rb"-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----")),
    ("GitHub token", re.compile(rb"\bgh[opusr]_[A-Za-z0-9_]{30,}\b")),
    ("AWS access key", re.compile(rb"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b")),
    ("OpenAI-style key", re.compile(rb"\bsk-[A-Za-z0-9_-]{20,}\b")),
    ("Google API key", re.compile(rb"\bAIza[0-9A-Za-z_-]{30,}\b")),
    ("Slack token", re.compile(rb"\bxox[baprs]-[A-Za-z0-9-]{20,}\b")),
    ("Stripe live key", re.compile(rb"\b[rs]k_live_[A-Za-z0-9]{16,}\b")),
)
ASSIGNMENT = re.compile(
    r"(?i)(?:^|[\s,{\[])[\"']?"
    r"(?:api[_-]?key|access[_-]?key|app[_-]?secret|authorization|client[_-]?secret|"
    r"cookie|credential|pass(?:word)?|private[_-]?key|secret|token)"
    r"[\"']?\s*[:=]\s*(?P<value>[^\r\n#,}]+)"
)
PLACEHOLDER = re.compile(
    r"(?i)^(?:[\"']?\s*)?(?:\$\{|\$[A-Z_]|<|example|sample|placeholder|redacted|"
    r"changeme|replace[_ -]?me|your[_ -]|none|null|os\.(?:environ|getenv)|"
    r"process\.env|env\.|\*{3,})"
)


def git(*args: str, input_bytes: bytes | None = None) -> bytes:
    proc = subprocess.run(
        ["git", *args],
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if proc.returncode:
        message = proc.stderr.decode("utf-8", "replace").strip()
        raise RuntimeError(f"git {' '.join(args)} failed: {message}")
    return proc.stdout


def repo_root() -> Path:
    return Path(git("rev-parse", "--show-toplevel").decode().strip()).resolve()


def protected_config_path(root: Path) -> Path:
    config_query = subprocess.run(
        ["git", "config", "--local", "--get", "hominal.protectedConfig"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if config_query.returncode not in (0, 1):
        message = config_query.stderr.decode("utf-8", "replace").strip()
        raise RuntimeError(f"cannot read hominal.protectedConfig: {message}")
    configured = config_query.stdout.decode().strip()
    if configured:
        path = Path(configured).expanduser()
        return path if path.is_absolute() else (root / path).resolve()
    override = os.environ.get("HOMINAL_PROTECTED_CONFIG")
    if override:
        return Path(override).expanduser().resolve()
    return (root.parent / "xconfigs" / "hominal" / "xconfig.yaml").resolve()


def load_protected_values(config_path: Path) -> list[tuple[str, bytes]]:
    if not config_path.is_file():
        raise RuntimeError(
            "protected configuration is unavailable; refusing to scan without it: "
            f"{config_path}"
        )
    try:
        import yaml  # type: ignore
    except ImportError as exc:
        raise RuntimeError("PyYAML is required for fail-closed secret comparison") from exc

    try:
        document = yaml.safe_load(config_path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise RuntimeError(f"cannot safely parse protected configuration: {config_path}") from exc

    found: list[tuple[str, bytes]] = []

    def visit(value: object, key_path: tuple[str, ...] = ()) -> None:
        if isinstance(value, dict):
            for key, child in value.items():
                visit(child, (*key_path, str(key)))
            return
        if isinstance(value, list):
            for index, child in enumerate(value):
                visit(child, (*key_path, str(index)))
            return
        if not isinstance(value, str) or not key_path or not SENSITIVE_KEY.search(key_path[-1]):
            return
        rendered = str(value)
        if not rendered or PLACEHOLDER.search(rendered.strip()):
            return
        found.append((".".join(key_path), rendered.encode("utf-8")))

    visit(document)
    if not found:
        raise RuntimeError(
            "no secret-like scalar values were found in the protected configuration; "
            "refusing to assume the comparison is complete"
        )
    return found


def path_is_forbidden(path: str) -> bool:
    normalized = PurePosixPath(path).as_posix()
    while normalized.startswith("./"):
        normalized = normalized[2:]
    if normalized.endswith("/.env.example") or normalized.endswith("/.env.sample"):
        return False
    return bool(FORBIDDEN_NAME.search(normalized))


def scan_blob(path: str, content: bytes, protected: list[tuple[str, bytes]]) -> list[str]:
    problems: list[str] = []
    if path_is_forbidden(path):
        problems.append("forbidden credential/configuration filename")

    for label, pattern in HIGH_CONFIDENCE_PATTERNS:
        if pattern.search(content):
            problems.append(label)

    for key_path, value in protected:
        if value in content:
            problems.append(f"matches protected value from {key_path}")

    text = content.decode("utf-8", "ignore")
    for match in ASSIGNMENT.finditer(text):
        candidate = match.group("value").strip().strip("\"'")
        if candidate and not PLACEHOLDER.search(candidate):
            line = text.count("\n", 0, match.start()) + 1
            problems.append(f"literal credential assignment near line {line}")

    return list(dict.fromkeys(problems))


def staged_blobs() -> Iterable[tuple[str, bytes]]:
    paths = git("diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")
    for raw_path in paths.split(b"\0"):
        if not raw_path:
            continue
        path = raw_path.decode("utf-8", "surrogateescape")
        yield path, git("show", f":{path}")


def worktree_blobs(root: Path) -> Iterable[tuple[str, bytes]]:
    paths = git("ls-files", "-co", "--exclude-standard", "-z")
    for raw_path in paths.split(b"\0"):
        if not raw_path:
            continue
        path = raw_path.decode("utf-8", "surrogateescape")
        full_path = root / path
        if full_path.is_file() and not full_path.is_symlink():
            yield path, full_path.read_bytes()


def commits_from_pre_push(lines: Iterable[str]) -> list[str]:
    positive: list[str] = []
    negative: list[str] = []
    for line in lines:
        fields = line.split()
        if len(fields) != 4:
            continue
        _local_ref, local_oid, _remote_ref, remote_oid = fields
        if local_oid != ZERO_OID:
            positive.append(local_oid)
        if remote_oid != ZERO_OID:
            negative.append(f"^{remote_oid}")
    if not positive:
        return []
    output = git("rev-list", *positive, *negative)
    return [line for line in output.decode().splitlines() if line]


def tree_blobs(commits: Iterable[str]) -> Iterable[tuple[str, bytes]]:
    seen: set[tuple[str, str]] = set()
    for commit in commits:
        records = git("ls-tree", "-r", "-z", commit)
        for record in records.split(b"\0"):
            if not record:
                continue
            metadata, raw_path = record.split(b"\t", 1)
            _mode, object_type, oid = metadata.decode().split()
            if object_type != "blob":
                continue
            path = raw_path.decode("utf-8", "surrogateescape")
            identity = (oid, path)
            if identity in seen:
                continue
            seen.add(identity)
            yield path, git("cat-file", "blob", oid)


def all_history_commits() -> list[str]:
    output = git("rev-list", "--all")
    return [line for line in output.decode().splitlines() if line]


def run_scan(blobs: Iterable[tuple[str, bytes]], protected: list[tuple[str, bytes]]) -> int:
    violations: list[tuple[str, list[str]]] = []
    for path, content in blobs:
        problems = scan_blob(path, content, protected)
        if problems:
            violations.append((path, problems))

    if not violations:
        print("secret scan passed")
        return 0

    print("secret scan blocked the Git operation; matched values are redacted", file=sys.stderr)
    for path, problems in violations:
        print(f"  {path}: {', '.join(problems)}", file=sys.stderr)
    return 1


def main() -> int:
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--staged", action="store_true")
    group.add_argument("--pre-push", action="store_true")
    group.add_argument("--worktree", action="store_true")
    group.add_argument("--all-history", action="store_true")
    args = parser.parse_args()

    root = repo_root()
    protected = load_protected_values(protected_config_path(root))
    if args.staged:
        blobs = staged_blobs()
    elif args.pre_push:
        blobs = tree_blobs(commits_from_pre_push(sys.stdin))
    elif args.worktree:
        blobs = worktree_blobs(root)
    else:
        blobs = tree_blobs(all_history_commits())
    return run_scan(blobs, protected)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"secret scan failed closed: {exc}", file=sys.stderr)
        raise SystemExit(2)
