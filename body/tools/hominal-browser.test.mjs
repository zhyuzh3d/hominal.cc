import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const helper = fileURLToPath(new URL('./hominal-browser.mjs', import.meta.url));

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
lines.on('line', line => {
  const message = JSON.parse(line);
  if (!message.id) return;
  if (message.method === 'initialize') {
    reply({jsonrpc:'2.0',id:message.id,result:{protocolVersion:'2025-03-26',capabilities:{}}});
    return;
  }
  if (message.method === 'tools/list') {
    reply({jsonrpc:'2.0',id:message.id,result:{tools:[
      {name:'fake_action',inputSchema:{type:'object'}},
      {name:'browser_snapshot',inputSchema:{type:'object'}},
      {name:'browser_run_code_unsafe',inputSchema:{type:'object'}}
    ]}});
    return;
  }
  if (message.method === 'tools/call') {
    if (message.params?.name === 'browser_snapshot') {
      snapshotCalls += 1;
      const drift = snapshotCalls > 1 ? '3 minutes ago A stable idea 5 likes' : '2 minutes ago A stable idea 4 likes';
      const noise = '- generic [ref=e9]: navigation noise\\n'.repeat(1200);
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Active surface\\n- Active control: textbox "Post text" (enabled; content length 12)\\n### Page\\n- Page URL: https://x.com/home\\n- Page Title: Home / X\\n### Snapshot\\n- heading "To view keyboard shortcuts" [ref=e0]\\n' + noise + '- article "Alice @alice ' + drift + ' Direct URL: https://x.com/alice/status/123" [ref=e1]'}]}});
      return;
    }
    if (message.params?.name === 'browser_run_code_unsafe') {
      reply({jsonrpc:'2.0',id:message.id,result:{content:[{type:'text',text:'### Result\\n{"preserved":true}'}]}});
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
    assert.ok(!description.operations.includes('observe'), 'passive host capability leaked into the action catalog');
    assert.match(description.guidance, /只从 operations 选择/);

    const observed = await run(['observe'], baseEnvironment);
    assert.equal(observed.code, 0, observed.stderr);
    const observation = JSON.parse(observed.stdout);
    assert.equal(observation.schema, 'hominal.organ-observation/v1');
    assert.equal(observation.organ_id, 'browser');
    assert.equal(observation.surface_id, 'chrome.current_page');
    assert.equal(observation.objects.length, 2);
    assert.match(observation.objects[0].content, /Active control/);
    assert.match(observation.objects[1].content, /A stable idea/);
    assert.match(observation.objects[1].content, /<relative-time>/);
    assert.match(observation.objects[1].content, /<metric> likes/);
    assert.doesNotMatch(JSON.stringify(observation.objects), /keyboard shortcuts/);
    const observedAgain = await run(['observe'], baseEnvironment);
    assert.equal(observedAgain.code, 0, observedAgain.stderr);
    const secondObservation = JSON.parse(observedAgain.stdout);
    assert.equal(secondObservation.objects[1].id, observation.objects[1].id, 'presentation drift changed the object identity');

    const directSnapshot = await run(['call', 'browser_snapshot', '{}'], baseEnvironment);
    assert.equal(directSnapshot.code, 0, directSnapshot.stderr);
    assert.ok(directSnapshot.stdout.length < 32000, 'the adapter leaked an unbounded accessibility tree into shell Reality');
    assert.match(directSnapshot.stdout, /A stable idea/);
    assert.doesNotMatch(directSnapshot.stdout, /navigation noise/);

    const oriented = await run(['orient'], baseEnvironment);
    assert.equal(oriented.code, 0, oriented.stderr);
    assert.equal(JSON.parse(oriented.stdout).status, 'preserved');

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
    assert.match(actionResult.output, /"text":"ok"/);
  } finally {
    daemon.kill('SIGTERM');
    await new Promise(resolve => daemon.once('exit', resolve));
    rmSync(root, { recursive: true, force: true });
  }
  assert.equal(daemonError, '');
});
