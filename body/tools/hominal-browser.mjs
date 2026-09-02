#!/usr/bin/env node

import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, rmSync, chmodSync, readFileSync } from 'node:fs';
import net from 'node:net';
import { createInterface } from 'node:readline';

const [operation, firstArgument, secondArgument] = process.argv.slice(2);
const publicOperations = new Set(['health', 'list', 'schema', 'call', 'perform', 'observe', 'orient', 'cancel']);
if (operation !== 'serve' && !publicOperations.has(operation)) {
  if (operation !== 'run-code' && operation !== 'describe') {
    console.error("usage: hominal-browser describe | health | observe | orient | list | schema <tool_name> | call <tool_name> '<json_arguments>' | perform '<organ_action_json>' | run-code <script.js>");
    process.exit(2);
  }
}
if (operation === 'run-code' && !firstArgument) {
  console.error('script path is required');
  process.exit(2);
}
if ((operation === 'schema' || operation === 'call' || operation === 'perform') && !firstArgument) {
  console.error(operation === 'perform' ? 'action envelope is required' : 'tool name is required');
  process.exit(2);
}

let actionRequest = null;
let toolName = firstArgument;
let argumentsText = secondArgument;
if (operation === 'perform') {
  try {
    actionRequest = JSON.parse(firstArgument);
  } catch (error) {
    console.error(`invalid action envelope: ${error.message}`);
    process.exit(2);
  }
  if (actionRequest?.schema !== 'hominal.organ-action/v1' || !actionRequest.action_id || !actionRequest.operation) {
    console.error('invalid action envelope');
    process.exit(2);
  }
  toolName = actionRequest.operation;
  argumentsText = actionRequest.input || '{}';
}

let toolArguments = {};
if ((operation === 'call' || operation === 'perform') && argumentsText) {
  try {
    toolArguments = JSON.parse(argumentsText);
  } catch (error) {
    console.error(`invalid JSON arguments: ${error.message}`);
    process.exit(2);
  }
}

const requestedOperation = operation === 'run-code' ? 'call' : operation;
const requestedTool = operation === 'run-code' ? 'browser_run_code_unsafe' : toolName;
let requestedArguments = toolArguments;
if (operation === 'run-code') {
  try {
    requestedArguments = { code: readFileSync(toolName, 'utf8') };
  } catch (error) {
    console.error(`cannot read browser script: ${error.message}`);
    process.exit(2);
  }
}

const command = process.env.HOMINAL_PLAYWRIGHT_MCP_COMMAND || '/usr/local/bin/hominal-playwright-mcp';
const timeoutMilliseconds = Number(process.env.HOMINAL_BROWSER_TIMEOUT_MS || 30000);
const requestedTimeoutMilliseconds = operation === 'perform'
  ? Math.max(1, Math.min(Number(actionRequest.timeout_milliseconds) || timeoutMilliseconds, 120000))
  : timeoutMilliseconds;
const organID = process.env.HOMINAL_ORGAN_ID || 'browser';
const socketPath = process.env.HOMINAL_ORGAN_SOCKET || process.env.HOMINAL_BROWSER_SOCKET || '/run/hominal/organs/browser.sock';
const caller = process.env.HOMINAL_ORGAN_CALLER || process.env.HOMINAL_BROWSER_CALLER || 'intentional-action';

// This is the stable intentional-action surface promised by the pinned
// Playwright MCP adapter. Passive host protocol verbs such as observe and
// orient deliberately stay out of this catalog: they are used by the body
// kernel, not guessed by cognition as Playwright tool names.
const actionOperations = [
  'browser_close',
  'browser_resize',
  'browser_console_messages',
  'browser_handle_dialog',
  'browser_evaluate',
  'browser_file_upload',
  'browser_drop',
  'browser_find',
  'browser_fill_form',
  'browser_press_key',
  'browser_type',
  'browser_navigate',
  'browser_navigate_back',
  'browser_network_requests',
  'browser_network_request',
  'browser_run_code_unsafe',
  'browser_take_screenshot',
  'browser_snapshot',
  'browser_click',
  'browser_drag',
  'browser_hover',
  'browser_select_option',
  'browser_tabs',
  'browser_wait_for',
];

