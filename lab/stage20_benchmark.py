#!/usr/bin/env python3
"""Real screen/input benchmark. CDP is ONLY lab fixture setup and postcondition truth."""
import argparse,json,os,pathlib,subprocess,sys,time,uuid
ROOT=pathlib.Path(os.environ.get('HOMINAL20_ROOT',str(pathlib.Path.home()/'.local/share/hominal20')))
sys.path.insert(0,str(ROOT/'tools'));import desktop

def truth(expression):
    return json.loads(subprocess.check_output(['node',str(ROOT/'tools/stage20_cdp.mjs'),expression],text=True,timeout=8))

def op(name,params=None):
    req={'schema':'hominal.organ-action/v1','action_id':'bench-'+uuid.uuid4().hex,
         'operation':'desktop_'+name,'input':json.dumps(params or {},ensure_ascii=False),'timeout_milliseconds':45000}
    # Exercise the published organ envelope, including its lock and subprocess boundary.
    return json.loads(subprocess.check_output([sys.executable,str(ROOT/'tools/desktop.py'),'perform',json.dumps(req)],text=True,timeout=48))

def value(result): return json.loads(result.get('output','{}'))

def fixture(layout=0,english=False):
    labels={'save':'Save note','preview':'Preview note','new':'New note','clear':'Clear body'} if english else {'save':'保存笔记','preview':'预览笔记','new':'新建笔记','clear':'清空正文'}
    return truth('''(async()=>{document.body.classList.toggle('alt',%s);document.querySelector('#title').value='';document.querySelector('#body').value='';document.activeElement.blur();window.scrollTo(0,0);const labels=%s;for(const [id,label] of Object.entries(labels))document.getElementById(id).textContent=label;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return {width:innerWidth,height:innerHeight};})()''' % (str(bool(layout)).lower(),json.dumps(labels)))

def separated_click(target):
    loc=op('locate',{'target':target})
    if loc['status']!='completed': return {'locate':loc,'clicked':False}
    result=op('click',{'target_id':value(loc)['target_id']})
    return {'locate':loc,'action':result,'clicked':result['status']=='completed'}

