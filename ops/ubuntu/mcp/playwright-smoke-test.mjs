import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';

const command = process.argv[2] || '/usr/local/bin/hominal-playwright-mcp';
const child = spawn(command, [], {
  cwd: '/agent/app/hominal.cc',
  env: process.env,
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
  const message = JSON.parse(line);
  if (message.id !== undefined && pending.has(message.id)) {
    const { resolve } = pending.get(message.id);
    pending.delete(message.id);
    resolve(message);
  }
});

function request(method, params) {
  const id = nextId++;
  const response = new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`Timed out waiting for ${method}: ${stderr}`));
    }, 15000);
    pending.set(id, {
      resolve: message => {
        clearTimeout(timer);
        resolve(message);
      },
    });
  });
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`);
  return response;
}

try {
  const initialized = await request('initialize', {
    protocolVersion: '2025-03-26',
    capabilities: {},
    clientInfo: { name: 'hominal-playwright-smoke', version: '1.0.0' },
  });
  if (initialized.error) throw new Error(JSON.stringify(initialized.error));

  child.stdin.write(`${JSON.stringify({
    jsonrpc: '2.0',
    method: 'notifications/initialized',
    params: {},
  })}\n`);

  const listed = await request('tools/list', {});
  if (listed.error) throw new Error(JSON.stringify(listed.error));
  const tools = listed.result?.tools || [];
  if (!tools.some(tool => tool.name === 'browser_snapshot')) {
    throw new Error('browser_snapshot tool is missing');
  }

  const snapshot = await request('tools/call', {
    name: 'browser_snapshot',
    arguments: {},
  });
  if (snapshot.error || snapshot.result?.isError) {
    throw new Error(JSON.stringify(snapshot.error || snapshot.result));
  }

  const snapshotText = (snapshot.result?.content || [])
    .filter(item => item.type === 'text')
    .map(item => item.text)
    .join('\n');

  console.log(JSON.stringify({
    protocolVersion: initialized.result.protocolVersion,
    server: initialized.result.serverInfo,
    toolCount: tools.length,
    snapshotConnected: snapshotText.length > 0,
    acceptancePageVisible: snapshotText.includes('Ubuntu GUI 运行环境已启动'),
  }, null, 2));
} finally {
  child.stdin.end();
  child.kill('SIGTERM');
}
