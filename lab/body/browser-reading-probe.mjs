// Isolated real Chrome/MCP verification. Only loopback pages, no LLM or account writes.
import assert from 'node:assert/strict';
import {spawn, execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtempSync, existsSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createRequire} from 'node:module';

const helper = process.argv[2];
assert.ok(helper, 'pass the candidate browser helper');
const state = execFileSync('systemctl', ['show', 'hominal.service', '--property=ActiveState', '--value'], {encoding:'utf8'}).trim();
assert.ok(['inactive', 'failed'].includes(state), 'stop the life before an isolated probe');
const require = createRequire(import.meta.url);
const {chromium} = require('/agent/state/development/npm-global/lib/node_modules/@playwright/mcp/node_modules/playwright');
const root = mkdtempSync(join(tmpdir(), 'hominal-browser-reading-'));
const socket = join(root, 'browser.sock');
const env = {...process.env, HOMINAL_BROWSER_SOCKET:socket, HOMINAL_BROWSER_TIMEOUT_MS:'12000'};
const longText = Array.from({length:30}, (_, i) => `<div>row-${i}-${'x'.repeat(1000)}-END-${i}</div>`).join('');
const server = createServer((request, response) => {
  response.setHeader('Content-Type', 'text/html; charset=utf-8');
  if (request.url === '/error') {
    response.statusCode = 404; response.end('<title>Missing</title><main>404 Not Found</main>');
  } else if (request.url === '/long') {
    response.end('<title>Long</title><style>body{margin:0;font:1px/2px monospace}</style>'+longText);
  } else if (request.url === '/loading') {
    response.end('<title>Changing</title><main>1</main><div role="progressbar">Loading</div>');
  } else response.end('<title>One</title><body>1</body>');
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.connectOverCDP('http://127.0.0.1:9222', {timeout:5000});
const context = browser.contexts()[0], original = context.pages()[0];
let page, daemon;
const report = {completed:false, cases:[]};
const run = args => new Promise((resolve, reject) => {
  const child = spawn(process.execPath, [helper, ...args], {env, stdio:['ignore','pipe','pipe']});
  let stdout='', stderr=''; child.stdout.on('data', d => stdout+=d); child.stderr.on('data', d=>stderr+=d);
  child.on('error', reject);
  child.on('exit', code => code === 0 ? resolve(JSON.parse(stdout)) : reject(new Error(stderr || stdout)));
});
const call = (tool, args) => run(['call', tool, JSON.stringify(args)]);
const perform = async path => {
  const start=performance.now();
  const result=await run(['perform', JSON.stringify({schema:'hominal.organ-action/v1', action_id:'isolated-'+path,
    operation:'browser_navigate', input:JSON.stringify({url:base+path}), timeout_milliseconds:12000})]);
  assert.equal(result.status,'completed',JSON.stringify(result));
  report.cases.push({path, milliseconds:Math.round(performance.now()-start)}); return result;
};
try {
  page=await context.newPage(); await page.goto(base+'/one', {waitUntil:'domcontentloaded',timeout:5000});
  daemon=spawn(process.execPath,[helper,'serve'],{env,stdio:['ignore','ignore','inherit']});
  for(let i=0;i<60&&!existsSync(socket);i++) await new Promise(r=>setTimeout(r,50));
  assert.ok(existsSync(socket));
  const listed=await call('browser_tabs',{action:'list'});
  const line=listed.content.map(x=>x.text||'').join('\n').split('\n').find(x=>x.includes(base+'/one'));
  assert.ok(line); await call('browser_tabs',{action:'select',index:Number(line.match(/^- (\d+):/)[1])});
  for(let i=0;i<3;i++) {
    const one=await perform('/one');
    assert.ok(one.observation.objects.some(x=>x.content.endsWith('; 1')),JSON.stringify(one.observation));
    const snapshot=await call('browser_snapshot',{});
    assert.ok(snapshot.hominal_observation.objects.some(x=>x.content.endsWith('; 1')));
  }
  const long=await perform('/long');
  assert.ok(long.observation.objects.some(x=>x.content.includes('END-29')));
  const snapshot=await call('browser_snapshot',{});
  assert.ok(snapshot.hominal_observation.objects.some(x=>x.content.includes('END-29')));
  const loaded=await perform('/loading');
  assert.equal(loaded.observation.facts.visible_loading,'true');
  assert.ok(loaded.observation.objects.some(x=>x.content.includes('Loading')));
  // Change the fixture only after the first complete observation has returned.
  await page.evaluate(()=>{document.body.innerHTML='<main>Later content arrived</main>';});
  const later=await run(['observe']);
  assert.equal(later.facts.visible_loading,'false');
  assert.ok(later.objects.some(x=>x.content.includes('Later content arrived')));
  const missing=await perform('/error');
  assert.equal(JSON.parse(missing.observation.facts.navigation).http_status,404);
  assert.ok(missing.observation.objects.some(x=>x.content.includes('404 Not Found')));
  report.completed=true;
} finally {
  if(daemon){const exit=new Promise(r=>daemon.once('exit',r));daemon.kill('SIGTERM');await exit;}
  await page?.close().catch(()=>{}); await original?.bringToFront().catch(()=>{}); await browser.close();
  server.closeAllConnections(); await new Promise(r=>server.close(r)); rmSync(root,{recursive:true,force:true});
  process.stdout.write(JSON.stringify(report)+'\n');
}
