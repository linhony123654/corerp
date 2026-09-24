import { serveStdio } from '@modelcontextprotocol/server/stdio';
import { RuntimeClient } from './runtime.js';
import { createServer } from './server.js';

try {
  const runtime = new RuntimeClient(process.env.CORERP_ORIGIN, process.env.CORERP_TOKEN);
  const handle = serveStdio(() => createServer(runtime));
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { void handle.close(); });
} catch {
  console.error('CoreRP MCP could not start. Check CORERP_ORIGIN and CORERP_TOKEN; credentials are never tool arguments.');
  process.exitCode = 1;
}
