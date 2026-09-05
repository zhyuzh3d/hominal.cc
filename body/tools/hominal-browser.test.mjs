import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { runInNewContext } from 'node:vm';

const helper = fileURLToPath(new URL('./hominal-browser.mjs', import.meta.url));

test('page feedback retains global receipts and links after a dialog closes', () => {
  const source = readFileSync(helper, 'utf8');
  const rect = {width: 300, height: 40, top: 20, bottom: 60, left: 0, right: 300};
  const node = (text, options = {}) => ({
    innerText: text, parentElement: options.parent || null,
    style: {display: 'block', visibility: 'visible', opacity: '1', ...options.style},
    href: options.href,
    closest: () => options.ariaHidden ? {} : null,
    getClientRects: () => [options.rect || rect],
    getBoundingClientRect: () => options.rect || rect,
    querySelectorAll: () => options.links || [],
  });
  const link = node('View', {href: 'https://example.test/created/42'});
  const hiddenParent = node('', {style: {opacity: '0'}});
  const toast = node('Your item was saved.\nView', {links: [link, node('bad', {href: 'javascript:void(0)'})]});
  let alerts = [
    node('hidden', {parent: hiddenParent}),
    node('offscreen', {rect: {...rect, top: 2000, bottom: 2040}}),
    node('aria-hidden', {ariaHidden: true}),
    toast, toast,
  ];
  // No main or dialog contains the notification. Only the document sees it.
  const read = runInNewContext(source.slice(source.indexOf('function readPageFeedback('), source.indexOf('const senseCode =')) + '\nreadPageFeedback', {
    document: {querySelectorAll: () => alerts},
    getComputedStyle: value => value.style, innerHeight: 600, innerWidth: 1000,
  });
  assert.deepEqual(Array.from(read()), ['Your item was saved. View · https://example.test/created/42']);
  alerts = Array.from({length: 120}, (_, index) => node(`${index} ` + 'x'.repeat(2000), {links: [link]}));
  const bounded = read();
  assert.equal(bounded.length, 8);
  assert.ok(bounded.every(value => value.length <= 1046 && value.includes(link.href)));
});

test('navigation identity preserves fragment, path slash and query', () => {
  const source = readFileSync(helper, 'utf8');
  const same = runInNewContext(source.slice(source.indexOf('function sameNavigationTarget('), source.indexOf('\nfunction actionEffect(')) + '\nsameNavigationTarget', {URL});
  assert.equal(same('https://EXAMPLE.test', 'https://example.test/'), true);
  assert.equal(same('https://example.test/examples/#first', 'https://example.test/examples/#second'), false);
  assert.equal(same('https://example.test/examples/', 'https://example.test/examples/#first'), false);
  assert.equal(same('https://example.test/a', 'https://example.test/a/'), false);
  assert.equal(same('https://example.test/?page=1', 'https://example.test/?page=2'), false);
  assert.equal(same('https://example.test/examples/#first', 'https://example.test/examples/#first'), true);
});

test('social capture reports the complete current viewport and loading without judging content', async () => {
  const source = readFileSync(helper, 'utf8');
  const code = runInNewContext(source.slice(source.indexOf('function readPageFeedback('), source.indexOf('\nfunction resultPayload')) + '\nsenseCode');
  const rect = {width: 300, height: 40, top: 20, bottom: 60, left: 0, right: 300};
  const node = (text, attributes = {}) => ({
    innerText: text, textContent: text, parentElement: null,
    getClientRects: () => [rect], getBoundingClientRect: () => rect,
    getAttribute: key => attributes[key] || null,
    closest: () => null, querySelectorAll: () => [],
  });
  let feedback = [node('1'), node('For you Following', {role: 'navigation'}), node('See new posts', {role: 'status'})];
  let busy = [node('', {role: 'progressbar'})];
  let objects = [], outcome = [];
  const document = {
    readyState: 'complete', title: 'Home / X', activeElement: null,
    body: node(''), documentElement: {scrollHeight: 600},
    createTreeWalker() {
      const nodes = [...feedback, ...objects, ...outcome].map(parentElement => ({parentElement, textContent: parentElement.innerText}));
      let i = 0; return {nextNode: () => nodes[i++] || null};
    },
    createRange: () => ({selectNodeContents() {}, getClientRects: () => [rect]}),
    querySelectorAll(selector) {
      if (selector.startsWith('[aria-live]')) return feedback;
      if (selector.startsWith('[role="progressbar"]')) return busy;
      if (selector === 'article') return objects;
      if (selector.includes('[data-testid="emptyState"]')) return outcome;
      return [];
    },
  };
  const sense = runInNewContext('(' + code + ')', {
    document, location: {href: 'https://x.com/home'}, innerHeight: 600, innerWidth: 1000,
    NodeFilter: {SHOW_TEXT: 4}, scrollY: 0,
    getComputedStyle: () => ({display: 'block', visibility: 'visible', opacity: '1'}),
  });
  const sample = () => sense({evaluate: callback => callback()});
  assert.equal((await sample()).readiness, 'loading', 'report the visible loading indicator');
  busy = [];
  assert.equal((await sample()).readiness, 'ready', 'do not invent loading because the page has no posts');
  assert.deepEqual(Array.from((await sample()).feedback), ['1', 'For you Following', 'See new posts'], 'retain feedback as observed facts');
  assert.deepEqual(Array.from((await sample()).blocks), ['1', 'For you Following', 'See new posts']);
  feedback = [node('1')];
  assert.deepEqual(Array.from((await sample()).blocks), ['1'], 'one character is legitimate complete content');

  const article = node('A concrete observation about the clock');
  article.querySelectorAll = selector => selector === 'a[href]'
    ? [{href: 'https://x.com/example/status/123', getAttribute: () => '/example/status/123'}] : [];
  objects = [article]; busy = [node('', {role: 'progressbar'})];
  assert.equal((await sample()).readiness, 'loading', 'report background loading alongside already readable objects');
  assert.ok((await sample()).blocks.includes('A concrete observation about the clock'));

  objects = []; busy = [];
  outcome = [node('No posts yet', {'data-testid': 'emptyState'})];
  let observed = await sample();
  assert.equal(observed.readiness, 'ready');
  assert.ok(observed.blocks.includes('No posts yet'));
  outcome = [node('The request could not be completed', {role: 'alert'})];
  observed = await sample();
  assert.equal(observed.readiness, 'ready');
  assert.ok(observed.blocks.includes('The request could not be completed'));
  busy = [node('', {role: 'progressbar'})];
  assert.equal((await sample()).readiness, 'loading', 'an unrelated alert does not settle a still-loading empty body');
});

