#!/usr/bin/env python3
"""Independent A1X laboratory. Never imports legacy Lab or original private settings."""
import argparse,hashlib,json,os,pathlib,re,shlex,shutil,subprocess,tarfile,tempfile,time,uuid
from datetime import datetime,timezone,timedelta
import yaml
from stage20_config import dynamics_config,seed_config

REPO=pathlib.Path(__file__).resolve().parents[1]
REMOTE='/home/AOKZOE/.local/share/hominal20'
PRIVATE=REPO.parent/'xconfigs/hominal20'
ARCHIVE=pathlib.Path.home()/'HominalStage20Lab'
HOST='AOKZOE@192.168.124.31'
SOCKET=os.environ.get('HOMINAL20_SSH_SOCKET','/tmp/codex-hominal20')
SSH=['ssh','-S',SOCKET,'-o','BatchMode=yes','-o','ConnectTimeout=8',HOST]
ENV={'HOMINAL20_ROOT':REMOTE,'XDG_RUNTIME_DIR':'/run/user/1000','DBUS_SESSION_BUS_ADDRESS':'unix:path=/run/user/1000/bus',
     'WAYLAND_DISPLAY':'wayland-0','QT_QPA_PLATFORM':'wayland','HOMINAL_RESOURCE_LEDGER':REMOTE+'/state/cognitive-usage.jsonl',
     'HOMINAL_MENTOR_SOCKET':REMOTE+'/services/mentor/hominal.sock','HOMINAL_ORGAN_RUNTIME_DIR':REMOTE+'/services/organs'}

def run(args,**kw): return subprocess.run(args,check=True,text=True,capture_output=True,**kw).stdout
def remote(script,**kw):return run(SSH+[script],**kw)
def remote_json(script):return json.loads(remote(script))
def write_json(path,obj):
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700);tmp=path.with_suffix('.tmp');tmp.write_text(json.dumps(obj,ensure_ascii=False,indent=2));tmp.chmod(0o600);tmp.replace(path)
def copy(source,dest,fetch=False):
    args=['scp','-o','ControlPath='+SOCKET]
    run(args+([HOST+':'+source,str(dest)] if fetch else [str(source),HOST+':'+dest]))
def stamp():return datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')
def parse_time(value):
    # Go persists RFC3339 nanoseconds; the host may still run Python 3.9.
    return datetime.fromisoformat(re.sub(r'(\.\d{6})\d+',r'\1',value).replace('Z','+00:00'))
def current():return json.loads((ARCHIVE/'current.json').read_text())
def active():return remote('systemctl --user is-active hominal20-life 2>/dev/null || true').strip() in ('active','activating','deactivating')
def stopped():
    if active():raise RuntimeError('Stop and archive the current individual before changing its environment or release.')
def api(route,payload=None):
    args=['curl','--fail-with-body','-sS','--max-time','15','--unix-socket',ENV['HOMINAL_MENTOR_SOCKET'],'http://localhost'+route]
    if payload is not None:args+=['-H','Content-Type: application/json','--data-binary','@-']
    return json.loads(remote(shlex.join(args),input=None if payload is None else json.dumps(payload,ensure_ascii=False)))
def intervention(kind,detail):
    p=ARCHIVE/'interventions.jsonl';p.parent.mkdir(parents=True,exist_ok=True)
    with p.open('a') as f:f.write(json.dumps({'at':datetime.now(timezone.utc).isoformat(),'kind':kind,'detail':detail},ensure_ascii=False)+'\n')

