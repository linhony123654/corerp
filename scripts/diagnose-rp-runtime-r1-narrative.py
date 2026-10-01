"""Replay the same committed test-world turn with two runtime binaries.

Only the already-authorized narrator talks to the configured Step relay.
Record response shape/counts and public prose, never credentials or reasoning.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shlex
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument('--source-db', required=True)
parser.add_argument('--turn-run-id', required=True)
parser.add_argument('--before-runtime', required=True)
parser.add_argument('--after-runtime', required=True)
args = parser.parse_args()
source_path = Path(args.source_db).resolve()
artifact = Path(tempfile.mkdtemp(prefix='corerp-r1-narrative-controls-'))
source = sqlite3.connect('file:' + str(source_path) + '?mode=ro', uri=True)
session, world = source.execute('SELECT session_id,instance_id FROM rp_turn_runs JOIN rp_sessions USING(session_id) WHERE turn_run_id=?', (args.turn_run_id,)).fetchone()
assert world == 'r1-short-test-world', 'this diagnostic accepts only the independent short-test world'
config = {}
for line in (Path('/home/ubuntu/.local/share/corerp-preview/env')).read_text().splitlines():
    name, sep, raw = line.removeprefix('export ').partition('=')
    if sep and name.strip() in ('CORERP_LLM_ENDPOINT', 'CORERP_LLM_API_KEY'):
        parts = shlex.split(raw, comments=True)
        if len(parts) == 1:
            config[name.strip()] = parts[0]
assert len(config) == 2
endpoint = config['CORERP_LLM_ENDPOINT'].rstrip('/')
if not endpoint.endswith('/chat/completions'):
    endpoint += '/chat/completions'
wire = []
current = {'label': ''}


class Relay(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        raw = self.rfile.read(int(self.headers['Content-Length']))
        request = json.loads(raw)
        row = {'label': current['label'], 'reasoning_effort': request.get('reasoning_effort'),
               'enable_thinking_present': 'enable_thinking' in request,
               'max_completion_tokens': request.get('max_completion_tokens'), 'request_bytes': len(raw)}
        started = time.monotonic()
        remote = urllib.request.Request(endpoint, data=raw, headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + config['CORERP_LLM_API_KEY']})
        try:
            with urllib.request.urlopen(remote, timeout=95) as response:
                code, body = response.status, response.read(2 << 20)
        except urllib.error.HTTPError as error:
            code, body = error.code, error.read(2 << 20)
        except (OSError, urllib.error.URLError):
            code, body = 502, b'{"error":{"message":"diagnostic transport unavailable"}}'
        row.update(http_status=code, seconds=round(time.monotonic() - started, 3))
        try:
            envelope = json.loads(body)
            choice = envelope.get('choices', [{}])[0]
            reason = choice.get('finish_reason')
            row['finish_reason'] = reason if reason in ('stop', 'length', 'tool_calls', 'content_filter') else 'other'
            message = choice.get('message', {})
            row['content_bytes'] = len((message.get('content') or '').encode())
            row['reasoning_bytes'] = {k: len((message.get(k) or '').encode()) for k in ('reasoning', 'reasoning_content')}
            usage = envelope.get('usage', {})
            row['usage'] = {k: usage[k] for k in ('prompt_tokens', 'completion_tokens', 'total_tokens') if isinstance(usage.get(k), int)}
        except (ValueError, KeyError, IndexError, TypeError, AttributeError):
            row['response_shape'] = 'unrecognized'
        wire.append(row)
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass


proxy = ThreadingHTTPServer(('127.0.0.1', 0), Relay)
proxy.daemon_threads = True
threading.Thread(target=proxy.serve_forever, daemon=True).start()
local_endpoint = 'http://127.0.0.1:' + str(proxy.server_port) + '/v1/chat/completions'
result = {'kind': 'corerp.r1-same-facts-narrative-controls.v1', 'world': world,
          'turn_run_id': args.turn_run_id, 'source_database': str(source_path), 'artifact_directory': str(artifact),
          'model': 'step-5-preview', 'human_experience': 'PENDING', 'runs': [],
          'limits': 'Same committed public turn and identical explicit model override; different runtime binaries. Two stochastic runs are not a latency or long-RP benchmark. Independent test-author world, not approved real Rongqing canon or frozen Golden.'}


def counts(conn):
    return {'head': conn.execute('SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?', (world, 'br_main')).fetchone()[0],
            'events': conn.execute('SELECT COUNT(*) FROM events WHERE instance_id=?', (world,)).fetchone()[0],
            'utterances': conn.execute('SELECT COUNT(*) FROM rp_utterances WHERE session_id=?', (session,)).fetchone()[0],
            'decisions': conn.execute('SELECT COUNT(*) FROM rp_npc_decisions').fetchone()[0],
            'turn_runs': conn.execute('SELECT COUNT(*) FROM rp_turn_runs').fetchone()[0]}


try:
    for label, binary in [('before', args.before_runtime), ('after', args.after_runtime)]:
        current['label'] = label
        db = artifact / (label + '.db')
        conn = sqlite3.connect(db)
        source.backup(conn)
        before = counts(conn)
        initial_call = conn.execute('SELECT COALESCE(MAX(rowid),0) FROM rp_provider_calls').fetchone()[0]
        token = secrets.token_hex(24)
        with socket.socket() as free:
            free.bind(('127.0.0.1', 0))
            port = free.getsockname()[1]
        origin = 'http://127.0.0.1:' + str(port)
        env = os.environ.copy()
        env.update(CORERP_AUTH_TOKENS_JSON=json.dumps({token: 'principal_m2_rp_player'}), CORERP_CURSOR_SECRET=secrets.token_hex(32),
                   CORERP_DECISION_PROVIDER='deterministic', CORERP_PROVIDER_ALLOWLIST='', CORERP_PROVIDER_LOCAL_ALLOWLIST=local_endpoint,
                   CORERP_NARRATIVE_PROVIDER='full_prose', CORERP_NARRATIVE_ENDPOINT=local_endpoint,
                   CORERP_NARRATIVE_API_KEY='local-diagnostic-key', CORERP_NARRATIVE_MODEL='step-5-preview',
                   CORERP_NARRATIVE_TIMEOUT='90s', CORERP_NARRATIVE_ATTEMPTS='2',
                   CORERP_NARRATIVE_REASONING_EFFORT='low', CORERP_NARRATIVE_DISABLE_THINKING='false')
        runtime = subprocess.Popen([binary, '-db', str(db), '-listen', '127.0.0.1:' + str(port)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        run = {'label': label, 'runtime_sha256': hashlib.sha256(Path(binary).read_bytes()).hexdigest(), 'status': 'NOT VERIFIED'}
        try:
            for _ in range(150):
                try:
                    with urllib.request.urlopen(origin + '/readyz', timeout=1) as response:
                        if response.status == 200:
                            break
                except (OSError, urllib.error.URLError):
                    time.sleep(.1)
            else:
                raise AssertionError('isolated runtime did not start')
            body = {'session_id': session, 'turn_run_id': args.turn_run_id,
                    # An explicit variant asks for a new render; an original
                    # read can return the saved official narrative with no call.
                    'style_override': {'narrative_density': 'standard', 'verbosity': 'normal',
                                       'dialogue_ratio': 60, 'description_density': 70, 'full_prose': True},
                    'model': {'endpoint': local_endpoint, 'model': 'step-5-preview', 'api_key': 'local-diagnostic-key',
                              'timeout_seconds': 120, 'reasoning_effort': 'low', 'full_prose': True}}
            request = urllib.request.Request(origin + '/api/v1/rp/narrative/render', data=json.dumps(body).encode(),
                                             headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
            started = time.monotonic()
            with urllib.request.urlopen(request, timeout=180) as response:
                data = json.load(response)['data']
            view = data['view']
            run.update(public_lines=view['lines'], source_event_ids=view['event_ids'],
                       prose_fallback=view.get('fallback_reason', ''), seconds=round(time.monotonic() - started, 3))
            run['world_before'] = before
            run['world_after'] = counts(conn)
            assert before == run['world_after'], 'render changed committed world or decisions'
            run['status'] = 'PASS' if not run['prose_fallback'] else 'FAIL: prose fallback'
        except Exception as error:
            run['status'] = 'FAIL'
            run['failure_kind'] = type(error).__name__
        finally:
            if runtime.poll() is None:
                runtime.terminate()
                try:
                    runtime.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    runtime.kill(); runtime.wait(timeout=5)
            run['provider_receipts'] = [dict(zip(('phase', 'provider_kind', 'result', 'attempt_count', 'fallback_kind', 'render_source'), row)) for row in conn.execute('SELECT phase,provider_kind,result,attempt_count,fallback_kind,render_source FROM rp_provider_calls WHERE rowid>? ORDER BY rowid', (initial_call,))]
            conn.close()
            result['runs'].append(run)
            (artifact / 'result.json').write_text(json.dumps({**result, 'wire': wire}, ensure_ascii=False, indent=2) + '\n')
            print(json.dumps(run, ensure_ascii=False), flush=True)
finally:
    proxy.shutdown(); proxy.server_close(); source.close()
    result['wire'] = wire
    (artifact / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'artifact': str(artifact / 'result.json'), 'statuses': [r['status'] for r in result['runs']]}, ensure_ascii=False), flush=True)
    if result['runs'][-1]['status'] != 'PASS':
        raise SystemExit(1)
