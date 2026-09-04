#!/usr/bin/env python3
"""Stage 20 desktop organ: actual screenshot, visual grounding and bounded input."""
import fcntl
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import time
import urllib.request
import urllib.error
import uuid
from PIL import Image, ImageChops

ROOT = pathlib.Path(os.environ.get('HOMINAL20_ROOT', str(pathlib.Path.home()/'.local/share/hominal20'))).resolve()
SAMPLE=os.environ.get('HOMINAL20_SAMPLE_ID','')
if SAMPLE and not re.fullmatch(r's20-[a-z0-9-]+',SAMPLE): raise ValueError('invalid sample namespace')
EVIDENCE = ROOT/'evidence'/SAMPLE
VISION = 'http://127.0.0.1:8765'
OPS = {'desktop_observe': '{}', 'desktop_locate': '{"target":"visible control description"}',
       'desktop_click': '{"target_id":"ID returned by desktop_locate"}',
       'desktop_type': '{"text":"exact text to enter"}',
       'desktop_key': '{"key":"Enter|Escape|Tab|Ctrl+A|Ctrl+S|Ctrl+V|Backspace"}',
       'desktop_scroll': '{"amount":-3} (negative scrolls downward; -8..8)'}


def now():
    from datetime import datetime, timezone
    return datetime.now(timezone.utc).isoformat().replace('+00:00', 'Z')


def emit(obj): print(json.dumps(obj, ensure_ascii=False))


def run(args, deadline, input=None):
    return subprocess.run(args, input=input, capture_output=True, text=True, check=True,
                          timeout=max(0.05, deadline-time.time())).stdout


def atomic(path, obj):
    tmp = path.with_suffix('.tmp.'+uuid.uuid4().hex)
    tmp.write_text(json.dumps(obj, ensure_ascii=False)); tmp.chmod(0o600); tmp.replace(path)


def fingerprint(image):
    return hashlib.sha256(image.tobytes()).hexdigest()


def same_scene(old, frame):
    if old.get('digest') == frame['digest']: return True
    try:
        a=Image.open(old['path']).convert('RGB'); b=Image.open(frame['path']).convert('RGB')
        if a.size!=b.size: return False
        box=ImageChops.difference(a,b).getbbox()
        # A blinking text insertion caret is not a changed control or window.
        return box is None or (box[2]-box[0]<=4 and box[3]-box[1]<=40)
    except (OSError, KeyError): return False


def capture(deadline):
    state = run(['/usr/lib64/qt6/bin/qdbus', 'org.freedesktop.ScreenSaver', '/ScreenSaver',
                 'org.freedesktop.ScreenSaver.GetActive'], deadline).strip()
    if state != 'false': raise ValueError('desktop session locked or unavailable')
    outputs=json.loads(run(['kscreen-doctor','-j'],deadline))['outputs']
    enabled=[o for o in outputs if o.get('enabled') and o.get('connected')]
    if len(enabled)!=1 or enabled[0].get('rotation',1)!=1:
        raise ValueError('this input calibration requires one unrotated output; recalibrate changed display topology')
    path = EVIDENCE/('frame-'+uuid.uuid4().hex+'.png')
    run(['spectacle', '-b', '-n', '-f', '--scaled', '-o', str(path)], deadline)
    img = Image.open(path).convert('RGB')
    if max(img.getextrema()[0]) - min(img.getextrema()[0]) < 2:
        raise ValueError('blank screen; wake display before using the organ')
    return {'id': path.stem, 'path': str(path), 'width': img.width, 'height': img.height,
            'digest': fingerprint(img), 'captured_at': now(), 'created': time.time()}