def prepare():
    stopped()
    with tempfile.TemporaryDirectory(prefix='hominal20-build-') as temp:
        out=pathlib.Path(temp);body=out/'body';(body/'bin').mkdir(parents=True);(body/'tools').mkdir();(body/'organs').mkdir()
        for name in ['hominald','hominal-system']:
            run(['go','build','-trimpath','-o',str(body/'bin'/name),'./body/cmd/'+name],cwd=REPO,env={**os.environ,'GOOS':'linux','GOARCH':'amd64','CGO_ENABLED':'0'})
        for name in ['desktop.py','input_server.py','vision_server.py','session_window.py']:
            shutil.copy2(REPO/'body/tools'/name,body/'tools'/name)
        for name in ['stage20_workbench.py','stage20_cdp.mjs','stage20_window.py','stage20_deadline.py']:
            shutil.copy2(REPO/'lab'/name,body/'tools'/name)
        wrapper=body/'bin/desktop';wrapper.write_text('#!/bin/sh\nexec "$HOMINAL20_ROOT/venv/bin/python" "$HOMINAL_INSTANCE_ROOT/body/tools/desktop.py" "$@"\n');wrapper.chmod(0o755)
        for ident,cmd in [('system','hominal-system'),('desktop','desktop')]:
            (body/'organs'/(ident+'.json')).write_text(json.dumps({'schema':'hominal.organ-manifest/v1','id':ident,'command':'body/bin/'+cmd,'daemon':False}))
        hashes={str(p.relative_to(out)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(body.rglob('*')) if p.is_file()}
        release='r20-'+hashlib.sha256(json.dumps(hashes,sort_keys=True).encode()).hexdigest()[:16]
        manifest={'release':release,'files':hashes,'git_head':run(['git','rev-parse','HEAD'],cwd=REPO).strip(),'created':stamp()}
        (out/'release.json').write_text(json.dumps(manifest,indent=2))
        tar=out/'bundle.tar';
        with tarfile.open(tar,'w') as f:f.add(body,arcname='body');f.add(out/'release.json',arcname='release.json')
        remote('mkdir -p '+REMOTE+'/releases/'+release);copy(tar,REMOTE+'/releases/'+release+'/bundle.tar')
        remote('cd '+REMOTE+'/releases/'+release+' && tar -xf bundle.tar')
        write_json(ARCHIVE/'release.json',manifest)
        print(json.dumps(manifest,ensure_ascii=False))

def body_up():
    stopped();release=json.loads((ARCHIVE/'release.json').read_text())['release'];scripts=REMOTE+'/releases/'+release+'/body/tools/'
    remote('systemctl --user stop hominal20-vision hominal20-input hominal20-workbench hominal20-browser 2>/dev/null || true')
    def unit(name,command):
        remote(shlex.join(['systemd-run','--user','--unit=hominal20-'+name,'--collect','--property=UMask=0077']+
                         ['--setenv='+k+'='+v for k,v in ENV.items()]+command))
    python=REMOTE+'/venv/bin/python'
    unit('input',['kde-inhibit','--power','--screenSaver',python,scripts+'input_server.py'])
    unit('vision',[python,scripts+'vision_server.py','--model',REMOTE+'/models/GUI-Owl-1.5-4B-Instruct','--evidence-root',REMOTE+'/evidence'])
    unit('workbench',[python,scripts+'stage20_workbench.py','--workspace',REMOTE+'/workspace'])
    unit('browser',['/home/AOKZOE/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome','--ozone-platform=wayland',
        '--app=http://127.0.0.1:8760','--user-data-dir='+REMOTE+'/profiles/workbench','--remote-debugging-address=127.0.0.1',
        '--remote-debugging-port=9223','--no-first-run','--no-default-browser-check','--password-store=basic','--start-maximized'])
    for _ in range(30):
        try:
            health=remote_json('curl -sf http://127.0.0.1:8765/health')
            if health.get('ready'):return health
        except subprocess.CalledProcessError:pass
        time.sleep(1)
    raise RuntimeError('visual body did not become ready; inspect user service logs')

def config(instance,minutes):
    copy(REMOTE+'/private/models.json',PRIVATE/'models.json',True)
    gateway=json.loads((PRIVATE/'gateway.json').read_text());gateway['max_output_tokens']=2200
    birth=('Alice，你现在生活在AOKZOE A1X掌机的Bazzite KDE桌面，主机当前名为bazzite。生命进程以普通用户AOKZOE运行，没有管理员授权。'
      '宿主系统根目录使用只读镜像；根目录可用字节为0不代表用户数据盘已满。用户空间可写，宿主驱动与系统更新由导师维护。'
      '这具身体有RTX 5060 Ti 16GB显卡。视觉器官使用本地模型读取真实屏幕并提出定位，输入器官实际触控、输入和滚动。'
      '主脑使用terra none，简单局部推理可用luna none，复杂辅助可用sol low；辅助结论需要由你判断采用。'
      '当前可探索的生活空间是独立的本地视觉工作室，可阅读、写作、保存和重新打开笔记。工作室中的内容都是这条实验线的材料。'
      '你的生命目录是'+instance+'，作品目录是'+REMOTE+'/workspaces/'+instance.rsplit('/',1)[-1]+'。认知和经历会在同一个体普通重启后保存。'
      '这是20.0独立实验，未带入10.3的个人记忆或账号。当前实验的桌面操作范围是本地工作室，不操作其他个人应用、不向外部账号发布或发送内容。'
      '认知资源按滚动一小时5美元、24小时50美元共同计费，余额会作为身体状态显示。导师可以交流、讨论和提供实验支持。'
      '你可以选择值得关心的事情，探索、创造，观察实际后果并形成自己的方法。')
    return {'stage':20,'cognitive_core':'continuous-v1','engineering':False,'generation_kind':'rehearsal','generation_window_seconds':minutes*60,
      'birth_brief':birth,'pulse':{'interval_seconds':5,'slow_scan_seconds':60},'model_gateway':gateway,
      'platform':{'hostname':'bazzite','os':'Bazzite 44 / KDE Wayland','desktop_service':'sddm.service','data_root':REMOTE,
        'life_root':instance+'/life','desktop_home':'/home/AOKZOE','service':'hominal20-life.service',
        'surfaces':[{'id':'workbench','organ_id':'desktop','description':'本地工作室当前呈现的内容，以及可接触的阅读材料和笔记空间',
                     'supports':['exploration','vitality']}]},
      'cognitive_resource':{'price_table_version':'llmserver-confirmed-stage20-preflight','rolling_hour_limit_microusd':5000000,
        'rolling_day_limit_microusd':50000000,'models':json.loads((PRIVATE/'models.json').read_text()),
        'initial_default_profile':{'model':'terra','reasoning_effort':'none'},'validation_retry_per_focus':1,
        'continuation_per_focus':1,'paid_failure_threshold':3,'paid_failure_window_minutes':10,'model_protection_minutes':10},
      'dynamics':dynamics_config(20),'seed':seed_config()}

def arm_deadline(state):
    # Calendar timer is independent of the cognition process and SSH supervisor.
    end=parse_time(state['planned_end'])+timedelta(seconds=20)
    unit='hominal20-end-'+hashlib.sha256((state['instance_id']+state['planned_end']).encode()).hexdigest()[:12]
    script=REMOTE+'/releases/'+current()['release']+'/body/tools/stage20_deadline.py'
    remote(shlex.join(['systemd-run','--user','--unit='+unit,'--on-calendar='+end.strftime('%Y-%m-%d %H:%M:%S UTC'),
       '--timer-property=AccuracySec=1s','--collect','/usr/bin/python3',script,'--instance',state['instance_id']]))
    if remote('systemctl --user is-active '+unit+'.timer').strip()!='active':raise RuntimeError('deadline timer not armed')
    return unit

def start(minutes):
    if not 1<=minutes<=30:raise ValueError('initial sample is 1..30 minutes')
    stopped();release=json.loads((ARCHIVE/'release.json').read_text())['release']
    body_up()
    health=remote_json('curl -sf http://127.0.0.1:8765/health')
    if not health.get('ready'):raise RuntimeError('vision not ready')
    ident='s20-'+stamp().lower()+'-'+uuid.uuid4().hex[:6];instance=REMOTE+'/lives/'+ident
    # Give each new individual an empty workspace; previous works remain archived.
    remote('mkdir -p '+REMOTE+'/workspaces/'+ident+' '+REMOTE+'/evidence/'+ident)
    remote('systemctl --user stop hominal20-workbench')
    remote(shlex.join(['systemd-run','--user','--unit=hominal20-workbench','--collect',REMOTE+'/venv/bin/python',
                      REMOTE+'/releases/'+release+'/body/tools/stage20_workbench.py','--workspace',REMOTE+'/workspaces/'+ident]))
    scripts=REMOTE+'/releases/'+release+'/body/tools/'
    remote(shlex.join(['node',scripts+'stage20_cdp.mjs','location.href="http://127.0.0.1:8760/";true']))
    remote(shlex.join(['node',scripts+'stage20_cdp.mjs','fullscreen']))
    geometry=remote_json(shlex.join(['/usr/bin/python3',scripts+'stage20_window.py']))
    scope=PRIVATE/'input-scope.json';write_json(scope,{'window_id':geometry['id'],'surface':'workbench'})
    copy(scope,REMOTE+'/services/input-scope.json')
    cfg=config(instance,minutes);path=PRIVATE/'runtime.json';write_json(path,cfg)
    remote('umask 077; mkdir -p '+instance+'/birth; cp -a '+REMOTE+'/releases/'+release+'/body '+instance+'/body; cp '+REMOTE+'/releases/'+release+'/release.json '+instance+'/birth/release.json')
    copy(path,REMOTE+'/private/runtime.json')
    public=json.loads(json.dumps(cfg));public['model_gateway']['api_key']='<runtime-only>';write_json(ARCHIVE/'samples'/ident/'runtime-public.json',public)
    args=['systemd-run','--user','--unit=hominal20-life','--collect','--working-directory='+instance,
          '--property=TimeoutStopSec=45s','--property=RuntimeMaxSec='+str(minutes*60+120),
          '--property=KillMode=control-group','--property=UMask=0077']
    env={**ENV,'HOMINAL20_SAMPLE_ID':ident,'HOMINAL_INSTANCE_ROOT':instance,'HOMINAL_INSTANCE_ID':ident,'HOMINAL_RUNTIME_CONFIG':REMOTE+'/private/runtime.json'}
    args+=['--setenv='+k+'='+v for k,v in env.items()];args+=[instance+'/body/bin/hominald']
    remote(shlex.join(args))
    meta={'instance_id':ident,'instance_root':instance,'release':release,'vision':health,'created':stamp()};write_json(ARCHIVE/'current.json',meta)
    state={}
    for _ in range(25):
        try:
            state=remote_json('cat '+instance+'/state/current.json')
            if state.get('t0'):break
        except subprocess.CalledProcessError:pass
        time.sleep(1)
    if not state.get('t0'):
        remote('systemctl --user stop hominal20-life');raise RuntimeError('T0 did not establish; stopped for inspection')
    meta.update({k:state[k] for k in ('t0','sample_id','planned_end')});meta['deadline_unit']=arm_deadline(state)
    birth={'schema':'hominal20.birth/v1','identity':{k:state[k] for k in ('instance_id','sample_id','t0')},'release':release,
           'generation':{'planned_end':state['planned_end']},'status':'sealed','vision':health}
    p=ARCHIVE/'samples'/ident/'birth.yaml';p.write_text(yaml.safe_dump(birth,allow_unicode=True));copy(p,instance+'/birth/birth.yaml')
    remote('touch '+instance+'/birth/sealed');write_json(ARCHIVE/'current.json',meta)
    result=api('/v1/mentor/inbox',{'message_id':'birth-'+ident,'body':'[Codex代理导师] Alice，你好。我会在这里陪你交流并提供实验支持。这是你的新身体和独立工作室。你愿意从什么开始，或者有什么想了解的，可以告诉我。'})
    intervention('birth',{'instance_id':ident,'receipt':result});print(json.dumps(meta,ensure_ascii=False))

def status():
    c=current();s=remote_json('cat '+c['instance_root']+'/state/current.json')
    summary={k:s.get(k) for k in ('instance_id','sample_id','t0','planned_end','pulse_id','last_pulse_at','current_focus','pending_action','lease','learning_feedback')}
    summary.update(active=active(),body=s.get('body'),self=s.get('self'),concerns=s.get('active_concerns'),resource=s.get('cognitive_resource'),experiences=s.get('experiences'))
    write_json(ARCHIVE/'samples'/c['instance_id']/'latest-state.json',s)
    print(json.dumps(summary,ensure_ascii=False,indent=2))

def extend(minutes):
    c=current();s=remote_json('cat '+c['instance_root']+'/state/current.json')
    end=parse_time(s['t0'])+timedelta(minutes=minutes)
    response=api('/v1/lab/deadline',{'planned_end':end.isoformat().replace('+00:00','Z')})
    s=remote_json('cat '+c['instance_root']+'/state/current.json');new=arm_deadline(s)
    remote('systemctl --user set-property --runtime hominal20-life RuntimeMaxSec='+str(minutes*60+120)+'s')
    remote('systemctl --user stop '+c['deadline_unit']+'.timer')
    c.update(planned_end=s['planned_end'],deadline_unit=new);write_json(ARCHIVE/'current.json',c);intervention('extend',response);print(json.dumps(c))

def stop(reason):
    c=current();remote('systemctl --user stop hominal20-life',timeout=60)
    if c.get('deadline_unit'):remote('systemctl --user stop '+c['deadline_unit']+'.timer 2>/dev/null || true')
    dest=ARCHIVE/'samples'/c['instance_id'];dest.mkdir(parents=True,exist_ok=True)
    tar=REMOTE+'/archives/'+c['instance_id']+'.tar.gz'
    remote('mkdir -p '+REMOTE+'/archives; tar -czf '+tar+' -C '+REMOTE+' lives/'+c['instance_id']+' workspaces/'+c['instance_id']+' state/cognitive-usage.jsonl evidence/'+c['instance_id'],timeout=60)
    copy(tar,dest/'archive.tar.gz',True)
    digest=hashlib.sha256((dest/'archive.tar.gz').read_bytes()).hexdigest();write_json(dest/'archive.json',{'reason':reason,'sha256':digest,'at':stamp(),'current':c})
    intervention('stop',{'instance_id':c['instance_id'],'reason':reason,'archive_sha256':digest});print(json.dumps({'archive':str(dest),'sha256':digest}))
    remote('systemctl --user stop hominal20-vision hominal20-input hominal20-workbench hominal20-browser 2>/dev/null || true')

def main():
    ARCHIVE.mkdir(mode=0o700,exist_ok=True);PRIVATE.mkdir(mode=0o700,exist_ok=True)
    p=argparse.ArgumentParser(description=__doc__);s=p.add_subparsers(dest='command',required=True)
    s.add_parser('prepare');s.add_parser('body-up');a=s.add_parser('start');a.add_argument('--minutes',type=int,default=10)
    s.add_parser('status');a=s.add_parser('extend');a.add_argument('--total-minutes',type=int,required=True)
    a=s.add_parser('stop');a.add_argument('--reason',default='manual')
    a=s.add_parser('mentor');a.add_argument('body');s.add_parser('outbox')
    a=p.parse_args()
    if a.command=='prepare':prepare()
    elif a.command=='body-up':print(json.dumps(body_up()))
    elif a.command=='start':start(a.minutes)
    elif a.command=='status':status()
    elif a.command=='extend':extend(a.total_minutes)
    elif a.command=='stop':stop(a.reason)
    elif a.command=='outbox':print(json.dumps(api('/v1/mentor/outbox'),ensure_ascii=False,indent=2))
    else:
        payload={'message_id':'mentor-'+uuid.uuid4().hex,'body':'[Codex代理导师] '+a.body}
        r=api('/v1/mentor/inbox',payload);intervention('mentor',{'payload':payload,'receipt':r});print(json.dumps(r))

if __name__=='__main__':main()
