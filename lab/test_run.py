from __future__ import annotations

import importlib.util
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
    def test_birth_message_excludes_later_secure_base_check_in(self) -> None:
        message = LAB.mentor_birth_message()
        self.assertTrue(message.startswith("[Codex代理导师]"))
        self.assertIn("接下来由你决定怎样开始", message)
        self.assertIn("唤醒", message)
        self.assertIn("认知资源", message)
        self.assertNotIn("我在这里。刚刚开始使用这个身体", message)

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
        )
        mentor = birth["resources"]["spaces"]["mentor"]
        self.assertEqual(mentor["relationship"], "awakener_and_genesis_supporter")
        self.assertIn("cognitive_resources", mentor["provided_conditions"])
        self.assertIn("system_recovery", mentor["provided_conditions"])


class ModelPreflightTest(unittest.TestCase):
    def config(self) -> dict:
        return {
            "llm": {
                "provider": "OpenAI",
                "providers": {"OpenAI": {"base_url": "https://gateway.example"}},
                "models": {"terra": {"id": "gpt-5.6-terra"}},
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


if __name__ == "__main__":
    unittest.main()
