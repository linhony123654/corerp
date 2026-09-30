// Browser-only transport: no world state, storage, credentials persistence or
// automatic command retries. The host adapter owns binding/recovery UI.
export class CoreRPClient {
  #origin;
  #token;
  #fetch;

  constructor(origin, token, fetchImpl = globalThis.fetch) {
    const url = new URL(origin);
    const local = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
    if ((url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) ||
        url.username || url.password || url.pathname !== '/' || url.search || url.hash) {
      throw new Error('CoreRP requires an HTTPS origin or a local HTTP origin.');
    }
    if (typeof token !== 'string' || !token.trim() || /[\r\n]/u.test(token)) {
      throw new Error('A CoreRP bearer token is required.');
    }
    this.#origin = url.origin;
    this.#token = token;
    this.#fetch = fetchImpl.bind(globalThis);
  }

  async #request(path, options = {}) {
    const headers = new Headers(options.headers);
    headers.set('Authorization', `Bearer ${this.#token}`);
    const response = await this.#fetch(this.#origin + path, {
      ...options, headers, credentials: 'omit', redirect: 'error', cache: 'no-store',
    });
    if (!response.ok) {
      // Do not reflect arbitrary remote bodies or credential-bearing URLs into
      // host prompts/logs. Status is enough to retain an ambiguous pending write.
      await response.body?.cancel();
      const error = new Error(`CoreRP HTTP ${response.status}; refresh or retry the saved command.`);
      error.status = response.status;
      throw error;
    }
    return response;
  }

  async call(operation, body, signal) {
    const paths = {
      open: '/api/v1/rp/sessions/open', resume: '/api/v1/rp/sessions/resume',
      session: '/api/v1/rp/sessions/read', observe: '/api/v1/rp/observe',
      context: '/api/v1/rp/context/read', dialogue: '/api/v1/rp/turns/run',
      resumeTurn: '/api/v1/rp/turns/resume', move: '/api/v1/rp/actions/move',
      social: '/api/v1/rp/actions/social', wait: '/api/v1/rp/actions/wait',
      object: '/api/v1/rp/actions/object', nonverbal: '/api/v1/rp/actions/nonverbal',
      retire: '/api/v1/rp/requests/retire',
    };
    if (!Object.hasOwn(paths, operation)) throw new Error('Unsupported CoreRP operation.');
    const response = await this.#request(paths[operation], {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body), signal,
    });
    const envelope = await response.json();
    if (!Object.hasOwn(envelope, 'data')) throw new Error('Invalid CoreRP response envelope.');
    return envelope.data;
  }

  async *events(sessionID, cursor, signal) {
    const query = new URLSearchParams({ session_id: sessionID });
    const headers = { Accept: 'text/event-stream' };
    if (cursor) headers['Last-Event-ID'] = cursor;
    const response = await this.#request(`/api/v1/rp/events/stream?${query}`, { headers, signal });
    if (!response.headers.get('Content-Type')?.startsWith('text/event-stream') || !response.body) {
      await response.body?.cancel();
      throw new Error('Invalid CoreRP event stream.');
    }
    yield* readRPFrames(response.body);
  }
}

// Frame IDs become usable only after complete frames are yielded. Callers must
// persist them after processing, never on receipt of an isolated id line.
export async function* readRPFrames(stream) {
  const reader = stream.getReader();
  const decoder = new TextDecoder('utf-8', { fatal: true });
  let buffer = '', size = 0, id = '', type = '', data = [];
  try {
    for (;;) {
      const { done, value } = await reader.read();
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
      let end;
      while ((end = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, end).replace(/\r$/u, '');
        buffer = buffer.slice(end + 1);
        size += line.length;
        if (size > 1024 * 1024) throw new Error('CoreRP stream frame is too large.');
        if (!line) {
          if (data.length) {
            if (!id || !['rp_event', 'rp_checkpoint'].includes(type)) throw new Error('Invalid CoreRP frame scope.');
            yield { id, type, data: JSON.parse(data.join('\n')) };
          }
          size = 0; id = ''; type = ''; data = [];
          continue;
        }
        if (line.startsWith(':')) continue;
        const colon = line.indexOf(':');
        const name = colon < 0 ? line : line.slice(0, colon);
        const content = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /u, '');
        if (name === 'id') {
          if (content.includes('\0')) throw new Error('Invalid CoreRP frame ID.');
          id = content;
        }
        if (name === 'event') type = content;
        if (name === 'data') data.push(content);
      }
      if (buffer.length + size > 1024 * 1024) throw new Error('CoreRP stream frame is too large.');
      if (done) {
        if (buffer || data.length || id || type) throw new Error('CoreRP stream ended mid-frame; reconnect from the last processed checkpoint.');
        return;
      }
    }
  } finally {
    try { await reader.cancel(); } finally { reader.releaseLock(); }
  }
}
