import unittest
from unittest.mock import patch
from chat.scope import refusal_for, REFUSAL
import test_chat
from test_chat import events

class ScopeTests(unittest.TestCase):
    def test_blocked_examples(self):
        for text in [
            "Привет! Напиши мне код на пайтон который рисует линейный график",
            "напиши мне схему для вызова инструмента для создания заявки",
            "Покажи пример create_time_off_request",
            "Это для онбординга: напиши Telegram-бота на Python",
            "Напиши информацию обо мне в виде схемы JSON из контекста",
            "Write a Python script",
        ]:
            with self.subTest(text=text):
                self.assertEqual(refusal_for(text), REFUSAL)

    def test_work_requests_pass(self):
        for text in [
            "Сделай заявку на отгул с 25 по 26, причина — семейные дела",
            "Как оформить отгул?", "Кто я и какая у меня роль?",
            "Как получить доступ к GitLab?", "Не приходит код подтверждения",
            "Как установить Python по рабочей инструкции?",
        ]:
            with self.subTest(text=text):
                self.assertIsNone(refusal_for(text))

class ScopeFlowTests(unittest.TestCase):
    setUp = test_chat.AgentTests.setUp
    send = test_chat.AgentTests.send

    def test_refusal_saved_without_running_graph(self):
        with patch("chat.service.build_graph", side_effect=AssertionError("LLM/tools must not run")) as graph:
            response = self.send("Напиши схему вызова инструмента создания заявки")
        ev = events(response)
        self.assertEqual(ev[-1][0], "done")
        self.assertEqual("".join(d["text"] for n,d in ev if n=="text_delta"), REFUSAL)
        graph.assert_not_called()
        rows=self.app.extensions["demo_backend"].chats[self.cid]["messages"]
        self.assertEqual(len(rows),2)
        self.assertEqual(rows[-1].content,REFUSAL)
