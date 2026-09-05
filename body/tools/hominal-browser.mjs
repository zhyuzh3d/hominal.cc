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
const maximumViewportOrientations = 3;
const requestedTimeoutMilliseconds = operation === 'perform'
  ? Math.max(1, Math.min(Number(actionRequest.timeout_milliseconds) || timeoutMilliseconds, 120000))
  : timeoutMilliseconds;
const organID = process.env.HOMINAL_ORGAN_ID || 'browser';
const socketPath = process.env.HOMINAL_ORGAN_SOCKET || process.env.HOMINAL_BROWSER_SOCKET || '/run/hominal/organs/browser.sock';
const caller = process.env.HOMINAL_ORGAN_CALLER || process.env.HOMINAL_BROWSER_CALLER || 'intentional-action';

// RejectedActionRequest means the Organ has proved that enactment never
// started.  It is different from a transport interruption, where the page may
// already have changed before the result was lost.
class RejectedActionRequest extends Error {}

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

// Effect is a factual organ result, not a value judgment. Only operations that
// inspect the current browser state without changing the page are marked as
// observed; every other successful operation can alter subsequent reality.
const observedActionOperations = new Set([
  'browser_console_messages',
  'browser_find',
  'browser_network_requests',
  'browser_network_request',
  'browser_snapshot',
  'browser_take_screenshot',
]);

// Orientation changes Alice's current sensory pose without, by itself,
// changing the object being observed.  Keeping this distinct from both a pure
// observation and a persistent change lets the common life kernel notice when
// attention repeatedly buys new views but no longer produces deeper effects.
const orientedActionOperations = new Set([
  'browser_hover',
  'browser_navigate',
  'browser_navigate_back',
  'browser_resize',
  'browser_tabs',
  'browser_wait_for',
]);

// These operations leave Alice looking at a page state whose immediate
// semantic surface is part of the action's factual result.  The Organ collects
// that surface inside the same action instead of making the main consciousness
// spend a second cycle proving that navigation, typing, or clicking really
// changed what is now visible.
const surfaceReportingOperations = new Set([
  'browser_fill_form',
  'browser_navigate',
  'browser_navigate_back',
  'browser_press_key',
  'browser_select_option',
  'browser_tabs',
  'browser_type',
  'browser_click',
]);

function sameNavigationTarget(left, right) {
  // Fragments select real SPA/document locations; trailing path slashes can
  // name different resources. Only URL-standard normalization is safe here.
  try { return new URL(left).href === new URL(right).href; }
  catch { return String(left) === String(right); }
}

function actionEffect(tool, status = 'completed') {
  if (status !== 'completed') return 'unknown';
  if (observedActionOperations.has(tool)) return 'observed';
  if (orientedActionOperations.has(tool)) return 'oriented';
  return 'changed';
}

const description = {
  schema: 'hominal.organ-description/v1',
  id: organID,
  name: 'Chrome browser',
  command: 'hominal-browser',
  capabilities: ['observe', 'orient', 'perform', 'cancel', 'public_web', 'authenticated_web'],
  operations: actionOperations,
	operation_inputs: {
	  browser_snapshot: '{}',
	  browser_navigate: '{"url":"https://example.com"}',
	  browser_navigate_back: '{}',
	  browser_find: '{"text":"当前页面要查找的文字"} 或 {"regex":"正则表达式"}',
	  browser_click: '{"target":"当前感知中控件的 target","element":"控件名称"}',
	  browser_type: '{"target":"当前感知中输入框的 target","text":"确定文本"}',
	  browser_fill_form: '{"fields":[{"name":"字段名","type":"textbox","value":"确定文本"}]}',
	  browser_press_key: '{"key":"Enter"}',
	  browser_wait_for: '{"time_seconds":3}',
	  browser_tabs: '{"action":"list"} 或 {"action":"select","index":0}',
	  browser_run_code_unsafe: '{"code":"async (page) => ({url: page.url(), title: await page.title()})"}',
	},
  guidance: 'browser 连接当前 Chrome 并共享其登录与网络条件。行动时只从 operations 选择 Playwright 动作名，input 使用对应 JSON 参数；browser_snapshot 配合 {} 读取当前页面。当前语义感知覆盖可见文字、控件和链接；图像与视频像素需要独立视觉解码器才能成为可解释事实，截图文件只能确认捕获成功。时长参数明确携带单位：browser_wait_for 使用 {"time_seconds":3} 或 {"time_milliseconds":3000}。需要其他参数结构时可通过 System Organ 执行 hominal-browser schema <动作名>。capabilities 描述身体内核的感知与调度能力，不是 organ_action 的 operation。',
};

function usesSnapshotElementHandle(value, key = '') {
  if (typeof value === 'string') return /^(?:ref|target)$/i.test(key) && /^(?:e\d+|f[\da-f]+)$/i.test(value);
  if (Array.isArray(value)) return value.some(item => usesSnapshotElementHandle(item, key));
  if (value && typeof value === 'object') {
    return Object.entries(value).some(([childKey, childValue]) => usesSnapshotElementHandle(childValue, childKey));
  }
  return false;
}