def vision(frame, mode, deadline, target=''):
    raw = json.dumps({'image': frame['path'], 'mode': mode, 'target': target, 'deadline': deadline}).encode()
    req = urllib.request.Request(VISION+'/interpret', raw, {'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=max(.1, deadline-time.time())) as response: return json.load(response)
    except urllib.error.HTTPError as exc:
        raise ValueError('vision service: '+exc.read(2000).decode(errors='replace')) from exc


def observation(frame, interpreted):
    text = interpreted['text']
    return {'schema': 'hominal.organ-observation/v1', 'organ_id': 'desktop', 'surface_id': 'kde.desktop',
            'observed_at': now(), 'context': [f"frame={frame['id']} size={frame['width']}x{frame['height']}",
             'source=local_visual_model_hypothesis; labels and state must be verified by actual consequences'],
            'objects': [{'id': hashlib.sha256(text.encode()).hexdigest()[:24], 'content': text}],
            'facts': {'frame': frame, 'visual_interpretation': interpreted}}


def observe(deadline):
    frame = capture(deadline)
    cache_file = ROOT/'services/observation.json'
    try: previous = json.loads(cache_file.read_text())
    except (OSError, ValueError): previous = {}
    if same_scene(previous, frame):
        interpreted = previous['interpretation']
    else:
        interpreted = vision(frame, 'describe', deadline)
        atomic(cache_file, {'digest': frame['digest'], 'path':frame['path'], 'interpretation': interpreted})
    return observation(frame, interpreted)


def verify_binding(binding, frame, at=None):
    if (time.time() if at is None else at) - binding['created'] > 60: raise ValueError('target expired; locate again')
    if binding.get('consumed'): raise ValueError('target already consumed; observe outcome and locate again')
    if not same_scene(binding,frame) or (frame['width'], frame['height']) != tuple(binding['size']):
        raise ValueError('scene changed; target has not been clicked, locate again')
    x, y = binding['point']
    if not (0 <= x < frame['width'] and 0 <= y < frame['height']): raise ValueError('target out of bounds')


def input_command(kind, payload, deadline):
    req = urllib.request.Request('http://127.0.0.1:8766/input',
         json.dumps({'kind': kind, 'payload': payload, 'deadline': deadline}).encode(),
         {'Content-Type': 'application/json','X-Hominal-Capability':(ROOT/'services/input.cap').read_text()})
    with urllib.request.urlopen(req, timeout=max(.1, deadline-time.time())) as response: return json.load(response)


def perform(req):
    deadline = time.time() + min(120, max(1, req.get('timeout_milliseconds', 30000)/1000))
    out = {'schema':'hominal.organ-action-result/v1','organ_id':'desktop','action_id':req['action_id'],
           'status':'failed','effect':'unknown','observed_at':now(),'summary':''}
    attempted = False
    try:
        op = req['operation']; data = json.loads(req.get('input') or '{}')
        if op not in OPS: raise ValueError('unsupported desktop operation')
        if op == 'desktop_observe':
            obs = observe(deadline); out['observation'] = obs
            result = obs['facts']; out['effect'] = 'observed'
        elif op == 'desktop_locate':
            target = data.get('target', '').strip()
            if not target or len(target) > 300: raise ValueError('target must describe one visible control')
            frame = capture(deadline); found = vision(frame, 'locate', deadline, target)
            if not found.get('found'): raise ValueError('visual model found no visible matching target')
            point = [min(frame['width']-1, round(found['x']*frame['width']/1000)),
                     min(frame['height']-1, round(found['y']*frame['height']/1000))]
            binding = {'target_id': 'target-'+uuid.uuid4().hex, 'target': target, 'point': point,
                       'digest':frame['digest'],'created':time.time(),'size':[frame['width'],frame['height']],
                       'frame_id':frame['id'],'path':frame['path'],'interpretation':found}
            atomic(EVIDENCE/(binding['target_id']+'.json'), binding)
            result = binding; out['effect'] = 'observed'
        else:
            before = capture(deadline)
            if op == 'desktop_click':
                ident = data.get('target_id', '')
                if not re.fullmatch(r'target-[0-9a-f]{32}', ident): raise ValueError('invalid target identity')
                binding = json.loads((EVIDENCE/(ident+'.json')).read_text())
                verify_binding(binding, before)
                payload = {'x': binding['point'][0]/before['width'], 'y': binding['point'][1]/before['height']}
                binding['consumed']=True; atomic(EVIDENCE/(ident+'.json'),binding)
                attempted = True; receipt = input_command('tap', payload, deadline)
            elif op == 'desktop_type':
                value = data.get('text')
                if not isinstance(value, str) or len(value) > 8000: raise ValueError('invalid text')
                attempted = True; receipt = input_command('text', {'text': value}, deadline)
            elif op == 'desktop_key':
                attempted = True; receipt = input_command('key', {'key': data.get('key')}, deadline)
            else:
                amount = data.get('amount')
                if type(amount) != int or not -8 <= amount <= 8: raise ValueError('scroll amount must be -8..8')
                attempted = True; receipt = input_command('scroll', {'amount': amount}, deadline)
            # Input delivery and screenshot availability are mechanical facts, not semantic success.
            after = capture(deadline)
            result = {'input_receipt':receipt,'before':before,'after':after,
                      'pixels_changed':after['digest']!=before['digest'], 'semantic_success':'not_decided'}
            try:
                interpreted = vision(after, 'describe', deadline)
                result['visual_after'] = interpreted; out['observation'] = observation(after, interpreted)
            except Exception as exc:
                result['visual_after_error'] = str(exc)[:200]
            out['effect'] = 'changed' if result['pixels_changed'] else 'unknown'
        out.update(status='completed', summary='实际操作及可核验观察已返回；目标是否实现由主脑结合证据判断。', output=json.dumps(result,ensure_ascii=False))
    except Exception as exc:
        out.update(status='unknown' if attempted else 'failed', summary=str(exc)[:300],
                   output=json.dumps({'error':type(exc).__name__,'input_attempted':attempted,'detail':str(exc)[:300]}))
    with (EVIDENCE/'actions.jsonl').open('a') as log: log.write(json.dumps(out,ensure_ascii=False)+'\n')
    return out


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True); (ROOT/'services').mkdir(exist_ok=True)
    op = sys.argv[1]
    if op == 'describe':
        return emit({'schema':'hominal.organ-description/v1','id':'desktop','name':'KDE visual desktop',
            'command':'desktop','capabilities':['observe','perform','cancel','desktop_ui','vision'],
            'operations':list(OPS),'operation_inputs':OPS,
            'guidance':'真实桌面视觉器官。先locate取得target_id，随后click；画面变化需重新定位。type向当前焦点输入原文，key/scroll作用于当前窗口。操作后返回截图与视觉推测，成功投递输入不等于目标完成。'})
    if op == 'health':
        try:
            with urllib.request.urlopen(VISION+'/health', timeout=1) as r: health=json.load(r)
            with urllib.request.urlopen('http://127.0.0.1:8766/health',timeout=1) as r: input_health=json.load(r)
            status=('busy' if health.get('busy') else 'ready') if input_health.get('ready') else 'unavailable'
        except Exception: status='unavailable'
        return emit({'schema':'hominal.organ-health/v1','id':'desktop','status':status,'accepting':status!='unavailable','in_flight':int(status=='busy'),'queued':0})
    with (ROOT/'services/desktop.lock').open('a') as lock:
        # Both perception and actions are bounded by the caller; no persistent action queue.
        fcntl.flock(lock, fcntl.LOCK_EX)
        if op == 'observe': emit(observe(time.time()+18))
        elif op == 'perform':
            req=json.loads(sys.argv[2])
            if req.get('schema')!='hominal.organ-action/v1' or not req.get('action_id'): raise ValueError('invalid envelope')
            emit(perform(req))
        else: raise ValueError('unsupported protocol operation')


if __name__ == '__main__':
    try: main()
    except Exception as exc:
        print(type(exc).__name__+': '+str(exc)[:300], file=sys.stderr); sys.exit(1)
