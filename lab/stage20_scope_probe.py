import sys,json,shlex
sys.path.insert(0,'lab');import stage20 as l
l.stopped();r=l.REMOTE;rel=json.loads((l.ARCHIVE/'release.json').read_text())['release'];scripts=r+'/releases/'+rel+'/body/tools/'
l.copy(l.REPO/'lab/stage20_window.py',r+'/tools/stage20_window.py')
env=shlex.join(['env']+[k+'='+v for k,v in l.ENV.items()]);geom=l.remote_json('/usr/bin/python3 '+r+'/tools/stage20_window.py --minimized false')
l.write_json(l.PRIVATE/'input-scope.json',{'window_id':geom['id'],'surface':'workbench'});l.copy(l.PRIVATE/'input-scope.json',r+'/services/input-scope.json')
probe="""import os,json,pathlib,urllib.request,urllib.error,time
r=pathlib.Path(os.environ['HOMINAL20_ROOT'])
req=urllib.request.Request('http://127.0.0.1:8766/input',json.dumps({'kind':'scope_probe_no_input','payload':{},'deadline':time.time()+10}).encode(),{'Content-Type':'application/json','X-Hominal-Capability':(r/'services/input.cap').read_text()})
try:
 with urllib.request.urlopen(req) as f:print(f.read().decode())
except urllib.error.HTTPError as e:print(e.read().decode())
"""
results={}
for value in ('true','false'):
 geom=l.remote_json('/usr/bin/python3 '+r+'/tools/stage20_window.py --minimized '+value)
 focus=l.remote_json(env+' /usr/bin/python3 '+scripts+'session_window.py')
 receipt=json.loads(l.remote(env+' /usr/bin/python3 -',input=probe))
 results[value]={'geometry':geom,'focus':focus,'receipt':receipt}
results['passed']=results['true']['geometry']['minimized'] and 'outside' in results['true']['receipt']['error'] and results['true']['receipt']['input_attempted'] is False and results['false']['receipt']['error']=='unsupported input'
l.write_json(l.ARCHIVE/'input-scope-validation-v2.json',results);l.copy(l.ARCHIVE/'input-scope-validation-v2.json',r+'/evidence/input-scope-validation-v2.json');print(json.dumps(results,ensure_ascii=False))
assert results['passed']
