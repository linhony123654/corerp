#!/usr/bin/env python3
"""BGE-small-zh embedding microservice for CoreRP.
Listens on localhost:8765, returns 512-dim vectors for Chinese text.
Start: python3 embed_server.py
"""

import json
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler

MODEL = None

def load_model():
    global MODEL
    from sentence_transformers import SentenceTransformer
    print(f"[embed] Loading BAAI/bge-small-zh-v1.5 ...", file=sys.stderr)
    MODEL = SentenceTransformer("BAAI/bge-small-zh-v1.5")
    print(f"[embed] Model loaded. dim={MODEL.get_sentence_embedding_dimension()}", file=sys.stderr)

class EmbedHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/embed":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        texts = body.get("texts", [])

        if not texts:
            self.send_error(400, "Missing 'texts' field")
            return

        # bge models need "为这个句子生成表示以用于检索相关文章：" prefix for queries
        embeddings = []
        for text in texts:
            vec = MODEL.encode(text, normalize_embeddings=True)
            embeddings.append(vec.tolist())

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        resp = json.dumps({"embeddings": embeddings, "dim": len(embeddings[0])}, ensure_ascii=False)
        self.wfile.write(resp.encode())

    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok"}')
        else:
            self.send_error(404)

    def log_message(self, format, *args):
        pass  # suppress noisy access logs

if __name__ == "__main__":
    load_model()
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8765
    server = HTTPServer(("127.0.0.1", port), EmbedHandler)
    print(f"[embed] Listening on 127.0.0.1:{port}", file=sys.stderr)
    server.serve_forever()
