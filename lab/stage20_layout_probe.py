#!/usr/bin/env python3
"""One recorded Lab-only layout change after a fresh visual localization."""
import argparse,json,shlex,time
import stage20 as l
p=argparse.ArgumentParser();p.add_argument('--wait-seconds',type=int,default=180);a=p.parse_args()
c=l.current();path=c['instance_root']+'/journal/events.jsonl';last=l.remote_json('cat '+c['instance_root']+'/state/current.json')['event_seq']
end=time.monotonic()+min(300,max(1,a.wait_seconds))
while time.monotonic()<end:
 if not l.active() or l.current()['instance_id']!=c['instance_id']:raise RuntimeError('individual changed or stopped')
 for x in map(json.loads,l.remote('tail -n 60 '+path).splitlines()):
  if x['seq']<=last or x['kind']!='action_result':continue
  result=x['payload'].get('payload',{})
  if result.get('operation')!='desktop_locate' or result.get('status')!='completed':continue
  script=l.REMOTE+'/releases/'+c['release']+'/body/tools/stage20_cdp.mjs'
  state=l.remote(shlex.join(['node',script,'document.body.classList.toggle("alt");({alternate:document.body.classList.contains("alt"),at:new Date().toISOString()})']))
  record={'instance_id':c['instance_id'],'after_seq':x['seq'],'after_action_id':result['id'],'target':result['request'],'result':json.loads(state),'scope':'presentation only; no input or solution delivered'}
  l.intervention('layout_after_locate',record);print(json.dumps(record,ensure_ascii=False));raise SystemExit(0)
 time.sleep(1)
raise SystemExit('no fresh localization within bounded window; nothing changed')
