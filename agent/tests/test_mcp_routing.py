"""Корпоративные инструменты уходят в mcp-corporate, остальные — в mcp-buddy."""
import unittest
from types import SimpleNamespace

from clients.mcp import CORPORATE_TOOLS, MCPGateway


class RoutingTests(unittest.TestCase):
    def test_corporate_tools_go_to_corporate_server(self):
        gw = MCPGateway(SimpleNamespace(mcp_url="http://buddy/mcp", mcp_corporate_url="http://corp/mcp"))
        for name in CORPORATE_TOOLS:
            self.assertEqual(gw.url_for(name), "http://corp/mcp")
        for name in ("whoami", "search_docs", "get_checklist", "update_task_status", "team_members"):
            self.assertEqual(gw.url_for(name), "http://buddy/mcp")

    def test_single_server_when_corporate_not_set(self):
        gw = MCPGateway(SimpleNamespace(mcp_url="http://buddy/mcp", mcp_corporate_url=""))
        self.assertEqual(gw.url_for("create_time_off_request"), "http://buddy/mcp")
        old = MCPGateway(SimpleNamespace(mcp_url="http://buddy/mcp"))  # настройки без поля
        self.assertEqual(old.url_for("list_my_requests"), "http://buddy/mcp")


if __name__ == "__main__":
    unittest.main()
