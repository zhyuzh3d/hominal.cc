// Lab-only truth/fixture control. Never installed in an organ's command surface.
const tabs = await (await fetch('http://127.0.0.1:9223/json/list')).json();
const tab = tabs.find(t => t.type === 'page' && t.url.startsWith('http://127.0.0.1:8760'));
if (!tab) throw new Error('isolated workbench tab unavailable');
const ws = new WebSocket(tab.webSocketDebuggerUrl);
await new Promise((ok, fail) => { ws.onopen=ok; ws.onerror=fail; });
let seq=0; const pending=new Map();
ws.onmessage=evt => { const r=JSON.parse(evt.data); if(pending.has(r.id)) { const p=pending.get(r.id); pending.delete(r.id); r.error?p.reject(r.error):p.resolve(r.result); } };
function call(method,params={}) {return new Promise((resolve,reject)=>{const id=++seq;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});}
try {
 if(process.argv[2]==='fullscreen') {
  await call('Page.bringToFront');
  const {windowId}=await call('Browser.getWindowForTarget',{targetId:tab.id});
  await call('Browser.setWindowBounds',{windowId,bounds:{windowState:'fullscreen'}});
  console.log(JSON.stringify({fullscreen:true}));
 } else if(process.argv[2]==='window') {
  const {windowId}=await call('Browser.getWindowForTarget',{targetId:tab.id});
  await call('Browser.setWindowBounds',{windowId,bounds:{windowState:'normal'}});
  await call('Browser.setWindowBounds',{windowId,bounds:JSON.parse(process.argv[3])});
  await call('Page.bringToFront');
  console.log(JSON.stringify(await call('Browser.getWindowBounds',{windowId})));
 } else {
  const r=await call('Runtime.evaluate',{expression:process.argv[2],returnByValue:true,awaitPromise:true});
  if(r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails));
  console.log(JSON.stringify(r.result.value));
 }
} finally {ws.close();}