const description = {
  schema: 'hominal.organ-description/v1',
  id: organID,
  name: 'Chrome browser',
  command: 'hominal-browser',
  capabilities: ['observe', 'orient', 'perform', 'cancel', 'public_web', 'authenticated_web'],
  operations: actionOperations,
  guidance: 'browser 连接当前 Chrome 并共享其登录与网络条件。行动时只从 operations 选择 Playwright 动作名，input 使用对应 JSON 参数；browser_snapshot 配合 {} 读取当前页面。需要参数结构时可通过 System Organ 执行 hominal-browser schema <动作名>。capabilities 描述身体内核的感知与调度能力，不是 organ_action 的 operation。',
};

function usesSnapshotElementHandle(value, key = '') {
  if (typeof value === 'string') return /target$/i.test(key) && /^e\d+$/.test(value);
  if (Array.isArray(value)) return value.some(item => usesSnapshotElementHandle(item, key));
  if (value && typeof value === 'object') {
    return Object.entries(value).some(([childKey, childValue]) => usesSnapshotElementHandle(childValue, childKey));
  }
  return false;
}

function normalizeToolArguments(name, value) {
  if (name !== 'browser_run_code_unsafe' || !value || typeof value.code !== 'string') return value;
  const code = value.code.trim();
  if (!code) return value;
  const alreadyFunction = /^(?:(?:async\s+)?function\b|(?:async\s*)?\(?\s*page\s*\)?\s*=>)/.test(code);
  const statementBlock = /[;\n]/.test(code) || /^(?:const|let|var|await|return|if|for|while|try)\b/.test(code);
  const invocation = alreadyFunction
    ? `await (async (console) => await (${code})(page))(__hominalConsole)`
    : statementBlock
      ? `await (async (console) => { ${code} })(__hominalConsole)`
      : `await (async (console) => await (${code}))(__hominalConsole)`;
  return {
    ...value,
    code: `async (page) => {
      const __hominalLogs = [];
      const __hominalConsole = Object.create(globalThis.console);
      __hominalConsole.log = (...values) => __hominalLogs.push(values.length === 1 ? values[0] : values);
      const __hominalValue = ${invocation};
      if (__hominalValue !== undefined) return __hominalValue;
      const __hominalNormalize = value => {
        if (typeof value !== 'string') return value;
        try { return JSON.parse(value); } catch { return value; }
      };
      if (__hominalLogs.length === 1) return __hominalNormalize(__hominalLogs[0]);
      return { logs: __hominalLogs.map(__hominalNormalize) };
    }`,
  };
}

function resultText(result) {
  return (result?.content || [])
    .filter(block => block?.type === 'text')
    .map(block => block.text || '')
    .join('\n');
}

