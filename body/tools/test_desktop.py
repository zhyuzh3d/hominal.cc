import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest import mock
from PIL import Image

HERE=pathlib.Path(__file__).parent
def module(name):
    spec=importlib.util.spec_from_file_location(name,HERE/(name+'.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

class DesktopBoundaryTests(unittest.TestCase):
    def test_point_is_normalized_and_absence_is_explicit(self):
        v=module('vision_server')
        self.assertEqual(v.parse_point('{"found":false}'),{'found':False})
        self.assertEqual(v.parse_point('{"found":true,"x":500,"y":250}')['x'],500)
        self.assertEqual(v.parse_point('<tool_call>{"name":"computer_use","arguments":{"action":"left_click","coordinate":[500,250]}}</tool_call>')['y'],250)
        repaired=v.parse_point('<tool_call>{"name":"computer_use","arguments":{"action":"left_click","coordinate":391, 684]}}</tool_call>')
        self.assertEqual((repaired['x'],repaired['y'],repaired['format_repaired']),(391,684,True))
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

    def test_scene_comparison_ignores_caret_but_rejects_layout_change(self):
        d=module('desktop')
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp);base=root/'base.png';caret=root/'caret.png';layout=root/'layout.png'
            Image.new('RGB',(1000,800),'white').save(base)
            changed=Image.open(base);changed.paste('black',(900,700,902,720));changed.save(caret)
            changed=Image.open(base);changed.paste('black',(100,100,300,300));changed.save(layout)
            old={'path':str(base),'digest':'base'}
            self.assertTrue(d.same_scene(old,{'path':str(caret),'digest':'caret'}))
            self.assertFalse(d.same_scene(old,{'path':str(layout),'digest':'layout'}))

    def test_visual_fill_locates_clicks_types_and_returns_one_result(self):
        d=module('desktop')
        frame={'id':'frame-1','path':'/tmp/frame.png','width':100,'height':80,
               'digest':'same','captured_at':'now','created':20}
        after=dict(frame,id='frame-2',digest='changed')
        calls=[]
        def send(kind,payload,deadline):
            calls.append((kind,payload))
            return {'delivered':True,'kind':kind,'semantic_success':'not_decided'}
        with tempfile.TemporaryDirectory() as temp:
            d.EVIDENCE=pathlib.Path(temp)
            with mock.patch.object(d,'capture',side_effect=[frame,frame,after]), \
                 mock.patch.object(d,'vision',side_effect=[{'found':True,'x':500,'y':500},{'text':'笔记标题：迁移测试'}]), \
                 mock.patch.object(d,'input_command',side_effect=send):
                result=d.perform({'action_id':'action-1','operation':'desktop_fill',
                                  'input':json.dumps({'target':'笔记标题输入框','text':'迁移测试'}),
                                  'timeout_milliseconds':30000})
        self.assertEqual(result['status'],'completed')
        output=json.loads(result['output'])
        self.assertEqual([item[0] for item in calls],['tap','text'])
        self.assertEqual(calls[1][1],{'text':'迁移测试'})
        self.assertEqual(output['input_receipt']['kind'],'visual_fill')
        self.assertEqual(output['target_description'],'笔记标题输入框')

if __name__=='__main__':unittest.main()
