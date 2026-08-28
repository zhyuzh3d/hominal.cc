#!/usr/bin/env node

import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';

const [operation, toolName, argumentsText] = process.argv.slice(2);
if (!['list', 'schema', 'call'].includes(operation)) {
  console.error("usage: hominal-browser list | hominal-browser schema <tool_name> | hominal-browser call <tool_name> '<json_arguments>'");
  process.exit(2);
}
if ((operation === 'schema' || operation === 'call') && !toolName) {
  console.error('tool name is required');
  process.exit(2);
}

let toolArguments = {};
if (operation === 'call' && argumentsText) {
  try {
    toolArguments = JSON.parse(argumentsText);
  } catch (error) {
    console.error(`invalid JSON arguments: ${error.message}`);
    process.exit(2);
  }
}

const command = process.env.HOMINAL_PLAYWRIGHT_MCP_COMMAND || '/usr/local/bin/hominal-playwright-mcp';
const timeoutMilliseconds = Number(process.env.HOMINAL_BROWSER_TIMEOUT_MS || 30000);
const child = spawn(command, [], {
  cwd: process.env.HOMINAL_INSTANCE_ROOT || process.cwd(),
  env: process.env,
  detached: true,
  stdio: ['pipe', 'pipe', 'pipe'],
});

let nextId = 1;
let stderr = '';
const pending = new Map();
const lines = createInterface({ input: child.stdout });

child.stderr.on('data', chunk => {
  stderr += chunk.toString();
});

lines.on('line', line => {
  if (!line.trim()) return;
  let message;
  try {
    message = JSON.parse(line);
  } catch {
    return;
  }
  const waiting = pending.get(message.id);
  if (!waiting) return;
  pending.delete(message.id);
  clearTimeout(waiting.timer);
  waiting.resolve(message);
});

function request(method, params) {
  const id = nextId++;
  const response = new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`timed out waiting for ${method}${stderr ? `: ${stderr.trim()}` : ''}`));
    }, timeoutMilliseconds);
    pending.set(id, { resolve, reject, timer });
  });
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`);
  return response;
}

try {
  const initialized = await request('initialize', {
    protocolVersion: '2025-03-26',
    capabilities: {},
    clientInfo: { name: 'hominal-browser', version: '1.0.0' },
  });
  if (initialized.error) throw new Error(JSON.stringify(initialized.error));
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized', params: {} })}\n`);

  const response = operation === 'call'
    ? await request('tools/call', { name: toolName, arguments: toolArguments })
    : await request('tools/list', {});
  if (response.error) throw new Error(JSON.stringify(response.error));
  let output = response.result;
  if (operation === 'list') {
    output = {
      tools: (response.result?.tools || []).map(({ name, description }) => ({ name, description })),
    };
  } else if (operation === 'schema') {
    output = (response.result?.tools || []).find(tool => tool.name === toolName);
    if (!output) throw new Error(`tool "${toolName}" not found`);
  }
  process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
  if (output?.isError) process.exitCode = 1;
} catch (error) {
  console.error(error.message);
  if (stderr.trim()) console.error(stderr.trim());
  process.exitCode = 1;
} finally {
  child.stdin.end();
  try {
    process.kill(-child.pid, 'SIGTERM');
  } catch {
    child.kill('SIGTERM');
  }
  lines.close();
  child.stdout.destroy();
  child.stderr.destroy();
}
