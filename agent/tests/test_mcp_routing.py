import unittest
from types import SimpleNamespace
from unittest.mock import Mock, AsyncMock, patch
from clients.mcp import MCPGateway
from clients.tools import discover_tools

class RoutingTests(unittest.TestCase):
    def test_new_tool_uses_discovered_schema_and_server(self):
        gateway=Mock()
        gateway.call.return_value={"ok":True}
        gateway.discover.return_value=[{"name":"brand_new_tool","description":"From MCP",
            "url":"http://corp/mcp","schema":{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}}]
        gateway.call.return_value={"ok":True}
        tool=discover_tools(gateway,"jwt",SimpleNamespace(run_id="run", user_message=SimpleNamespace(content="семейные дела")),Mock())[0]
        self.assertEqual(tool.description,"From MCP")
        tool.invoke({"value":42})
        gateway.call.assert_called_once_with("brand_new_tool",{"value":42},"jwt",url="http://corp/mcp")

    def test_missing_reason_never_calls_server_and_key_is_hidden(self):
        gateway=Mock()
        gateway.call.return_value={"ok":True}
        gateway.discover.return_value=[{"name":"create_time_off_request","description":"Create",
            "url":"http://corp/mcp","schema":{"type":"object","properties":{
                "reason":{"type":"string"},"idempotency_key":{"type":"string"}},
                "required":["reason","idempotency_key"]}}]
        tool=discover_tools(gateway,"jwt",SimpleNamespace(run_id="run", user_message=SimpleNamespace(content="семейные дела")),Mock())[0]
        self.assertNotIn("idempotency_key",tool.args_schema["properties"])
        self.assertIn("error",tool.invoke({}))
        self.assertIn("error",tool.invoke({"reason":"  "}))
        self.assertIn("error",tool.invoke({"reason":"Не указана причина. Пожалуйста, уточните."}))
        gateway.call.assert_not_called()
        tool.invoke({"reason":"семейные дела"})
        self.assertEqual(gateway.call.call_args.args[1]["idempotency_key"],"time-off:run")

    def test_direct_call_discovers_route(self):
        gw=MCPGateway(SimpleNamespace(mcp_url="http://buddy/mcp",timeout=5))
        gw.discover=Mock(return_value=[{"name":"new_tool","url":"http://new/mcp"}])
        gw._call=AsyncMock(return_value={"ok":True})
        self.assertEqual(gw.call("new_tool",{},"jwt"),{"ok":True})
        gw._call.assert_awaited_once_with("new_tool",{},"jwt","http://new/mcp")