test('document sense reads rendered text across markup, preserves viewport and bounds', async () => {
  const source = readFileSync(helper, 'utf8');
  const sense = runInNewContext(source.slice(source.indexOf('function readPageFeedback('), source.indexOf('\nfunction resultPayload')) + '\nsenseCode');
  const rect = {width: 100, height: 20, top: 20, bottom: 40, left: 0, right: 100};
  const element = (parent = null, display = 'block', extra = {}) => ({
    parentElement: parent,
    style: {display, visibility: 'visible', opacity: '1', ...extra},
    closest(selector) {
      for (let ancestor = this; ancestor; ancestor = ancestor.parentElement) {
        if (ancestor.navigation && selector.includes('nav')) return ancestor;
      }
      return null;
    },
  });
  const root = element();
  const main = element(root);
  const card = element(root);
  const inline = element(card, 'inline');
  const hidden = element(root, 'block', {opacity: '0'});
  const hiddenChild = element(hidden, 'inline');
  const navigation = element(root);
  navigation.navigation = true;
  let nodes = [
    {textContent: 'Navigation toolbar noise', parentElement: element(navigation), rect},
    {textContent: 'Tesla releases', parentElement: inline, rect},
    {textContent: ' quarterly results', parentElement: element(card, 'inline'), rect},
    {textContent: 'A visible search result link', parentElement: element(root), rect},
    {textContent: 'Hidden text', parentElement: hiddenChild, rect},
    {textContent: 'Below the viewport', parentElement: element(main), rect: {...rect, top: 2000, bottom: 2020}},
  ];
  const document = {
    title: 'Public document', readyState: 'complete', body: root,
    documentElement: {scrollHeight: 2000},
    querySelector: () => main, querySelectorAll: () => [],
    createTreeWalker(scope) {
      const included = nodes.filter(node => {
        for (let ancestor = node.parentElement; ancestor; ancestor = ancestor.parentElement) {
          if (ancestor === scope) return true;
        }
        return false;
      });
      let index = 0;
      return {nextNode: () => included[index++] || null};
    },
    createRange() {let current; return {selectNodeContents: node => {current = node;}, getClientRects: () => [current.rect]};},
  };
  const fn = runInNewContext('(' + sense + ')', {
    document, location: {href: 'https://example.test/document'},
    NodeFilter: {SHOW_TEXT: 4}, getComputedStyle: node => node.style,
    innerHeight: 600, innerWidth: 1280, scrollY: 0,
  });
  const observed = await fn({evaluate: callback => callback()});
  assert.deepEqual(Array.from(observed.blocks), ['Navigation toolbar noise', 'Tesla releases quarterly results', 'A visible search result link']);
  assert.equal(observed.readiness, 'ready');
  // Conventional in-main text remains visible too; the fix does not exchange
  // one incomplete sampling region for a different one.
  nodes.push({textContent: 'Ordinary main content', parentElement: element(main), rect});
  assert.ok((await fn({evaluate: callback => callback()})).blocks.includes('Ordinary main content'));
  nodes = Array.from({length: 30}, (_, i) => ({textContent: `row ${i} ` + 'x'.repeat(1000), parentElement: element(root), rect}));
  const bounded = await fn({evaluate: callback => callback()});
  assert.equal(bounded.blocks.length, 30);
  assert.ok(bounded.blocks.every(block => block.length > 1000));
  assert.equal(bounded.coverage.limited, false);
  nodes = Array.from({length: 6001}, () => ({textContent: 'Offscreen', parentElement: element(root), rect: {...rect, top: 2000, bottom: 2020}}));
  const limited = await fn({evaluate: callback => callback()});
  assert.equal(limited.coverage.visited_text_nodes, 6001);
  assert.equal(limited.coverage.limited, false);
  nodes = [{textContent: 'x'.repeat(1000001), parentElement: root, rect}];
  await assert.rejects(() => fn({evaluate: callback => callback()}), /read_incomplete/);
  nodes = [];
  const empty = await fn({evaluate: callback => callback()});
  assert.equal(empty.readiness, 'ready');
  assert.equal(empty.coverage.limited, false);
});

