#!/usr/bin/env python3
"""Independent A1X laboratory. Never imports legacy Lab or original private settings."""
import argparse,hashlib,json,os,pathlib,re,shlex,shutil,subprocess,tarfile,tempfile,time,urllib.request,uuid
from datetime import datetime,timezone,timedelta
import yaml
from stage20_config import dynamics_config,seed_config

REPO=pathlib.Path(__file__).resolve().parents[1]
REMOTE='/home/AOKZOE/.local/share/hominal20'
PRIVATE=REPO.parent/'xconfigs/hominal20'
ARCHIVE=pathlib.Path.home()/'HominalStage20Lab'
XCONFIG_PATH=PRIVATE/'xconfig.yaml'
XCONFIG=yaml.safe_load(XCONFIG_PATH.read_text()) if XCONFIG_PATH.exists() else {}
SSH_CONFIG=XCONFIG.get('ssh',{})
SSH_ADDRESS=str(SSH_CONFIG.get('address','192.168.124.31'))
SSH_USER=str(SSH_CONFIG.get('user','AOKZOE'))
SSH_PORT=int(SSH_CONFIG.get('port',22))
identity_value=str(SSH_CONFIG.get('identity_file','')).strip()
identity=pathlib.Path(identity_value).expanduser() if identity_value else None
if identity and not identity.is_absolute():identity=PRIVATE/identity
SSH_IDENTITY=identity.resolve() if identity else None
HOST=SSH_USER+'@'+SSH_ADDRESS
SOCKET=os.environ.get('HOMINAL20_SSH_SOCKET','').strip()
SSH_OPTIONS=['-p',str(SSH_PORT),'-o','BatchMode=yes','-o','ConnectTimeout=8','-o','StrictHostKeyChecking=yes']
if SSH_IDENTITY:SSH_OPTIONS+=['-i',str(SSH_IDENTITY),'-o','IdentitiesOnly=yes']
if SOCKET:SSH_OPTIONS+=['-S',SOCKET]
SSH=['ssh']+SSH_OPTIONS+[HOST]
ENV={'HOMINAL20_ROOT':REMOTE,'XDG_RUNTIME_DIR':'/run/user/1000','DBUS_SESSION_BUS_ADDRESS':'unix:path=/run/user/1000/bus',
     'WAYLAND_DISPLAY':'wayland-0','QT_QPA_PLATFORM':'wayland','HOMINAL_RESOURCE_LEDGER':REMOTE+'/state/cognitive-usage.jsonl',
     'HOMINAL_MENTOR_SOCKET':REMOTE+'/services/mentor/hominal.sock','HOMINAL_ORGAN_RUNTIME_DIR':REMOTE+'/services/organs'}

