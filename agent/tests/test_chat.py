import json
import unittest
from uuid import uuid4
from dataclasses import replace
from unittest.mock import Mock
import httpx
from langchain_core.language_models.fake_chat_models import FakeListChatModel, FakeMessagesListChatModel
from langchain_core.messages import AIMessage
from langchain_core.callbacks import BaseCallbackHandler
from config import Settings
from main import create_app
from chat.models import User, AgentError, Turn, Summary, Message
from clients.go import GoStore
from demo.backend import MemoryBackend, DemoGateway
from memory.context import ContextManager, build_input, input_size

def make_settings(**overrides):
    values = dict(mode="demo", service_key="test-service", go_url="http://go.test", public_go_url="http://localhost:8080",
        mcp_url="http://mcp.test/mcp", whoami_tool="whoami", ollama_url="http://ollama.test",
        model="test-model", context_tokens=16384, output_tokens=1024, safety_tokens=1024,
        summary_chars=1600, max_steps=6, timeout=5,
        history_search_enabled=True, summary_enabled=True)
    values.update(overrides)
    return Settings(**values)

class TextModel(FakeListChatModel):
    def bind_tools(self, tools, **kwargs):
        return self

class ToolModel(FakeMessagesListChatModel):
    def bind_tools(self, tools, **kwargs):
        return self

def events(response):
    body = response.get_data(as_text=True)
    result = []
    for frame in body.strip().split("\n\n"):
        lines = frame.splitlines()
        if lines:
            result.append((lines[0].split(": ", 1)[1], json.loads(lines[1][6:])))
    return result

