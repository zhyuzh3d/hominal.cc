#!/usr/bin/env python3
"""One paid, non-enacting contract probe. Never imports results into a life.

--inspect-schema retains the requested schema as the first alternative but
allows an object through the gateway for local diagnosis. That is a different
generation constraint, not proof of what a discarded earlier response was.
"""
import argparse
import copy
import json
import time
import urllib.error
import urllib.request
from pathlib import Path

import jsonschema
import run


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("request", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--inspect-schema", action="store_true")
    args = parser.parse_args()
    body = json.loads(args.request.read_text())
    body.pop("llmserver", None)
    originals = {tool["name"]: copy.deepcopy(tool["parameters"]) for tool in body["tools"]}
    if args.inspect_schema:
        for tool in body["tools"]:
            tool["strict"] = False
            tool["parameters"] = {"type": "object", "anyOf": [tool["parameters"], {"type": "object"}]}
    gateway = run.model_gateway_settings(run.SETTINGS["config"]["llm"])
    if gateway["adapter"] != "llmserver":
        raise RuntimeError("probe requires selected llmserver")
    request = urllib.request.Request(
        gateway["base_url"] + "/v1/responses", data=json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + gateway["api_key"], "Content-Type": "application/json"},
    )
    started = time.monotonic()
    try:
        with urllib.request.urlopen(request, timeout=90) as response:
            status, raw = response.status, response.read()
            request_id = response.headers.get("x-llmserver-request-id", "")
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read()
        request_id = error.headers.get("x-llmserver-request-id", "")
    result = json.loads(raw)
    violations = []
    functions = [item for item in result.get("output", []) if item.get("type") == "function_call"]
    if result.get("error"):
        violations.append({"error": "response_failed", "detail": result.get("error")})
    if body.get("tool_choice") != "none" and len(functions) != 1:
        violations.append({"error": "expected exactly one function call", "count": len(functions)})
    for item in functions:
        try:
            value = json.loads(item["arguments"])
        except (KeyError, TypeError, ValueError) as error:
            violations.append({"error": "invalid function JSON", "detail": str(error)})
            continue
        schema = originals.get(item["name"])
        if schema is None:
            violations.append({"error": "undeclared function", "name": item["name"]})
            continue
        for error in jsonschema.Draft202012Validator(schema).iter_errors(value):
            violations.append({"path": list(error.absolute_path), "error": error.message})
    billing = result.get("llmserver_billing") or {}
    if billing.get("settlement_status") != "confirmed" or billing.get("currency") != "USD":
        violations.append({"error": "billing is not a confirmed USD settlement"})
    record = {"status": status, "seconds": time.monotonic() - started, "request_id": request_id,
              "diagnostic_relaxation": args.inspect_schema, "violations": violations, "response": result}
    with args.output.open("x") as output:
        args.output.chmod(0o600)
        json.dump(record, output, ensure_ascii=False, indent=2)
    print(json.dumps({key: value for key, value in record.items() if key != "response"}, ensure_ascii=False))
    print(json.dumps({"id": result.get("id"), "error": result.get("error"),
                      "billing": result.get("llmserver_billing")}, ensure_ascii=False))
    return 0 if status == 200 and not violations else 1


if __name__ == "__main__":
    raise SystemExit(main())
