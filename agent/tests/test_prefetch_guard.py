"""Автопоиск по базе знаний до модели и защита от отправки задачи по вопросу «как выполнить»."""
import json
import unittest
from unittest.mock import Mock
from langchain_core.messages import AIMessage

from agent.graph import reports_progress
import test_task_review
from test_chat import events
from test_task_review import update_call

DOCS_MARK = "Найденные документы базы знаний"


class ReportDetectionTests(unittest.TestCase):
    def test_reports(self):
        for text in ("Я получил доступ к GitLab", "Сделал GitLab", "настроила VPN", "Прошёл встречу с наставником",
                     "отметь GitLab", "Отправь задачу про Jira на проверку", "GitLab готово",
                     "Начал задачу про Jira", "я познакомилась с командой", "Получила доступ, что дальше?"):
            self.assertTrue(reports_progress(text), text)

    def test_not_reports(self):
        for text in ("Как выполнить задачу «Доступ к GitLab»?", "Как получить доступ к GitLab?",
                     "Я ещё не получил доступ к GitLab", "Не настроил VPN, что делать?", "Привет",
                     "Что делать дальше?", "Подскажи, как настроить VPN", "Где взять доступ к Jira?",
                     "А как отметить задачу?", "Когда и кто основал Авито?"):
            self.assertFalse(reports_progress(text), text)


class PrefetchAndGuardTests(test_task_review.TaskReviewTests):
    """setUp/хелперы — из TaskReviewTests (режим go, фейковый чек-лист, перехват промптов)."""

    def setUp(self):
        super().setUp()
        self.gateway.search_docs = Mock(return_value={"found": True, "fragments": [
            {"source": "common/playbook/avito-history.md", "content": "Авито появился в октябре 2007 года."}]})

    def test_docs_are_searched_by_code_before_model(self):
        self.model(AIMessage(content="Авито основан в 2007 году."))
        ev = events(self.send("Когда и кто основал Авито?"))
        self.assertEqual(ev[-1][0], "done")
        self.gateway.search_docs.assert_called_once()
        self.assertEqual(self.gateway.search_docs.call_args.args[0], "Когда и кто основал Авито?")
        self.assertIn(DOCS_MARK, self.prompts[0])
        self.assertIn("avito-history.md", self.prompts[0])
        self.assertIn(("status", {"text": "Ищу в базе знаний"}), ev)
        # автопоиск не пишется в историю чата отдельными сообщениями
        self.assertEqual([m.role for m in self.rows()], ["user", "assistant"])

    def test_docs_stay_for_the_answer_after_tools(self):
        self.model(AIMessage(content="", tool_calls=[{"id": "c1", "name": "get_checklist", "args": {}}]),
                   AIMessage(content="Вот твой прогресс."))
        events(self.send("Что мне делать дальше?"))
        self.assertEqual(len(self.prompts), 2)
        self.assertIn(DOCS_MARK, self.prompts[1])
        self.gateway.search_docs.assert_called_once()  # ищем один раз за ход, не на каждом шаге

    def test_search_failure_does_not_break_turn(self):
        self.gateway.search_docs.side_effect = RuntimeError("mcp down")
        self.model(AIMessage(content="Не удалось найти подтверждение."))
        ev = events(self.send("Когда основали Авито?"))
        self.assertEqual(ev[-1][0], "done")
        self.assertNotIn(DOCS_MARK, self.prompts[0])

    def test_nothing_found_adds_no_block(self):
        self.gateway.search_docs.return_value = {"found": False, "fragments": []}
        self.model(AIMessage(content="В базе этого нет."))
        events(self.send("Какая зарплата у стажёра?"))
        self.assertNotIn(DOCS_MARK, self.prompts[0])

    def test_how_to_question_does_not_submit_task(self):
        # заготовка с главной: модель по ошибке вызывает update_task_status — код не даёт сменить статус
        self.model(update_call(), AIMessage(content="Чтобы получить доступ, оформи заявку в GitLab."))
        ev = events(self.send("Как выполнить задачу «Доступ к GitLab»?"))
        self.assertEqual(ev[-1][0], "done")
        self.assertEqual(self.updates(), [])
        tool = [m for m in self.rows() if m.role == "tool"][-1]
        self.assertIn("Статус не изменён", json.loads(tool.content)["error"])

    def test_quoted_task_description_is_added_by_code(self):
        base = self.gateway.call.side_effect

        def call(name, args, token, **kwargs):
            if name == "get_checklist":
                return {"stages": [{"title": "Доступы и инструменты", "tasks": [
                    {"id": 8, "title": "Настроить VPN", "status": "todo",
                     "description": "Установи и проверь корпоративный VPN."}]}]}
            return base(name, args, token)
        self.gateway.call.side_effect = call
        self.gateway.search_docs.return_value = {"found": False, "fragments": []}
        self.model(AIMessage(content="Установи корпоративный VPN и проверь подключение."))
        events(self.send("Как выполнить задачу «Настроить VPN»?"))
        self.assertIn("Задача из чек-листа", self.prompts[0])
        self.assertIn("Установи и проверь корпоративный VPN.", self.prompts[0])
        self.assertIn("Доступы и инструменты", self.prompts[0])
        self.assertEqual(self.updates(), [])

    def test_unknown_quoted_title_adds_nothing(self):
        self.gateway.search_docs.return_value = {"found": False, "fragments": []}
        self.model(AIMessage(content="Такой задачи нет."))
        events(self.send("Как выполнить задачу «Полететь на Марс»?"))
        self.assertNotIn(DOCS_MARK, self.prompts[0])

    def test_checklist_failure_keeps_docs(self):
        self.gateway.call.side_effect = RuntimeError("go down")
        self.model(AIMessage(content="ok"))
        ev = events(self.send("Как выполнить задачу «Настроить VPN»?"))
        self.assertEqual(ev[-1][0], "done")
        self.assertIn("avito-history.md", self.prompts[0])
        self.assertNotIn("Задача из чек-листа", self.prompts[0])

    def test_report_still_submits_task(self):
        self.model(update_call(), AIMessage(content="Отправил задачу наставнику на проверку."))
        events(self.send("Я получил доступ к GitLab"))
        self.assertEqual(self.updates(), [{"task_id": 7, "status": "done"}])

    def test_prompt_has_no_copyable_gitlab_example(self):
        self.model(AIMessage(content="ok"))
        self.send("Привет")
        self.assertNotIn('update_task_status(task="GitLab"', self.prompts[0])
        self.assertIn("КАК выполнить задачу, только объясняй", self.prompts[0])


# Не запускать унаследованные тесты второй раз.
for _name in [n for n in dir(test_task_review.TaskReviewTests) if n.startswith("test_")]:
    if _name not in PrefetchAndGuardTests.__dict__:
        setattr(PrefetchAndGuardTests, _name, None)

if __name__ == "__main__":
    unittest.main()