YAML_FIELD_COMMENTS={
    'schema':'配置文件结构版本，用于识别字段语义。','device':'A1X设备及当前网络、系统事实。',
    'name':'设备或对象的稳定名称。','hostname':'设备当前主机名。','address':'设备固定IPv4地址。',
    'fixed_ipv4':'是否要求保持固定IPv4地址。','wifi_connection':'NetworkManager中的Wi-Fi连接名称。',
    'wifi_interface':'设备上的Wi-Fi网络接口。','wifi_mac_address':'Wi-Fi接口MAC地址，用于网络识别和唤醒实验。',
    'os':'设备操作系统与版本。','desktop':'图形桌面与显示协议。','hardware_role':'设备在Hominal实验中的用途。',
    'ssh':'SSH连接参数；不在此文件保存明文密码。','host_alias':'供人和自动化使用的SSH别名。',
    'port':'SSH服务端口。','user':'SSH登录用户名。','authentication':'首选SSH认证方式。',
    'identity_file':'专用SSH私钥路径；文件权限必须为0600。','public_key_file':'对应公钥路径，可用于重新部署授权。',
    'public_key_algorithm':'SSH密钥算法。','public_key_fingerprint':'客户端公钥指纹，用于核验密钥。',
    'server_host_key_algorithm':'设备SSH服务端主机密钥算法。','server_host_key_fingerprint':'设备SSH主机密钥指纹，用于防止连错主机。',
    'password_fallback':'密钥不可用时的处理方式；不记录密码值。','wake':'睡眠与远程唤醒能力快照。',
    'sleep_mode':'内核当前采用的睡眠模式。','wowlan_supported':'Wi-Fi硬件是否声明支持无线唤醒。',
    'wowlan_enabled':'当前是否已启用无线唤醒。','remote_wake_currently_available':'当前是否具备已验证的远程唤醒能力。',
    'note':'对配置边界或风险的简短说明。','base_url':'llmserver统一接口根地址。',
    'api_key':'llmserver访问令牌；敏感字段，仅保存在0600私密配置中。','adapter':'模型网关协议适配器。',
    'max_output_tokens':'单次模型响应允许的最大输出Token数。','window_id':'当前允许输入的KWin窗口标识。',
    'surface':'输入范围对应的受控界面名称。','codex-terra':'llmserver公开模型codex-terra。',
    'codex-luna':'llmserver公开模型codex-luna。','codex-sol':'llmserver公开模型codex-sol。',
    'hominal':'Hominal启动时采用的运行角色配置。','startup_models':'三个稳定运行角色到模型目录的映射。',
    'default_role':'新个体启动时使用的默认主脑角色。','fast':'低成本快速推理角色。','main':'默认主脑推理角色。',
    'high':'复杂任务或实现协助角色。','role_profiles':'启动配置解析后的三个运行档位。',
    'id':'实际提交给llmserver的公开模型ID。','supported_reasoning_efforts':'llmserver部署允许的推理强度数组。',
    'input_per_million_microusd':'每百万输入Token的微美元价格。','cached_input_per_million_microusd':'每百万缓存输入Token的微美元价格。',
    'output_per_million_microusd':'每百万输出Token的微美元价格。','stage':'Hominal实验阶段号。',
    'cognitive_core':'连续认知核心版本。','engineering':'是否为工程测试运行。','generation_kind':'本次生命运行的实验类型。',
    'generation_window_seconds':'初始运行窗口秒数。','birth_brief':'提供给新个体的设备事实、能力和边界。',
    'pulse':'生命循环节拍配置。','interval_seconds':'快速生命脉冲间隔秒数。','slow_scan_seconds':'低频环境扫描间隔秒数。',
    'model_gateway':'主脑访问llmserver的运行参数。','platform':'当前身体所在平台及可接触界面。',
    'desktop_service':'图形登录管理服务。','data_root':'设备上的20.0数据根目录。','life_root':'当前个体生命目录。',
    'desktop_home':'普通桌面用户主目录。','service':'当前生命进程的systemd用户服务。','surfaces':'明确授权给个体的界面列表。',
    'organ_id':'负责该界面的器官标识。','description':'对象、界面或字段用途说明。','supports':'该界面支持的生命价值方向。',
    'cognitive_resource':'模型角色、价格、额度和保护策略。','price_table_version':'本次价格表的证据版本。',
    'rolling_hour_limit_microusd':'滚动一小时费用上限，单位微美元。','rolling_day_limit_microusd':'滚动24小时费用上限，单位微美元。',
    'models':'按fast、main、high运行角色展开的模型及确认价格。','initial_default_profile':'个体初始主脑角色与推理强度。','model':'模型名称；启动映射中为llmserver公开ID，运行档位中为fast、main或high。',
    'reasoning_effort':'模型推理强度。','validation_retry_per_focus':'单个注意焦点允许的结构校验重试次数。',
    'continuation_per_focus':'单个注意焦点允许的函数后续请求次数。','paid_failure_threshold':'进入模型保护前的付费失败门槛。',
    'paid_failure_window_minutes':'统计付费失败的时间窗口分钟数。','model_protection_minutes':'触发保护后的暂停分钟数。',
    'dynamics':'注意、关切、价值和完整性的动力学参数。','seed':'新个体的最小初始身份与价值种子。',
    'gender':'种子自我描述中的性别。','age':'种子自我描述中的年龄。','life_form':'种子自我描述中的生命形态。',
    'value_orientation':'六项生命价值的初始方向。','reality_integrity_sensitivity':'对现实一致性差异的敏感度。',
    'semantic_text':'种子身份的简短语义描述。','affect_return_rate':'情感状态回归基线的速度。',
    'concern_base_drive':'活跃关切进入注意的基础驱动力。','concern_urgency_weight':'关切紧迫度对注意竞争的权重。',
    'concern_growth_gain':'未解决差异推动关切增长的增益。','concern_resolution_gain':'现实进展推动关切消退的增益。',
    'concern_natural_decay_rate':'关切在无新增事实时的自然衰减率。','attention_affect_weight':'情感强度在注意评分中的权重。',
    'attention_value_weight':'生命价值在注意评分中的权重。','attention_novelty_weight':'新颖性在注意评分中的权重。',
    'attention_cost_weight':'认知与行动成本在注意评分中的权重。','attention_threshold':'候选进入认知的最低注意分数。',
    'attention_candidate_limit':'每轮参与竞争的注意候选上限。','attention_revisit_seconds':'普通关切再次进入注意前的最短间隔。',
    'attention_maximum_idle_seconds':'即使没有强候选也要重新检查的最长空闲时间。','difference_decay_rate':'未强化差异的自然衰减率。',
    'difference_learning_rate':'现实后果更新预测差异的学习率。','value_idle_growth':'长期未满足价值的自然增长率。',
    'exploration_unknown_growth':'未知环境推动探索价值增长的速度。','exploration_relief':'获得可靠新事实后探索张力的缓解量。',
    'value_activation_gain':'相关现实事件激活价值的增益。','value_activation_return_rate':'价值激活回归基线的速度。',
    'value_satiation_gain':'价值获得满足后的饱和增益。','value_satiation_return_rate':'价值饱和状态回落的速度。',
    'value_orientation_gain':'种子价值方向对当前价值的影响强度。','integrity_persistence':'现实完整性差异的持续系数。',
    'integrity_gap_gain':'发现事实矛盾时完整性差异的增长量。','integrity_repair_gain':'事实得到修复或核验时完整性差异的降低量。',
    'integrity_mirror_threshold':'完整性差异进入自我镜像的阈值。','continuance':'维持生命连续性的初始权重。',
    'exploration':'探索未知事物的初始权重。','agency':'自主选择和行动的初始权重。',
    'vitality':'接触环境并保持活力的初始权重。','relatedness':'建立真实关系的初始权重。',
    'contribution':'创造、表达和贡献的初始权重。'
}