function run(arguments_, environment) {
  return new Promise(resolve => {
    const started = performance.now();
    const child = spawn(process.execPath, [helper, ...arguments_], {
      env: { ...process.env, ...environment },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', value => { stdout += value; });
    child.stderr.on('data', value => { stderr += value; });
    child.once('exit', (code, signal) => resolve({
      code,
      signal,
      stdout,
      stderr,
      elapsedMilliseconds: performance.now() - started,
    }));
  });
}

test('page identity acquisition and restoration share the remaining operation deadline', async () => {
  const source = readFileSync(helper, 'utf8');
  let clock = 10000;
  const Clock = class extends Date { static now() { return clock; } };
  const Connection = runInNewContext(source.slice(source.indexOf('class MCPConnection {'), source.indexOf('\nasync function createConnection')) + '\nMCPConnection', {
    Date: Clock,
    resultPayload: result => result.payload,
  });
  const connection = Object.create(Connection.prototype);
  const budgets = [];
  let phase = 'cold';
  connection.request = async (method, params, remaining) => {
    budgets.push(remaining);
    const code = params.arguments?.code || '';
    const duration = phase === 'cold' ? 1800 : code.includes('hominalRestoreTarget') ? 2800 : 100;
    assert.ok(remaining >= duration, 'an inner timer aborted a valid step before its operation deadline');
    clock += duration;
    return {result: {payload: code.includes('hominalRestoreTarget') ? {index: 1} : {target_id: 'original-target'}}};
  };
  assert.equal((await connection.capturePosition(5000)).target_id, 'original-target');
  phase = 'restore';
  budgets.length = 0;
  await connection.restorePosition({target_id: 'original-target'}, 5000);
  assert.deepEqual(budgets, [5000, 2200, 2100], 'each step must use the shrinking shared budget');
  assert.equal(connection.positionRestored, true);
});

test('reconnecting a preempted sense preserves page identity and invalidates session handles', {timeout: 15000}, async () => {
  const root = mkdtempSync(join(tmpdir(), 'hominal-browser-position-'));
  const socket = join(root, 'browser.sock');
  const fakeMCP = join(root, 'fake-mcp.mjs');
  const writes = join(root, 'writes');
  const missing = join(root, 'missing');
  writeFileSync(fakeMCP, `#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {appendFileSync, existsSync} from 'node:fs';
let selected = 0;
let saved = 0;
const reply = (id, payload) => process.stdout.write(JSON.stringify({jsonrpc:'2.0', id, result:payload})+'\\n');
const result = value => ({content:[{type:'text',text:'### Result\\n'+JSON.stringify(value)}]});
createInterface({input:process.stdin}).on('line', line => {
  const m = JSON.parse(line); if (!m.id) return;
  if(m.method==='initialize') return reply(m.id,{});
  if(m.method==='tools/list') return reply(m.id,{tools:['browser_tabs','browser_run_code_unsafe','browser_click','browser_find','browser_snapshot','fake_slow'].map(name=>({name,inputSchema:{type:'object'}}))});
  const {name,arguments:a={}}=m.params;
  if(name==='fake_slow') return setTimeout(()=>reply(m.id,result('done')),800);
  if(name==='browser_tabs') {
    if(a.action==='select') selected=Number(a.index);
    return reply(m.id,{content:[{type:'text',text:'### Open tabs\\n- 0: '+(selected===0?'(current) ':'')+'[Same](https://example.test/same)\\n- 1: '+(selected===1?'(current) ':'')+'[Same](https://example.test/same)'}]});
  }
  if(name==='browser_run_code_unsafe') {
    const code=String(a.code||'');
    if(code.includes('hominalBrowserPosition')) return reply(m.id,result({target_id:'target-'+selected}));
    if(code.includes('hominalRestoreTarget')) return reply(m.id,result({index:existsSync(${JSON.stringify(missing)})?-1:1}));
    if(code.includes('const hominalSense =')) return reply(m.id,result({url:'https://example.test/same',title:'Same',readyState:'complete',readiness:'semantic_ready',kind:'document',blocks:['Body '+selected+' saved '+saved],controls:[],feedback:[]}));
    return reply(m.id,result({body:selected}));
  }
  if(name==='browser_click') { saved++; appendFileSync(${JSON.stringify(writes)},String(selected)+'\\n'); }
  return reply(m.id,result({body:selected,ref:'f123ab'}));
});
`, {mode: 0o755});
  const environment = {HOMINAL_BROWSER_SOCKET:socket,HOMINAL_PLAYWRIGHT_MCP_COMMAND:fakeMCP,HOMINAL_BROWSER_TIMEOUT_MS:'1800'};
  const daemon=spawn(process.execPath,[helper,'serve'],{env:{...process.env,...environment},stdio:['ignore','ignore','pipe']});
  let stderr=''; daemon.stderr.on('data',v=>{stderr+=v;});
  const call = async (tool,args={}) => {
    const out=await run(['call',tool,JSON.stringify(args)],environment);
    assert.equal(out.code,0,out.stderr); return JSON.parse(out.stdout);
  };
  const perform = async args => {
    const out=await run(['perform',JSON.stringify({schema:'hominal.organ-action/v1',action_id:'test-context',operation:'browser_click',input:JSON.stringify(args),timeout_milliseconds:1800})],environment);
    assert.equal(out.code,0,out.stderr); return JSON.parse(out.stdout);
  };
  const preempt = async action => {
    const slow=run(['call','fake_slow','{}'],{...environment,HOMINAL_BROWSER_CALLER:'passive-perception'});
    for (let i=0;i<30;i++) {
      const health=await run(['health'],environment);
      if(JSON.parse(health.stdout).status==='busy') break;
      await new Promise(resolve=>setTimeout(resolve,10));
      if(i===29) assert.fail('passive request never started');
    }
    const result=await action();
    assert.notEqual((await slow).code,0,'intentional work must still preempt the bounded passive request');
    return result;
  };
  try {
    await waitFor(()=>existsSync(socket));
    await call('browser_tabs',{action:'select',index:1});
    const started=performance.now();
    const clicked=await preempt(()=>perform({target:'#save',element:'save'}));
    assert.equal(clicked.status,'completed',clicked.output);
    assert.equal(readFileSync(writes,'utf8'),'1\n','the resumed action touched another page with the same URL');
    assert.ok(performance.now()-started<1400,'context recovery must remain bounded');

    // A ref is tied to the old MCP session even when its tab survives.
    const stale=await perform({target:'f123ab',element:'save'});
    assert.equal(stale.status,'failed');
    assert.match(stale.output,/fresh.*browser_find|browser_find.*fresh/i);
    assert.equal(readFileSync(writes,'utf8'),'1\n');
    const oldIndex=await run(['call','browser_tabs','{"action":"select","index":1}'],environment);
    assert.notEqual(oldIndex.code,0,'an index from the previous session must be listed again');
    await call('browser_tabs',{action:'list'});
    await call('browser_tabs',{action:'select',index:1});
    await call('browser_find',{text:'save'});
    assert.equal((await perform({target:'f123ab',element:'save'})).status,'completed');
    assert.equal(readFileSync(writes,'utf8'),'1\n1\n');

    // A vanished target is not permission to click the adapter's default page.
    writeFileSync(missing,'yes');
    const vanished=await preempt(()=>perform({target:'#save',element:'save'}));
    assert.equal(vanished.status,'failed');
    assert.match(vanished.output,/original.*page|page.*unavailable/i);
    assert.equal(readFileSync(writes,'utf8'),'1\n1\n');
    const observed=await run(['observe'],environment);
    assert.equal(observed.code,0,observed.stderr);
    assert.match(observed.stdout,/Body 0/,'fresh sensing must make the remaining real scene available');
    assert.equal((await perform({target:'#save',element:'save'})).status,'completed');
    assert.equal(readFileSync(writes,'utf8'),'1\n1\n0\n');
  } finally {
    const exited=new Promise(resolve=>daemon.once('exit',resolve)); daemon.kill('SIGTERM'); await exited;
    rmSync(root,{recursive:true,force:true});
  }
  assert.equal(stderr,'');
});

async function waitFor(predicate, timeoutMilliseconds = 3000) {
  const deadline = Date.now() + timeoutMilliseconds;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw new Error('condition did not become true before timeout');
}

test('health bypasses passive work and an intentional action preempts and recovers it', { timeout: 10000 }, async () => {
  const root = mkdtempSync(join(tmpdir(), 'hominal-browser-test-'));
  const socket = join(root, 'browser.sock');
  const fakeMCP = join(root, 'fake-mcp.mjs');
  writeFileSync(fakeMCP, `#!/usr/bin/env node
import { createInterface } from 'node:readline';
const lines = createInterface({input: process.stdin});
const reply = value => process.stdout.write(JSON.stringify(value) + '\\n');
let snapshotCalls = 0;
let senseCalls = 0;
let orientCalls = 0;
let selectedTab = 0;
let slowSenseRemaining = 0;
let stuckSense = false;
let mediumSense = false;
let senseReadFailure = false;
let composerContentLength = 12;
let composerSenseRemaining = 0;
let alreadyAtTargetSenseRemaining = 0;
let navigationURL = '';
lines.on('line', line => {
  const message = JSON.parse(line);
  if (!message.id) return;
  if (message.method === 'initialize') {
    reply({jsonrpc:'2.0',id:message.id,result:{protocolVersion:'2025-03-26',capabilities:{}}});
    return;
  }
  if (message.method === 'tools/list') {
    reply({jsonrpc:'2.0',id:message.id,result:{tools:[
      {name:'fake_action',inputSchema:{type:'object',properties:{delay_ms:{type:'number'}},additionalProperties:false}},
      {name:'browser_snapshot',inputSchema:{type:'object',properties:{test_medium:{type:'boolean'},test_read_failure:{type:'boolean'}},additionalProperties:true}},
      {name:'browser_find',inputSchema:{type:'object',properties:{text:{type:'string'},regex:{type:'string'}},additionalProperties:false}},
      {name:'browser_navigate',inputSchema:{type:'object',properties:{url:{type:'string'}},additionalProperties:false}},
      {name:'browser_click',inputSchema:{type:'object',properties:{target:{type:'string'},element:{type:'string'},button:{type:'string'}},additionalProperties:false}},
      {name:'browser_type',inputSchema:{type:'object',properties:{target:{type:'string'},text:{type:'string'}},additionalProperties:false}},
      {name:'browser_fill_form',inputSchema:{type:'object',properties:{fields:{type:'array'}},additionalProperties:false}},
      {name:'browser_wait_for',inputSchema:{type:'object',properties:{time:{type:'number'}},additionalProperties:false}},
      {name:'browser_tabs',inputSchema:{type:'object',properties:{action:{type:'string'},index:{type:'number'}},additionalProperties:false}},
      {name:'browser_run_code_unsafe',inputSchema:{type:'object',properties:{code:{type:'string'}},additionalProperties:false}}
    ]}});
    return;
  }
  if (message.method === 'tools/call') {
    if (message.params?.name === 'browser_snapshot') {
      snapshotCalls += 1;
      if (message.params?.arguments?.test_read_failure) senseReadFailure = true;
      if (message.params?.arguments?.test_medium) {
        mediumSense = true;
        // Small enough to remain uncompressed inside the daemon, but rich in
        // escaped characters so the enclosing action result exceeds the
        // snapshot threshold.  The public writer must preserve the envelope.
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Page\\n- Page URL: https://example.test/medium\\n- Page Title: Medium\\n' + '\"'.repeat(7000)}]}});
        return;
      }
      if (snapshotCalls >= 4) {
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Page\\n- Page URL: https://en.wikipedia.org/wiki/Gisele\\n- Page Title: Gisele - Wikipedia\\n### Snapshot\\n- main [ref=k1]:\\n  - heading "Gisele Grimm" [ref=k2]\\n  - link "Life" [ref=k3]\\n  - link "Career" [ref=k4]'}]}});
        return;
      }
      const drift = snapshotCalls > 1 ? '3 minutes ago A stable idea 5 likes Image' : '2 minutes ago A stable idea 4 likes Image';
      const noise = '- generic [ref=e9]: navigation noise\\n'.repeat(1200);
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Page\\n- Page URL: https://x.com/home\\n- Page Title: Home / X\\n### Snapshot\\n- heading "To view keyboard shortcuts" [ref=e0]\\n' + noise + '- article "Alice @alice ' + drift + ' Direct URL: https://x.com/alice/status/123" [ref=e1]'}]}});
      return;
    }
    if (message.params?.name === 'browser_run_code_unsafe') {
      const code = String(message.params?.arguments?.code || '');
      if (code.includes('hominalBrowserPosition')) {
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n'+JSON.stringify({target_id:'target-'+selectedTab})}]}});
        return;
      }
      if (code.includes('hominalRestoreTarget')) {
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n'+JSON.stringify({index:code.includes('"target-1"')?1:0})}]}});
        return;
      }
      try {
        new Function('return (' + code + ');');
      } catch (error) {
        reply({jsonrpc:'2.0',id:message.id,error:{code:-32602,message:String(error)}});
        return;
      }
      if (code.includes('const hominalSense =')) {
        if (senseReadFailure) {
          senseReadFailure = false;
          reply({jsonrpc:'2.0',id:message.id,result:{isError:true,content:[{type:'text',text:'read_incomplete: capture failed'}]}});
          return;
        }
        if (mediumSense) {
          mediumSense = false;
          reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n'+JSON.stringify({url:'https://example.test/medium',title:'Medium',readyState:'complete',readiness:'ready',kind:'document',blocks:['"'.repeat(7000)+' FULL_CAPTURE_END'],feedback:[],coverage:{limited:false}})}]}});
          return;
        }
        senseCalls += 1;
        const payload = stuckSense
          ? {url:'https://x.com/stuck',title:'X',readyState:'loading',readiness:'loading',kind:'social',objects:[],controls:[],feedback:[]}
          : slowSenseRemaining > 0
          ? (slowSenseRemaining -= 1, {url:'https://x.com/slow',title:'X',readyState:'loading',readiness:'loading',kind:'social',objects:[],controls:[],feedback:[]})
          : composerSenseRemaining > 0
          ? (composerSenseRemaining -= 1, {url:'https://x.com/home',title:'Home / X',readyState:'complete',readiness:'semantic_ready',kind:'social',objects:[{text:'Alice @alice 2 minutes ago A stable idea 4 likes',url:'https://x.com/alice/status/123',controls:[{role:'button',name:'4 Likes. Like',disabled:false,target:'article:has(a[href="/alice/status/123"]) [data-testid="like"]'}]}],controls:[{role:'textbox',name:'Post text',disabled:false,contentLength:composerContentLength,target:'[role="dialog"] [data-testid="tweetTextarea_0"]'}],feedback:[]})
          : alreadyAtTargetSenseRemaining > 0
          ? (alreadyAtTargetSenseRemaining -= 1, {url:'https://x.com/home',title:'Home / X',readyState:'complete',readiness:'semantic_ready',kind:'social',objects:[{text:'Alice @alice 2 minutes ago A stable idea 4 likes',url:'https://x.com/alice/status/123',controls:[{role:'button',name:'4 Likes. Like',disabled:false,target:'article:has(a[href="/alice/status/123"]) [data-testid="like"]'}]}],controls:[],feedback:[]})
          : navigationURL
          ? {url:navigationURL,title:'Gisele - Wikipedia',readyState:'complete',readiness:'semantic_ready',kind:'document',scrollY:600,maxScrollY:2400,blocks:['Gisele Grimm was a German scholar.','Her career crossed several fields.']}
          : senseCalls >= 3
          ? {url:'https://en.wikipedia.org/wiki/Gisele',title:'Gisele - Wikipedia',readyState:'complete',readiness:'semantic_ready',kind:'document',scrollY:600,maxScrollY:2400,blocks:['Gisele Grimm was a German scholar.','Her career crossed several fields.']}
          : {url:'https://x.com/home',title:'Home / X',readyState:'complete',readiness:'semantic_ready',kind:'social',objects:[{text:'Alice @alice 2 minutes ago A stable idea 4 likes',url:'https://x.com/alice/status/123',controls:[{role:'button',name:'4 Likes. Like',disabled:false,target:'article:has(a[href="/alice/status/123"]) [data-testid="like"]'}]}],controls:[{role:'textbox',name:'Post text',disabled:false,contentLength:composerContentLength,target:'[role="dialog"] [data-testid="tweetTextarea_0"]'}],feedback:[]};
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify(payload)}]}});
        return;
      }
      if (code.includes('const hominalNavigation =')) {
        const url = code.includes('https://x.com/stuck') ? 'https://x.com/stuck'
          : code.includes('https://x.com/slow') ? 'https://x.com/slow'
          : (code.match(/requested:\\s*"([^"]+)"/)?.[1] || 'https://en.wikipedia.org/wiki/Gisele');
        if (code.includes('already_at_target') && url === 'https://x.com/home' && !navigationURL) {
          alreadyAtTargetSenseRemaining = 1;
          reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify({before:'https://x.com/home',requested:url,status:'already_at_target',url})}]}});
          return;
        }
        stuckSense = code.includes('https://x.com/stuck');
        if (code.includes('https://x.com/slow')) slowSenseRemaining = 3;
        navigationURL = code.includes('https://x.com/slow') ? 'https://en.wikipedia.org/wiki/Gisele' : url;
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify({before:'https://x.com/home',requested:url,status:'domcontentloaded',url})}]}});
        return;
      }
      if (code.includes('const viewportBlocks =')) {
        const payload = {url:'https://en.wikipedia.org/wiki/Gisele',title:'Gisele - Wikipedia',scrollY:600,maxScrollY:2400,blocks:['Gisele Grimm was a German scholar.','Her career crossed several fields.']};
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify(payload)}]}});
        return;
      }
      if (code.includes('const objects =')) {
        const payload = {objects:[{text:'Alice @alice 2 minutes ago A stable idea 4 likes',url:'https://x.com/alice/status/123'}],controls:[{role:'textbox',name:'Post text',disabled:false,contentLength:12,target:'[role="dialog"] [data-testid="tweetTextarea_0"]'}],feedback:[]};
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify(payload)}]}});
        return;
      }
      if (code.includes('const active =')) {
        orientCalls += 1;
        const payload = orientCalls === 1
          ? {url:'https://x.com/home',preserved:true}
          : orientCalls === 2
            ? {url:'https://x.com/home',before:0,after:0,exhausted:true}
            : {url:'https://x.com/home',before:(orientCalls - 3) * 600,after:(orientCalls - 2) * 600,mode:'scroll'};
        reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n' + JSON.stringify(payload)}]}});
        return;
      }
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n{"preserved":true}'}]}});
      return;
    }
    if (message.params?.name === 'browser_tabs') {
      if (message.params?.arguments?.action === 'select') selectedTab = Number(message.params.arguments.index);
      const first = selectedTab === 0 ? '(current) ' : '';
      const second = selectedTab === 1 ? '(current) ' : '';
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Open tabs\\n- 0: ' + first + '[Home / X](https://x.com/home)\\n- 1: ' + second + '[Wikipedia](https://en.wikipedia.org/wiki/Main_Page)'}]}});
      return;
    }
    if (message.params?.name === 'browser_type' && message.params?.arguments?.text === 'force browser failure') {
      reply({jsonrpc:'2.0',id:message.id,result:{isError:true,content:[{type:'text',text:'### Error\\nThe field rejected this input.'}]}});
      return;
    }
    if (message.params?.name === 'browser_navigate') {
      stuckSense = message.params?.arguments?.url === 'https://x.com/stuck';
      if (message.params?.arguments?.url === 'https://x.com/slow') slowSenseRemaining = 3;
    }
    if (message.params?.name === 'browser_find' || message.params?.name === 'browser_navigate' || message.params?.name === 'browser_click' || message.params?.name === 'browser_type' || message.params?.name === 'browser_fill_form' || message.params?.name === 'browser_wait_for') {
      if (message.params?.name === 'browser_fill_form') {
        composerContentLength = String(message.params?.arguments?.fields?.[0]?.value || '').length;
        composerSenseRemaining = 1;
      }
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:JSON.stringify(message.params.arguments)}]}});
      return;
    }
    const delay = Number(message.params?.arguments?.delay_ms || 0);
    setTimeout(() => reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'ok'}]}}), delay);
  }
});
`, { mode: 0o755 });
  chmodSync(fakeMCP, 0o755);
  const baseEnvironment = {
    HOMINAL_BROWSER_SOCKET: socket,
    HOMINAL_PLAYWRIGHT_MCP_COMMAND: fakeMCP,
    HOMINAL_INSTANCE_ROOT: root,
  };
  const daemon = spawn(process.execPath, [helper, 'serve'], {
    env: { ...process.env, ...baseEnvironment, HOMINAL_BROWSER_TIMEOUT_MS: '1000' },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let daemonError = '';
  daemon.stderr.on('data', value => { daemonError += value; });
  try {
    await waitFor(() => existsSync(socket));
    const described = await run(['describe'], baseEnvironment);
    assert.equal(described.code, 0, described.stderr);
    const description = JSON.parse(described.stdout);
    assert.deepEqual(description.capabilities.slice(0, 4), ['observe', 'orient', 'perform', 'cancel']);
    assert.ok(description.operations.includes('browser_snapshot'));
    assert.ok(description.operations.includes('browser_click'));
	assert.equal(description.operation_inputs.browser_snapshot, '{}');
	assert.match(description.operation_inputs.browser_click, /target/);
    assert.ok(!description.operations.includes('observe'), 'passive host capability leaked into the action catalog');
    assert.match(description.guidance, /只从 operations 选择/);
    assert.match(description.guidance, /time_seconds/);

    const observed = await run(['observe'], baseEnvironment);
    assert.equal(observed.code, 0, observed.stderr);
    const observation = JSON.parse(observed.stdout);
    assert.equal(observation.schema, 'hominal.organ-observation/v1');
    assert.equal(observation.organ_id, 'browser');
    assert.equal(observation.surface_id, 'chrome.current_page');
    assert.ok(observation.context.some(value => value.includes('独立视觉解码器')));
    assert.equal(observation.objects.length, 2, JSON.stringify(observation));
    assert.match(observation.objects[0].content, /Active control/);
    assert.match(observation.objects[0].content, /tweetTextarea_0/);
    assert.match(observation.objects[1].content, /A stable idea/);
    assert.match(observation.objects[1].content, /Available actions on this object/);
    assert.match(observation.objects[1].content, /data-testid.*like/);
    assert.match(observation.objects[1].content, /媒体感知边界/);
    assert.match(observation.objects[1].content, /Direct URL: https:\/\/x\.com\/alice\/status\/123$/);
    assert.match(observation.objects[1].content, /2 minutes ago/);
    assert.match(observation.objects[1].content, /4 likes/);
    assert.doesNotMatch(JSON.stringify(observation.objects), /keyboard shortcuts/);
    const observedAgain = await run(['observe'], baseEnvironment);
    assert.equal(observedAgain.code, 0, observedAgain.stderr);
    const secondObservation = JSON.parse(observedAgain.stdout);
    assert.equal(secondObservation.objects[1].id, observation.objects[1].id, 'presentation drift changed the object identity');

    const semanticallyFilled = await run(['call', 'browser_fill_form', JSON.stringify({
      fields: [{ name: 'Post text', type: 'textbox', value: 'resolved from current perception' }],
    })], baseEnvironment);
    assert.equal(semanticallyFilled.code, 0, semanticallyFilled.stderr);
    const semanticFillOutput = JSON.stringify(JSON.parse(semanticallyFilled.stdout));
    assert.match(semanticFillOutput, /tweetTextarea_0/);
    assert.match(semanticFillOutput, /resolved from current perception/);

    const alreadyThere = await run(['call', 'browser_navigate', JSON.stringify({url: 'https://x.com/home'})], baseEnvironment);
    assert.equal(alreadyThere.code, 0, alreadyThere.stderr);
    assert.match(alreadyThere.stdout, /already_at_target/);
    assert.match(alreadyThere.stdout, /A stable idea/, 'a satisfied navigation destroyed the current semantic surface');

    const hydratedNavigation = await run(['call', 'browser_navigate', JSON.stringify({url: 'https://x.com/slow'})], baseEnvironment);
    assert.equal(hydratedNavigation.code, 0, hydratedNavigation.stderr);
    assert.match(hydratedNavigation.stdout, /Gisele Grimm/, 'dynamic social navigation returned its pre-hydration empty surface');

    const stuckAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-stuck',
      operation: 'browser_navigate',
      input: JSON.stringify({ url: 'https://x.com/stuck' }),
      timeout_milliseconds: 500,
    };
    const stuckNavigation = await run(['perform', JSON.stringify(stuckAction)], baseEnvironment);
    assert.equal(stuckNavigation.code, 0, stuckNavigation.stderr);
    const stuckResult = JSON.parse(stuckNavigation.stdout);
    assert.equal(stuckResult.status, 'unknown');
    assert.equal(stuckResult.effect, 'unknown');
    assert.equal(stuckResult.observation.facts.readiness, '"loading"');
    assert.match(stuckResult.summary, /未能确认页面达到稳定后置状态/);

    const recoveredFromStuck = await run(['call', 'browser_navigate', JSON.stringify({url: 'https://example.test/recovered'})], baseEnvironment);
    assert.equal(recoveredFromStuck.code, 0, recoveredFromStuck.stderr);
    assert.match(recoveredFromStuck.stdout, /Gisele Grimm/);

    const directSnapshot = await run(['call', 'browser_snapshot', '{}'], baseEnvironment);
    assert.equal(directSnapshot.code, 0, directSnapshot.stderr);
    assert.ok(directSnapshot.stdout.length < 32000, 'the adapter leaked an unbounded accessibility tree into shell Reality');
    assert.match(directSnapshot.stdout, /Gisele Grimm/);
    assert.doesNotMatch(directSnapshot.stdout, /navigation noise/);

    const misleadingSnapshot = await run(['call', 'browser_snapshot', JSON.stringify({
      url: 'https://x.com/home',
    })], baseEnvironment);
    assert.notEqual(misleadingSnapshot.code, 0, 'an ignored snapshot URL was reported as a successful action');
    assert.match(misleadingSnapshot.stderr, /does not accept url/);

    const knowledgeObserved = await run(['observe'], baseEnvironment);
    assert.equal(knowledgeObserved.code, 0, knowledgeObserved.stderr);
    const knowledge = JSON.parse(knowledgeObserved.stdout);
    assert.equal(knowledge.objects.length, 1, JSON.stringify(knowledge));
    assert.ok(knowledge.objects.some(object => /Active viewport:.*scroll 600\/2400/.test(object.content)));
    assert.ok(knowledge.objects.some(object => /Gisele Grimm was a German scholar/.test(object.content)));

    const oriented = await run(['orient'], baseEnvironment);
    assert.equal(oriented.code, 0, oriented.stderr);
    assert.equal(JSON.parse(oriented.stdout).status, 'preserved');

    const rotated = await run(['orient'], baseEnvironment);
    assert.equal(rotated.code, 0, rotated.stderr);
    const rotatedBody = JSON.parse(rotated.stdout);
    assert.equal(rotatedBody.status, 'moved');
    assert.match(rotatedBody.detail, /en\.wikipedia\.org/);

    for (let index = 0; index < 3; index += 1) {
      const scrolled = await run(['orient'], baseEnvironment);
      assert.equal(scrolled.code, 0, scrolled.stderr);
      assert.match(JSON.parse(scrolled.stdout).detail, /mode.*scroll/);
    }
    const rotatedAfterBoundedViewportScan = await run(['orient'], baseEnvironment);
    assert.equal(rotatedAfterBoundedViewportScan.code, 0, rotatedAfterBoundedViewportScan.stderr);
    const boundedRotation = JSON.parse(rotatedAfterBoundedViewportScan.stdout);
    assert.equal(boundedRotation.status, 'moved');
    assert.match(boundedRotation.detail, /existing_tab/);
    assert.match(boundedRotation.detail, /x\.com\/home/);

    const slowPromise = run(['call', 'fake_action', '{"delay_ms":5000}'], {
      ...baseEnvironment,
      HOMINAL_BROWSER_TIMEOUT_MS: '3000',
      HOMINAL_BROWSER_CALLER: 'passive-perception',
    });
    await new Promise(resolve => setTimeout(resolve, 40));
    const health = await run(['health'], {
      ...baseEnvironment,
      HOMINAL_BROWSER_TIMEOUT_MS: '100',
    });
    assert.equal(health.code, 0, health.stderr);
    assert.ok(health.elapsedMilliseconds < 300, `health waited ${health.elapsedMilliseconds}ms behind the action queue`);
    const healthBody = JSON.parse(health.stdout);
    assert.equal(healthBody.schema, 'hominal.organ-health/v1');
    assert.equal(healthBody.id, 'browser');
    assert.equal(healthBody.accepting, true);
    assert.equal(healthBody.status, 'busy');

    const recoveredPromise = run(['call', 'fake_action', '{"delay_ms":0}'], {
      ...baseEnvironment,
      HOMINAL_BROWSER_TIMEOUT_MS: '1000',
      HOMINAL_BROWSER_CALLER: 'intentional-action',
    });
    const slow = await slowPromise;
    assert.notEqual(slow.code, 0, 'the bounded slow request unexpectedly succeeded');
    assert.ok(slow.elapsedMilliseconds < 700, `slow request remained stuck for ${slow.elapsedMilliseconds}ms`);

    const recovered = await recoveredPromise;
    assert.equal(recovered.code, 0, recovered.stderr);
    assert.match(recovered.stdout, /"text":\s*"ok"/);
    assert.ok(recovered.elapsedMilliseconds < 800, `recovered request took ${recovered.elapsedMilliseconds}ms`);

    const action = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-test',
      operation: 'fake_action',
      input: JSON.stringify({ delay_ms: 0 }),
      timeout_milliseconds: 1000,
    };
    const performed = await run(['perform', JSON.stringify(action)], baseEnvironment);
    assert.equal(performed.code, 0, performed.stderr);
    const actionResult = JSON.parse(performed.stdout);
    assert.equal(actionResult.schema, 'hominal.organ-action-result/v1');
    assert.equal(actionResult.action_id, action.action_id);
    assert.equal(actionResult.status, 'completed');
    assert.equal(actionResult.effect, 'changed');
    assert.match(actionResult.output, /"text":"ok"/);

    const failedAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-failure',
      operation: 'browser_type',
      input: JSON.stringify({ target: '[data-testid=tweetTextarea_0]', text: 'force browser failure' }),
      timeout_milliseconds: 1000,
    };

    const rejectedSnapshotAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-rejected-snapshot',
      operation: 'browser_snapshot',
      input: JSON.stringify({ url: 'https://x.com/home' }),
      timeout_milliseconds: 1000,
    };
    const rejectedSnapshot = await run(['perform', JSON.stringify(rejectedSnapshotAction)], baseEnvironment);
    assert.equal(rejectedSnapshot.code, 0, rejectedSnapshot.stderr);
    const rejectedSnapshotResult = JSON.parse(rejectedSnapshot.stdout);
    assert.equal(rejectedSnapshotResult.status, 'failed');
    assert.equal(rejectedSnapshotResult.effect, 'unknown');
    assert.match(rejectedSnapshotResult.output, /does not accept url/);
    const performedFailure = await run(['perform', JSON.stringify(failedAction)], baseEnvironment);
    assert.equal(performedFailure.code, 0, performedFailure.stderr);
    const failureResult = JSON.parse(performedFailure.stdout);
    assert.equal(failureResult.status, 'failed');
    assert.equal(failureResult.effect, 'unknown');
    assert.match(failureResult.output, /field rejected this input/);

    const snapshotAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-snapshot',
      operation: 'browser_snapshot',
      input: '{}',
      timeout_milliseconds: 1000,
    };
    const performedSnapshot = await run(['perform', JSON.stringify(snapshotAction)], baseEnvironment);
    assert.equal(performedSnapshot.code, 0, performedSnapshot.stderr);
    const snapshotActionResult = JSON.parse(performedSnapshot.stdout);
    assert.equal(snapshotActionResult.effect, 'observed');
    assert.equal(snapshotActionResult.observation.schema, 'hominal.organ-observation/v1');
    assert.equal(snapshotActionResult.observation.surface_id, 'chrome.current_page');
    assert.ok(snapshotActionResult.observation.objects.length > 0);

    const clicked = await run(['call', 'browser_click', JSON.stringify({
      ref: 'f8e269', target: 'Post', name: 'Post', type: 'button',
    })], baseEnvironment);
    assert.equal(clicked.code, 0, clicked.stderr);
    const clickOutput = JSON.stringify(JSON.parse(clicked.stdout));
    assert.match(clickOutput, /f8e269/);
    assert.match(clickOutput, /element/);
    assert.doesNotMatch(clickOutput, /\\"ref\\"|\\"name\\"|\\"type\\"/);

    const selectorClicked = await run(['call', 'browser_click', JSON.stringify({
      selector: '[data-testid=tweetButton]',
    })], baseEnvironment);
    assert.equal(selectorClicked.code, 0, selectorClicked.stderr);
    const selectorClickOutput = JSON.stringify(JSON.parse(selectorClicked.stdout));
    assert.match(selectorClickOutput, /tweetButton/);
    assert.doesNotMatch(selectorClickOutput, /selector/);

    const typed = await run(['call', 'browser_type', JSON.stringify({
      selector: '[data-testid=tweetTextarea_0]', value: 'hello from Alice',
    })], baseEnvironment);
    assert.equal(typed.code, 0, typed.stderr);
    const typeOutput = JSON.stringify(JSON.parse(typed.stdout));
    assert.match(typeOutput, /tweetTextarea_0/);
    assert.match(typeOutput, /hello from Alice/);
    assert.doesNotMatch(typeOutput, /selector|value/);

    const filled = await run(['call', 'browser_fill_form', JSON.stringify({
      fields: [{ ref: 'f31e79', text: 'hello from Alice' }],
    })], baseEnvironment);
    assert.equal(filled.code, 0, filled.stderr);
    const fillOutput = JSON.stringify(JSON.parse(filled.stdout));
    assert.match(fillOutput, /f31e79/);
    assert.match(fillOutput, /hello from Alice/);
    assert.match(fillOutput, /textbox/);
    assert.doesNotMatch(fillOutput, /\\"ref\\"|\\"text\\"/);

    const selectorFilled = await run(['call', 'browser_fill_form', JSON.stringify({
      selector: '[data-testid=tweetTextarea_0]', text: 'selector alias',
    })], baseEnvironment);
    assert.equal(selectorFilled.code, 0, selectorFilled.stderr);
    const selectorFillOutput = JSON.stringify(JSON.parse(selectorFilled.stdout));
    assert.match(selectorFillOutput, /tweetTextarea_0/);
    assert.match(selectorFillOutput, /selector alias/);

    const found = await run(['call', 'browser_find', JSON.stringify({
      query: 'Alice',
    })], baseEnvironment);
    assert.equal(found.code, 0, found.stderr);
    const findOutput = JSON.stringify(JSON.parse(found.stdout));
    assert.match(findOutput, /Alice/);
    assert.doesNotMatch(findOutput, /query/);

    const navigated = await run(['call', 'browser_navigate', JSON.stringify({
      target: 'https://example.test/alice',
    })], baseEnvironment);
    assert.equal(navigated.code, 0, navigated.stderr);
    const navigateOutput = JSON.stringify(JSON.parse(navigated.stdout));
    assert.ok(navigateOutput.includes('example.test/alice'));
    assert.match(navigateOutput, /Semantic snapshot/);
    assert.doesNotMatch(navigateOutput, /target/);

    const navigateAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-navigate',
      operation: 'browser_navigate',
      input: JSON.stringify({ url: 'https://example.test/bob' }),
      timeout_milliseconds: 1000,
    };
    const performedNavigation = await run(['perform', JSON.stringify(navigateAction)], baseEnvironment);
    assert.equal(performedNavigation.code, 0, performedNavigation.stderr);
    const navigationResult = JSON.parse(performedNavigation.stdout);
    assert.equal(navigationResult.effect, 'oriented');
    assert.equal(navigationResult.observation.schema, 'hominal.organ-observation/v1');
    assert.match(navigationResult.output, /Semantic snapshot/);

    const waited = await run(['call', 'browser_wait_for', JSON.stringify({
      time_milliseconds: 1000,
    })], baseEnvironment);
    assert.equal(waited.code, 0, waited.stderr);
    assert.match(JSON.stringify(JSON.parse(waited.stdout)), /time.*1/);

    const legacyMilliseconds = await run(['call', 'browser_wait_for', JSON.stringify({
      time: 3000,
    })], baseEnvironment);
    assert.equal(legacyMilliseconds.code, 0, legacyMilliseconds.stderr);
    assert.match(JSON.stringify(JSON.parse(legacyMilliseconds.stdout)), /time.*3/);
    assert.doesNotMatch(selectorFillOutput, /\\"selector\\"|\\"text\\"/);

    const mediumSnapshotAction = {
      schema: 'hominal.organ-action/v1',
      action_id: 'action-browser-medium-snapshot',
      operation: 'browser_snapshot',
      input: JSON.stringify({ test_medium: true }),
      timeout_milliseconds: 1000,
    };
    const mediumSnapshot = await run(['perform', JSON.stringify(mediumSnapshotAction)], baseEnvironment);
    assert.equal(mediumSnapshot.code, 0, mediumSnapshot.stderr);
    const mediumSnapshotResult = JSON.parse(mediumSnapshot.stdout);
    assert.equal(mediumSnapshotResult.schema, 'hominal.organ-action-result/v1');
    assert.equal(mediumSnapshotResult.action_id, mediumSnapshotAction.action_id);
    assert.equal(mediumSnapshotResult.status, 'completed');
    assert.equal(mediumSnapshotResult.effect, 'observed');
    assert.match(mediumSnapshotResult.output, /example\.test\/medium/);
    assert.match(mediumSnapshotResult.output, /FULL_CAPTURE_END/);
    assert.ok(mediumSnapshotResult.observation.objects[0].content.includes('"'.repeat(7000)), 'complete reading was silently clipped during transport');
    const readFailure = await run(['perform', JSON.stringify({...mediumSnapshotAction,
      action_id: 'read-failure', input: JSON.stringify({test_read_failure:true}),
    })], baseEnvironment);
    assert.equal(JSON.parse(readFailure.stdout).status, 'failed', 'a failed read became a successful partial snapshot');
    assert.match(readFailure.stdout, /read_incomplete/);
  } finally {
    daemon.kill('SIGTERM');
    await new Promise(resolve => daemon.once('exit', resolve));
    rmSync(root, { recursive: true, force: true });
  }
  assert.equal(daemonError, '');
});
