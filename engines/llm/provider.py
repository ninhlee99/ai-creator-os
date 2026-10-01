"""LLM engine: Gemini free tier -> Ollama local.

Priority: free realtime API first, local fallback. Paid tier is opt-in only.
"""
from __future__ import annotations

import json
import urllib.request

from ..base import Engine, EngineResult


class GeminiFree(Engine):
    name = "gemini-free"

    def __init__(self, api_key: str, model: str = "gemini-2.0-flash"):
        self.api_key = api_key
        self.model = model

    def run(self, prompt: str, system: str = "", **kw) -> EngineResult:
        if not self.api_key:
            return EngineResult(False, self.name, error="no API key")
        url = (f"https://generativelanguage.googleapis.com/v1beta/models/"
               f"{self.model}:generateContent?key={self.api_key}")
        body = {"contents": [{"parts": [{"text": prompt}]}]}
        if system:
            body["system_instruction"] = {"parts": [{"text": system}]}
        req = urllib.request.Request(
            url, data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"}, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                data = json.load(r)
            text = data["candidates"][0]["content"]["parts"][0]["text"]
            return EngineResult(True, self.name, payload=text)
        except Exception as e:
            return EngineResult(False, self.name, error=str(e))


class OllamaLocal(Engine):
    name = "ollama-local"

    def __init__(self, base_url: str, model: str):
        self.base_url = base_url.rstrip("/")
        self.model = model

    def run(self, prompt: str, system: str = "", **kw) -> EngineResult:
        body = {"model": self.model, "prompt": prompt,
                "system": system, "stream": False}
        req = urllib.request.Request(
            f"{self.base_url}/api/generate", data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"}, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                data = json.load(r)
            return EngineResult(True, self.name,
                                payload=data.get("response", ""))
        except Exception as e:
            return EngineResult(False, self.name, error=str(e))


def build_llm(api_key: str, ollama_url: str, ollama_model: str,
              meter=None):
    from ..base import Chain
    return Chain([GeminiFree(api_key), OllamaLocal(ollama_url, ollama_model)],
                 meter=meter)