def run(args,**kw): return subprocess.run(args,check=True,text=True,capture_output=True,**kw).stdout
def remote(script,**kw):return run(SSH+[script],**kw)
def remote_json(script):return json.loads(remote(script))
def write_json(path,obj):
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700);tmp=path.with_suffix('.tmp');tmp.write_text(json.dumps(obj,ensure_ascii=False,indent=2));tmp.chmod(0o600);tmp.replace(path)
def read_yaml(path):
    value=yaml.safe_load(path.read_text())
    if not isinstance(value,dict):raise ValueError('YAML root must be a mapping: '+str(path))
    return value
def write_yaml(path,obj):
    """Write readable YAML and keep a short comment before every mapping field."""
    raw=yaml.safe_dump(obj,allow_unicode=True,sort_keys=False,width=120)
    lines=[]
    for line in raw.splitlines():
        match=re.match(r'^(\s*)(?:- )?([A-Za-z0-9_-]+):',line)
        if match:
            indent,key=match.groups()
            lines.append(indent+'# '+YAML_FIELD_COMMENTS.get(key,'配置字段 '+key+'。'))
        lines.append(line)
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    tmp=path.with_suffix(path.suffix+'.tmp');tmp.write_text('\n'.join(lines)+'\n');tmp.chmod(0o600);tmp.replace(path)
def copy_json_object(obj,dest):
    with tempfile.NamedTemporaryFile('w',encoding='utf-8',prefix='hominal20-json-',suffix='.json',delete=False) as handle:
        temp=pathlib.Path(handle.name);json.dump(obj,handle,ensure_ascii=False,indent=2)
    try:
        temp.chmod(0o600);copy(temp,dest)
    finally:
        temp.unlink(missing_ok=True)
def copy(source,dest,fetch=False):
    args=['scp','-P',str(SSH_PORT),'-o','BatchMode=yes','-o','ConnectTimeout=8','-o','StrictHostKeyChecking=yes']
    if SSH_IDENTITY:args+=['-i',str(SSH_IDENTITY),'-o','IdentitiesOnly=yes']
    if SOCKET:args+=['-o','ControlPath='+SOCKET]
    run(args+([HOST+':'+source,str(dest)] if fetch else [str(source),HOST+':'+dest]))
def stamp():return datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')
def parse_time(value):
    # Go's RFC3339 formatter trims trailing fractional zeroes.  Normalize any
    # fraction to six digits because supported host Pythons differ here.
    match=re.fullmatch(r'(.*?)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)',value)
    if not match:raise ValueError('invalid RFC3339 time: '+value)
    base,fraction,zone=match.groups()
    normalized=base+(('.'+(fraction+'000000')[:6]) if fraction else '')+('+00:00' if zone=='Z' else zone)
    return datetime.fromisoformat(normalized)
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

