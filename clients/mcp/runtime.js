// Fixed, operator-configured upstream. Tools cannot choose URLs or credentials.
export class RuntimeClient {
  #origin; #token; #fetch;
  constructor(origin, token, fetchImpl = globalThis.fetch) {
    let url;
    try { url = new URL(origin); } catch { throw new Error('Invalid CoreRP origin.'); }
    if ((url.protocol !== 'https:' && !(url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))) ||
        url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw new Error('CoreRP requires HTTPS or loopback HTTP origin without path or credentials.');
    if (typeof token !== 'string' || !token.trim() || /[\r\n]/u.test(token)) throw new Error('CoreRP token is required.');
    this.#origin = url.origin; this.#token = token; this.#fetch = fetchImpl.bind(globalThis);
  }

  async call(route, body, signal, method = 'POST') {
    try {
      const response = await this.#fetch(`${this.#origin}/api/v1/rp/${route}`, {
        method, body: method === 'POST' ? JSON.stringify(body) : undefined,
        headers: { Authorization: `Bearer ${this.#token}`, 'Content-Type': 'application/json' },
        credentials: 'omit', redirect: 'error', cache: 'no-store',
        signal: AbortSignal.any([AbortSignal.timeout(120_000), ...(signal ? [signal] : [])]),
      });
      const reader = response.body?.getReader();
      if (!reader) throw new Error('missing body');
      let size = 0; const chunks = [];
      try {
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          size += value.byteLength;
          if (size > 4 * 1024 * 1024) throw new Error('oversized response');
          chunks.push(value);
        }
      } finally { await reader.cancel(); reader.releaseLock(); }
      const envelope = JSON.parse(Buffer.concat(chunks).toString('utf8'));
      if (!response.ok) {
        const known = new Set(['INVALID_ARGUMENT', 'AUTHENTICATION_REQUIRED', 'NOT_FOUND', 'PERMISSION_DENIED', 'IDEMPOTENCY_PAYLOAD_MISMATCH', 'COMMAND_IN_PROGRESS', 'BRANCH_VERSION_CONFLICT', 'REQUEST_RETIRED', 'INSUFFICIENT_FUNDS', 'INSUFFICIENT_STOCK', 'STORAGE_FAILURE']);
        const code = known.has(envelope.error?.code) ? envelope.error.code : 'RUNTIME_ERROR';
        return { error: { code, http_status: response.status, message: 'Runtime rejected or could not finish the call. Preserve any original pending request; an error does not prove non-acceptance.' } };
      }
      if (!envelope || typeof envelope.data !== 'object' || envelope.data === null) throw new Error('invalid envelope');
      return { data: envelope.data };
    } catch {
      // Never print raw fetch errors, URLs, upstream messages or bearer tokens.
      return { error: { code: 'TRANSPORT_UNCERTAIN', message: 'No confirmed result. Preserve the exact request/key; retry it or resolve with corerp_request_retire. Do not assume cancellation rolled back effects.' } };
    }
  }
}
