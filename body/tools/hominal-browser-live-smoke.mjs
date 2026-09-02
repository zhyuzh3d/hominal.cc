#!/usr/bin/env node

// Non-mutating smoke test for the deployed Chrome organ. Chrome must already
// expose CDP on 127.0.0.1:9222. The test proves that health bypasses an active
// action, a timed-out action is cancelled, the MCP connection recovers, and
// recovery does not move the active page.

import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const helper = process.argv[2] || fileURLToPath(new URL('./hominal-browser.mjs', import.meta.url));

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
    child.once('exit', code => resolve({
      code,
      stdout,
      stderr,
      elapsedMilliseconds: performance.now() - started,
    }));
  });
}

async function waitFor(predicate, timeoutMilliseconds = 30000) {
  const deadline = Date.now() + timeoutMilliseconds;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  throw new Error('browser daemon did not become ready');
}

function resultText(result) {
  const body = JSON.parse(result.stdout);
  return (body.content || []).filter(block => block?.type === 'text').map(block => block.text || '').join('\n');
}

function returnedString(result) {
  const text = resultText(result);
  const match = text.match(/### Result\n("(?:[^"\\]|\\.)*")/);
  if (!match) throw new Error('browser action did not return a string result');
  return JSON.parse(match[1]);
}

const root = mkdtempSync(join(tmpdir(), 'hominal-browser-live-'));
const socket = join(root, 'browser.sock');
const environment = {
  HOMINAL_BROWSER_SOCKET: socket,
  HOMINAL_BROWSER_TIMEOUT_MS: '15000',
  HOMINAL_INSTANCE_ROOT: root,
};
const daemon = spawn(process.execPath, [helper, 'serve'], {
  env: { ...process.env, ...environment },
  stdio: ['ignore', 'pipe', 'pipe'],
});
let daemonError = '';
daemon.stderr.on('data', value => { daemonError += value; });

try {
  await waitFor(() => existsSync(socket));

  const tabsBefore = await run(['call', 'browser_tabs', '{"action":"list"}'], environment);
  assert.equal(tabsBefore.code, 0, tabsBefore.stderr);
  const beforeText = resultText(tabsBefore);
  assert.match(beforeText, /https:\/\/x\.com\//);
  assert.match(beforeText, /https:\/\/en\.wikipedia\.org\//);

  const surfaceBefore = await run(
    ['call', 'browser_run_code_unsafe', '{"code":"return await page.url()"}'],
    environment,
  );
  assert.equal(surfaceBefore.code, 0, surfaceBefore.stderr);
  const activeURLBefore = returnedString(surfaceBefore);

  const slow = run(
    ['call', 'browser_run_code_unsafe', '{"code":"await page.waitForTimeout(3000); return await page.url()"}'],
    { ...environment, HOMINAL_BROWSER_TIMEOUT_MS: '5000', HOMINAL_BROWSER_CALLER: 'passive-perception' },
  );
  await new Promise(resolve => setTimeout(resolve, 120));

  const health = await run(['health'], { ...environment, HOMINAL_BROWSER_TIMEOUT_MS: '300' });
  assert.equal(health.code, 0, health.stderr);
  assert.ok(health.elapsedMilliseconds < 700, `health waited ${health.elapsedMilliseconds.toFixed(0)}ms`);
  const healthBody = JSON.parse(health.stdout);
  assert.equal(healthBody.status, 'busy');
  assert.equal(healthBody.accepting, true);

  const surfaceAfterPromise = run(
    ['call', 'browser_run_code_unsafe', '{"code":"return await page.url()"}'],
    { ...environment, HOMINAL_BROWSER_CALLER: 'intentional-action' },
  );
  const timedOut = await slow;
  assert.notEqual(timedOut.code, 0, 'passive action did not yield to an intentional action');
  assert.ok(timedOut.elapsedMilliseconds < 2000, `preemption took ${timedOut.elapsedMilliseconds.toFixed(0)}ms`);

  const surfaceAfter = await surfaceAfterPromise;
  assert.equal(surfaceAfter.code, 0, surfaceAfter.stderr);
  const activeURLAfter = returnedString(surfaceAfter);
  assert.equal(activeURLAfter, activeURLBefore, 'browser recovery changed Alice\'s active page');

  const tabsAfter = await run(['call', 'browser_tabs', '{"action":"list"}'], environment);
  assert.equal(tabsAfter.code, 0, tabsAfter.stderr);
  const afterText = resultText(tabsAfter);
  assert.match(afterText, /https:\/\/x\.com\//);
  assert.match(afterText, /https:\/\/en\.wikipedia\.org\//);

  process.stdout.write(`${JSON.stringify({
    healthMilliseconds: Math.round(health.elapsedMilliseconds),
    passivePreemptionMilliseconds: Math.round(timedOut.elapsedMilliseconds),
    recoveredActionMilliseconds: Math.round(surfaceAfter.elapsedMilliseconds),
    activePagePreserved: true,
    xAvailable: true,
    wikipediaAvailable: true,
  })}\n`);
} finally {
  daemon.kill('SIGTERM');
  await new Promise(resolve => daemon.once('exit', resolve));
  rmSync(root, { recursive: true, force: true });
}

assert.equal(daemonError, '');
