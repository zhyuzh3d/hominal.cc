#!/usr/bin/env python3
"""Live, non-enacting rejection tests plus moved-window visual grounding."""
import json,os,pathlib,subprocess,sys,time
from stage20_benchmark import ROOT,truth,op,value,fixture
import desktop

def main():
    records=[]
    fixture()
    def check(name,result):
        record={'case':name,'passed':result['status']=='failed' and not value(result).get('input_attempted',True),'result':result}
        records.append(record);print(json.dumps({k:v for k,v in record.items() if k!='result'}),flush=True)
    for name,change in [('expired',lambda b:b.update(created=time.time()-61)),('bounds',lambda b:b.update(point=[-1,0])),('consumed',lambda b:b.update(consumed=True))]:
        r=op('locate',{'target':'笔记标题输入框'});b=value(r)
        if r['status']!='completed':raise RuntimeError('probe could not establish a valid target')
        change(b);(desktop.EVIDENCE/(b['target_id']+'.json')).write_text(json.dumps(b))
        check(name,op('click',{'target_id':b['target_id']}))
    r=op('locate',{'target':'保存笔记按钮'});b=value(r)
    truth('document.body.classList.toggle("alt");true')
    check('changed-scene',op('click',{'target_id':b['target_id']}))
    fixture()
    # The model sees the actual whole screenshot. Bounding boxes stay in this Lab process.
    for i,window in enumerate([{'left':30,'top':30,'width':1100,'height':710},{'left':150,'top':65,'width':950,'height':680}]):
        subprocess.run(['/usr/bin/python3',str(ROOT/'tools/stage20_window.py'),'--rect',json.dumps([window[k] for k in ('left','top','width','height')])],check=True,capture_output=True)
        truth('new Promise(r=>setTimeout(()=>r(true),250))')
        geometry=json.loads(subprocess.check_output(['/usr/bin/python3',str(ROOT/'tools/stage20_window.py')],text=True))['frame']
        for ident,target in [('title','工作室里的笔记标题输入框'),('body','工作室里的笔记内容输入框'),('new','工作室里的新建笔记按钮')]:
            box=truth('(()=>{let b=document.querySelector("#'+ident+'").getBoundingClientRect();const g='+json.dumps(geometry)+';return {x:b.x+g[0],y:b.y+g[1]+g[3]-innerHeight,w:b.width,h:b.height};})()')
            r=op('locate',{'target':target});p=value(r).get('point',[-1,-1])
            passed=r['status']=='completed' and box['x']<=p[0]<box['x']+box['w'] and box['y']<=p[1]<box['y']+box['h']
            records.append({'case':'window-'+str(i)+'-'+ident,'passed':passed,'point':p,'truth':box,'result':r})
            print(json.dumps({'case':records[-1]['case'],'passed':passed,'point':p,'truth':box}),flush=True)
    subprocess.run(['node',str(ROOT/'tools/stage20_cdp.mjs'),'fullscreen'],check=True,capture_output=True)
    p=ROOT/'evidence/boundaries-v2.json';p.write_text(json.dumps(records,ensure_ascii=False,indent=2))
    print(json.dumps({'passed':sum(r['passed'] for r in records),'total':len(records),'artifact':str(p)}))

if __name__=='__main__':main()