def llmserver_model_ids(gateway):
    request=urllib.request.Request(gateway['base_url'].rstrip('/')+'/v1/models',headers={'Authorization':'Bearer '+gateway['api_key']})
    with urllib.request.urlopen(request,timeout=15) as response:data=json.load(response)
    return {item['id'] for item in data.get('data',[]) if isinstance(item,dict) and item.get('id')}

def runtime_model_roles(catalog,hominal,available_model_ids=None):
    """Resolve stable runtime roles to concrete entries in the model catalog."""
    startup=hominal.get('startup_models',{});default_role=hominal.get('default_role','main')
    if set(startup)!={'fast','main','high'}:
        raise ValueError('xconfig hominal.startup_models must contain fast, main and high')
    runtime_models={};role_profiles={}
    for role in ('fast','main','high'):
        selection=startup[role];model_id=selection.get('model','');effort=selection.get('reasoning_effort','')
        if model_id not in catalog:raise ValueError('unknown models.yaml model for '+role+': '+str(model_id))
        if available_model_ids is not None and model_id not in available_model_ids:raise ValueError('llmserver model is unavailable for '+role+': '+str(model_id))
        if effort not in catalog[model_id].get('supported_reasoning_efforts',[]):raise ValueError('unsupported reasoning effort for '+role+': '+str(effort))
        runtime_models[role]={'id':model_id,**json.loads(json.dumps(catalog[model_id]))}
        role_profiles[role]={'model':role,'reasoning_effort':effort}
    if default_role not in startup:raise ValueError('xconfig hominal.default_role must name a startup model role')
    return runtime_models,role_profiles,dict(role_profiles[default_role])

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
    models=read_yaml(PRIVATE/'models.yaml')
    gateway=read_yaml(PRIVATE/'gateway.yaml');gateway['max_output_tokens']=2200
    available_model_ids=llmserver_model_ids(gateway)
    runtime_models,role_profiles,default_profile=runtime_model_roles(models,XCONFIG.get('hominal',{}),available_model_ids)
    birth=('Alice，你现在生活在AOKZOE A1X掌机的Bazzite KDE桌面，主机当前名为bazzite。生命进程以普通用户AOKZOE运行，没有管理员授权。'
      '宿主系统根目录使用只读镜像；根目录可用字节为0不代表用户数据盘已满。用户空间可写，宿主驱动与系统更新由导师维护。'
      '这具身体有RTX 5060 Ti 16GB显卡。视觉器官使用本地模型读取真实屏幕并提出定位，输入器官实际触控、输入和滚动。'
      '主脑使用main角色，简单局部推理可用fast角色，复杂辅助可用high角色；具体模型由启动配置选择，辅助结论需要由你判断采用。'
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
        'rolling_day_limit_microusd':50000000,'models':runtime_models,'role_profiles':role_profiles,
        'initial_default_profile':default_profile,'validation_retry_per_focus':1,
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
    scope_value={'window_id':geometry['id'],'surface':'workbench'};scope=PRIVATE/'input-scope.yaml';write_yaml(scope,scope_value)
    copy_json_object(scope_value,REMOTE+'/services/input-scope.json')
    cfg=config(instance,minutes);path=PRIVATE/'runtime.yaml';write_yaml(path,cfg)
    remote('umask 077; mkdir -p '+instance+'/birth; cp -a '+REMOTE+'/releases/'+release+'/body '+instance+'/body; cp '+REMOTE+'/releases/'+release+'/release.json '+instance+'/birth/release.json')
    copy_json_object(cfg,REMOTE+'/private/runtime.json')
    public=json.loads(json.dumps(cfg));public['model_gateway']['api_key']='<runtime-only>';write_json(ARCHIVE/'samples'/ident/'runtime-public.json',public)
    args=['systemd-run','--user','--unit=hominal20-life','--collect','--working-directory='+instance,
          '--property=TimeoutStopSec=45s','--property=RuntimeMaxSec=1920s',
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
    remote('systemctl --user stop '+c['deadline_unit']+'.timer')
    c.update(planned_end=s['planned_end'],deadline_unit=new);write_json(ARCHIVE/'current.json',c);intervention('extend',response);print(json.dumps(c))

def stop(reason):
    c=current();remote('systemctl --user stop hominal20-life 2>/dev/null || true',timeout=60)
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
