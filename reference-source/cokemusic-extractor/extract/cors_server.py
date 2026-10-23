#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""CORS-enabled static file server for dirplayer cast extraction."""
import sys
import os
from http.server import HTTPServer, SimpleHTTPRequestHandler


class CorsHandler(SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "*")
        super().end_headers()

    def do_OPTIONS(self):
        self.send_response(204)
        self.end_headers()


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8765
    serve_dir = sys.argv[2] if len(sys.argv) > 2 else "."
    os.chdir(serve_dir)
    print(f"CORS-enabled server on http://127.0.0.1:{port} serving {os.getcwd()}", flush=True)
    HTTPServer(("127.0.0.1", port), CorsHandler).serve_forever()
