#!/usr/bin/env python3
"""Read-only evidence extraction; counts do not decide Stage20 acceptance."""
import collections,json,pathlib,sys
import stage20 as l
c=l.current();dest=l.ARCHIVE/'samples'/c['instance_id']
s=l.remote_json('cat '+c['instance_root']+'/state/current.json')
rows=[json.loads(x) for x in l.remote('cat '+c['instance_root']+'/journal/events.jsonl').splitlines()]
commits={x['payload']['id']:x['payload'] for x in rows if x['kind']=='action_committed'}
recall={x['correlation_id']:x['payload'] for x in rows if x['kind']=='memory_recalled'}
steps=[]
for x in rows:
 if x['kind']!='action_result':continue
 a=x['payload'].get('payload',{});parent=commits.get(a.get('commitment_id'),{});prior=recall.get(parent.get('lease_id'),{})
 steps.append({'seq':x['seq'],'time':x['time'],'action':a,'commitment':parent,'recalled_experiences':prior.get('experiences'),'recalled_memory_ids':[m.get('id') for m in prior.get('memories',[]) or []]})
spends=[x['payload'] for x in rows if x['kind']=='cognition_spend']
report={'instance_id':c['instance_id'],'release':c['release'],'t0':s['t0'],'last_pulse_at':s['last_pulse_at'],'elapsed_seconds':(l.parse_time(s['last_pulse_at'])-l.parse_time(s['t0'])).total_seconds(),
 'journal_kinds':dict(collections.Counter(x['kind'] for x in rows)),'steps':steps,'spend_records':spends,
 'learning_events':[x for x in rows if x['kind']=='learning_committed'],'scope':'Evidence only. Locate, delivered input, file truth, and claimed learning require separate interpretation.'}
l.write_json(dest/'evidence.json',report)
print(json.dumps({'path':str(dest/'evidence.json'),'elapsed_seconds':report['elapsed_seconds'],'action_count':len(steps),'journal_kinds':report['journal_kinds']},ensure_ascii=False))
