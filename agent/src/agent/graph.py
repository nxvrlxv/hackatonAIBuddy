"""Граф одного хода. Идентичность проверена ДО создания графа и не хранит токен."""
import json
import re
from typing import TypedDict
from langchain_core.messages import AIMessage, SystemMessage, ToolMessage
from langchain_core.tools import tool
from langchain_core.utils.function_calling import convert_to_openai_tool
from langgraph.config import get_stream_writer
from langgraph.graph import START, END, StateGraph

from chat.models import AgentError
from clients.tools import discover_tools
from memory.context import build_input, units, text_of, merge_system_messages

class State(TypedDict):
    '''Состояние графа'''
    messages: list
    history: list
    memories: list
    steps: int
    nudged: bool  # подсказка про ложную отметку уже давалась в этом ходе
    docs: str     # автопоиск по базе знаний до первого вызова модели ("" — не нашли или не влезло)

# Защита от «вранья про отметку»: модель пишет «отмечаю задачу», не вызвав update_task_status.
# Проверка детерминированная, по тексту финального ответа хода (без tool_calls).
_SEND_TARGET = re.compile(r"провер|наставник")
_STATUS = re.compile(r"статус")
_TASK = re.compile(r"задач")
# (шаблон, обязательное слово в том же предложении, пассивная форма)
_CLAIM_RULES = [
    (re.compile(r"\b(?:от|по)ме(?:чаю|чу|тил[аи]?)\b"), None, False),
    (re.compile(r"\bотправ(?:ляю|лю|ил[аи]?)\b"), _SEND_TARGET, False),
    (re.compile(r"\bобнов(?:ляю|лю|ил[аи]?)\b"), _STATUS, False),
    (re.compile(r"\bотправлен[аоы]?\b"), _SEND_TARGET, True),
    (re.compile(r"\b(?:отмечен|помечен|выполнен)[аоы]?\b"), _TASK, True),
    (re.compile(r"\bобновл[её]н\b"), _STATUS, True),
]
# «не отметил», «ты отметил», «задача ещё не выполнена» — не заявление агента.
_NOT_CLAIM = {"не", "ни", "ты", "вы", "нельзя"}

NUDGE = ("Ты заявил об отметке или отправке задачи, но не вызвал update_task_status. "
         "Если пользователь сообщил, что выполнил задачу, вызови update_task_status сейчас. "
         "Иначе честно скажи, что задачу не отмечал.")
HONEST = ("Поправка: не получилось отправить задачу на проверку. Напиши ещё раз, какую задачу "
          "ты выполнил, или нажми «Отправить на проверку» на главной.")


# Защита от случайной отправки: «Как выполнить задачу «GitLab»?» (заготовка с главной) — вопрос,
# а не сообщение о выполнении. Менять статус можно, только если пользователь сообщил, что сделал/начал
# задачу, или прямо попросил отметить. Решает код, а не модель.
_REPORT = re.compile(
    r"\b(?:сделал|выполнил|получил|настроил|прош[её]л|прошла|закончил|завершил|оформил|прочитал|прочёл|"
    r"познакомил(?:ся|ась)|установил|подключил|созда[лв]|сходил|встретил(?:ся|ась)|склонировал|собрал|"
    r"запустил|открыл|начал|приступил|взял(?:ся|ась)?)(?:а|и|ась|ись)?\b"
    r"|\bготов[оа]?\b|\b(?:отправ|отмет|помет)(?:ь|ьте|ляй|ить)\b")
_HOWTO = re.compile(r"^\W*(?:а\s+)?(?:как|каким образом|где|что нужно|что надо|зачем|почему|подскажи|расскажи|объясни)\b")
_NEGATION = {"не", "ещё", "еще", "пока"}


def reports_progress(text):
    """Сообщает ли пользователь, что сделал/начал задачу (или просит отметить), а не спрашивает «как»."""
    text = str(text).lower()
    if _HOWTO.search(text):
        return False
    for m in _REPORT.finditer(text):
        before = text[:m.start()].split()[-1:]
        if not _NEGATION.intersection(w.strip(",:;«»\"'()—-") for w in before):
            return True
    return False


def content_text(content):
    if isinstance(content, str):
        return content
    return "".join(b.get("text", "") for b in content if isinstance(b, dict) and b.get("type") == "text")


