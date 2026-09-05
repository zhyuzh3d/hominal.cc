import pathlib
import re
import tempfile
import unittest

import stage20


class Stage20ControlTests(unittest.TestCase):
    def test_automation_browser_is_pinned_to_the_integrated_radeon(self):
        self.assertEqual(stage20.AMD_RENDER_NODE,
                         '/dev/dri/by-path/pci-0000:c6:00.0-render')
        self.assertEqual(stage20.BROWSER_GPU_ENV['DRI_PRIME'],'pci-0000_c6_00_0')
        self.assertEqual(stage20.BROWSER_GPU_ENV['__GLX_VENDOR_LIBRARY_NAME'],'mesa')
        self.assertEqual(stage20.BROWSER_GPU_ENV['MESA_VK_DEVICE_SELECT'],'1002:150e!')
        self.assertEqual(stage20.BROWSER_GPU_ENV['CUDA_VISIBLE_DEVICES'],'-1')
        self.assertEqual(stage20.VISION_GPU_ENV['CUDA_VISIBLE_DEVICES'],'0')

    def test_runtime_roles_resolve_catalog_models_and_default(self):
        catalog={
            'codex-luna':{'supported_reasoning_efforts':['none','low']},
            'codex-terra':{'supported_reasoning_efforts':['none','low','high']},
            'codex-sol':{'supported_reasoning_efforts':['none','low','high']},
        }
        hominal={'startup_models':{
            'fast':{'model':'codex-luna','reasoning_effort':'low'},
            'main':{'model':'codex-terra','reasoning_effort':'none'},
            'high':{'model':'codex-sol','reasoning_effort':'high'},
        },'default_role':'main'}

        models,profiles,default_profile=stage20.runtime_model_roles(catalog,hominal,set(catalog))

        self.assertEqual({role:value['id'] for role,value in models.items()},
                         {'fast':'codex-luna','main':'codex-terra','high':'codex-sol'})
        self.assertEqual(profiles,{
            'fast':{'model':'fast','reasoning_effort':'low'},
            'main':{'model':'main','reasoning_effort':'none'},
            'high':{'model':'high','reasoning_effort':'high'},
        })
        self.assertEqual(default_profile,{'model':'main','reasoning_effort':'none'})

    def test_runtime_roles_reject_missing_role(self):
        with self.assertRaisesRegex(ValueError,'fast, main and high'):
            stage20.runtime_model_roles({}, {'startup_models':{}})

    def test_runtime_roles_reject_model_missing_from_llmserver(self):
        catalog={'codex-luna':{'supported_reasoning_efforts':['none']}}
        hominal={'startup_models':{
            role:{'model':'codex-luna','reasoning_effort':'none'} for role in ('fast','main','high')
        },'default_role':'main'}
        with self.assertRaisesRegex(ValueError,'llmserver model is unavailable'):
            stage20.runtime_model_roles(catalog,hominal,set())

    def test_runtime_roles_reject_unsupported_effort(self):
        catalog={'codex-luna':{'supported_reasoning_efforts':['none']}}
        hominal={'startup_models':{
            role:{'model':'codex-luna','reasoning_effort':'none'} for role in ('fast','main','high')
        },'default_role':'main'}
        hominal['startup_models']['fast']['reasoning_effort']='ultra'
        with self.assertRaisesRegex(ValueError,'unsupported reasoning effort for fast'):
            stage20.runtime_model_roles(catalog,hominal,{'codex-luna'})

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