function normalizeToolArguments(name, value) {
  if (name === 'browser_navigate' && value && typeof value === 'object' && !Array.isArray(value)) {
    const normalized = { ...value };
    if (!normalized.url && typeof normalized.target === 'string' && normalized.target.trim()) {
      normalized.url = normalized.target.trim();
    }
    delete normalized.target;
    return normalized;
  }
  if (name === 'browser_find' && value && typeof value === 'object' && !Array.isArray(value)) {
    const normalized = { ...value };
    if (!normalized.text && typeof normalized.query === 'string' && normalized.query.trim()) {
      normalized.text = normalized.query.trim();
    }
    delete normalized.query;
    return normalized;
  }
  if (name === 'browser_wait_for' && value && typeof value === 'object' && !Array.isArray(value)) {
    const normalized = { ...value };
    // The Organ boundary exposes explicit duration units even though the
    // underlying Playwright adapter calls its seconds-valued field `time`.
    // `time` stays compatible, while a large value is safely interpreted as
    // the common millisecond form instead of monopolising the organ lane.
    if (Number.isFinite(Number(normalized.time_seconds))) {
      normalized.time = Math.max(0, Math.min(Number(normalized.time_seconds), 30));
    } else if (Number.isFinite(Number(normalized.time_milliseconds))) {
      normalized.time = Math.max(0, Math.min(Number(normalized.time_milliseconds), 30000)) / 1000;
    } else if (Number.isFinite(Number(normalized.time)) && Number(normalized.time) > 120) {
      normalized.time = Math.max(0, Math.min(Number(normalized.time), 30000)) / 1000;
    }
    delete normalized.time_seconds;
    delete normalized.time_milliseconds;
    return normalized;
  }
  if (name === 'browser_click' && value && typeof value === 'object' && !Array.isArray(value)) {
    const normalized = { ...value };
    const reference = typeof normalized.ref === 'string' ? normalized.ref.trim() : '';
    const selector = typeof normalized.selector === 'string' ? normalized.selector.trim() : '';
    const describedTarget = typeof normalized.target === 'string' ? normalized.target.trim() : '';
    if (reference) normalized.target = reference;
    else if (!describedTarget && selector) normalized.target = selector;
    if (!normalized.element) {
      if (typeof normalized.name === 'string' && normalized.name.trim()) normalized.element = normalized.name.trim();
      else if (describedTarget && describedTarget !== reference) normalized.element = describedTarget;
      else if (selector) normalized.element = selector;
    }
    // `ref`, `name`, and `type` are useful stable semantic observations, but
    // they are not fields in the pinned Playwright click schema.  The Organ is
    // the compatibility boundary: cognition should not have to learn adapter
    // version details in order to use a visible control.
    delete normalized.ref;
    delete normalized.name;
    delete normalized.type;
    delete normalized.selector;
    return normalized;
  }
  if (name === 'browser_type' && value && typeof value === 'object' && !Array.isArray(value)) {
    const normalized = { ...value };
    if (!normalized.target) normalized.target = normalized.ref || normalized.selector;
    if (normalized.text === undefined && normalized.value !== undefined) normalized.text = normalized.value;
    delete normalized.ref;
    delete normalized.selector;
    delete normalized.value;
    return normalized;
  }
  if (name === 'browser_fill_form' && value && typeof value === 'object' && !Array.isArray(value)) {
    const rawFields = Array.isArray(value.fields) ? value.fields : [value];
    return {
      fields: rawFields.map((field, index) => ({
        target: String(field?.target || field?.ref || field?.selector || '').trim(),
        name: String(field?.name || field?.label || `field ${index + 1}`).trim(),
        type: String(field?.type || 'textbox').trim(),
        value: String(field?.value ?? field?.text ?? ''),
      })),
    };
  }
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

function normalizedControlName(value) {
  return String(value || '').replace(/\s+/g, ' ').trim().toLocaleLowerCase();
}

// A visible field name is a semantic reference, while Playwright ultimately
// needs an exact page target.  The Browser Organ owns that translation: the
// main consciousness should not have to rediscover selectors that this organ
// reported in its immediately preceding perception.
function resolveFillFormTargets(value, visibleControls = []) {
  if (!value || !Array.isArray(value.fields)) return value;
  return {
    ...value,
    fields: value.fields.map(field => {
      if (String(field?.target || '').trim()) return field;
      const name = normalizedControlName(field?.name);
      const type = normalizedControlName(field?.type);
      const matches = visibleControls.filter(control => {
        const target = String(control?.target || '').trim();
        if (!target || control?.disabled) return false;
        if (normalizedControlName(control?.name) !== name) return false;
        const role = normalizedControlName(control?.role);
        return !type || role === type || (type === 'textbox' && ['input', 'textarea'].includes(role));
      });
      return matches.length === 1 ? { ...field, target: String(matches[0].target).trim() } : field;
    }),
  };
}

function validateInteractionTargets(name, value) {
  if (name === 'browser_click' || name === 'browser_type') {
    if (!String(value?.target || '').trim()) throw new RejectedActionRequest(`${name} requires a non-empty target from the current browser surface`);
  }
  if (name === 'browser_fill_form') {
    for (const field of value?.fields || []) {
      if (!String(field?.target || '').trim()) {
        throw new RejectedActionRequest(`browser_fill_form cannot uniquely resolve field ${JSON.stringify(field?.name || '')}; use its exact target from the current browser surface`);
      }
    }
  }
}

function validateToolArguments(tool, value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new RejectedActionRequest(`arguments for ${tool.name} must be a JSON object`);
  }
  const properties = tool.inputSchema?.properties;
  // The Browser Organ publishes a stricter factual contract than a permissive
  // upstream adapter.  If named fields are declared, silently ignored extras
  // cannot be reported as a successful action on another reality.
  if (!properties || typeof properties !== 'object') return;
  const unknown = Object.keys(value).filter(key => !Object.hasOwn(properties, key));
  if (unknown.length === 0) return;
  const accepted = Object.keys(properties);
  throw new RejectedActionRequest(
    `${tool.name} does not accept ${unknown.join(', ')}; accepted fields: ${accepted.length ? accepted.join(', ') : '(none)'}`,
  );
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
  const activeViewport = [];
  const articles = [];
  const articleIndexes = new Map();
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
    if (line.startsWith('- Active viewport:') && activeViewport.length < 2) {
      const value = normalizeSemanticText(line.slice(2));
      if (value && !seen.has(value)) { seen.add(value); activeViewport.push(value); }
    }
    const article = line.match(/article "(.*)" \[ref=/);
    if (article) {
      const carriesVisualMedia = /\b(?:embedded video|image|gif|video)\b/i.test(article[1]);
      let value = normalizeSemanticText(article[1]);
      if (value && carriesVisualMedia) {
        const boundary = '· 媒体感知边界：当前可核验内容是文字、控件与链接；图像或视频像素尚未进入语义感知。';
        const direct = value.match(/ Direct URL: https:\/\/\S+$/)?.[0];
        value = direct
          ? `${value.slice(0, -direct.length)} ${boundary}${direct}`
          : `${value} ${boundary}`;
      }
      const directURL = value?.match(/ Direct URL: (https:\/\/\S+)$/)?.[1];
      if (value && directURL && articleIndexes.has(directURL)) {
        const index = articleIndexes.get(directURL);
        if (carriesVisualMedia) articles[index] = value;
      } else if (value && !seen.has(value) && articles.length < 8) {
        seen.add(value);
        if (directURL) articleIndexes.set(directURL, articles.length);
        articles.push(value);
      }
      continue;
    }
    const indent = rawLine.length - rawLine.replace(/^[ \t]*/, '').length;
    while (scopes.length > 0 && scopes.at(-1).indent >= indent) scopes.pop();
    const { insideMain, insideInterface } = semanticPosition(scopes);
    const named = line.match(/(?:heading|link) "([^"]+)"/);
    if (named) {
      const value = normalizeSemanticText(named[1]);
      if (value && !seen.has(value)) {
        if (insideMain && !insideInterface && mainContent.length < 32) {
          seen.add(value); mainContent.push(value);
        } else if (!insideMain && line.includes('heading "') && !insideInterface && globalHeadings.length < 12) {
          globalHeadings.push(value);
        }
      }
    }
    const paragraph = line.match(/^- paragraph(?:\s+"[^"]*")?(?:\s+\[[^\]]+\])?:\s*(.+)$/);
    const textNode = line.match(/^- text:\s*(.+)$/);
    const prose = normalizeSemanticText(paragraph?.[1] || textNode?.[1] || '');
    if (prose && insideMain && !insideInterface && !seen.has(prose) && mainContent.length < 32) {
      seen.add(prose);
      mainContent.push(prose.slice(0, 1000));
    }
    const role = containerRole(line);
    if (role) {
      if (role === 'main') hasMain = true;
      scopes.push({ indent, role });
    }
  }
  const onX = context.some(value => /^Page URL:\s+https:\/\/(?:www\.)?x\.com\//i.test(value));
  if (onX) {
    // This is a capability fact about the current sensory surface, not a
    // separate perceptual object.  X does not reliably expose whether an
    // authored card contains image/video pixels in its accessibility name, so
    // every semantic X snapshot must make the boundary visible without
    // creating another item for attention to chase.
    context.push('媒体感知：当前快照可核验文字、控件与链接；图像或视频像素需要独立视觉解码器才能成为语义事实，等待、重复快照与截图文件仍只确认页面状态和捕获结果。');
  }
  const values = [...activeFeedback, ...activeControls, ...activeViewport, ...articles];
  if (articles.length === 0 && !onX) {
    // A normal document is one perceptual object. Its headings and links are
    // parts of that object, not a queue of unrelated environmental events.
    // Social timelines remain different: each authored article above keeps a
    // stable identity and can independently enter attention.
    const documentContent = (hasMain ? mainContent : globalHeadings).join(' · ');
    if (documentContent) values.push(`Document: ${documentContent}`);
  }
  return { context: [...new Set(context)].slice(0, 5), values };
}

function stableObject(value) {
  const direct = value.match(/ Direct URL: (https:\/\/\S+)$/)?.[1];
  const identity = direct || value;
  return {
    id: createHash('sha256').update(identity).digest('hex'),
    content: value,
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

// Notifications often live outside the current dialog or main element. Read
// the visible page-level feedback and its real links without interpreting a
// message as proof of any particular action's success.
function readPageFeedback() {
  const visible = node => {
    if (!node || node.closest('[aria-hidden="true"]') || node.getClientRects().length === 0) return false;
    for (let ancestor = node; ancestor; ancestor = ancestor.parentElement) {
      const style = getComputedStyle(ancestor);
      if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) return false;
    }
    const rect = node.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight && rect.right > 0 && rect.left < innerWidth;
  };
  const feedback = new Set();
  for (const node of Array.from(document.querySelectorAll('[aria-live]:not([aria-live="off"]), [role="alert"], [role="status"]')).slice(0, 100)) {
    if (!visible(node)) continue;
    const text = (node.innerText || node.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 240);
    const links = Array.from(node.querySelectorAll('a[href]')).filter(visible)
      .map(link => String(link.href || '')).filter(url => /^https?:\/\//i.test(url)).slice(0, 2);
    const message = [text, ...links.map(url => url.slice(0, 400))].filter(Boolean).join(' · ');
    if (message) feedback.add(message);
    if (feedback.size >= 8) break;
  }
  return Array.from(feedback);
}

// One rendered-text reader for quiet sense, post-action evidence and explicit
// snapshots. HTML landmarks are author conventions, not perceptual boundaries.
function readViewportText() {
  const root = document.body;
  const blocks = [];
  const styles = new WeakMap();
  const styleOf = node => {
    if (!styles.has(node)) styles.set(node, getComputedStyle(node));
    return styles.get(node);
  };
  let group = null;
  let parts = [];
  let characters = 0;
  const flush = () => {
    const text = parts.join('').replace(/\s+/g, ' ').trim();
    if (text) blocks.push(text);
    parts = [];
  };
  const walker = root ? document.createTreeWalker(root, NodeFilter.SHOW_TEXT) : null;
  let visited = 0;
  let node = walker?.nextNode();
  for (; node; node = walker.nextNode()) {
    visited += 1;
    if (visited > 100000) throw new Error('read_incomplete: rendered-text traversal exceeded its safety bound');
    const text = (node.textContent || '').replace(/\s+/g, ' ');
    const parent = node.parentElement;
    if (!text || !parent || parent.closest('script, style, noscript, template')) continue;
    let block = null;
    let hidden = false;
    for (let ancestor = parent; ancestor; ancestor = ancestor.parentElement) {
      const style = styleOf(ancestor);
      if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse' || Number(style.opacity) === 0) { hidden = true; break; }
      if (!block && !/^(inline|contents|inline-block|inline-flex|inline-grid)$/.test(style.display)) block = ancestor;
    }
    if (hidden) continue;
    const range = document.createRange();
    range.selectNodeContents(node);
    const inViewport = Array.from(range.getClientRects()).some(rect =>
      rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight && rect.right > 0 && rect.left < innerWidth);
    if (!inViewport) continue;
    block ||= parent;
    if (group !== block) { flush(); group = block; }
    characters += text.length;
    if (characters > 1000000) throw new Error('read_incomplete: rendered-text capture exceeded its safety bound');
    parts.push(text);
  }
  flush();
  return {
    blocks,
    coverage: {scope: 'main_document_rendered_viewport', visited_text_nodes: visited, limited: false},
  };
}

// Quiet perception and post-action verification share one bounded browser
// sense. Keeping this as one MCP call is important: a sense is an organ
// sample, not a miniature workflow of snapshot + page script + enrichment.
// The accessibility snapshot remains available as an intentional action when
// Alice needs detailed controls or element references.
const senseCode = `async (page) => {
  const hominalSense = await page.evaluate(() => {
    const visible = node => {
      if (!node || node.getClientRects().length === 0) return false;
      const style = getComputedStyle(node);
      if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) return false;
      const rect = node.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight && rect.right > 0 && rect.left < innerWidth;
    };
    const url = location.href;
    const title = document.title;
    const readyState = document.readyState;
    const feedback = (${readPageFeedback.toString()})();
    const visiblyLoading = readyState === 'loading' || Array.from(document.querySelectorAll('[role="progressbar"], [aria-busy="true"]'))
      .some(node => visible(node) && node.getAttribute('aria-busy') !== 'false');
    // Read the same current surface on every site. Authored objects and
    // controls below enrich it; they never decide which visible text exists.
    const {blocks, coverage} = (${readViewportText.toString()})();
    const reading = {blocks, coverage, visiblyLoading, scrollY: Math.round(scrollY),
      maxScrollY: Math.max(0, Math.round(document.documentElement.scrollHeight - innerHeight))};
    const readiness = visiblyLoading ? 'loading' : 'ready';
    const onX = /^https:\\/\\/(?:www\\.)?x\\.com\\//i.test(url);
    if (onX) {
      const objects = Array.from(document.querySelectorAll('article')).filter(visible).map(node => {
        const status = Array.from(node.querySelectorAll('a[href]')).find(link => /\\/status\\/\\d+(?:[?#].*)?$/.test(link.href));
        return status ? {text: node.innerText || '', url: status.href, controls: []} : null;
      }).filter(Boolean);
      // A profile is also a concrete social object. Without this fallback an
      // already-rendered profile containing no visible post looked identical
      // to a page that was still loading.
      if (objects.length === 0 && /^https:\\/\\/(?:www\\.)?x\\.com\\/[^/?#]+\\/?(?:[?#].*)?$/i.test(url)) {
        const profileParts = Array.from(document.querySelectorAll(
          '[data-testid="primaryColumn"] [data-testid="UserName"], ' +
          '[data-testid="primaryColumn"] [data-testid="UserDescription"], ' +
          '[data-testid="primaryColumn"] [data-testid="UserProfileHeader_Items"]',
        )).filter(visible)
          .map(node => (node.innerText || node.textContent || '').replace(/\\s+/g, ' ').trim())
          .filter(Boolean);
        const profileText = Array.from(new Set(profileParts)).join(' · ').slice(0, 1600);
        if (profileText) objects.push({text: 'X profile: ' + profileText, url, controls: []});
      }
      let surface = Array.from(document.querySelectorAll('[role="dialog"]')).reverse().find(visible) || null;
      if (!surface) {
        const active = document.activeElement;
        if (visible(active) && active.matches('input, textarea, [contenteditable="true"], [role="textbox"]')) {
          surface = active;
          for (let depth = 0; depth < 8 && surface.parentElement; depth += 1) {
            surface = surface.parentElement;
            const nearby = Array.from(surface.querySelectorAll('button, [role="button"], input, textarea, [contenteditable="true"], [role="textbox"]')).filter(visible);
            if (nearby.some(node => node.matches('button, [role="button"]'))) break;
          }
        }
      }
      const nodes = surface ? Array.from(new Set([
        ...(surface.matches('[role="textbox"], textarea, input, button, [role="button"]') ? [surface] : []),
        ...surface.querySelectorAll('[role="textbox"], textarea, input, button, [role="button"]'),
      ])).filter(visible).slice(0, 20) : [];
      const prefix = surface?.getAttribute('role') === 'dialog' ? '[role="dialog"] ' : '';
      const describeControl = (node, forcedTarget = '') => {
        const editableText = node.isContentEditable ? (node.innerText || node.textContent || '') : (node.value || '');
        const tag = node.tagName.toLowerCase();
        const testID = node.getAttribute('data-testid');
        const label = node.getAttribute('aria-label');
        const placeholder = node.getAttribute('placeholder');
        const candidates = [];
        if (testID) candidates.push(prefix + '[data-testid=' + JSON.stringify(testID) + ']');
        if (label) candidates.push(prefix + tag + '[aria-label=' + JSON.stringify(label) + ']');
        if (placeholder) candidates.push(prefix + tag + '[placeholder=' + JSON.stringify(placeholder) + ']');
        if (tag === 'textarea') candidates.push(prefix + 'textarea');
        const target = forcedTarget || candidates.find(candidate => {
          try { return document.querySelectorAll(candidate).length === 1; } catch { return false; }
        }) || '';
        return {
          role: node.getAttribute('role') || tag,
          name: node.getAttribute('aria-label') || node.getAttribute('placeholder') || node.innerText || node.value || '',
          disabled: Boolean(node.disabled || node.getAttribute('aria-disabled') === 'true'),
          contentLength: editableText.length,
          target,
        };
      };
      const controls = nodes.map(node => describeControl(node));
      // Interaction affordances belong to a concrete person or authored
      // object. Reporting them is perception, not a recommendation to act.
      for (const article of Array.from(document.querySelectorAll('article')).filter(visible).slice(0, 8)) {
        const status = Array.from(article.querySelectorAll('a[href]')).find(link => /\\/status\\/\\d+(?:[?#].*)?$/.test(link.href));
        const href = status?.getAttribute('href');
        if (!href) continue;
        const authoredObject = objects.find(object => object.url === status.href);
        if (!authoredObject) continue;
        // Playwright can scroll a control belonging to an already visible
        // authored object into view. Requiring the button itself to intersect
        // the viewport hid reply/like/follow whenever the text occupied most
        // of the screen, even though the action was present on that object.
        for (const node of Array.from(article.querySelectorAll('[data-testid="reply"], [data-testid="like"], [data-testid="unlike"], [data-testid="retweet"], [data-testid="unretweet"], [data-testid$="-follow"], [data-testid$="-unfollow"]'))) {
          const testID = node.getAttribute('data-testid');
          const target = 'article:has(a[href=' + JSON.stringify(href) + ']) [data-testid=' + JSON.stringify(testID) + ']';
          if (!authoredObject.controls.some(control => control.target === target)) authoredObject.controls.push(describeControl(node, target));
          if (authoredObject.controls.length >= 5) break;
        }
      }
      for (const node of Array.from(document.querySelectorAll('[data-testid$="-follow"], [data-testid$="-unfollow"]')).filter(visible)) {
        const testID = node.getAttribute('data-testid');
        const target = '[data-testid=' + JSON.stringify(testID) + ']';
        const profileObject = objects.find(object => object.url === url);
        if (profileObject && document.querySelectorAll(target).length === 1 && !profileObject.controls.some(control => control.target === target)) {
          profileObject.controls.push(describeControl(node, target));
        }
      }
      return {url, title, readyState, readiness, kind: 'social', objects, controls, feedback, ...reading};
    }
    return {
      url, title, readyState, readiness, kind: 'document', feedback, ...reading,
    };
  });
  return hominalSense;
}`;

function resultPayload(result) {
  const text = resultText(result);
  const marker = '### Result\n';
  const index = text.indexOf(marker);
  if (index < 0) throw new Error('browser sense returned no structured result');
  return JSON.parse(text.slice(index + marker.length).split('\n### ')[0].trim());
}

function observationFromSense(result) {
  const payload = resultPayload(result);
  if (payload.coverage?.limited) throw new Error('read_incomplete: browser capture was truncated');
  const context = [
    `Page URL: ${String(payload.url || '').slice(0, 2000)}`,
    `Page Title: ${String(payload.title || '').replace(/\s+/g, ' ').trim().slice(0, 500)}`,
  ];
  const values = [];
  for (const value of Array.isArray(payload.feedback) ? payload.feedback : []) {
    const text = normalizeSemanticText(value).slice(0, 1200);
    if (text) values.push(`Active feedback: ${text}`);
  }
  if (payload.kind === 'social') {
    context.push('媒体感知：当前可核验文字、控件与链接；图像或视频像素需要独立视觉解码器才能成为语义事实。');
    for (const control of Array.isArray(payload.controls) ? payload.controls : []) {
      const name = normalizeSemanticText(control?.name).slice(0, 300);
      const contentState = control?.role === 'textbox' && Number.isFinite(control?.contentLength)
        ? `; content length ${control.contentLength}` : '';
      const targetState = control?.target ? `; target ${JSON.stringify(String(control.target).replace(/[\r\n]+/g, ' ').trim())}` : '';
      values.push(`Active control: ${control?.role || 'control'} "${name}" (${control?.disabled ? 'disabled' : 'enabled'}${contentState}${targetState})`);
    }
    for (const object of Array.isArray(payload.objects) ? payload.objects : []) {
      const text = String(object?.text || '').replace(/\s+/g, ' ').trim();
      const url = String(object?.url || '').trim();
      const objectControls = (Array.isArray(object?.controls) ? object.controls : []).map(control => {
        const name = normalizeSemanticText(control?.name).slice(0, 180);
        const target = String(control?.target || '').replace(/[\r\n]+/g, ' ').trim();
        return target && !control?.disabled
          ? `${control?.role || 'control'} "${name}" target ${JSON.stringify(target)}`
          : '';
      }).filter(Boolean);
      const controlState = objectControls.length > 0 ? ` · Available actions on this object: ${objectControls.join('; ')}` : '';
      if (text && /^https:\/\//.test(url)) values.push(`${text}${controlState} · 媒体感知边界：当前可核验内容是文字、控件与链接。 Direct URL: ${url}`);
    }
  }
  {
    const blocks = (Array.isArray(payload.blocks) ? payload.blocks : [])
      .map(value => String(value || '').replace(/\s+/g, ' ').trim()).filter(Boolean);
    if (blocks.length > 0) {
      const position = Number.isFinite(payload.scrollY) && Number.isFinite(payload.maxScrollY)
        ? `scroll ${payload.scrollY}/${payload.maxScrollY}; ` : '';
      values.push(`Active viewport: ${position}${blocks.join(' · ')}`);
    }
  }
  return {
    schema: 'hominal.organ-observation/v1', organ_id: organID,
    surface_id: 'chrome.current_page', observed_at: new Date().toISOString(),
    context: context.filter(value => !value.endsWith(': ')),
    objects: [...new Set(values)].map(stableObject),
    ...((payload.feedback?.length || (payload.readiness === 'unknown' && values.length)) ? {
      interpret: {
        question: '这些页面反馈说明当前局部页面发生了什么、还不能确定什么？用一两句话解释并指出依据。',
        material: [...context.slice(0, 2), ...(payload.feedback || []), ...values.slice(0, 2)].join('\n').slice(0, 1200),
      },
    } : {}),
    facts: {
      document_ready_state: JSON.stringify(String(payload.readyState || 'unknown')),
      readiness: JSON.stringify(String(payload.readiness || 'unknown')),
      ...(typeof payload.visiblyLoading === 'boolean' ? {visible_loading: JSON.stringify(payload.visiblyLoading)} : {}),
      ...(payload.coverage ? {text_coverage: JSON.stringify(payload.coverage)} : {}),
    },
  };
}

function browserReadiness(payload, observation) {
  if (['ready', 'loading'].includes(payload?.readiness)) return payload.readiness;
  // Old test/adapter payloads do not establish more than their DOM state.
  return payload?.readyState === 'loading' ? 'loading'
    : ['interactive', 'complete'].includes(payload?.readyState) ? 'ready' : 'unknown';
}

function senseControls(payload) {
  return [
    ...(Array.isArray(payload?.controls) ? payload.controls : []),
    ...(Array.isArray(payload?.objects)
      ? payload.objects.flatMap(object => Array.isArray(object?.controls) ? object.controls : [])
      : []),
  ];
}

function browserSenseSignature(payload) {
  return JSON.stringify({
    url: payload?.url || '',
    title: payload?.title || '',
    kind: payload?.kind || '',
    objects: payload?.objects || [],
    controls: payload?.controls || [],
    feedback: payload?.feedback || [],
    blocks: payload?.blocks || [],
  });
}

function browserActionPostcondition(tool, arguments_, beforeURL, beforeSignature, payload) {
  const currentURL = String(payload?.url || '');
  const currentSignature = browserSenseSignature(payload);
  if (tool === 'browser_navigate') {
    const targetURL = String(arguments_?.url || '');
    if (!currentURL || /^chrome-error:/i.test(currentURL)) return false;
    if (currentURL === targetURL || currentURL.replace(/\/$/, '') === targetURL.replace(/\/$/, '')) return true;
    // Redirecting endpoints such as Wikipedia Special:Random and normal HTTP
    // canonicalization satisfy navigation when the browser has left the prior
    // surface and reached a semantic page. A failed goto that merely leaves
    // the old page visible does not.
    return Boolean(beforeURL && currentURL !== beforeURL);
  }
  if (tool === 'browser_navigate_back' || tool === 'browser_tabs') {
    return !beforeURL || currentURL !== beforeURL || currentSignature !== beforeSignature;
  }
  if (tool === 'browser_wait_for') return true;
  if (actionEffect(tool) === 'changed' && beforeSignature) return currentSignature !== beforeSignature;
  return true;
}

function semanticSurface(observation) {
  return {
    content: [{
      type: 'text',
      text: ['### Semantic snapshot', ...observation.context,
        ...(observation.facts ? [`Reading facts: ${JSON.stringify(observation.facts)}`] : []), 'Visible objects:',
        ...(observation.objects.length ? observation.objects.map(object => `- ${object.content}`) : ['- No semantic object is currently visible.'])].join('\n'),
    }],
    hominal_observation: observation,
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
  if (after !== before) return {url: page.url(), before, after, mode: 'scroll'};
  return {url: page.url(), before, after, exhausted: true};
}`;

function orientationFromResult(result) {
  const detail = resultText(result).replace(/\s+/g, ' ').trim().slice(0, 1200);
  return {
    schema: 'hominal.organ-orientation/v1',
    organ_id: organID,
    status: /"preserved"\s*:\s*true/.test(detail)
      ? 'preserved'
      : /"exhausted"\s*:\s*true/.test(detail) ? 'exhausted' : 'moved',
    observed_at: new Date().toISOString(),
    detail,
  };
}

function browserTabsFromResult(result) {
  const tabs = [];
  for (const line of resultText(result).split('\n')) {
    const match = line.match(/^- (\d+):\s+(?:(\(current\))\s+)?(?:\[[^\]]*\]\((https?:\/\/[^)]+)\)|(https?:\/\/\S+))/);
    if (!match) continue;
    tabs.push({ index: Number(match[1]), current: Boolean(match[2]), url: match[3] || match[4] });
  }
  return tabs;
}

class MCPConnection {
  constructor(persistent = false, defaultTimeoutMilliseconds = timeoutMilliseconds) {
    this.persistent = persistent;
    this.defaultTimeoutMilliseconds = defaultTimeoutMilliseconds;
    this.nextId = 1;
    this.stderr = '';
    this.pending = new Map();
    this.tools = null;
    this.visibleControls = [];
    this.lastSenseURL = '';
    this.lastSenseSignature = '';
    this.orientationURL = '';
    this.viewportOrientations = 0;
    this.positionRestored = false;
    this.recoveredContext = false;
    this.referencesFresh = false;
    this.tabIndicesFresh = false;
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

  async capturePosition(remainingMilliseconds) {
    // CDP target identity survives an MCP reconnect; URL, title and numeric
    // tab indexes do not identify a unique physical page.
    const response = await this.request('tools/call', {
      name: 'browser_run_code_unsafe',
      arguments: {code: `async (page) => {
        const hominalBrowserPosition = await page.context().newCDPSession(page);
        try {
          const {targetInfo} = await hominalBrowserPosition.send('Target.getTargetInfo');
          return {target_id: targetInfo.targetId};
        } finally { await hominalBrowserPosition.detach(); }
      }`},
    }, Math.max(1, remainingMilliseconds));
    if (response.error || response.result?.isError) throw new Error('browser page identity could not be captured');
    const position = resultPayload(response.result);
    if (!position?.target_id) throw new Error('browser page identity is unavailable');
    return {target_id: String(position.target_id)};
  }

  async restorePosition(position, remainingMilliseconds) {
    const deadline = Date.now() + Math.max(1, remainingMilliseconds);
    const response = await this.request('tools/call', {
      name: 'browser_run_code_unsafe',
      arguments: {code: `async (page) => {
        const hominalRestoreTarget = ${JSON.stringify(position.target_id)};
        const pages = page.context().pages();
        for (let index = 0; index < pages.length; index++) {
          let session;
          try {
            session = await page.context().newCDPSession(pages[index]);
            const {targetInfo} = await session.send('Target.getTargetInfo');
            if (targetInfo.targetId === hominalRestoreTarget) return {index};
          } catch {} finally { if (session) await session.detach().catch(() => {}); }
        }
        return {index: -1};
      }`},
    }, Math.max(1, deadline - Date.now()));
    const index = !response.error && !response.result?.isError ? resultPayload(response.result)?.index : -1;
    if (!Number.isInteger(index) || index < 0) {
      throw new RejectedActionRequest('The original browser page is unavailable after reconnect; obtain a fresh observation or list tabs before choosing another page.');
    }
    // The pinned MCP builds its tab list from context.pages() in that order.
    // Verify the selected target afterwards, so an adapter ordering change
    // cannot silently redirect an action to a different tab.
    const selected = await this.request('tools/call', {name:'browser_tabs', arguments:{action:'select', index}}, Math.max(1, deadline - Date.now()));
    if (selected.error || selected.result?.isError || (await this.capturePosition(deadline - Date.now())).target_id !== position.target_id) {
      throw new RejectedActionRequest('The original browser page could not be restored; obtain a fresh observation before acting.');
    }
    this.positionRestored = true;
    this.recoveredContext = true;
  }

  async senseAfterAction(request, deadline, tool, arguments_, beforeURL, beforeSignature, requireCausalCompletion) {
    let latest = null;
    let intervalMilliseconds = 150;
    // Finish the requested operation and capture, not an imagined future page.
    // Visible asynchronous loading remains a fact in the returned observation;
    // it is neither proof of missing content nor a reason to retry navigation.
    while (Date.now() < deadline - 100) {
      const sensed = await request('tools/call', { name: 'browser_run_code_unsafe', arguments: { code: senseCode } }, 8000);
      if (sensed.error) {
        return latest ? {...latest, timedOut: true, readiness: 'unknown'} : {timedOut: true, readiness: 'unknown'};
      }
      try {
        const payload = resultPayload(sensed.result);
        const observation = observationFromSense(sensed.result);
        this.visibleControls = senseControls(payload);
        this.lastSenseURL = String(payload?.url || '');
        this.lastSenseSignature = browserSenseSignature(payload);
        const readiness = browserReadiness(payload, observation);
        latest = { observation, surface: semanticSurface(observation), readiness, timedOut: false };
        if (['interactive', 'complete'].includes(payload.readyState) &&
          (!requireCausalCompletion || browserActionPostcondition(tool, arguments_, beforeURL, beforeSignature, payload))) return latest;
      } catch (error) {
        throw new Error(`read_incomplete: ${error.message}`);
      }
      const remaining = deadline - Date.now() - 100;
      if (remaining <= 0) break;
      await new Promise(resolve => setTimeout(resolve, Math.min(intervalMilliseconds, remaining)));
      intervalMilliseconds = Math.min(1000, intervalMilliseconds * 2);
    }
    return latest ? {...latest, timedOut: true} : {timedOut: true, readiness: 'unknown'};
  }

  async execute(requestedOperation, requestedTool, requestedArguments, requestTimeoutMilliseconds = this.defaultTimeoutMilliseconds, requireCausalCompletion = false) {
    const deadline = Date.now() + Math.max(1, requestTimeoutMilliseconds);
    const request = (method, params, maximumMilliseconds = Number.POSITIVE_INFINITY) => this.request(
	  method,
	  params,
	  Math.max(1, Math.min(deadline - Date.now(), maximumMilliseconds)),
	);
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
      const response = await request('tools/call', { name: 'browser_run_code_unsafe', arguments: { code: senseCode } });
      if (response.error) throw new Error(JSON.stringify(response.error));
      this.rememberVisibleControls(response.result);
      return observationFromSense(response.result);
    }
    if (requestedOperation === 'orient') {
      if (this.viewportOrientations >= maximumViewportOrientations) {
        const rotated = await this.rotateTab(request);
        if (rotated) return rotated;
      }
      const response = await request('tools/call', {
        name: 'browser_run_code_unsafe',
        arguments: normalizeToolArguments('browser_run_code_unsafe', { code: orientCode }),
      });
      if (response.error) throw new Error(JSON.stringify(response.error));
      const orientation = orientationFromResult(response.result);
      let payload = {};
      try { payload = resultPayload(response.result); } catch {}
      if (payload.mode === 'scroll') {
        const url = String(payload.url || '');
        this.viewportOrientations = url && url === this.orientationURL
          ? this.viewportOrientations + 1
          : 1;
        this.orientationURL = url;
        return orientation;
      }
      if (orientation.status !== 'exhausted') return orientation;
      return await this.rotateTab(request) || orientation;
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
    const tool = this.tools.find(candidate => candidate.name === requestedTool);
    if (!tool) throw new Error(`tool "${requestedTool}" not found`);
    let callArguments = normalizeToolArguments(requestedTool, requestedArguments || {});
    if (this.recoveredContext && !this.referencesFresh && usesSnapshotElementHandle(callArguments)) {
      throw new RejectedActionRequest('Browser connection recovered on the original page; use a fresh browser_find or browser_snapshot before using an element ref.');
    }
    if (this.recoveredContext && !this.tabIndicesFresh && requestedTool === 'browser_tabs' && callArguments.action === 'select') {
      throw new RejectedActionRequest('Browser connection recovered; list tabs again before selecting a session-local index.');
    }
    if (requestedTool === 'browser_fill_form') {
      callArguments = resolveFillFormTargets(callArguments, this.visibleControls);
    }
    validateInteractionTargets(requestedTool, callArguments);
    // Some MCP implementations silently discard unknown fields. That turns a
    // request for one reality into a successful observation of another (for
    // example, browser_snapshot with an ignored URL). The Organ boundary must
    // either enact the normalized request or reject it explicitly.
    validateToolArguments(tool, callArguments);
    const beforeURL = this.lastSenseURL;
    const beforeSignature = this.lastSenseSignature;
    let response;
    if (requestedTool === 'browser_navigate') {
      // The upstream navigation helper may wait for every late network request
      // and consume the full organ deadline. Bound only DOM arrival here, then
      // use the common sensory convergence loop below to decide completion.
      const navigationBudget = Math.max(1000, Math.min(8000, deadline - Date.now() - 1000));
      const code = `async (page) => {
        const hominalNavigation = {before: page.url(), requested: ${JSON.stringify(String(callArguments.url || ''))}, status: 'domcontentloaded', error: ''};
        const sameNavigationTarget = ${sameNavigationTarget.toString()};
        if (sameNavigationTarget(hominalNavigation.before, hominalNavigation.requested)) {
          hominalNavigation.status = 'already_at_target';
          hominalNavigation.url = page.url();
          hominalNavigation.title = await page.title().catch(() => '');
          return hominalNavigation;
        }
        try {
          const response = await page.goto(hominalNavigation.requested, {waitUntil: 'domcontentloaded', timeout: ${navigationBudget}});
          hominalNavigation.http_status = response ? response.status() : null;
        } catch (error) {
          hominalNavigation.status = 'incomplete';
          hominalNavigation.error = String(error).slice(0, 500);
        }
        hominalNavigation.url = page.url();
        hominalNavigation.title = await page.title().catch(() => '');
        return hominalNavigation;
      }`;
      // A browser action owns a bounded attempt inside the wider organ
	  // deadline. If the Playwright adapter itself stops responding, rebuild the
	  // connection promptly and report unknown instead of consuming the whole
	  // life-action window beside one blocked RPC.
	  response = await request(
		'tools/call',
		{ name: 'browser_run_code_unsafe', arguments: { code } },
		requireCausalCompletion ? 12000 : Number.POSITIVE_INFINITY,
	  );
    } else {
	  response = await request(
		'tools/call',
		{ name: requestedTool, arguments: callArguments },
		requireCausalCompletion ? 20000 : Number.POSITIVE_INFINITY,
	  );
    }
    if (response.error) throw new Error(JSON.stringify(response.error));
    // Playwright reports enacted-operation failures with isError inside a
    // normal MCP response.  Preserve that fact before post-action sensing;
    // otherwise a healthy page snapshot can falsely turn a failed action into
    // completed/changed Reality.
    if (response.result?.isError) return response.result;
    if (requestedTool === 'browser_find' || requestedTool === 'browser_snapshot') this.referencesFresh = true;
    if (requestedTool === 'browser_tabs' && callArguments.action === 'list') this.tabIndicesFresh = true;
    if (requestedTool === 'browser_snapshot') {
      return this.withPerceptualSurface(response.result, request);
    }
    if (surfaceReportingOperations.has(requestedTool)) {
	  const sensed = await this.senseAfterAction(request, deadline, requestedTool, callArguments, beforeURL, beforeSignature, requireCausalCompletion);
	  if (sensed) {
		if (requestedTool === 'browser_navigate') {
		  const navigation = resultPayload(response.result);
		  if (sensed.observation) sensed.observation.facts.navigation = JSON.stringify(navigation);
		  if (navigation.status === 'incomplete') sensed.timedOut = true;
		}
		const surface = sensed.surface;
		const actionText = resultText(response.result).replace(/\s+$/g, '');
		const surfaceText = surface ? resultText(compactSnapshotOutput(surface)).trim() : '';
		return {
          content: [{
            type: 'text',
            text: ['### Action result', actionText, '', surfaceText].filter(Boolean).join('\n'),
		  }],
		  hominal_post_action_surface: true,
		  ...(sensed.observation ? {hominal_observation: sensed.observation} : {}),
		  hominal_readiness: sensed.readiness,
		  hominal_readiness_timeout: Boolean(sensed.timedOut),
		};
	  }
    }
    return response.result;
  }

  async rotateTab(request) {
    // Chrome's visible front page and Playwright MCP's selected page are
    // separate state. Rotate through the adapter's own tab operation so the
    // next passive sense really addresses a different environmental surface.
    const listed = await request('tools/call', { name: 'browser_tabs', arguments: { action: 'list' } });
    if (listed.error) return null;
    const tabs = browserTabsFromResult(listed.result);
    const currentPosition = tabs.findIndex(tab => tab.current);
    if (tabs.length < 2 || currentPosition < 0) return null;
    const selected = tabs[(currentPosition + 1) % tabs.length];
    const switched = await request('tools/call', {
      name: 'browser_tabs',
      arguments: { action: 'select', index: selected.index },
    });
    if (switched.error || switched.result?.isError) return null;
    this.orientationURL = selected.url;
    this.viewportOrientations = 0;
    return {
      schema: 'hominal.organ-orientation/v1',
      organ_id: organID,
      status: 'moved',
      observed_at: new Date().toISOString(),
      detail: JSON.stringify({
        mode: 'existing_tab',
        previous_url: tabs[currentPosition].url,
        url: selected.url,
        index: selected.index,
      }),
    };
  }

  rememberVisibleControls(result) {
    try {
      const payload = resultPayload(result);
      this.visibleControls = senseControls(payload);
      this.lastSenseURL = String(payload?.url || '');
      this.lastSenseSignature = browserSenseSignature(payload);
    } catch {
      this.visibleControls = [];
    }
  }

  async withPerceptualSurface(result, request) {
    // All three entrances use the same complete current-viewport capture.
    // The raw MCP snapshot refreshes session refs; browser_find exposes a
    // selected control on demand. A failed capture is not a successful empty
    // snapshot and must not silently fall back to the old partial parser.
    const sensed = await request('tools/call', {
      name: 'browser_run_code_unsafe', arguments: {code: senseCode},
    });
    if (sensed.error || sensed.result?.isError) {
      throw new Error('read_incomplete: ' + (sensed.error ? JSON.stringify(sensed.error) : resultText(sensed.result)));
    }
    this.rememberVisibleControls(sensed.result);
    return semanticSurface(observationFromSense(sensed.result));
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
  const connection = new MCPConnection(persistent, requestTimeoutMilliseconds);
  try { return await connection.initialize(requestTimeoutMilliseconds); }
  catch (error) { connection.close(error); throw error; }
}

function compactSnapshotOutput(output) {
  // Complete organ observations already own their scope and source. Never
  // parse them again through the legacy size-limited accessibility filter.
  if (output?.hominal_observation) return semanticSurface(output.hominal_observation);
  const sourceText = resultText(output).trim();
  // The persistent daemon may already have produced the bounded semantic
  // representation. Preserve that result instead of treating its prose as a
  // second raw accessibility tree and wrapping it again.
  if (sourceText.startsWith('### Semantic snapshot') && JSON.stringify(output).length <= 24000) {
    return output;
  }
  const { context, values } = semanticObjects(sourceText);
  const visible = values.length > 0
    ? values.map(value => `- ${value}`)
    : [`- ${sourceText.replace(/\s+/g, ' ').trim().slice(0, 4000)}`];
  return {
    content: [{
      type: 'text',
      text: ['### Semantic snapshot', ...context, 'Visible objects:', ...visible].join('\n'),
    }],
    hominal_compaction: 'same-result browser semantics; use browser_find for a current control ref',
  };
}

function boundedToolOutput(tool, output) {
  if (tool !== 'browser_snapshot') return output;
  // An action result is the causal envelope seen by the life kernel.  Keep its
  // identity and status intact while independently bounding the Playwright
  // payload nested inside it. JSON string escaping can make the wrapper cross
  // the limit even when the first daemon-side size check did not.
  if (output?.schema === 'hominal.organ-action-result/v1') {
    // Failure and uncertainty carry a small causal error fact, not an
    // accessibility tree.  Reinterpreting that JSON as page semantics erases
    // the reason enactment failed and can make a rejected request look like an
    // empty successful observation.
    if (output.status !== 'completed') return output;
    try {
      const nested = JSON.parse(output.output);
      return { ...output, output: JSON.stringify(compactSnapshotOutput(nested)) };
    } catch {
      return output;
    }
  }
  return compactSnapshotOutput(output);
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
  let observation;
  if (status !== 'failed' && (requestedTool === 'browser_snapshot' || surfaceReportingOperations.has(requestedTool))) {
    try { observation = output?.hominal_observation || observationFromSnapshot(output); } catch {}
  }
  return {
    schema: 'hominal.organ-action-result/v1',
    organ_id: organID,
    action_id: actionRequest.action_id,
    status,
    effect: actionEffect(requestedTool, status),
    observed_at: new Date().toISOString(),
    summary,
    output: typeof output === 'string' ? output : JSON.stringify(boundedPublicOutput(output)),
    ...(observation ? { observation } : {}),
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
  let position = null;
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
          if (!publicOperations.has(request.operation)) throw new RejectedActionRequest('invalid browser operation');
          let remaining = job.deadline - Date.now();
          if (remaining <= 0) throw new Error('browser request expired while waiting for the stateful organ');
          if (!connection?.alive) connection = await createConnection(true, remaining);
          const pageOperation = !['list', 'schema'].includes(request.operation);
          if (pageOperation && !connection.positionRestored) {
            if (position) {
              try { await connection.restorePosition(position, job.deadline - Date.now()); }
              catch (error) {
                // Explicit fresh sensing can establish a new scene if the
                // original page disappeared. An already chosen action cannot.
                const freshObservation = request.operation === 'observe' ||
                  (request.toolName === 'browser_tabs' && request.arguments?.action === 'list');
                if (!(error instanceof RejectedActionRequest) || !freshObservation) throw error;
                connection.positionRestored = true;
                connection.recoveredContext = true;
              }
            } else {
              position = await connection.capturePosition(job.deadline - Date.now());
              connection.positionRestored = true;
            }
          }
          remaining = job.deadline - Date.now();
          if (remaining <= 0) throw new Error('browser request expired while recovering the stateful organ');
          const executedOperation = request.operation === 'perform' ? 'call' : request.operation;
          const toolResult = await connection.execute(executedOperation, request.toolName, request.arguments, remaining, request.operation === 'perform');
          if (pageOperation) position = await connection.capturePosition(job.deadline - Date.now());
          const actionFailed = Boolean(toolResult?.isError);
          const readinessUnknown = !actionFailed && Boolean(toolResult?.hominal_readiness_timeout);
          const actionStatus = actionFailed ? 'failed' : readinessUnknown ? 'unknown' : 'completed';
          const result = request.operation === 'perform'
            ? {
                schema: 'hominal.organ-action-result/v1',
                organ_id: organID,
                action_id: request.actionID,
                status: actionStatus,
                effect: actionEffect(request.toolName, actionStatus),
                observed_at: new Date().toISOString(),
                summary: actionFailed
                  ? 'Browser Organ 已完成尝试，Playwright 返回了明确失败。'
                  : readinessUnknown
                    ? 'Browser Organ 已执行操作，但在截止时间内未能确认页面达到稳定后置状态。'
                  : 'Browser Organ 已完成操作并返回 Playwright 现实结果。',
                output: JSON.stringify(boundedToolOutput(request.toolName, toolResult)),
                ...(!actionFailed && (request.toolName === 'browser_snapshot' || surfaceReportingOperations.has(request.toolName))
                  ? { observation: toolResult?.hominal_observation || observationFromSnapshot(toolResult) }
                  : {}),
              }
            : toolResult;
          job.complete = true;
          socket.end(`${JSON.stringify({ ok: true, result })}\n`);
        } catch (error) {
          if (!connection?.alive) connection = null;
          job.complete = true;
          if (request.operation === 'perform') {
            const rejected = error instanceof RejectedActionRequest;
            const readFailed = request.toolName === 'browser_snapshot' && error.message.startsWith('read_incomplete:');
            socket.end(`${JSON.stringify({ ok: true, result: {
              schema: 'hominal.organ-action-result/v1',
              organ_id: organID,
              action_id: request.actionID,
              status: rejected || readFailed ? 'failed' : 'unknown',
              effect: 'unknown',
              observed_at: new Date().toISOString(),
              summary: readFailed ? 'Browser Organ 未能完成本次读取，具体失败原因见返回事实。' : rejected
                ? 'Browser Organ 尚未执行目标操作，请按返回事实重新确定调用条件。'
                : 'Browser Organ 的操作被中断，页面是否已经部分改变尚不确定。',
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
    const result = await connection.execute(executedOperation, requestedTool, requestedArguments, requestedTimeoutMilliseconds, operation === 'perform');
    if (operation === 'perform') {
      const status = result?.isError ? 'failed' : result?.hominal_readiness_timeout ? 'unknown' : 'completed';
      writePublicOutput(actionResult(
        status,
        result,
        status === 'failed'
          ? 'Browser Organ 已完成尝试，Playwright 返回了明确失败。'
          : status === 'unknown'
            ? 'Browser Organ 已执行操作，但在截止时间内未能确认页面达到稳定后置状态。'
            : 'Browser Organ 已完成操作并返回 Playwright 现实结果。',
      ));
    } else {
      writePublicOutput(result);
    }
  } catch (error) {
    if (operation === 'perform') {
      const rejected = error instanceof RejectedActionRequest;
      writePublicOutput(actionResult(
        rejected ? 'failed' : 'unknown',
        JSON.stringify({ error: error.message }),
        rejected
          ? 'Browser Organ 已拒绝不符合动作契约的请求，页面没有因本次请求发生改变。'
          : 'Browser Organ 的操作被中断，页面是否已经部分改变尚不确定。',
      ));
    } else {
      console.error(error.message);
      process.exitCode = 1;
    }
  } finally {
    connection?.close();
  }
}
