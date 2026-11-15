"""Апрув задач наставником: review вместо done и защита от ложного «отметил задачу»."""
import json
import unittest
from dataclasses import replace
from unittest.mock import Mock
from langchain_core.callbacks import BaseCallbackHandler
from langchain_core.messages import AIMessage

from agent.graph import HONEST, claims_task_mark
import test_chat
from test_chat import ToolModel, events


def checklist(status="in_progress"):
    return {"stages": [{"title": "Доступы", "tasks": [
        {"id": 7, "title": "Доступ к GitLab", "status": status},
        {"id": 8, "title": "Настроить VPN", "status": "todo"},
    ]}], "progress": {"done": 0, "total": 2, "percent": 0}}


def update_call(call_id="u1", task="GitLab"):
    return AIMessage(content="", tool_calls=[{"id": call_id, "name": "update_task_status",
                                              "args": {"task": task}}])


class ClaimDetectionTests(unittest.TestCase):
    def test_claims(self):
        for text in ("Отмечаю задачу «GitLab» как выполненную.", "Отметил задачу, молодец!",
                     "Отправил задачу наставнику на проверку.", "Задача «Доступ к GitLab» выполнена!",
                     "Сейчас помечу её в чек-листе.", "Обновил статус задачи."):
            self.assertTrue(claims_task_mark(text), text)

    def test_not_claims(self):
        for text in ("Привет! Чем помочь?", "Я не отметил задачу.", "Ты уже отметил задачу на главной?",
                     "Задача ещё не выполнена.", "Нажми «Отправить на проверку» на главной.",
                     "Можешь отметить задачу сам.", "Отправил запрос в поиск, вот что нашёл."):
            self.assertFalse(claims_task_mark(text), text)

    def test_status_report_after_checklist(self):
        self.assertFalse(claims_task_mark("Задача «VPN» выполнена, GitLab на проверке.", reported=True))
        self.assertTrue(claims_task_mark("Отмечаю задачу VPN.", reported=True))


