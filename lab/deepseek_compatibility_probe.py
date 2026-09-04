#!/usr/bin/env python3
"""Isolated fixed_shape_v1 tests. No returned function is ever executed.

The cognition phase changes only the declared function surface: one fixed
function for each original action branch. Original field constraints and the
frozen personal context remain. It does not implement a runtime adapter.
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


def call(body, label, destination, expected=None, expected_status=200, expected_text=None):
    gateway = run.model_gateway_settings(run.SETTINGS["config"]["llm"])
    if gateway["adapter"] != "llmserver":
        raise RuntimeError("Selected gateway is not llmserver")
    request = urllib.request.Request(
        gateway["base_url"] + "/v1/responses", data=json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + gateway["api_key"], "Content-Type": "application/json"})
    started = time.monotonic()
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            status, raw, headers = response.status, response.read(), response.headers
    except urllib.error.HTTPError as error:
        status, raw, headers = error.code, error.read(), error.headers
    except (TimeoutError, OSError) as error:
        status, raw, headers = 0, json.dumps({"error": {"code": type(error).__name__}}).encode(), {}
    result = json.loads(raw)
    violations = []
    functions = [item for item in result.get("output", []) if item.get("type") == "function_call"]
    schemas = {tool["name"]: tool["parameters"] for tool in body.get("tools", [])}
    if status != expected_status:
        violations.append("unexpected HTTP status")
    if expected_status == 200 and status == 200:
        if expected and (len(functions) != 1 or functions[0].get("name") not in expected):
            violations.append("expected exactly one permitted function")
        for function in functions:
            if not function.get("call_id") or function.get("name") not in schemas:
                violations.append("invalid function identity")
                continue
            try:
                value = json.loads(function["arguments"])
                for error in jsonschema.Draft202012Validator(schemas[function["name"]]).iter_errors(value):
                    violations.append({"path": list(error.absolute_path), "message": error.message})
            except (KeyError, ValueError, TypeError) as error:
                violations.append(str(error))
        text = result.get("output_text", "") or "".join(
            block.get("text", "") for item in result.get("output", []) for block in item.get("content", []))
        if expected_text and (text.strip() != expected_text or functions):
            violations.append("function-result continuation did not return the synthetic marker")
        if result.get("llmserver_billing", {}).get("settlement_status") != "confirmed":
            violations.append("no confirmed bill")
    summary = {"label": label, "model": body["model"], "effort": body["reasoning"]["effort"],
               "status": status, "seconds": round(time.monotonic()-started, 3),
               "request_id": headers.get("x-llmserver-request-id", ""),
               "functions": [f.get("name") for f in functions], "violations": violations,
               "error": result.get("error"),
               "billed_usd": result.get("llmserver_billing", {}).get("charges", {}).get("total")}
    with (destination / (label + ".json")).open("x") as output:
        (destination / (label + ".json")).chmod(0o600)
        json.dump({"summary": summary, "request": body, "response": result}, output, ensure_ascii=False, indent=2)
    print(json.dumps(summary, ensure_ascii=False), flush=True)
    return result, not violations


def fixed_cognition(original):
    body = copy.deepcopy(original)
    source = body["tools"][0]
    shape = source["parameters"]
    branches = shape["properties"]["action"]["anyOf"]
    body["tools"] = []
    for branch in branches:
        kind = branch["properties"]["kind"]["enum"][0]
        tool = copy.deepcopy(source)
        tool.update(name="cognitive_" + kind, strict=False)
        tool["description"] += "本函数的 action 使用 " + kind + " 固定结构。"
        tool["parameters"]["properties"]["action"] = copy.deepcopy(branch)
        body["tools"].append(tool)
    body["tool_choice"] = "required"
    body["instructions"] += "\n提交格式：选择一个 cognitive_none、cognitive_organ_action 或 cognitive_mentor_send 函数完成本轮。它们承载同一次完整认知，只是身体动作结构不同；按所选函数的字段提交。"
    return body


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=["guards", "basic", "cognition"])
    parser.add_argument("--frozen-request", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--model", default="deepseek-v4-flash")
    parser.add_argument("--effort", default="none")
    parser.add_argument("--repeat", type=int, default=3)
    args = parser.parse_args()
    args.output_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    original = json.loads(args.frozen_request.read_text())
    original.pop("llmserver", None)
    original["model"] = args.model
    original["reasoning"] = {"effort": args.effort}
    checks = []
    if args.phase == "guards":
        for label, strict in [("old_strict", True), ("old_union", False)]:
            body = copy.deepcopy(original)
            for tool in body["tools"]:
                tool["strict"] = strict
            _, valid = call(body, label, args.output_dir, expected_status=422)
            checks.append(valid)
    elif args.phase == "cognition":
        body = fixed_cognition(original)
        names = {tool["name"] for tool in body["tools"]}
        for index in range(args.repeat):
            # Each attempt is independent and charged once; no hidden retry.
            _, valid = call(body, f"cognition_{index+1}", args.output_dir, expected=names)
            checks.append(valid)
    else:
        text = {"type": "string", "minLength": 1}
        def function(name, properties):
            return {"type": "function", "name": name, "strict": False, "description": name,
                    "parameters": {"type": "object", "properties": properties,
                                   "required": list(properties), "additionalProperties": False}}
        tools = [function("finish_without_action", {"observation": text}), function("request_action", {
            "operation": {"type": "string", "enum": ["read_document", "search_records"]},
            "input": {"type": "object", "properties": {"query": text}, "required": ["query"], "additionalProperties": False},
            "intent": text, "prediction": text, "reality_check": text})]
        base = {"model": args.model, "reasoning": {"effort": args.effort}, "store": False,
                "max_output_tokens": 400, "tools": tools, "parallel_tool_calls": False, "tool_choice": "required"}
        for label, prompt, name in [
            ("observe", "当前只观察到天气晴朗，请记录观察，不发起外部动作。", "finish_without_action"),
            ("action", "请提出搜索 Ice People 纪录片资料的行动，用于了解其主题，填写目的、预测和核验方法；本测试不会执行该行动。收到合成工具结果后仅返回其 marker。", "request_action")]:
            body = {**base, "input": prompt}
            response, valid = call(body, label, args.output_dir, expected={name})
            checks.append(valid)
            if label == "action" and valid:
                fc = next(item for item in response["output"] if item.get("type") == "function_call")
                follow = {**base, "tool_choice": "none", "input": [
                    {"role": "user", "content": prompt}, *response["output"],
                    {"type": "function_call_output", "call_id": fc["call_id"],
                     "output": json.dumps({"synthetic": True, "marker": "verified_43"})}]}
                _, valid = call(follow, "followup", args.output_dir, expected_text="verified_43")
                checks.append(valid)
    return 0 if checks and all(checks) else 1


if __name__ == "__main__":
    raise SystemExit(main())
