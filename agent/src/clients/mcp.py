"""MCP-транспорт. Токен добавляет код; LLM его не видит."""
import asyncio
import json
from datetime import timedelta
import httpx
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

from chat.models import AgentError, User

class MCPGateway:
    def __init__(self, settings):
        self.settings = settings

    async def _discover(self, token):
        found = []
        urls = list(dict.fromkeys(filter(None, [
            self.settings.mcp_url, getattr(self.settings, "mcp_corporate_url", "")])))
        for url in urls:
            async with httpx.AsyncClient(headers={"Authorization": f"Bearer {token}"},
                                         timeout=self.settings.timeout) as client:
                async with streamable_http_client(url, http_client=client) as streams:
                    async with ClientSession(streams[0], streams[1],
                            read_timeout_seconds=timedelta(seconds=self.settings.timeout)) as session:
                        await session.initialize()
                        cursor = None
                        seen = set()
                        while True:
                            page = await session.list_tools(cursor=cursor)
                            found.extend({"url": url, "name": t.name, "description": t.description or "",
                                          "schema": t.inputSchema} for t in page.tools)
                            cursor = page.nextCursor
                            if not cursor:
                                break
                            if cursor in seen:
                                raise AgentError("mcp_invalid_pagination")
                            seen.add(cursor)
        names = [t["name"] for t in found]
        if len(names) != len(set(names)):
            raise AgentError("mcp_duplicate_tool_name")
        return found

    def discover(self, token):
        try:
            return asyncio.run(asyncio.wait_for(self._discover(token), timeout=self.settings.timeout))
        except AgentError:
            raise
        except Exception:
            raise AgentError("mcp_discovery_unavailable") from None

    async def _call(self, name, arguments, token, url=None):
        async with httpx.AsyncClient(
            headers={"Authorization": f"Bearer {token}"},
            timeout=self.settings.timeout,
        ) as client:
            async with streamable_http_client(url or self.settings.mcp_url, http_client=client) as streams:
                async with ClientSession(streams[0], streams[1],
                    read_timeout_seconds=timedelta(seconds=self.settings.timeout)) as session:
                    await session.initialize()
                    result = await session.call_tool(name, arguments)
        if result.isError:
            return {"error": "\n".join(p.text for p in result.content if p.type == "text")[:2000]}
        if result.structuredContent is not None:
            return result.structuredContent
        texts = [part.text for part in result.content if part.type == "text"]
        raw = "\n".join(texts)
        try:
            return json.loads(raw)
        except ValueError:
            return {"text": raw}

    def call(self, name, arguments, token, *, url=None):
        try:
            if url is None and name != getattr(self.settings, "whoami_tool", "whoami"):
                tool = next((t for t in self.discover(token) if t["name"] == name), None)
                if tool is None:
                    raise AgentError("mcp_unknown_tool")
                url = tool["url"]
            # Сессия и клиент создаются/закрываются в том же event loop.
            # Нет общего изменяемого токена между Flask-запросами.
            return asyncio.run(asyncio.wait_for(
                self._call(name, arguments, token, url), timeout=self.settings.timeout))
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