class TaskReviewTests(test_chat.AgentTests):
    """Наследуем setUp/send у AgentTests; его собственные тесты здесь не повторяем."""

    def setUp(self):
        super().setUp()
        self.service = self.app.extensions["chat_service"]
        self.service.settings = replace(self.service.settings, mode="go")
        self.status = "in_progress"

        def call(name, args, token):
            if name == "get_checklist":
                return checklist(self.status)
            if name == "update_task_status":
                return {"task": {"id": args["task_id"], "title": "Доступ к GitLab", "status": "review",
                                 "submitted_at": "2026-11-15T10:12:00Z"},
                        "progress": {"done": 0, "total": 2, "percent": 0}}
            raise AssertionError(name)
        self.gateway.call = Mock(side_effect=call)
        self.prompts = []
        prompts = self.prompts

        class Capture(BaseCallbackHandler):
            def on_chat_model_start(self, serialized, messages, **kwargs):
                prompts.append("\n".join(str(m.content) for m in messages[0]))
        self.capture = Capture()

    def model(self, *responses):
        self.service.model = ToolModel(responses=list(responses), callbacks=[self.capture])

    def updates(self):
        return [c.args[1] for c in self.gateway.call.call_args_list if c.args[0] == "update_task_status"]

    def text(self, ev):
        return "".join(d["text"] for name, d in ev if name == "text_delta")

    def rows(self):
        return self.app.extensions["demo_backend"].chats[self.cid]["messages"]

    def test_false_claim_gets_one_retry_and_tool_is_called(self):
        self.model(AIMessage(content="Отмечаю задачу «GitLab» как выполненную."), update_call(),
                   AIMessage(content="Отправил задачу «Доступ к GitLab» наставнику на проверку."))
        ev = events(self.send("Я получил доступ к GitLab"))
        self.assertEqual(ev[-1][0], "done")
        self.assertEqual(self.updates(), [{"task_id": 7, "status": "done"}])
        self.assertIn("не вызвал update_task_status", self.prompts[1])
        self.assertNotIn("не вызвал update_task_status", self.prompts[2])  # подсказка не копится
        self.assertNotIn(HONEST, self.text(ev))
        self.assertTrue(self.text(ev).endswith("наставнику на проверку."))
        rows = self.rows()
        self.assertEqual([m.role for m in rows], ["user", "assistant", "assistant", "tool", "assistant"])
        self.assertFalse(any("не вызвал" in str(m.content) for m in rows))
        self.assertIn("отправлена наставнику на проверку", rows[3].content)

    def test_false_claim_twice_becomes_honest_text(self):
        self.model(AIMessage(content="Отмечаю задачу GitLab."), AIMessage(content="Готово, отметил!"))
        ev = events(self.send("Я получил доступ к GitLab"))
        self.assertEqual(ev[-1][0], "done")
        self.assertEqual(self.updates(), [])
        text = self.text(ev)
        self.assertTrue(text.endswith(HONEST))
        self.assertNotIn("Готово, отметил!", text)  # повтор не стримится
        rows = self.rows()
        self.assertEqual([m.role for m in rows], ["user", "assistant", "assistant"])
        self.assertEqual(rows[-1].content, HONEST)

    def test_retry_with_honest_answer_is_shown(self):
        self.model(AIMessage(content="Отмечаю задачу."), AIMessage(content="Я ничего не менял в чек-листе."))
        ev = events(self.send("Что делать дальше?"))
        self.assertEqual(self.updates(), [])
        self.assertTrue(self.text(ev).endswith("Я ничего не менял в чек-листе."))
        self.assertNotIn(HONEST, self.text(ev))

    def test_failed_update_then_claim_is_corrected(self):
        self.gateway.call.side_effect = lambda name, args, token: (
            checklist() if name == "get_checklist" else {"error": "task already approved"})
        self.model(update_call(), AIMessage(content="Отметил задачу!"))
        ev = events(self.send("Я получил доступ к GitLab"))
        self.assertTrue(self.text(ev).endswith(HONEST))

    def test_plain_answer_untouched(self):
        self.model(AIMessage(content="Привет! Чем помочь?"))
        ev = events(self.send("Привет"))
        self.assertEqual(self.text(ev), "Привет! Чем помочь?")
        self.assertEqual(len(self.prompts), 1)
        self.assertEqual([m.role for m in self.rows()], ["user", "assistant"])

    def test_successful_update_claim_is_not_questioned(self):
        self.model(update_call(), AIMessage(content="Отправил задачу наставнику на проверку."))
        ev = events(self.send("Я получил доступ к GitLab"))
        self.assertEqual(len(self.prompts), 2)
        self.assertEqual(self.text(ev), "Отправил задачу наставнику на проверку.")

    def test_task_in_review_or_done_is_not_resubmitted(self):
        for status, phrase in (("review", "уже на проверке"), ("done", "уже принята")):
            self.status = status
            self.model(update_call(call_id="u-" + status),
                       AIMessage(content="Задача уже отправлена на проверку."))
            ev = events(self.send("Я получил доступ к GitLab"))
            self.assertEqual(ev[-1][0], "done")
            self.assertEqual(self.updates(), [])
            tool = [m for m in self.rows() if m.role == "tool"][-1]
            self.assertIn(phrase, json.loads(tool.content)["summary"])
            self.assertNotIn(HONEST, self.text(ev))

    def test_resolver_prefers_the_only_open_match(self):
        self.gateway.call.side_effect = lambda name, args, token: (
            {"stages": [{"tasks": [{"id": 1, "title": "GitLab: аккаунт", "status": "done"},
                                   {"id": 2, "title": "GitLab: репозиторий", "status": "todo"}]}]}
            if name == "get_checklist" else {"task": {"id": args["task_id"], "status": "review"}})
        self.model(update_call(), AIMessage(content="Отправил наставнику на проверку."))
        events(self.send("Сделал GitLab"))
        self.assertEqual(self.updates(), [{"task_id": 2, "status": "done"}])

    def test_prompt_explains_review(self):
        self.model(AIMessage(content="ok"))
        self.send("Привет")
        self.assertIn("review = на проверке у наставника, НЕ выполнена", self.prompts[0])


# Не запускать унаследованные тесты AgentTests второй раз.
for _name in [n for n in dir(test_chat.AgentTests) if n.startswith("test_")]:
    setattr(TaskReviewTests, _name, None)

if __name__ == "__main__":
    unittest.main()