def claims_task_mark(text, reported=False):
    """Заявляет ли текст, что агент отметил/отправил задачу.
    reported: в ходе был get_checklist — пассивные формы («задача выполнена») считаем пересказом статуса."""
    for sentence in re.split(r"[.!?\n]+", text.lower()):
        for rx, need, passive in _CLAIM_RULES:
            if passive and reported:
                continue
            if need and not need.search(sentence):
                continue
            for m in rx.finditer(sentence):
                before = [w.strip(",:;«»\"'()—-") for w in sentence[:m.start()].split()[-3:]]
                if not _NOT_CLAIM.intersection(before):
                    return True
    return False


def _tool_calls_by_id(messages):
    return {c["id"]: c["name"] for m in messages if isinstance(m, AIMessage) for c in m.tool_calls}


def update_succeeded(messages):
    """Был ли в ходе update_task_status с результатом без ошибки."""
    names = _tool_calls_by_id(messages)
    for m in messages:
        if isinstance(m, ToolMessage) and names.get(m.tool_call_id) == "update_task_status":
            try:
                data = json.loads(m.content)
            except (TypeError, ValueError):
                continue
            if isinstance(data, dict) and "error" not in data:
                return True
    return False


def false_claim(messages):
    """Последний ответ без tool_calls заявляет отметку, а успешного update_task_status не было."""
    last = messages[-1]
    if last.tool_calls or update_succeeded(messages):
        return False
    reported = "get_checklist" in _tool_calls_by_id(messages).values()
    return claims_task_mark(content_text(last.content), reported)