class AgentTests(unittest.TestCase):
    def setUp(self):
        self.gateway = Mock(wraps=DemoGateway())
        self.app = create_app(make_settings(mode="demo"), model=TextModel(responses=["Привет!"]),
                              gateway=self.gateway, summary_model=TextModel(responses=["Краткое резюме."]))
        self.app.testing = True
        self.client = self.app.test_client()
        self.headers = {"Authorization": "Bearer demo-alice"}
        self.cid = self.client.post("/internal/conversations", json={"title": "Git"}, headers=self.headers).json["id"]

    def send(self, content="Привет", client_id=None, cid=None, headers=None, buffered=True):
        return self.client.post("/internal/chat", headers=headers or self.headers, buffered=buffered, json={
            "conversation_id": cid or self.cid, "content": content,
            "client_message_id": client_id or str(uuid4()),
        })

    def test_stream_history_and_identity_each_request(self):
        before = self.gateway.whoami.call_count
        response = self.send()
        ev = events(response)
        self.assertEqual(ev[-1][0], "done")
        deltas = [data["text"] for name, data in ev if name == "text_delta"]
        self.assertEqual("".join(deltas), "Привет!")
        self.assertGreater(len(deltas), 1)
        rows = self.client.get(f"/internal/conversations/{self.cid}/messages", headers=self.headers).json["messages"]
        self.assertEqual([m["role"] for m in rows], ["user", "assistant"])
        self.assertEqual(self.gateway.whoami.call_count, before + 2)

    def test_invalid_identity_never_calls_model_or_store(self):
        before = len(self.app.extensions["demo_backend"].chats[self.cid]["messages"])
        response = self.send(headers={"Authorization": "Bearer BAD-SECRET"})
        self.assertEqual(response.status_code, 401)
        self.assertNotIn("BAD-SECRET", response.get_data(as_text=True))
        self.assertEqual(len(self.app.extensions["demo_backend"].chats[self.cid]["messages"]), before)

    def test_cross_user_access(self):
        bob = {"Authorization": "Bearer demo-bob"}
        self.assertEqual(self.send(headers=bob).status_code, 404)
        self.assertEqual(self.client.get(f"/internal/conversations/{self.cid}/messages", headers=bob).status_code, 404)

    def test_duplicate_does_not_generate_again(self):
        key = str(uuid4())
        self.assertEqual(events(self.send(client_id=key))[-1][0], "done")
        self.assertEqual(self.send(client_id=key).status_code, 409)
        self.assertEqual(len(self.app.extensions["demo_backend"].chats[self.cid]["messages"]), 2)

    def test_busy_and_close_release_run(self):
        response = self.send(buffered=False)
        self.assertEqual(self.send().status_code, 409)
        response.close()
        self.assertEqual(events(self.send())[-1][0], "done")

    def test_failure_persists_partial_and_no_done(self):
        self.app.extensions["chat_service"].model = TextModel(responses=["Привет"], error_on_chunk_number=2)
        ev = events(self.send())
        self.assertEqual(ev[-1][0], "error")
        self.assertFalse(any(name == "done" for name, _ in ev))
        rows = self.app.extensions["demo_backend"].chats[self.cid]["messages"]
        self.assertTrue(any(m.status == "failed" for m in rows))

    def test_profile_survives_summary_and_token_absent(self):
        seen = []
        class Capture(BaseCallbackHandler):
            def on_chat_model_start(self, serialized, messages, **kwargs):
                seen.append(messages[0])
        service = self.app.extensions["chat_service"]
        service.settings = replace(service.settings, context_tokens=11000)
        service.model = TextModel(responses=["Ответ"], callbacks=[Capture()])
        # Первый запрос помещается, второй требует сжатия предыдущего хода.
        self.assertEqual(events(self.send("Git " + "a" * 2000))[-1][0], "done")
        self.assertEqual(events(self.send("Продолжим " + "b" * 2200))[-1][0], "done")
        chat = self.app.extensions["demo_backend"].chats[self.cid]
        self.assertGreater(chat["summary"].through_sequence, 0)
        self.assertEqual(len(chat["messages"]), 4)
        for prompt in seen:
            text = "\n".join(str(m.content) for m in prompt)
            self.assertIn("Аня", text)
            self.assertNotIn("demo-alice", text)

    def test_memory_across_chats_is_user_scoped(self):
        self.send("Запомни обсуждение GitLab и репозитория payments")
        gateway = DemoGateway()
        store = self.app.extensions["demo_backend"].bind(gateway.whoami("demo-alice"), "")
        self.assertTrue(store.search("GitLab"))
        bob = self.app.extensions["demo_backend"].bind(gateway.whoami("demo-bob"), "")
        self.assertEqual(bob.search("GitLab"), [])

    def test_time_off_identity_and_key_are_supplied_by_code(self):
        service = self.app.extensions["chat_service"]
        service.settings = replace(service.settings, mode="go")
        self.gateway.call = Mock(return_value={"id": 42, "status": "submitted"})
        service.model = ToolModel(responses=[
            AIMessage(content="", tool_calls=[{"id": "leave1", "name": "create_time_off_request",
                "args": {"start_date": "2099-01-20", "end_date": "2099-01-20"}}]),
            AIMessage(content="Заявка №42 подана, ещё не согласована."),
        ])
        self.assertEqual(events(self.send("Оформи отгул на 20 января 2099"))[-1][0], "done")
        name, args, token = self.gateway.call.call_args.args
        self.assertEqual(name, "create_time_off_request")
        self.assertEqual(token, "demo-alice")
        self.assertNotIn("user_id", args)
        self.assertTrue(args["idempotency_key"].startswith("time-off:"))
        self.assertEqual(args["reason"], "")

    def test_tool_cycle_saved_with_call_id(self):
        self.app.extensions["chat_service"].model = ToolModel(responses=[
            AIMessage(content="", tool_calls=[{"id":"call1","name":"search_docs",
                                              "args":{"query":"GitLab"}}]),
            AIMessage(content="Обратись к наставнику."),
        ])
        ev = events(self.send("Как получить доступ к GitLab?"))
        self.assertEqual(ev[-1][0], "done")
        rows = self.app.extensions["demo_backend"].chats[self.cid]["messages"]
        self.assertEqual([m.role for m in rows], ["user","assistant","tool","assistant"])
        self.assertEqual(rows[2].tool_call_id, "call1")
        raw = rows[2].model_dump()
        self.assertEqual(raw["payload"]["tool_call_id"], "call1")
        self.assertNotIn("tool_call_id", raw)
        self.assertNotIn("content", raw)

    def test_go_requires_service_credential_before_whoami(self):
        app = create_app(make_settings(mode="go", service_key="service-secret"),
            model=TextModel(responses=["OK"]), gateway=self.gateway,
            store_factory=self.app.extensions["demo_backend"].bind)
        before = self.gateway.whoami.call_count
        response = app.test_client().post("/internal/chat", headers=self.headers, json={})
        self.assertEqual(response.status_code, 403)
        self.assertEqual(before, self.gateway.whoami.call_count)

    def test_invalid_payload(self):
        for content in ("", "  ", "a"*12001):
            self.assertEqual(self.send(content).status_code, 422)

    def test_no_token_in_go_payload(self):
        calls = []
        def handler(req):
            calls.append(req)
            return httpx.Response(200, json={"conversations":[]})
        store = GoStore(make_settings(service_key="key"), User(id=1,full_name="Аня",role="intern"),
                        "private", transport=httpx.MockTransport(handler))
        store.conversations()
        self.assertEqual(calls[0].headers["Authorization"], "Bearer private")
        self.assertNotIn(b"private", calls[0].content)

    def test_mismatched_go_owner_rejected(self):
        turn = Turn(run_id="r",conversation_id=1,user_id=2,history=[],
                    user_message=Message(id=1,sequence_number=1,run_id="r",role="user",payload={}))
        store = GoStore(make_settings(service_key="key"), User(id=1,full_name="Аня",role="intern"),"private",
            transport=httpx.MockTransport(lambda req: httpx.Response(200,json=turn.model_dump())))
        with self.assertRaises(AgentError):
            store.begin(1,"Hi","x")


    def test_index_failure_can_be_retried_without_new_answer(self):
        service = self.app.extensions["chat_service"]
        original = service.store_factory
        def unavailable_index(user, token):
            store = original(user, token)
            store.index_run = Mock(side_effect=AgentError("index_unavailable"))
            return store
        service.store_factory = unavailable_index
        ev = events(self.send("GitLab"))
        self.assertIn(("warning", {"code":"index_pending"}), ev)
        self.assertEqual(ev[-1][0], "done")
        service.store_factory = original
        response = self.client.post(f"/internal/conversations/{self.cid}/reindex", headers=self.headers)
        self.assertEqual(response.json, {"indexed":1, "failed":0})
        self.assertEqual(len(self.app.extensions["demo_backend"].chats[self.cid]["messages"]), 2)

    def test_tool_limit_ends_and_preserves_pairs(self):
        service = self.app.extensions["chat_service"]
        service.settings = replace(service.settings, max_steps=1)
        service.model = ToolModel(responses=[
            AIMessage(content="", tool_calls=[{"id":"c1","name":"search_docs","args":{"query":"x"}}])
        ])
        self.assertEqual(events(self.send())[-1][0], "done")
        rows = self.app.extensions["demo_backend"].chats[self.cid]["messages"]
        self.assertEqual([m.role for m in rows], ["user", "assistant", "tool", "assistant"])

    def test_summary_text_is_not_streamed(self):
        service = self.app.extensions["chat_service"]
        service.settings = replace(service.settings, context_tokens=11000)
        service.summary_model = TextModel(responses=["HIDDEN-SUMMARY"])
        self.send("a"*2000)
        response = self.send("b"*2200)
        self.assertEqual(events(response)[-1][0], "done")
        self.assertNotIn("HIDDEN-SUMMARY", response.get_data(as_text=True))

    def test_storage_failure_does_not_emit_done(self):
        service = self.app.extensions["chat_service"]
        original = service.store_factory
        def broken(user, token):
            store = original(user, token)
            store.finish = Mock(side_effect=AgentError("storage_unavailable"))
            return store
        service.store_factory = broken
        ev = events(self.send())
        self.assertEqual(ev[-1], ("error", {"code":"storage_unavailable"}))
        self.assertFalse(any(name == "done" for name, _ in ev))

    def test_go_adapter_contract_end_to_end(self):
        backend = MemoryBackend()
        gateway = DemoGateway()
        user = gateway.whoami("demo-alice")
        bound = backend.bind(user, "")
        cid = bound.create_conversation("HTTP contract")["id"]
        paths = []
        def handler(req):
            self.assertEqual(req.headers["X-Agent-Key"], "key")
            self.assertEqual(req.headers["Authorization"], "Bearer demo-alice")
            path = req.url.path.removeprefix("/internal/agent")
            paths.append(path)
            body = json.loads(req.content) if req.content else {}
            parts = path.strip("/").split("/")
            if path == "/memory/search":
                self.assertEqual(set(body), {"query", "limit"})
                data = {"hits":[m.model_dump() for m in bound.search(body["query"])]}
            elif parts[-1] == "turns":
                turn = bound.begin(cid, body["payload"]["content"], body["client_message_id"])
                handler.turn = turn
                data = turn.model_dump()
                data.pop("conversation_id")
                data.pop("user_id")
                data["user_message"].pop("status")
                data["user_message"].update(conversation_id=cid, event_key="user", created_at="2026-11-13T00:00:00Z")
            elif parts[-1] == "messages":
                key = body.pop("event_key")
                self.assertIn("payload", body)
                self.assertNotIn("tool_calls", body)
                self.assertNotIn("status", body)
                data = bound.append(handler.turn, key, **body).model_dump()
                data.pop("status")
                data.update(conversation_id=cid, event_key=key, created_at="2026-11-13T00:00:00Z")
            elif parts[-1] == "summary":
                data = bound.save_summary(handler.turn, Summary(text=body["text"],
                    through_sequence=body["through_sequence"], version=body["expected_version"])).model_dump()
            elif parts[-1] == "index":
                self.assertEqual(body, {})
                data = bound.index_run(handler.turn)
            elif parts[-1] == "finish":
                data = bound.finish(handler.turn, body["status"])
            else:
                raise AssertionError(path)
            return httpx.Response(200,json=data)
        settings = make_settings(mode="go",service_key="key",
                                 history_search_enabled=False, summary_enabled=False)
        app = create_app(settings,model=TextModel(responses=["OK"]), gateway=gateway,
            store_factory=lambda user, token: GoStore(settings,user,token,transport=httpx.MockTransport(handler)))
        response = app.test_client().post("/internal/chat",
            headers={"Authorization":"Bearer demo-alice","X-Agent-Key":"key"},
            json={"conversation_id":cid,"content":"Hi","client_message_id":"id"}, buffered=True)
        self.assertEqual(events(response)[-1][0],"done")
        self.assertTrue(any(p.endswith("/finish") for p in paths))
        self.assertEqual(len(backend.chats[cid]["messages"]),2)
        self.assertFalse(any(p.endswith("/index") or p.endswith("/summary") or p == "/memory/search" for p in paths))

if __name__ == "__main__":
    unittest.main()
