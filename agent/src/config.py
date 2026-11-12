"""Настройки приложения поступают только из окружения процесса."""
import os
from dataclasses import dataclass

def required(name):
    value = os.environ.get(name, "").strip()
    if not value:
        raise ValueError(f"Не задана переменная окружения {name}")
    return value

def flag(name):
    value = required(name).lower()
    if value not in ("true", "false"):
        raise ValueError(f"{name}: ожидается true или false")
    return value == "true"

@dataclass(frozen=True)
class Settings:
    mode: str
    service_key: str
    go_url: str
    mcp_url: str
    whoami_tool: str
    ollama_url: str
    model: str
    context_tokens: int
    output_tokens: int
    safety_tokens: int
    summary_chars: int
    max_steps: int
    timeout: float
    history_search_enabled: bool
    summary_enabled: bool

    @property
    def input_budget(self):
        return self.context_tokens - self.output_tokens - self.safety_tokens

    @classmethod
    def from_env(cls):
        mode = required("AGENT_MODE")
        if mode not in ("go", "demo"):
            raise ValueError("AGENT_MODE должен быть go или demo")
        s = cls(
            mode=mode,
            history_search_enabled=flag("HISTORY_SEARCH_ENABLED"),
            summary_enabled=flag("SUMMARY_ENABLED"),
            service_key=required("AGENT_SERVICE_TOKEN") if mode == "go" else "",
            go_url=required("GO_API_URL") if mode == "go" else "",
            mcp_url=required("MCP_URL") if mode == "go" else "",
            whoami_tool=required("WHOAMI_TOOL") if mode == "go" else "",
            ollama_url=required("OLLAMA_URL"),
            model=required("OLLAMA_MODEL"),
            context_tokens=int(required("CONTEXT_TOKENS")),
            output_tokens=int(required("OUTPUT_TOKENS")),
            safety_tokens=int(required("SAFETY_TOKENS")),
            summary_chars=int(required("SUMMARY_CHARS")),
            max_steps=int(required("MAX_STEPS")),
            timeout=float(required("REQUEST_TIMEOUT")),
        )
        if (s.input_budget < 4000 or s.max_steps < 1 or s.output_tokens < 1
            or s.safety_tokens < 0 or s.summary_chars < 1 or s.timeout <= 0):
            raise ValueError("Некорректные бюджеты или таймауты")
        return s