def build_graph(model, context, settings, user, turn, store, gateway, token, memory, saved):
    '''Создает граф
    Args:
        {model}: ChatOllama
        {context}: memory.context.ContextManager
        {settings}: 
    
    '''
    # Токен только в замыкании транспорта: в state и схемах tools его нет.
    @tool
    def search_docs(query: str) -> str:
        """Поиск корпоративных фактов: миссия и ценности Авито, команды, продукты, проекты, правила и доступы. Обязательно используй перед ответом на вопросы о компании; не подменяй поиск сведениями из профиля пользователя."""
        result = gateway.search_docs(query, token)
        return json.dumps(result, ensure_ascii=False)[:6000]

    @tool
    def search_history(query: str) -> str:
        """Найти прошлые разговоры этого пользователя с датами/диапазонами источников."""
        return json.dumps([m.model_dump() for m in memory.search(query)], ensure_ascii=False)[:6000]

    def resolve_task(task):
        # Модель передаёт id или часть названия; задачу (и её статус) находит код, а не LLM.
        task = str(task).strip()
        checklist = gateway.call("get_checklist", {}, token)
        tasks = [t for st in checklist.get("stages", []) for t in st.get("tasks", [])]
        if task.isdigit():
            found = [t for t in tasks if str(t.get("id")) == task]
            # Чек-лист недоступен — отдаём id бэкенду как раньше, он сам проверит.
            return (found[0] if found else {"id": int(task)}), None
        found = [t for t in tasks if task.lower() in t.get("title", "").lower()]
        if len(found) > 1:
            # «GitLab» может встречаться в нескольких задачах; уже отправленные не мешают выбору.
            open_tasks = [t for t in found if t.get("status") not in ("review", "done")]
            if len(open_tasks) == 1:
                found = open_tasks
        if len(found) == 1:
            return found[0], None
        titles = [t["title"] for t in (found or tasks)]
        return None, {"error": "Уточни задачу: " + ("несколько совпадений" if found else "не найдена"),
                      "tasks": titles}

    @tool
    def update_task_status(task: str, status: str = "done") -> str:
        """Сообщить о задаче чек-листа. task — часть названия задачи (например «GitLab», «VPN») или её id.
        status: done (выполнил — задача уйдёт наставнику на проверку, по умолчанию), in_progress (начал) или todo.
        Вызывай, только когда пользователь пишет, что УЖЕ сделал, получил, настроил или прошёл что-то
        из чек-листа; get_checklist перед этим не нужен. На вопрос «как выполнить задачу» не вызывай.
        В ответе status review = «отправил наставнику на проверку», а НЕ «выполнено»."""
        if not reports_progress(turn.user_message.content):
            return json.dumps({"error": "Статус не изменён: пользователь не сообщал, что выполнил задачу. "
                                        "Объясни, как её выполнить, и не говори, что отправил её на проверку."},
                              ensure_ascii=False)
        found, problem = resolve_task(task)
        if problem:
            return json.dumps(problem, ensure_ascii=False)
        title, current = found.get("title", ""), found.get("status")
        if status in ("done", "review") and current in ("done", "review"):
            # Повторно не отправляем: бэкенд вернул бы 409 или сбросил submitted_at.
            message = ("Задача уже принята наставником, отправлять не нужно." if current == "done"
                       else "Задача уже на проверке у наставника, повторно отправлять не нужно.")
            return json.dumps({"task": title, "status": current, "summary": message}, ensure_ascii=False)
        result = gateway.call("update_task_status", {"task_id": found["id"], "status": status}, token)
        task_data = result.get("task") if isinstance(result, dict) else None
        if isinstance(task_data, dict) and task_data.get("status") == "review":
            result["summary"] = (f"Задача «{task_data.get('title', title)}» отправлена наставнику на проверку. "
                                 "Выполненной она станет, когда наставник её примет.")
        return json.dumps(result, ensure_ascii=False)

    tools = (discover_tools(gateway, token, turn, update_task_status, settings.whoami_tool)
             if settings.mode == "go" else [search_docs])
    if settings.history_search_enabled:
        tools.append(search_history)
    by_name = {t.name: t for t in tools}
    llm = model.bind_tools(tools)
    tool_cost = units([convert_to_openai_tool(t) for t in tools])

    def prefetch(state):
        # Поиск по базе знаний делает код, а не модель: qwen3:4b часто отвечает, не вызвав search_docs.
        # Ошибка поиска не роняет ход: модель ответит сама или вызовет search_docs.
        question = text_of(turn.user_message.content).strip()
        if not question:
            return {"docs": ""}
        get_stream_writer()({"type": "status", "text": "Ищу в базе знаний"})
        parts = []
        task = quoted_task(question)
        if task:
            parts.append("Задача из чек-листа пользователя (объясняй по её описанию):\n"
                         + json.dumps(task, ensure_ascii=False))
        try:
            found = gateway.search_docs(question, token)
        except Exception:
            found = None
        if found and not (isinstance(found, dict) and found.get("found") is False):
            parts.append(json.dumps(found, ensure_ascii=False))
        return {"docs": "\n".join(parts)[:6000]}

    def quoted_task(question):
        # «Как выполнить задачу «Настроить VPN»?» — заготовка с главной: описание задачи берёт код,
        # модель сама get_checklist на такой вопрос часто не вызывает. Только чтение, статус не меняется.
        if "get_checklist" not in by_name:
            return None
        m = re.search(r"«([^«»]{3,120})»", question)
        if not m:
            return None
        try:
            checklist = gateway.call("get_checklist", {}, token)
        except Exception:
            return None
        title = m.group(1).strip().lower()
        for stage in checklist.get("stages", []) if isinstance(checklist, dict) else []:
            for t in stage.get("tasks", []):
                if t.get("title", "").strip().lower() == title:
                    return {k: t[k] for k in ("title", "description", "status", "review_comment")
                            if t.get(k) is not None} | {"stage": stage.get("title", "")}
        return None

    def prepare(state):
        before = turn.summary.through_sequence
        summary, history, memories, docs = context.compact(
            user, turn, state["history"], state["messages"], state["memories"], store, tool_cost,
            state.get("docs", ""))
        if summary.through_sequence != before:
            get_stream_writer()({"type": "status", "text": "Сохранил резюме старой части разговора"})
        return {"history": history, "memories": memories, "docs": docs}

    def call_model(state):
        step = state["steps"] + 1
        key = f"assistant:{step}"
        get_stream_writer()({"type": "message_start", "message_id": key})
        answer = llm.invoke(merge_system_messages(build_input(
            user, turn.summary, state["history"], state["messages"], state["memories"],
            history_search_enabled=settings.history_search_enabled, docs=state.get("docs", ""))))
        # Сохранение до сообщения о завершении. При повторной записи key уникален в run.
        stored = store.append(turn, key, role="assistant", content=answer.content,
                              tool_calls=answer.tool_calls, status="completed")
        saved.append(stored)
        get_stream_writer()({"type": "message_done", "message_id": key,
                             "stored_message_id": stored.id, "content": answer.content})
        return {"messages": [*state["messages"], answer], "steps": step}

    guard = "update_task_status" in by_name

    def emit(key, text):
        # Ответ, который не стримился токенами: отдаём целиком через message_done.
        get_stream_writer()({"type": "message_start", "message_id": key})
        get_stream_writer()({"type": "message_done", "message_id": key, "content": text})

    def route(state):
        if state["messages"][-1].tool_calls:
            return "tools"
        if guard and false_claim(state["messages"]):
            return "honest" if state.get("nudged") else "retry"
        return END

    def retry(state):
        # Один повтор после ложного заявления. Узел не "call_llm", поэтому токены не уходят клиенту:
        # показываем ответ только если он честный или содержит вызов инструмента.
        step = state["steps"] + 1
        key = f"assistant:{step}"
        answer = llm.invoke(merge_system_messages(build_input(
            user, turn.summary, state["history"], state["messages"], state["memories"],
            history_search_enabled=settings.history_search_enabled, docs=state.get("docs", ""))
            + [SystemMessage(content=NUDGE)]))
        messages = [*state["messages"], answer]
        if false_claim(messages):
            return honest({**state, "steps": step})
        stored = store.append(turn, key, role="assistant", content=answer.content,
                              tool_calls=answer.tool_calls, status="completed")
        saved.append(stored)
        text = content_text(answer.content)
        if text:
            emit(key, text)
        return {"messages": messages, "steps": step, "nudged": True}

    def honest(state):
        answer = AIMessage(content=HONEST)
        saved.append(store.append(turn, "honest", role="assistant", content=HONEST, status="completed"))
        emit("honest", HONEST)
        return {"messages": [*state["messages"], answer], "steps": state["steps"], "nudged": True}

    def after_retry(state):
        return "tools" if state["messages"][-1].tool_calls else END

    def call_tools(state):
        results = []
        calls = state["messages"][-1].tool_calls
        if len(calls) > 8:
            raise AgentError("too_many_tool_calls")
        for call in calls:
            get_stream_writer()({"type": "status", "text": "Ищу информацию", "tool": call["name"]})
            try:
                if state["steps"] >= settings.max_steps:
                    content = "Достигнут лимит вызовов. Сформулируй ответ по уже найденным данным."
                elif call["name"] not in by_name:
                    content = "Такого инструмента нет."
                else:
                    content = by_name[call["name"]].invoke(call["args"])
            except Exception:
                content = "Источник сейчас недоступен. Не выдумывай результат."
            message = ToolMessage(content=content, tool_call_id=call["id"])
            stored = store.append(turn, "tool:" + call["id"], role="tool", content=content,
                                  tool_call_id=call["id"], status="completed")
            saved.append(stored)
            results.append(message)
        return {"messages": [*state["messages"], *results]}

    def after_tools(state):
        # Последняя попытка вызвать tools уже получила корректные ToolMessage.
        # Завершаем с явным ответом вместо бесконечного цикла.
        return "limit" if state["steps"] >= settings.max_steps else "prepare"

    def limit(state):
        answer = AIMessage(content="Достигнут лимит шагов. Уточни вопрос, чтобы продолжить.")
        saved.append(store.append(turn, "limit", role="assistant", content=answer.content,
                                  status="completed"))
        get_stream_writer()({"type": "message_start", "message_id": "limit"})
        get_stream_writer()({"type": "text_delta", "text": answer.content})
        get_stream_writer()({"type": "message_done", "message_id": "limit"})
        return {"messages": [*state["messages"], answer]}

    graph = StateGraph(State)
    graph.add_node("prefetch", prefetch)
    graph.add_node("prepare", prepare)
    graph.add_node("call_llm", call_model)
    graph.add_node("tools", call_tools)
    graph.add_node("limit", limit)
    graph.add_node("retry", retry)
    graph.add_node("honest", honest)
    graph.add_edge(START, "prefetch")
    graph.add_edge("prefetch", "prepare")
    graph.add_edge("prepare", "call_llm")
    graph.add_conditional_edges("call_llm", route,
                                {"tools": "tools", "retry": "retry", "honest": "honest", END: END})
    graph.add_conditional_edges("retry", after_retry, {"tools": "tools", END: END})
    graph.add_edge("honest", END)
    graph.add_conditional_edges("tools", after_tools, {"limit": "limit", "prepare": "prepare"})
    graph.add_edge("limit", END)
    return graph.compile()
