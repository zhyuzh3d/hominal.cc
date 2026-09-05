#!/usr/bin/env python3
"""Paid strict-function and billing preflight; writes only the Stage20 ledger."""
import json, os, pathlib, time, urllib.request, uuid
from decimal import Decimal, ROUND_CEILING
from datetime import datetime, timezone

ROOT=pathlib.Path(os.environ.get('HOMINAL20_ROOT',str(pathlib.Path.home()/'.local/share/hominal20')))

def main():
    gateway=json.loads((ROOT/'private/gateway.json').read_text())
    records=[]; validated={}
    if (ROOT/'evidence/model-preflight.json').exists():
        raise RuntimeError('preflight already recorded; inspect it before authorizing another paid run')
    for role,model_id,effort in [('main','codex-terra','none'),('fast','codex-luna','none'),('high','codex-sol','low')]:
        body={'model':model_id,'input':'Call gateway_probe with ok=true. No other action.',
          'reasoning':{'effort':effort},'max_output_tokens':1024,'store':False,
          'tools':[{'type':'function','name':'gateway_probe','description':'Confirm the real function contract.',
            'strict':True,'parameters':{'type':'object','properties':{'ok':{'type':'boolean'}},'required':['ok'],'additionalProperties':False}}],
          'tool_choice':{'type':'function','name':'gateway_probe'},'parallel_tool_calls':False}
        start=time.monotonic(); call_id='preflight-'+uuid.uuid4().hex
        stamp=datetime.now(timezone.utc).isoformat().replace('+00:00','Z')
        reserve={'call_id':call_id,'lease_id':call_id,'time':stamp,'requested_model':role,'reasoning_effort':effort,
                 'status':'interrupted_unknown','actual_microusd':100000,'cost_confirmed':False}
        with (ROOT/'state/cognitive-usage.jsonl').open('a') as f:
            f.write(json.dumps(reserve)+'\n');f.flush();os.fsync(f.fileno())
        req=urllib.request.Request(gateway['base_url']+'/v1/responses',json.dumps(body).encode(),
             {'Authorization':'Bearer '+gateway['api_key'],'Content-Type':'application/json'})
        with urllib.request.urlopen(req,timeout=90) as r: response=json.load(r)
        billing=response.get('llmserver_billing',{})
        if billing.get('settlement_status')!='confirmed' or billing.get('currency')!='USD':
            raise RuntimeError('unconfirmed preflight bill; stop and reconcile before a new probe')
        now=datetime.now(timezone.utc).isoformat().replace('+00:00','Z')
        amount=int((Decimal(billing['charges']['total'])*1000000).to_integral_value(rounding=ROUND_CEILING))
        usage={'call_id':call_id,'lease_id':call_id,'time':now,'requested_model':role,'reasoning_effort':effort,
               'status':'completed','actual_microusd':amount,'cost_confirmed':True,'request_id':billing['request_id']}
        with (ROOT/'state/cognitive-usage.jsonl').open('a') as f:
            f.write(json.dumps(usage)+'\n');f.flush();os.fsync(f.fileno())
        calls=[x for x in response.get('output',[]) if x.get('type')=='function_call']
        valid=len(calls)==1 and calls[0]['name']=='gateway_probe' and json.loads(calls[0]['arguments'])=={'ok':True}
        record={'role':role,'model':model_id,'effort':effort,'valid':valid,'seconds':round(time.monotonic()-start,3),'billing':billing,'response':response}
        records.append(record)
        (ROOT/'evidence/model-preflight.json').write_text(json.dumps(records,ensure_ascii=False,indent=2))
        print(json.dumps({k:v for k,v in record.items() if k!='response'}),flush=True)
        if not valid: raise RuntimeError('function contract failed')
        p=billing['unit_prices']
        validated[model_id]={'validated_reasoning_efforts':[effort],
            'input_per_million_microusd':int(Decimal(p['input_per_million'])*1000000),
            'cached_input_per_million_microusd':int(Decimal(p.get('cached_input_per_million',p['input_per_million']))*1000000),
            'output_per_million_microusd':int(Decimal(p['output_per_million'])*1000000)}
    # A successful probe proves only the tested effort. The complete supported
    # effort arrays remain configuration facts sourced from llmserver, so a
    # one-effort smoke test must never overwrite the model catalog.
    (ROOT/'evidence/model-preflight-catalog.json').write_text(json.dumps(validated,indent=2))

if __name__=='__main__': main()
