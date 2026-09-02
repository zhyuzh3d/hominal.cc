from __future__ import annotations

import importlib.util
import json
from pathlib import Path
import subprocess
import unittest
from unittest import mock


MODULE_PATH = Path(__file__).with_name("run.py")
SPEC = importlib.util.spec_from_file_location("hominal_lab_run", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
LAB = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(LAB)


class SystemDeltaTest(unittest.TestCase):
    def test_identical_inventory_has_no_mapping_changes(self) -> None:
        inventory = {
            "captured_at": "2026-08-26T00:00:00+00:00",
            "boot_id": "a",
            "root_free_bytes": 100,
            "agent_free_bytes": 200,
            "lvm": {"report": []},
            "packages": {"a": "1"},
            "unit_files": {"a.service": "enabled"},
            "files": {"/etc/a": {"sha256": "one"}},
        }
        delta = LAB.system_inventory_delta(inventory, inventory)
        self.assertTrue(
            all(
                count == 0
                for section in delta["summary"].values()
                for count in section.values()
            )
        )

    def test_delta_preserves_added_removed_and_changed_facts(self) -> None:
        before = {
            "packages": {"kept": "1", "removed": "1"},
            "unit_files": {"a.service": "disabled"},
            "files": {"/etc/a": {"sha256": "one"}},
        }
        after = {
            "packages": {"kept": "2", "added": "1"},
            "unit_files": {"a.service": "enabled"},
            "files": {"/etc/a": {"sha256": "two"}},
        }
        delta = LAB.system_inventory_delta(before, after)
        self.assertEqual(delta["packages"]["added"], {"added": "1"})
        self.assertEqual(delta["packages"]["removed"], {"removed": "1"})
        self.assertEqual(
            delta["packages"]["changed"]["kept"],
            {"before": "1", "after": "2"},
        )
        self.assertEqual(delta["summary"]["unit_files"]["changed"], 1)
        self.assertEqual(delta["summary"]["files"]["changed"], 1)

    def test_go_nanosecond_timestamp_parses(self) -> None:
        value = LAB.parse_rfc3339("2026-08-25T20:01:01.573476915Z")
        self.assertEqual(value.microsecond, 573476)


class BodySurfaceTest(unittest.TestCase):
    def test_life_is_exposed_as_a_bind_mounted_directory(self) -> None:
        launcher = (MODULE_PATH.parent.parent / "deploy" / "hominal-launcher").read_text(
            encoding="utf-8"
        )
        self.assertIn('mount --bind "$life_source" "$life_surface"', launcher)
        self.assertNotIn('ln -sfn', launcher)


class EcologicalEncounterTest(unittest.TestCase):
    def test_encounters_remain_available_after_stage_five(self) -> None:
        self.assertFalse(LAB.ecological_encounter_available(4))
        self.assertTrue(LAB.ecological_encounter_available(5))
        self.assertTrue(LAB.ecological_encounter_available(8))
        self.assertTrue(LAB.ecological_encounter_available(9))

    def test_encounter_event_identifies_the_object_as_a_directory(self) -> None:
        source = MODULE_PATH.read_text(encoding="utf-8")
        self.assertIn('"object_kind": "directory"', source)


class GenerationDeadlineTest(unittest.TestCase):
    def test_deadline_uses_absolute_calendar_time(self) -> None:
        source = MODULE_PATH.read_text(encoding="utf-8")
        self.assertIn("--on-calendar=", source)
        self.assertNotIn("--on-active=", source)
        self.assertIn('"clock": "wall_calendar"', source)


class MentorProtocolTest(unittest.TestCase):
    def test_stage_ten_birth_message_exposes_body_without_a_task(self) -> None:
        message = LAB.mentor_birth_message()
        self.assertTrue(message.startswith("[Codex代理导师]"))
        self.assertIn("系统管理员权限", message)
        self.assertIn("以 `root` 身份运行", message)
        self.assertIn("图形桌面账户是 `hominal`", message)
        self.assertIn("Wikipedia", message)
        self.assertIn("@hominal_cc", message)
        self.assertIn("公开内容跨实验持续保存", message)
        self.assertIn("新的状态 URL", message)
        self.assertIn("微信", message)
        self.assertIn("编写并运行代码", message)
        self.assertIn("唤醒", message)
        self.assertIn("认知资源", message)
        self.assertNotIn("我在这里。刚刚开始使用这个身体", message)
        self.assertNotIn("几个彼此独立的外部物件", message)
        self.assertNotIn("请按照", message)

    def test_birth_manifest_states_the_real_mentor_support_relation(self) -> None:
        birth = LAB.prepared_birth(
            kind="rehearsal",
            window_seconds=1200,
            instance_id="g0r-example",
            release={
                "release_id": "g0s9-example",
                "bundle_sha256": "bundle",
                "git_commit": "commit",
            },
            probe={"x_session_state": "authenticated"},
            stage=10,
        )
        self.assertEqual(birth["identity"]["experiment_stage"], 10)
        self.assertEqual(birth["body"]["configured_capabilities"]["body_action_identity"], "root")
        self.assertEqual(birth["body"]["configured_capabilities"]["runtime_home_directory"], "/root")
        self.assertEqual(birth["body"]["configured_capabilities"]["runtime_working_directory"], "/agent/lives/g0r-example")
        self.assertEqual(birth["body"]["configured_capabilities"]["desktop_session_identity"], "hominal")
        self.assertEqual(birth["resources"]["spaces"]["workspace"], "/life/workspace")
        expression = birth["resources"]["spaces"]["public_expression"]
        self.assertEqual(expression["content_persistence"], "cross_generation")
        self.assertEqual(expression["existing_content_role"], "lineage_environment")
        self.assertEqual(expression["current_generation_publication_evidence"], "action_result_with_new_status_url")
        mentor = birth["resources"]["spaces"]["mentor"]
        self.assertEqual(mentor["relationship"], "awakener_and_genesis_supporter")
        self.assertIn("cognitive_resources", mentor["provided_conditions"])
        self.assertIn("system_recovery", mentor["provided_conditions"])
        cognition = birth["resources"]["cognition"]
        self.assertEqual(cognition["roles"]["main"]["profile"]["model"], "codex-terra")
        self.assertEqual(cognition["roles"]["action_assistance"]["profile"], {"model": "codex-sol", "reasoning_effort": "low"})
        self.assertEqual(cognition["roles"]["body_reflex"]["implementation"], "deterministic_kernel")
        self.assertIn("cost", cognition["roles"]["main"])
        self.assertIn("cost", cognition["roles"]["action_assistance"])


class StageTenDeadlineTest(unittest.TestCase):
    def current(self, planned_end: str = "2026-08-30T01:00:00Z") -> dict:
        return {
            "t0": "2026-08-30T00:00:00Z",
            "planned_end": planned_end,
        }

    def test_deadline_extends_by_thirty_minutes_within_two_hours(self) -> None:
        observed = LAB.extended_planned_end(self.current(), 30)
        self.assertEqual(observed.isoformat(), "2026-08-30T01:30:00+00:00")

    def test_deadline_rejects_more_than_two_hours_from_t0(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "exceed two hours"):
            LAB.extended_planned_end(self.current("2026-08-30T01:30:00Z"), 60)

    def test_deadline_rejects_unbounded_increment(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "between 1 and 60"):
            LAB.extended_planned_end(self.current(), 61)


class StageTenBrowserSurfaceTest(unittest.TestCase):
    def test_chrome_autostart_exposes_x_and_public_web(self) -> None:
        entry = (MODULE_PATH.parent.parent / "deploy" / "desktop" / "chrome-autostart.desktop").read_text(
            encoding="utf-8"
        )
        self.assertIn("https://x.com/home", entry)
        self.assertIn("https://en.wikipedia.org/wiki/Main_Page", entry)

    def test_one_failed_duplicate_does_not_hide_a_ready_public_web_surface(self) -> None:
        tabs = """### Result
- 0: (current) [Wikipedia](chrome-error://chromewebdata/)
- 1: [Wikipedia, the free encyclopedia](https://en.wikipedia.org/wiki/Main_Page)
"""
        self.assertTrue(LAB.tabs_have_ready_public_web(tabs))

    def test_only_failed_public_web_surface_is_not_ready(self) -> None:
        tabs = "- 0: (current) [Wikipedia](chrome-error://chromewebdata/)"
        self.assertFalse(LAB.tabs_have_ready_public_web(tabs))

    def test_chrome_uses_clash_and_playwright_shares_the_context(self) -> None:
        root = MODULE_PATH.parent.parent
        chrome = (root / "ops" / "ubuntu" / "bin" / "hominal-chrome").read_text(encoding="utf-8")
        playwright = (root / "ops" / "ubuntu" / "bin" / "hominal-playwright-mcp").read_text(
            encoding="utf-8"
        )
        self.assertIn("--proxy-server=http://127.0.0.1:7897", chrome)
        self.assertIn("--shared-browser-context", playwright)

    def test_life_process_uses_the_same_clash_route(self) -> None:
        service = (MODULE_PATH.parent.parent / "deploy" / "hominal.service").read_text(encoding="utf-8")
        self.assertIn("HTTP_PROXY=http://127.0.0.1:7897", service)
        self.assertIn("HTTPS_PROXY=http://127.0.0.1:7897", service)
        self.assertIn("NO_PROXY=127.0.0.1,localhost,::1,192.168.124.161,ai.ai-mesh.cn", service)

    def test_stage_ten_seal_prepares_and_verifies_the_real_browser_body(self) -> None:
        current = {"instance_id": "g0r-example", "stage": 10}
        with mock.patch.object(LAB, "ensure_chrome_session") as ensure, mock.patch.object(
            LAB, "wait_for_runtime"
        ) as wait_runtime, mock.patch.object(
            LAB, "wait_for_browser_body"
        ) as wait_browser, mock.patch.object(
            LAB, "cmd_browser_check"
        ) as browser_check:
            LAB.prepare_generation_body_for_birth(current)

        ensure.assert_called_once_with()
        wait_runtime.assert_called_once_with("g0r-example")
        wait_browser.assert_called_once_with("g0r-example")
        browser_check.assert_called_once_with()

    def test_stage_ten_engineering_health_gate_does_not_race_the_live_browser_queue(self) -> None:
        current = {"instance_id": "g0s10-example", "stage": 10}
        with mock.patch.object(LAB, "ensure_chrome_session") as ensure, mock.patch.object(
            LAB, "wait_for_runtime"
        ) as wait_runtime, mock.patch.object(
            LAB, "wait_for_browser_body"
        ) as wait_browser, mock.patch.object(
            LAB, "cmd_browser_check"
        ) as browser_check:
            LAB.prepare_generation_body_for_birth(current, verify_public_surfaces=False)

        ensure.assert_called_once_with()
        wait_runtime.assert_called_once_with("g0s10-example")
        wait_browser.assert_called_once_with("g0s10-example")
        browser_check.assert_not_called()

    def test_earlier_stage_does_not_require_stage_ten_browser_surfaces(self) -> None:
        current = {"instance_id": "g0r-example", "stage": 9}
        with mock.patch.object(LAB, "ensure_chrome_session") as ensure, mock.patch.object(
            LAB, "wait_for_runtime"
        ) as wait_runtime, mock.patch.object(
            LAB, "wait_for_browser_body"
        ) as wait_browser, mock.patch.object(
            LAB, "cmd_browser_check"
        ) as browser_check:
            LAB.prepare_generation_body_for_birth(current)

        ensure.assert_called_once_with()
        wait_runtime.assert_called_once_with("g0r-example")
        wait_browser.assert_not_called()
        browser_check.assert_not_called()


class ModelPreflightTest(unittest.TestCase):
    def config(self) -> dict:
        return {
            "llm": {
                "provider": "OpenAI",
                "providers": {"OpenAI": {"base_url": "https://gateway.example"}},
                "models": {"terra": {"id": "gpt-5.6-terra"}},
                "runtime": {"initial_profile": {"model": "terra", "reasoning_effort": "none"}},
                "credentials": {"environment": {"OPENAI_API_KEY": "test-secret"}},
            }
        }

    def test_model_preflight_runs_on_the_body_without_putting_secret_in_command(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[], returncode=0,
            stdout='{"http_status":200,"requested_model":"gpt-5.6-terra","effective_model":"gpt-5.6-terra","response_id_present":true,"usage_present":true,"valid":true}\n',
            stderr="",
        )
        with mock.patch.dict(LAB.SETTINGS, {"config": self.config()}), mock.patch.object(
            LAB, "ssh", return_value=completed
        ) as remote:
            observed = LAB.verify_model_response()
        self.assertTrue(observed["valid"])
        command = remote.call_args.args[0]
        self.assertNotIn("test-secret", command)
        self.assertIn("test-secret", remote.call_args.kwargs["input_text"])

    def test_model_preflight_rejects_upstream_quota_failure(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[], returncode=2,
            stdout='{"http_status":429,"message":"quota exhausted","valid":false}\n',
            stderr="",
        )
        with mock.patch.dict(LAB.SETTINGS, {"config": self.config()}), mock.patch.object(
            LAB, "ssh", return_value=completed
        ):
            with self.assertRaisesRegex(RuntimeError, "HTTP 429: quota exhausted"):
                LAB.verify_model_response()

    def test_llmserver_preflight_requires_a_confirmed_bill(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[], returncode=3,
            stdout='{"http_status":200,"response_id_present":true,"usage_present":true,"billing_confirmed":false,"valid":false}\n',
            stderr="",
        )
        gateway = {
            "name": "llmserver", "adapter": "llmserver",
            "base_url": "http://192.168.124.161:4815", "api_" + "key": "example",
        }
        with mock.patch.dict(LAB.SETTINGS, {"config": self.config()}), mock.patch.object(
            LAB, "model_gateway_settings", return_value=gateway
        ), mock.patch.object(LAB, "ssh", return_value=completed):
            with self.assertRaisesRegex(RuntimeError, "invalid response"):
                LAB.verify_model_response()

    def test_llmserver_preflight_forces_one_native_function(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[], returncode=0,
            stdout='{"http_status":200,"response_id_present":true,"usage_present":true,"billing_confirmed":true,"function_calling_native":true,"valid":true}\n',
            stderr="",
        )
        gateway = {
            "name": "llmserver", "adapter": "llmserver",
            "base_url": "http://192.168.124.161:4815", "api_" + "key": "example",
        }
        with mock.patch.dict(LAB.SETTINGS, {"config": self.config()}), mock.patch.object(
            LAB, "model_gateway_settings", return_value=gateway
        ), mock.patch.object(LAB, "ssh", return_value=completed) as remote:
            observed = LAB.verify_model_response()
        self.assertTrue(observed["valid"])
        payload = json.loads(remote.call_args.kwargs["input_text"])
        body = payload["body"]
        self.assertEqual(body["tools"][0]["name"], "gateway_probe")
        self.assertTrue(body["tools"][0]["strict"])
        self.assertEqual(body["tool_choice"], {"type": "function", "name": "gateway_probe"})
        self.assertFalse(body["parallel_tool_calls"])


if __name__ == "__main__":
    unittest.main()
