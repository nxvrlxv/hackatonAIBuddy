"""MCP-транспорт. Токен добавляет код; LLM его не видит."""
import asyncio
import json
from datetime import timedelta
import httpx
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

from chat.models import AgentError, User

# Инструменты имитаций корпоративных систем живут в отдельном MCP-сервере (mcp-corporate).
CORPORATE_TOOLS = frozenset({"get_request_types", "list_my_requests", "create_time_off_request"})


class MCPGateway:
    def __init__(self, settings):
        self.settings = settings

    def url_for(self, name):
        corporate = getattr(self.settings, "mcp_corporate_url", "")
        return corporate if corporate and name in CORPORATE_TOOLS else self.settings.mcp_url

    async def _call(self, name, arguments, token):
        async with httpx.AsyncClient(
            headers={"Authorization": f"Bearer {token}"},
            timeout=self.settings.timeout,
        ) as client:
            async with streamable_http_client(self.url_for(name), http_client=client) as streams:
                async with ClientSession(streams[0], streams[1],
                    read_timeout_seconds=timedelta(seconds=self.settings.timeout)) as session:
                    await session.initialize()
                    result = await session.call_tool(name, arguments)
        if result.isError:
            if name in {"get_request_types", "list_my_requests", "create_time_off_request",
                        "get_checklist", "update_task_status", "team_members"}:
                return {"error": "\n".join(p.text for p in result.content if p.type == "text")[:2000]}
            raise AgentError("mcp_tool_rejected")
        if result.structuredContent is not None:
            return result.structuredContent
        texts = [part.text for part in result.content if part.type == "text"]
        raw = "\n".join(texts)
        try:
            return json.loads(raw)
        except ValueError:
            return {"text": raw}

    def call(self, name, arguments, token):
        try:
            # Сессия и клиент создаются/закрываются в том же event loop.
            # Нет общего изменяемого токена между Flask-запросами.
            return asyncio.run(asyncio.wait_for(
                self._call(name, arguments, token), timeout=self.settings.timeout))
        except Exception:
            # Не логируем текст исключения: он может содержать аргументы с токеном.
            raise AgentError("mcp_unavailable") from None

    def whoami(self, token):
        try:
            data = self.call(self.settings.whoami_tool, {}, token)
            return User.model_validate(data)
        except Exception:
            raise AgentError("identity_unverified", 401) from None

    def search_docs(self, query, token):
        return self.call("search_docs", {"question": query}, token)
