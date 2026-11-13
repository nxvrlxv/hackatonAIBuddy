import unittest
from unittest.mock import Mock
from langchain_core.messages import AIMessage
import test_chat
from test_chat import events, make_settings
from clients.mcp import MCPGateway
from chat.models import Message
from memory.context import complete_history, build_input

class MVPTests(unittest.TestCase):
    setUp = test_chat.AgentTests.setUp
    send = test_chat.AgentTests.send
    def test_disabled_memory_handles_long_chat_without_summary(self):
        service = self.app.extensions["chat_service"]
        service.settings = make_settings(context_tokens=9500,
            history_search_enabled=False, summary_enabled=False)
        service.summary_model = Mock()
        factory = service.store_factory
        stores = []
        def bind(user, token):
            store = factory(user, token)
            store.search = Mock(side_effect=AssertionError("search disabled"))
            store.index_run = Mock(side_effect=AssertionError("index disabled"))
            store.save_summary = Mock(side_effect=AssertionError("summary disabled"))
            stores.append(store)
            return store
        service.store_factory = bind
        self.assertEqual(events(self.send("a"*2000))[-1][0], "done")
        self.assertEqual(events(self.send("b"*2200))[-1][0], "done")
        for store in stores:
            store.search.assert_not_called()
            store.index_run.assert_not_called()
            store.save_summary.assert_not_called()
        service.summary_model.invoke.assert_not_called()
        chat = self.app.extensions["demo_backend"].chats[self.cid]
        self.assertEqual(chat["summary"].version, 0)
        self.assertEqual(len(chat["messages"]), 4)

    def test_finish_conflict_never_reports_success(self):
        service = self.app.extensions["chat_service"]
        factory = service.store_factory
        def bind(user, token):
            store = factory(user, token)
            store.finish = Mock(return_value={"status": "interrupted"})
            return store
        service.store_factory = bind
        ev = events(self.send())
        self.assertEqual(ev[-1], ("error", {"code": "run_not_active"}))
        self.assertNotIn("done", [name for name, _ in ev])

class ContractTests(unittest.TestCase):
    def test_live_demo_uses_public_go_url_without_service_key(self):
        import test_chat
        from main import create_app
        app = create_app(make_settings(mode="go", service_key="private-service-key"),
                         model=test_chat.TextModel(responses=["OK"]))
        page = app.test_client().get("/demo")
        self.assertEqual(page.status_code, 200)
        text = page.get_data(as_text=True)
        self.assertIn('"public_go_url": "http://localhost:8080"', text)
        self.assertIn('/api/login', text)
        self.assertNotIn('private-service-key', text)
        self.assertNotIn('X-Agent-Key', text)

    def test_whoami_has_empty_arguments(self):
        gateway = MCPGateway(make_settings())
        gateway.call = Mock(return_value={"id":1,"full_name":"Аня","role":"intern"})
        gateway.whoami("private")
        gateway.call.assert_called_once_with("whoami", {}, "private")

    def test_history_drops_cut_or_unfinished_tool_turns(self):
        def msg(i, run, role, **payload):
            return Message(id=i, sequence_number=i, run_id=run, role=role, payload=payload)
        history = [
            msg(1, "cut", "tool", content="old", tool_call_id="c"),
            msg(2, "cut", "assistant", content="old"),
            msg(3, "good", "user", content="question"),
            msg(4, "good", "assistant", content="", tool_calls=[
                {"id":"c","name":"search_docs","args":{}}]),
            msg(5, "good", "tool", content="result", tool_call_id="c"),
            msg(6, "good", "assistant", content="answer"),
            msg(7, "broken", "user", content="question"),
            msg(8, "broken", "assistant", content="", tool_calls=[
                {"id":"d","name":"search_docs","args":{}}]),
        ]
        self.assertEqual([m.id for m in complete_history(history)], [3,4,5,6])
