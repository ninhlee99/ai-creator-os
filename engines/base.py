"""Engine interfaces: every capability is a chain (free API -> local -> paid)."""
from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass


@dataclass
class EngineResult:
    ok: bool
    provider: str
    payload: bytes | str | None = None
    error: str = ""


class Engine(ABC):
    name: str = "engine"

    @abstractmethod
    def run(self, *args, **kwargs) -> EngineResult:
        ...


class Chain(Engine):
    """Try providers in order; first success wins. Usage is metered."""

    def __init__(self, providers: list[Engine], meter=None):
        self.providers = providers
        self.meter = meter

    def run(self, *args, **kwargs) -> EngineResult:
        last_error = "no providers configured"
        for p in self.providers:
            try:
                res = p.run(*args, **kwargs)
            except Exception as e:  # provider must never crash the chain
                res = EngineResult(ok=False, provider=p.name, error=str(e))
            if self.meter:
                self.meter(p.name, res.ok)
            if res.ok:
                return res
            last_error = res.error or f"{p.name} failed"
        return EngineResult(ok=False, provider="chain", error=last_error)
