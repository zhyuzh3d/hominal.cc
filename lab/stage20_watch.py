import sys,json,collections,datetime
sys.path.insert(0,'lab');import stage20 as l
c=l.current();dest=l.ARCHIVE/'samples'/c['instance_id'];cursor=dest/'watch-cursor.json';seq=json.loads(cursor.read_text()).get('seq',0) if cursor.exists() else 0
s=l.remote_json('cat '+c['instance_root']+'/state/current.json');l.write_json(dest/'latest-state.json',s)
rows=[json.loads(x) for x in l.remote('cat '+c['instance_root']+'/journal/events.jsonl').splitlines()];l.write_json(dest/'latest-journal.json',rows)
out=[]
for x in rows:
 if x['seq']<=seq:continue
 k=x['kind'];p=x.get('payload',{});v=None
 if k=='aip_commit':v={'action':p.get('action_kind'),'thought':p.get('thought_thread'),'continues':p.get('continues_concern_id'),'resource':p.get('resource_choice')}
 elif k=='action_started':v=p
 elif k=='action_result':
  a=p.get('payload',{});v={z:a.get(z) for z in ['id','operation','status','effect','request']};v['result']=str(a.get('result',''))[:700] if a.get('status')!='completed' else ''
 elif k=='learning_committed' and p.get('experiences'):v={'experiences':p['experiences']}
 elif k in ['cognition_failed','global_block','mentor_queued','generation_ended']:v=p
 if v is not None:out.append({'seq':x['seq'],'at':x['time'][11:19],'kind':k,'data':v})
body=s['body'];summary={'instance':c['instance_id'],'at':s['last_pulse_at'],'active':l.active(),'minute':round((l.parse_time(s['last_pulse_at'])-l.parse_time(s['t0'])).total_seconds()/60,1),'event_seq':s['event_seq'],'hour_spent':body['cognitive_hour_spent_microusd']/1e6,'hour_remaining':body['cognitive_hour_remaining_microusd']/1e6,'lease':s.get('lease',{}).get('id'),'new':out}
print(json.dumps(summary,ensure_ascii=False,indent=2));l.write_json(cursor,{'seq':max(x['seq'] for x in rows)})
