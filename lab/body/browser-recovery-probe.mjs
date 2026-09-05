// Independent organ diagnostic. Run on the Ubuntu body while no life runs.
// Only creates two temporary loopback pages; no account interaction or LLM.
import assert from 'node:assert/strict';
import {spawn, execFileSync} from 'node:child_process';
import {mkdtempSync, existsSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:http';
import {createRequire} from 'node:module';

const helper = process.argv[2];
assert.ok(helper, 'pass the candidate hominal-browser.mjs path');
const lifeState = execFileSync('systemctl', ['show', 'hominal.service', '--property=ActiveState', '--value'], {encoding:'utf8'}).trim();
assert.ok(['inactive', 'failed'].includes(lifeState), 'the life service must be fully stopped before probing its browser');
const require = createRequire(import.meta.url);
const {chromium} = require(process.env.HOMINAL_PLAYWRIGHT_MODULE || '/agent/state/development/npm-global/lib/node_modules/@playwright/mcp/node_modules/playwright');
const root = mkdtempSync(join(tmpdir(), 'hominal-browser-recovery-'));
const socket = join(root, 'browser.sock');
const environment = {...process.env, HOMINAL_BROWSER_SOCKET:socket, HOMINAL_BROWSER_TIMEOUT_MS:'8000'};
const server = createServer((_request, response) => {
  response.setHeader('Content-Type', 'text/html');
  response.end('<title>Organ recovery probe</title><main><button id="save" onclick="document.getElementById(\'count\').textContent=++window.saved">Save locally</button><p id="count">0</p></main><script>window.saved=0</script>');
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
process.stderr.write('probe: connecting CDP\n');
const browser = await chromium.connectOverCDP('http://127.0.0.1:9222', {timeout:5000});
const context = browser.contexts()[0], original = context.pages()[0], pages = [];
let daemon;
const run = (args, extra = {}) => new Promise(resolve => {
  const started = performance.now();
  const child = spawn(process.execPath, [helper, ...args], {env:{...environment, ...extra}, stdio:['ignore','pipe','pipe']});
  let stdout='', stderr=''; child.stdout.on('data', d => {stdout+=d;}); child.stderr.on('data', d => {stderr+=d;});
  child.on('exit', code => resolve({code, stdout, stderr, milliseconds:performance.now()-started}));
});
const call = async (tool, args) => {
  const result = await run(['call', tool, JSON.stringify(args)]);
  assert.equal(result.code, 0, result.stderr); return result;
};
const report = {schema:'hominal.browser-recovery-probe/v1', completed:false, recovery_ms:[], ordinary_read_ms:[]};
try {
  const url = `http://127.0.0.1:${server.address().port}/same`;
  process.stderr.write('probe: creating loopback pages\n');
  for (let i=0;i<2;i++) {
    const page=await context.newPage(); pages.push(page);
    await page.goto(url,{waitUntil:'domcontentloaded',timeout:5000});
    await page.evaluate(marker=>{window.probeMarker=marker;},i===0?'alpha':'beta');
  }
  daemon=spawn(process.execPath,[helper,'serve'],{env:environment,stdio:['ignore','ignore','inherit']});
  for (let i=0;i<60&&!existsSync(socket);i++) await new Promise(resolve=>setTimeout(resolve,50));
  assert.ok(existsSync(socket));
  const listed=await call('browser_tabs',{action:'list'});
  const text=JSON.parse(listed.stdout).content.map(c=>c.text||'').join('\n');
  const indexes=text.split('\n').filter(line=>line.includes(url)).map(line=>Number(line.match(/^- (\d+):/)?.[1])).filter(Number.isInteger);
  assert.equal(indexes.length,2,text);
  let selected=false;
  for (const index of indexes) {
    await call('browser_tabs',{action:'select',index});
    const check=await call('browser_run_code_unsafe',{code:'async (page) => ({marker:await page.evaluate(()=>window.probeMarker)})'});
    const content=JSON.parse(check.stdout).content.map(c=>c.text||'').join('\n');
    if (/"marker"\s*:\s*"beta"/.test(content)) {selected=true;break;}
  }
  assert.ok(selected,'MCP must first select the independently identified beta page');
  process.stderr.write('probe: selected duplicate URL page\n');
  for (let i=0;i<3;i++) {
    const slow=run(['call','browser_run_code_unsafe',JSON.stringify({code:'async (page) => { await new Promise(resolve => setTimeout(resolve, 6000)); return page.url(); }'})],{HOMINAL_BROWSER_CALLER:'passive-perception'});
    let busy=false;
    for (let attempt=0;attempt<30;attempt++) {
      const health=await run(['health']);
      if (health.code===0 && JSON.parse(health.stdout).status==='busy') {busy=true;break;}
      await new Promise(resolve=>setTimeout(resolve,50));
    }
    assert.ok(busy,'passive request not active');
    const action={schema:'hominal.organ-action/v1',action_id:'isolated-reconnect-'+i,operation:'browser_click',input:JSON.stringify({target:'#save',element:'Save locally'}),timeout_milliseconds:8000};
    const performed=await run(['perform',JSON.stringify(action)]);
    assert.equal(performed.code,0,performed.stderr);
    const result=JSON.parse(performed.stdout);
    assert.equal(result.status,'completed',result.output);
    assert.notEqual((await slow).code,0,'slow read was not preempted');
    assert.equal(await pages[0].evaluate(()=>window.saved),0,'wrong physical tab was changed');
    assert.equal(await pages[1].evaluate(()=>window.saved),i+1,'chosen physical tab did not change');
    report.recovery_ms.push(Math.round(performed.milliseconds));
    process.stderr.write('probe: recovery '+i+' passed\n');
    const read=await call('browser_run_code_unsafe',{code:'async (page) => ({url:page.url(), saved:await page.evaluate(()=>window.saved)})'});
    report.ordinary_read_ms.push(Math.round(read.milliseconds));
  }
  report.completed=true;
} finally {
  if (daemon) {const exited=new Promise(resolve=>daemon.once('exit',resolve));daemon.kill('SIGTERM');await exited;}
  for (const page of pages) await page.close().catch(()=>{});
  await original?.bringToFront().catch(()=>{});
  await browser.close(); server.closeAllConnections(); await new Promise(resolve=>server.close(resolve));
  rmSync(root,{recursive:true,force:true});
  process.stdout.write(JSON.stringify(report)+'\n');
}