function normalizeSemanticText(value) {
  return String(value || '')
    .replace(/\b\d+\s+(?:seconds?|minutes?|hours?|days?)\s+ago\b/gi, '<relative-time>')
    .replace(/\b\d+(?:\.\d+)?[KMB]?\s+(reply|replies|repost|reposts|like|likes|bookmark|bookmarks|view|views)\b/gi, '<metric> $1')
    .replace(/\b(?:play\s+)?\d{1,2}:\d{2}\b/gi, '<media-time>')
    .replace(/\s+(?:embedded video|previous image|<metric>\s+(?:reply|replies))\b.*$/gi, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function containerRole(line) {
  const normalized = line.trim().replace(/^-\s*/, '');
  for (const role of ['main', 'banner', 'contentinfo', 'complementary', 'navigation', 'search', 'toolbar', 'menubar', 'menu', 'tablist']) {
    if (normalized === role || normalized.startsWith(`${role} `) || normalized.startsWith(`${role} "`)) return role;
  }
  return '';
}

function semanticPosition(scopes) {
  let insideMain = false;
  let insideInterface = false;
  for (const scope of scopes) {
    if (scope.role === 'main') insideMain = true;
    if (['banner', 'contentinfo', 'complementary', 'navigation', 'search', 'toolbar', 'menubar', 'menu', 'tablist'].includes(scope.role)) {
      insideInterface = true;
    }
  }
  return { insideMain, insideInterface };
}

function semanticObjects(snapshot) {
  const context = [];
  const activeFeedback = [];
  const activeControls = [];
  const articles = [];
  const mainContent = [];
  const globalHeadings = [];
  const scopes = [];
  const seen = new Set();
  let hasMain = false;
  for (const rawLine of snapshot.split('\n')) {
    const line = rawLine.trim();
    if (line.startsWith('- Page URL:')) context.push(line.slice(2));
    if (line.startsWith('- Page Title:')) context.push(line.slice(2));
    if (line.startsWith('- Active control:') && activeControls.length < 20) {
      const value = normalizeSemanticText(line.slice(2));
      if (value && !seen.has(value)) { seen.add(value); activeControls.push(value); }
    }
    if (line.startsWith('- Active feedback:') && activeFeedback.length < 8) {
      const value = normalizeSemanticText(line.slice(2));
      if (value && !seen.has(value)) { seen.add(value); activeFeedback.push(value); }
    }
    const article = line.match(/article "(.*)" \[ref=/);
    if (article) {
      const value = normalizeSemanticText(article[1]);
      if (value && !seen.has(value) && articles.length < 8) { seen.add(value); articles.push(value); }
      continue;
    }
    const indent = rawLine.length - rawLine.replace(/^[ \t]*/, '').length;
    while (scopes.length > 0 && scopes.at(-1).indent >= indent) scopes.pop();
    const { insideMain, insideInterface } = semanticPosition(scopes);
    const named = line.match(/(?:heading|link) "([^"]+)"/);
    if (named) {
      const value = normalizeSemanticText(named[1]);
      if (value && !seen.has(value)) {
        if (insideMain && !insideInterface && mainContent.length < 20) {
          seen.add(value); mainContent.push(value);
        } else if (!insideMain && line.includes('heading "') && !insideInterface && globalHeadings.length < 12) {
          globalHeadings.push(value);
        }
      }
    }
    const role = containerRole(line);
    if (role) {
      if (role === 'main') hasMain = true;
      scopes.push({ indent, role });
    }
  }
  const onX = context.some(value => /^Page URL:\s+https:\/\/(?:www\.)?x\.com\//i.test(value));
  const values = [...activeFeedback, ...activeControls, ...articles];
  if (articles.length === 0 && !onX) values.push(...(hasMain ? mainContent : globalHeadings));
  return { context: [...new Set(context)].slice(0, 4), values };
}

function stableObject(value) {
  const direct = value.match(/ Direct URL: (https:\/\/\S+)$/)?.[1];
  const identity = direct || value;
  return {
    id: createHash('sha256').update(identity).digest('hex'),
    content: value.slice(0, 4000),
  };
}

function observationFromSnapshot(result) {
  const snapshot = resultText(result).trim();
  if (!snapshot) throw new Error('browser snapshot contained no visible text');
  const { context, values } = semanticObjects(snapshot);
  return {
    schema: 'hominal.organ-observation/v1',
    organ_id: organID,
    surface_id: 'chrome.current_page',
    observed_at: new Date().toISOString(),
    context,
    objects: values.map(stableObject),
  };
}

const orientCode = `async (page) => {
  const active = await page.evaluate(() => {
    const node = document.activeElement;
    return Boolean(node && node.matches('input, textarea, [contenteditable="true"], [role="textbox"]'));
  });
  if (active) return {url: page.url(), preserved: true};
  const before = await page.evaluate(() => window.scrollY);
  await page.evaluate(() => window.scrollBy(0, Math.max(window.innerHeight * 0.85, 600)));
  await page.waitForTimeout(1500);
  const after = await page.evaluate(() => window.scrollY);
  return {url: page.url(), before, after};
}`;

function orientationFromResult(result) {
  const detail = resultText(result).replace(/\s+/g, ' ').trim().slice(0, 1200);
  return {
    schema: 'hominal.organ-orientation/v1',
    organ_id: organID,
    status: /"preserved"\s*:\s*true/.test(detail) ? 'preserved' : 'moved',
    observed_at: new Date().toISOString(),
    detail,
  };
}

class MCPConnection {
  constructor(persistent = false, defaultTimeoutMilliseconds = timeoutMilliseconds) {
    this.persistent = persistent;
    this.defaultTimeoutMilliseconds = defaultTimeoutMilliseconds;
    this.nextId = 1;
    this.stderr = '';
    this.pending = new Map();
    this.tools = null;
    this.alive = true;
    this.child = spawn(command, [], {
      cwd: process.env.HOMINAL_INSTANCE_ROOT || process.cwd(),
      env: process.env,
      detached: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    this.lines = createInterface({ input: this.child.stdout });
    this.child.stderr.on('data', chunk => {
      this.stderr = (this.stderr + chunk.toString()).slice(-8192);
    });
    this.child.stdin.on('error', () => {
      // Exit/timeout handling rejects the matching request. Keep a late EPIPE
      // from escaping as an unrelated daemon-level exception.
    });
    this.lines.on('line', line => {
      if (!line.trim()) return;
      let message;
      try {
        message = JSON.parse(line);
      } catch {
        return;
      }
      const waiting = this.pending.get(message.id);
      if (!waiting) return;
      this.pending.delete(message.id);
      clearTimeout(waiting.timer);
      waiting.resolve(message);
    });
    this.child.once('exit', (code, signal) => {
      this.alive = false;
      const detail = `Playwright MCP exited code=${code ?? 'none'} signal=${signal ?? 'none'}`;
      for (const waiting of this.pending.values()) {
        clearTimeout(waiting.timer);
        waiting.reject(new Error(detail));
      }
      this.pending.clear();
    });
  }

  request(method, params, requestTimeoutMilliseconds = this.defaultTimeoutMilliseconds) {
    if (!this.alive) return Promise.reject(new Error('Playwright MCP connection is not alive'));
    const id = this.nextId++;
    const response = new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        const error = new Error(`timed out waiting for ${method}${this.stderr ? `: ${this.stderr.trim()}` : ''}`);
        reject(error);
        this.close(error);
      }, Math.max(1, requestTimeoutMilliseconds));
      this.pending.set(id, { resolve, reject, timer });
    });
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`);
    return response;
  }

  async initialize(requestTimeoutMilliseconds = this.defaultTimeoutMilliseconds) {
    const deadline = Date.now() + Math.max(1, requestTimeoutMilliseconds);
    const request = (method, params) => this.request(method, params, Math.max(1, deadline - Date.now()));
    const initialized = await request('initialize', {
      protocolVersion: '2025-03-26',
      capabilities: {},
      clientInfo: { name: 'hominal-browser', version: '1.1.0' },
    });
    if (initialized.error) throw new Error(JSON.stringify(initialized.error));
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized', params: {} })}\n`);
    const listed = await request('tools/list', {});
    if (listed.error) throw new Error(JSON.stringify(listed.error));
    this.tools = listed.result?.tools || [];
    return this;
  }

  async execute(requestedOperation, requestedTool, requestedArguments, requestTimeoutMilliseconds = this.defaultTimeoutMilliseconds) {
    const deadline = Date.now() + Math.max(1, requestTimeoutMilliseconds);
    const request = (method, params) => this.request(method, params, Math.max(1, deadline - Date.now()));
    if (requestedOperation === 'list') {
      return {
        tools: this.tools.map(({ name }) => ({ name })),
        hint: 'Use hominal-browser schema <tool_name> for the arguments of one selected action.',
      };
    }
    if (requestedOperation === 'schema') {
      const schema = this.tools.find(tool => tool.name === requestedTool);
      if (!schema) throw new Error(`tool "${requestedTool}" not found`);
      return schema;
    }
    if (requestedOperation === 'observe') {
      const response = await request('tools/call', { name: 'browser_snapshot', arguments: {} });
      if (response.error) throw new Error(JSON.stringify(response.error));
      return observationFromSnapshot(await this.withDirectContactRoutes(response.result, request));
    }
    if (requestedOperation === 'orient') {
      const response = await request('tools/call', {
        name: 'browser_run_code_unsafe',
        arguments: normalizeToolArguments('browser_run_code_unsafe', { code: orientCode }),
      });
      if (response.error) throw new Error(JSON.stringify(response.error));
      return orientationFromResult(response.result);
    }
    // Fallback calls made without the body daemon have session-local tab and
    // aria-ref maps. Rebuild those maps inside that one MCP process. The
    // persistent body session deliberately keeps the maps produced by Alice's
    // preceding observation instead.
    if (!this.persistent && requestedTool === 'browser_tabs' && requestedArguments?.action === 'select') {
      const listed = await request('tools/call', { name: 'browser_tabs', arguments: { action: 'list' } });
      if (listed.error) throw new Error(JSON.stringify(listed.error));
    }
    if (!this.persistent && requestedTool !== 'browser_snapshot' && usesSnapshotElementHandle(requestedArguments)) {
      const snapshot = await request('tools/call', { name: 'browser_snapshot', arguments: {} });
      if (snapshot.error) throw new Error(JSON.stringify(snapshot.error));
    }
    const callArguments = normalizeToolArguments(requestedTool, requestedArguments || {});
    const response = await request('tools/call', { name: requestedTool, arguments: callArguments });
    if (response.error) throw new Error(JSON.stringify(response.error));
    if (requestedTool === 'browser_snapshot') {
      return this.withDirectContactRoutes(response.result, request);
    }
    return response.result;
  }

  async withDirectContactRoutes(result, request) {
    const snapshotText = (result?.content || [])
      .filter(block => block?.type === 'text')
      .map(block => block.text || '')
      .join('\n');
    if (!/Page URL:\s+https:\/\/(?:www\.)?x\.com\//i.test(snapshotText)) return result;
    const code = `async (page) => {
      const objects = await page.locator('article').evaluateAll(nodes => nodes
        .filter(node => node.getClientRects().length > 0)
        .slice(0, 8)
        .map(node => {
          const links = Array.from(node.querySelectorAll('a[href]'));
          const status = links.find(link => /\\/status\\/\\d+(?:[?#].*)?$/.test(link.href));
          return status ? {text: (node.innerText || '').slice(0, 1600), url: status.href} : null;
        })
        .filter(Boolean));
      const dialogs = Array.from(await page.locator('[role="dialog"]').elementHandles());
      let controls = [];
      let feedback = [];
      for (const dialog of dialogs.reverse()) {
        if (!(await dialog.isVisible().catch(() => false))) continue;
        controls = await dialog.$$eval('[role="textbox"], textarea, input, button', nodes => nodes
          .filter(node => node.getClientRects().length > 0)
          .slice(0, 20)
          .map(node => {
            const editableText = node.isContentEditable ? (node.innerText || node.textContent || '') : (node.value || '');
            return {
              role: node.getAttribute('role') || node.tagName.toLowerCase(),
              name: node.getAttribute('aria-label') || node.getAttribute('placeholder') || node.innerText || node.value || '',
              disabled: Boolean(node.disabled || node.getAttribute('aria-disabled') === 'true'),
              contentLength: editableText.length,
            };
          }));
        feedback = await dialog.$$eval('[aria-live]:not([aria-live="off"]), [role="alert"], [role="status"]', nodes => Array.from(new Set(nodes
          .filter(node => node.getClientRects().length > 0)
          .map(node => (node.innerText || node.textContent || '').replace(/\s+/g, ' ').trim())
          .filter(Boolean)))
          .slice(0, 8));
        break;
      }
      return {objects, controls, feedback};
    }`;
    try {
      const routed = await request('tools/call', { name: 'browser_run_code_unsafe', arguments: { code } });
      if (routed.error) return result;
      const routeText = (routed.result?.content || [])
        .filter(block => block?.type === 'text')
        .map(block => block.text || '')
        .join('\n');
      const marker = '### Result\n';
      const markerIndex = routeText.indexOf(marker);
      if (markerIndex < 0) return result;
      const payloadText = routeText.slice(markerIndex + marker.length).split('\n### ')[0].trim();
      const payload = JSON.parse(payloadText);
      const objects = Array.isArray(payload?.objects) ? payload.objects : [];
      const controls = Array.isArray(payload?.controls) ? payload.controls : [];
      const feedback = Array.isArray(payload?.feedback) ? payload.feedback : [];
      if (objects.length === 0 && controls.length === 0 && feedback.length === 0) return result;
      const controlLines = controls.map(control => {
        const name = String(control.name || '')
          .replace(/["\\\r\n]+/g, ' ')
          .replace(/\s+/g, ' ')
          .trim();
        const contentState = control.role === 'textbox' && Number.isFinite(control.contentLength)
          ? `; content length ${control.contentLength}`
          : '';
        return `- Active control: ${control.role} "${name}" (${control.disabled ? 'disabled' : 'enabled'}${contentState})`;
      });
      const feedbackLines = feedback.map(value => {
        const message = String(value || '')
          .replace(/["\\\r\n]+/g, ' ')
          .replace(/\s+/g, ' ')
          .trim()
          .slice(0, 400);
        return `- Active feedback: "${message}"`;
      });
      const routes = objects.map((object, index) => {
        const label = String(object.text || '')
          .replace(/["\\\r\n]+/g, ' ')
          .replace(/\s+/g, ' ')
          .trim();
        return `- article "${label} Direct URL: ${object.url}" [ref=hominal-route-${index + 1}]`;
      });
      return {
        ...result,
        content: [
          { type: 'text', text: `### Active surface\n${[...feedbackLines, ...controlLines, ...routes].join('\n')}` },
          ...(result?.content || []),
        ],
      };
    } catch {
      // Direct routes enrich the same observation. The underlying browser
      // snapshot remains valid when a page does not expose authored objects.
      return result;
    }
  }

  close(reason = new Error('Playwright MCP connection closed')) {
    if (!this.alive) return;
    this.alive = false;
    for (const waiting of this.pending.values()) {
      clearTimeout(waiting.timer);
      waiting.reject(reason);
    }
    this.pending.clear();
    this.child.stdin.end();
    try {
      process.kill(-this.child.pid, 'SIGTERM');
    } catch {
      this.child.kill('SIGTERM');
    }
    this.lines.close();
    this.child.stdout.destroy();
    this.child.stderr.destroy();
  }
}

async function createConnection(persistent = false, requestTimeoutMilliseconds = timeoutMilliseconds) {
  return new MCPConnection(persistent, requestTimeoutMilliseconds).initialize(requestTimeoutMilliseconds);
}

function boundedToolOutput(tool, output) {
  if (tool !== 'browser_snapshot' || JSON.stringify(output).length <= 24000) return output;
  const { context, values } = semanticObjects(resultText(output));
  return {
    content: [{
      type: 'text',
      text: ['### Semantic snapshot', ...context, 'Visible objects:', ...values.map(value => `- ${value}`)].join('\n'),
    }],
    hominal_compaction: 'same-result browser semantics; use browser_find for a current control ref',
  };
}

function boundedPublicOutput(output) {
  return boundedToolOutput(requestedTool, output);
}

function writePublicOutput(output) {
  const bounded = boundedPublicOutput(output);
  process.stdout.write(`${JSON.stringify(bounded)}\n`);
  if (output?.isError) process.exitCode = 1;
}

function actionResult(status, output, summary) {
  return {
    schema: 'hominal.organ-action-result/v1',
    organ_id: organID,
    action_id: actionRequest.action_id,
    status,
    observed_at: new Date().toISOString(),
    summary,
    output: typeof output === 'string' ? output : JSON.stringify(boundedPublicOutput(output)),
  };
}

async function requestDaemon(payload) {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(socketPath);
    let responseText = '';
    const timer = setTimeout(() => {
      socket.destroy();
      reject(new Error('timed out waiting for persistent browser session'));
    }, requestedTimeoutMilliseconds + 1000);
    socket.setEncoding('utf8');
    socket.on('connect', () => socket.write(`${JSON.stringify({
      ...payload,
      timeoutMilliseconds: requestedTimeoutMilliseconds,
      caller,
    })}\n`));
    socket.on('data', chunk => { responseText += chunk; });
    socket.on('error', error => {
      clearTimeout(timer);
      reject(error);
    });
    socket.on('end', () => {
      clearTimeout(timer);
      try {
        resolve(JSON.parse(responseText));
      } catch {
        reject(new Error('persistent browser session returned invalid JSON'));
      }
    });
  });
}

async function serve() {
  mkdirSync(socketPath.slice(0, socketPath.lastIndexOf('/')), { recursive: true, mode: 0o755 });
  rmSync(socketPath, { force: true });
  // Publish the small gateway before Playwright initialization. Chrome may be
  // starting at the same time as the body; the gateway remains healthy and the
  // first real action performs bounded recovery instead of making systemd
  // restart the whole life process behind a missing socket.
  let connection = null;
  let queue = Promise.resolve();
  let queued = 0;
  let active = null;
  const server = net.createServer(socket => {
    socket.setEncoding('utf8');
    let requestText = '';
    let handled = false;
    socket.on('data', chunk => {
      requestText += chunk;
      if (handled || !requestText.includes('\n')) return;
      handled = true;
      let request;
      try {
        request = JSON.parse(requestText.trim());
      } catch (error) {
        socket.end(`${JSON.stringify({ ok: false, error: error.message })}\n`);
        return;
      }
      if (request.operation === 'health') {
        const status = active ? 'busy' : connection?.alive ? 'ready' : 'recovering';
        socket.end(`${JSON.stringify({ ok: true, result: {
          schema: 'hominal.organ-health/v1',
          id: organID,
          status,
          accepting: true,
          connection_alive: Boolean(connection?.alive),
          in_flight: active ? 1 : 0,
          queued,
        } })}\n`);
        return;
      }
      if (request.operation === 'cancel') {
        const cancelled = Boolean(active && connection?.alive);
        if (cancelled) {
          connection.close(new Error('browser action cancelled by organ host'));
          connection = null;
        }
        socket.end(`${JSON.stringify({ ok: true, result: { cancelled } })}\n`);
        return;
      }
      const job = {
        socket,
        request,
        caller: String(request.caller || 'intentional-action'),
        deadline: Date.now() + Math.max(1, Math.min(Number(request.timeoutMilliseconds) || timeoutMilliseconds, 60000)),
        active: false,
        complete: false,
        closed: false,
      };
      // A quiet sensory glance must yield when Alice forms an intentional
      // action. Cancelling its MCP call is cheaper and safer than letting a
      // background snapshot hold the only stateful page lane until timeout.
      if (job.caller !== 'passive-perception' && active?.caller === 'passive-perception' && connection?.alive) {
        connection.close(new Error('passive browser perception yielded to an intentional action'));
        connection = null;
      }
      socket.on('close', () => {
        job.closed = true;
        if (job.active && !job.complete && connection?.alive) {
          connection.close(new Error(`browser client ${job.caller} disconnected`));
          connection = null;
        }
      });
      queued += 1;
      queue = queue.then(async () => {
        queued -= 1;
        if (job.closed) return;
        job.active = true;
        active = job;
        try {
          if (!publicOperations.has(request.operation)) throw new Error('invalid browser operation');
          let remaining = job.deadline - Date.now();
          if (remaining <= 0) throw new Error('browser request expired while waiting for the stateful organ');
          if (!connection?.alive) connection = await createConnection(true, remaining);
          remaining = job.deadline - Date.now();
          if (remaining <= 0) throw new Error('browser request expired while recovering the stateful organ');
          const executedOperation = request.operation === 'perform' ? 'call' : request.operation;
          const toolResult = await connection.execute(executedOperation, request.toolName, request.arguments, remaining);
          const actionFailed = Boolean(toolResult?.isError);
          const result = request.operation === 'perform'
            ? {
                schema: 'hominal.organ-action-result/v1',
                organ_id: organID,
                action_id: request.actionID,
                status: actionFailed ? 'failed' : 'completed',
                observed_at: new Date().toISOString(),
                summary: actionFailed
                  ? 'Browser Organ 已完成尝试，Playwright 返回了明确失败。'
                  : 'Browser Organ 已完成操作并返回 Playwright 现实结果。',
                output: JSON.stringify(boundedToolOutput(request.toolName, toolResult)),
              }
            : toolResult;
          job.complete = true;
          socket.end(`${JSON.stringify({ ok: true, result })}\n`);
        } catch (error) {
          if (!connection?.alive) connection = null;
          job.complete = true;
          if (request.operation === 'perform') {
            socket.end(`${JSON.stringify({ ok: true, result: {
              schema: 'hominal.organ-action-result/v1',
              organ_id: organID,
              action_id: request.actionID,
              status: 'unknown',
              observed_at: new Date().toISOString(),
              summary: 'Browser Organ 的操作被中断，页面是否已经部分改变尚不确定。',
              output: JSON.stringify({ error: error.message }),
            } })}\n`);
          } else {
            socket.end(`${JSON.stringify({ ok: false, error: error.message })}\n`);
          }
        } finally {
          job.active = false;
          if (active === job) active = null;
        }
      }).catch(error => {
        socket.end(`${JSON.stringify({ ok: false, error: error.message })}\n`);
      });
    });
  });
  server.listen(socketPath, () => chmodSync(socketPath, 0o660));
  const shutdown = () => {
    server.close();
    connection?.close();
    rmSync(socketPath, { force: true });
  };
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}

if (operation === 'describe') {
  writePublicOutput(description);
} else if (operation === 'serve') {
  try {
    await serve();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
} else if (existsSync(socketPath)) {
  try {
    const response = await requestDaemon({
      operation: requestedOperation,
      toolName: requestedTool,
      arguments: requestedArguments,
      actionID: actionRequest?.action_id,
    });
    if (!response.ok) throw new Error(response.error || 'persistent browser request failed');
    writePublicOutput(response.result);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
} else {
  let connection;
  try {
    if (operation === 'health') throw new Error('persistent browser session is unavailable');
    connection = await createConnection(false, requestedTimeoutMilliseconds);
    const executedOperation = requestedOperation === 'perform' ? 'call' : requestedOperation;
    const result = await connection.execute(executedOperation, requestedTool, requestedArguments, requestedTimeoutMilliseconds);
    writePublicOutput(operation === 'perform'
      ? actionResult(
          result?.isError ? 'failed' : 'completed',
          result,
          result?.isError ? 'Browser Organ 已完成尝试，Playwright 返回了明确失败。' : 'Browser Organ 已完成操作并返回 Playwright 现实结果。',
        )
      : result);
  } catch (error) {
    if (operation === 'perform') {
      writePublicOutput(actionResult(
        'unknown',
        JSON.stringify({ error: error.message }),
        'Browser Organ 的操作被中断，页面是否已经部分改变尚不确定。',
      ));
    } else {
      console.error(error.message);
      process.exitCode = 1;
    }
  } finally {
    connection?.close();
  }
}
