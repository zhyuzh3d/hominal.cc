import importlib.util
import pathlib
import unittest

HERE=pathlib.Path(__file__).parent
def module(name):
    spec=importlib.util.spec_from_file_location(name,HERE/(name+'.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

class DesktopBoundaryTests(unittest.TestCase):
    def test_point_is_normalized_and_absence_is_explicit(self):
        v=module('vision_server')
        self.assertEqual(v.parse_point('{"found":false}'),{'found':False})
        self.assertEqual(v.parse_point('{"found":true,"x":500,"y":250}')['x'],500)
        self.assertEqual(v.parse_point('<tool_call>{"name":"computer_use","arguments":{"action":"left_click","coordinate":[500,250]}}</tool_call>')['y'],250)
        for invalid in ['{"x":1001,"y":100}','{"x":true,"y":100}','click 100 200','{"x":500,"y":500}','{"found":true,"x":281,793,"y":793}']:
            with self.assertRaises(ValueError):v.parse_point(invalid)

    def test_target_cannot_outlive_or_escape_its_scene(self):
        d=module('desktop');b={'created':10,'digest':'a','size':[100,100],'point':[20,30]}
        f={'digest':'a','width':100,'height':100}
        d.verify_binding(b,f,at=30)
        with self.assertRaises(ValueError):d.verify_binding(b,f,at=80)
        with self.assertRaises(ValueError):d.verify_binding(dict(b,consumed=True),f,at=30)
        with self.assertRaises(ValueError):d.verify_binding(b,dict(f,digest='b'),at=30)
        with self.assertRaises(ValueError):d.verify_binding(dict(b,point=[101,1]),f,at=30)

if __name__=='__main__':unittest.main()
