# Playwright MCP for the current Chrome

The Ubuntu host uses `@playwright/mcp@0.0.79` in the persistent npm prefix under `/agent/state/development/npm-global`.

`/usr/local/bin/hominal-playwright-mcp` is the MCP stdio command. It detects the current desktop user by UID, starts the existing GUI Chrome when necessary, waits for `127.0.0.1:9222`, and then connects Playwright MCP with `--cdp-endpoint=http://127.0.0.1:9222`.

Use `playwright-current-chrome.json` as the generic MCP client configuration, or configure the client to run this command directly:

```text
/usr/local/bin/hominal-playwright-mcp
```

Run the protocol and browser attachment smoke test on Ubuntu with:

```bash
node /agent/app/hominal.cc/ops/ubuntu/mcp/playwright-smoke-test.mjs
```

The CDP endpoint is intentionally bound to Ubuntu loopback only. Playwright MCP controls the live Chrome profile and tabs, so its callers receive the same authority as the logged-in browser session.
