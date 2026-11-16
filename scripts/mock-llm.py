#!/usr/bin/env python3
"""Deterministic OpenAI-compatible mock for local SELA container tests.

It implements POST /v1/chat/completions and returns:
- a memory JSON payload for the memory extraction prompt,
- an insight JSON payload with evidence copied from the prompt sources,
- plain companion text otherwise.
"""

import json
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SOURCE_RE = re.compile(r'<(journal|message|memory) id="([^"]+)"')

MEMORY = {
    "memories": [
        {
            "category": "routine",
            "content": "Pengguna ingin tidur lebih awal dan lebih tenang menghadapi pekerjaan.",
            "confidence": 0.9,
            "certainty_type": "explicit",
            "source_ids": [],
            "sensitivity": "normal",
            "conflicts_with": [],
        }
    ]
}


def insight_payload(system: str) -> dict:
    evidence = [
        {
            "source_type": source_type,
            "source_id": source_id,
            "note": "Synthetic evidence from the local smoke source block.",
        }
        for source_type, source_id in SOURCE_RE.findall(system)[:3]
    ]
    return {
        "title": "Synthetic pattern from local smoke",
        "summary": "Dua catatan sintetis menunjukkan keinginan untuk tidur lebih awal dan mengelola kecemasan kerja.",
        "observation": "Catatan menyebut tidur lebih awal dan kecemasan pekerjaan.",
        "interpretation": "Ini bisa menjadi sinyal awal keinginan membangun rutinitas yang lebih tenang.",
        "hypothesis": "Rutinitas malam yang konsisten dapat menurunkan kecemasan.",
        "confidence": 0.6,
        "reflection_question": "Apa satu langkah kecil yang bisa dicoba malam ini?",
        "suggested_direction": "improve",
        "evidence": evidence,
    }


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_POST(self):  # noqa: N802 - stdlib naming
        if not self.path.endswith("/chat/completions"):
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            self.send_error(400)
            return

        messages = body.get("messages") or []
        system = next((m.get("content", "") for m in messages if m.get("role") == "system"), "")
        user = next((m.get("content", "") for m in messages if m.get("role") == "user"), "")

        if "Insight Generation" in system:
            content = json.dumps(insight_payload(system))
        elif "Memory Extraction" in system:
            memory = json.loads(json.dumps(MEMORY))
            ids = SOURCE_RE.findall(user)
            if ids:
                memory["memories"][0]["source_ids"] = [ids[0][1]]
            else:
                memory = {"memories": []}
            content = json.dumps(memory)
        else:
            content = "Saya mendengar Anda. Apa yang paling penting saat ini?"

        response = {
            "id": "mock-completion",
            "object": "chat.completion",
            "created": 0,
            "model": body.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": content},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
        }
        data = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        sys.stderr.write(fmt % args + "\n")


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 9090), Handler).serve_forever()