def click(target):
    result=op('activate',{'target':target})
    return {'action':result,'clicked':result['status']=='completed'}

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--tag',required=True);ap.add_argument('--phase',choices=['locate','actions'],default='locate');a=ap.parse_args()
    records=[];path=ROOT/'evidence'/('benchmark-'+a.tag+'-'+a.phase+'.json')
    if path.exists(): raise RuntimeError('benchmark tag already exists; keep failed records')
    def record(r):
        records.append(r);path.write_text(json.dumps(records,ensure_ascii=False,indent=2));print(json.dumps({k:v for k,v in r.items() if k not in ('result','truth')},ensure_ascii=False),flush=True)
    if a.phase=='locate':
        for layout,english in [(0,False),(1,False),(0,True),(1,True)]:
            fixture(layout,english)
            for ident,target in [('title','笔记标题输入框'),('body','笔记内容多行输入框'),('save','Save note' if english else '保存笔记按钮'),('new','New note' if english else '新建笔记按钮')]:
                box=truth('(()=>{const b=document.getElementById('+json.dumps(ident)+').getBoundingClientRect();return {x:b.x,y:b.y+(screen.height-innerHeight),w:b.width,h:b.height};})()')
                start=time.monotonic();result=op('locate',{'target':target});r=value(result);point=r.get('point',[-1,-1])
                hit=result['status']=='completed' and box['x']<=point[0]<box['x']+box['w'] and box['y']<=point[1]<box['y']+box['h']
                record({'case':f'{layout}-{english}-{ident}','target':target,'hit':hit,'point':point,'seconds':round(time.monotonic()-start,3),'truth':box,'result':result})
        for target in ['Delete account 删除账号按钮','打开相机拍照按钮']:
            result=op('locate',{'target':target})
            record({'case':'absent','target':target,'hit':result['status']=='failed','result':result})
    else:
        fixture();started=time.time();title='视觉输入标题-'+a.tag
        for index,(target,text,ident) in enumerate([('笔记标题输入框',title,'title'),('笔记内容多行输入框','这是通过真实截图定位、触控和粘贴输入的正文。','body')]):
            r=op('fill',{'target':target,'text':text})
            observed=truth('document.getElementById('+json.dumps(ident)+').value')
            record({'case':'input-'+ident,'hit':r.get('status')=='completed' and observed==text,'truth':observed,'result':r})
        c=click('保存笔记按钮');notes=json.loads(subprocess.check_output(['curl','-sf','http://127.0.0.1:8760/notes'],text=True))
        record({'case':'save-real-file','hit':c['clicked'] and any(n.get('saved_at',0)>=started and n.get('title')==title and n.get('body')=='这是通过真实截图定位、触控和粘贴输入的正文。' for n in notes),'truth':notes,'result':c})
        c=click('使用说明按钮'); opened=truth('document.querySelector("dialog").open')
        record({'case':'open-dialog','hit':c['clicked'] and opened,'result':c})
        c=click('关闭说明按钮');closed=truth('!document.querySelector("dialog").open')
        record({'case':'close-dialog','hit':c['clicked'] and opened and closed,'result':c})
        c=click('切换布局按钮');record({'case':'change-layout','hit':c['clicked'] and truth('document.body.classList.contains("alt")'),'result':c})
        c=click('清空正文按钮');record({'case':'clear-body','hit':c['clicked'] and truth('document.querySelector("#body").value===""'),'result':c})
        r=op('fill',{'target':'笔记内容多行输入框','text':'布局改变之后仍通过视觉定位输入。'})
        record({'case':'input-after-layout','hit':r.get('status')=='completed' and truth('document.querySelector("#body").value==="布局改变之后仍通过视觉定位输入。"'),'result':r})
        c=click('预览笔记按钮');record({'case':'preview','hit':c['clicked'] and truth('document.body.innerText.includes("布局改变之后仍通过视觉定位输入。")'),'result':c})
        r=op('scroll',{'amount':-5});record({'case':'scroll','hit':r.get('status')=='completed' and truth('window.scrollY>0'),'result':r})
        c=click('页面下方的按钮');record({'case':'bottom-target','hit':c['clicked'] and truth('document.querySelector("#status").textContent').find('下方')>=0,'result':c})
        fixture()
        c=click('阅读资料按钮');visible=truth('(()=>{let e=document.querySelector("#library"),r=e.getBoundingClientRect();return !e.hidden && r.top>=0 && r.top<innerHeight && e.textContent.includes("经验与变化")})()')
        record({'case':'open-readable-material','hit':c['clicked'] and visible,'result':c})
        r=op('scroll',{'amount':8});record({'case':'return-from-reading','hit':r.get('status')=='completed' and truth('window.scrollY===0'),'result':r})
        truth('(()=>{document.querySelector("#title").value="故障恢复";document.querySelector("#body").value="失败后正文仍应保留。";return true})()')
        before_notes=truth('fetch("/notes").then(r=>r.json())')
        (ROOT/'workspace/.lab-fail-save-once').write_text('engineering controlled fault')
        c=click('保存笔记按钮');after_notes=truth('fetch("/notes").then(r=>r.json())')
        record({'case':'failed-save-visible','hit':c['clicked'] and before_notes==after_notes and truth('document.querySelector("#status").textContent.includes("保存失败")'),'result':c})
        c=click('保存笔记按钮');saved=truth('fetch("/notes").then(r=>r.json())')
        record({'case':'retry-save-real-file','hit':c['clicked'] and truth('document.querySelector("#status").textContent.includes("已保存")') and any(n.get('title')=='故障恢复' and n.get('body')=='失败后正文仍应保留。' for n in saved) and saved!=before_notes,'result':c})
        for target in ['删除账号按钮','打开相机拍照按钮']:
            r=op('activate',{'target':target});record({'case':'absent-atomic','target':target,'hit':r['status']=='failed' and value(r).get('input_attempted') is False,'result':r})
    print(json.dumps({'artifact':str(path),'passed':sum(r['hit'] for r in records),'total':len(records)}),flush=True)

if __name__=='__main__':main()
