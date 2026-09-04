#!/usr/bin/env python3
"""Read actual captured screens; all expected text stays in Lab, outside the vision request."""
import argparse,json,os,pathlib,re,time,urllib.request,unicodedata
ROOT=pathlib.Path(os.environ['HOMINAL20_ROOT'])
p=argparse.ArgumentParser();p.add_argument('--tag',required=True);a=p.parse_args()
output=ROOT/'evidence'/('reading-'+a.tag+'.json')
if output.exists():raise RuntimeError('preserve previous reading evidence')
records=json.loads((ROOT/'evidence/benchmark-owl-v5-actions.json').read_text());by={x['case']:x for x in records}
def action(name,typed=False):
 r=by[name]['result'];return json.loads((r['type'] if typed else r['action'])['output'])
cases=[('blank',action('input-title',True)['before'],['一页空白','为这页内容起个名字']),
 ('title',action('input-title',True)['after'],['视觉输入标题-owl-v5']),
 ('body',action('input-body',True)['after'],['这是通过真实截图定位','触控和粘贴输入的正文']),
 ('saved',action('save-real-file')['after'],['已保存']),
 ('help',action('open-dialog')['after'],['布局变化不会改变笔记内容']),
 ('reading',action('open-readable-material')['after'],['学习一个操作','窗口移动以后','相同方法在不同情境里可能需要调整']),
 ('failed-save',action('failed-save-visible')['after'],['保存失败','尚未写入文件']),
 ('changed-layout',action('input-after-layout',True)['after'],['布局改变之后仍通过视觉定位输入'])]
def normalized(s):return re.sub(r'\s+','',unicodedata.normalize('NFKC',s))
rows=[]
for name,frame,expected in cases:
 request=urllib.request.Request('http://127.0.0.1:8765/interpret',json.dumps({'image':frame['path'],'mode':'describe','deadline':time.time()+50}).encode(),{'Content-Type':'application/json'})
 with urllib.request.urlopen(request,timeout=55) as response:r=json.load(response)
 matches=[normalized(s) in normalized(r['text']) for s in expected]
 row={'case':name,'image':frame['path'],'expected':expected,'matched':matches,'passed':all(matches) and not r.get('output_truncated'),'result':r};rows.append(row)
 output.write_text(json.dumps(rows,ensure_ascii=False,indent=2));print(json.dumps({'case':name,'passed':row['passed'],'seconds':r['seconds'],'text':r['text']},ensure_ascii=False),flush=True)
print(json.dumps({'passed':sum(x['passed'] for x in rows),'total':len(rows),'artifact':str(output)}),flush=True)
