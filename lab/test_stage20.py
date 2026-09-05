import pathlib
import re
import tempfile
import unittest

import stage20


class Stage20ControlTests(unittest.TestCase):
    def test_runtime_roles_resolve_catalog_models_and_default(self):
        catalog={
            'luna':{'id':'codex-luna','supported_reasoning_efforts':['none']},
            'terra':{'id':'codex-terra','supported_reasoning_efforts':['none','low']},
            'sol':{'id':'codex-sol','supported_reasoning_efforts':['low']},
        }
        hominal={'startup_models':{
            'fast':{'catalog_key':'luna','reasoning_effort':'none'},
            'main':{'catalog_key':'terra','reasoning_effort':'none'},
            'high':{'catalog_key':'sol','reasoning_effort':'low'},
        },'default_role':'main'}

        models,default_profile=stage20.runtime_model_roles(catalog,hominal)

        self.assertEqual({role:value['id'] for role,value in models.items()},
                         {'fast':'codex-luna','main':'codex-terra','high':'codex-sol'})
        self.assertEqual(default_profile,{'model':'main','reasoning_effort':'none'})

    def test_runtime_roles_reject_missing_role(self):
        with self.assertRaisesRegex(ValueError,'fast, main and high'):
            stage20.runtime_model_roles({}, {'startup_models':{}})

    def test_parse_time_accepts_go_trimmed_and_nanosecond_rfc3339(self):
        expected='2026-09-04T19:17:13.576750+00:00'
        for value in (
            '2026-09-04T19:17:13.57675Z',
            '2026-09-04T19:17:13.576750Z',
            '2026-09-04T19:17:13.576750649Z',
            '2026-09-04T19:17:13.57675+00:00',
        ):
            with self.subTest(value=value):
                self.assertEqual(stage20.parse_time(value).isoformat(),expected)

    def test_parse_time_accepts_whole_seconds(self):
        self.assertEqual(stage20.parse_time('2026-09-04T19:17:13Z').isoformat(),
                         '2026-09-04T19:17:13+00:00')

    def test_commented_yaml_round_trips_and_comments_every_field(self):
        value={'device':{'name':'hominal-a1x','fixed_ipv4':True},
               'surfaces':[{'id':'workbench','supports':['exploration']}]}
        with tempfile.TemporaryDirectory() as directory:
            path=pathlib.Path(directory)/'config.yaml'
            stage20.write_yaml(path,value)
            self.assertEqual(stage20.read_yaml(path),value)
            lines=path.read_text().splitlines()
            for index,line in enumerate(lines):
                if re.match(r'^\s*(?:- )?[A-Za-z0-9_-]+:',line):
                    self.assertGreater(index,0)
                    self.assertRegex(lines[index-1],r'^\s*# ')
            self.assertEqual(path.stat().st_mode & 0o777,0o600)


if __name__ == '__main__':
    unittest.main()
