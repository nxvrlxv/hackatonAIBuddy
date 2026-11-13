"""Граф одного хода. Идентичность проверена ДО создания графа и не хранит токен."""
import json
from typing import TypedDict
from langchain_core.messages import AIMessage, ToolMessage
from langchain_core.tools import tool
from langchain_core.utils.function_calling import convert_to_openai_tool
from langgraph.config import get_stream_writer
from langgraph.graph import START, END, StateGraph

from chat.models import AgentError
from memory.context import build_input, units, text_of

class State(TypedDict):
    '''Состояние графа'''
    messages: list
    history: list
    memories: list
    steps: int

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

    @tool
    def get_request_types() -> str:
        """Получить типы заявок и обязательные поля перед оформлением отгула."""
        return json.dumps(gateway.call("get_request_types", {}, token), ensure_ascii=False)

    @tool
    def list_my_requests() -> str:
        """Получить свои заявки и статусы. submitted означает подана, а не согласована."""
        return json.dumps(gateway.call("list_my_requests", {}, token), ensure_ascii=False)

    @tool
    def create_time_off_request(start_date: str, end_date: str, reason: str = "") -> str:
        """Создать отгул по явной просьбе оформить. Даты YYYY-MM-DD включительно.
        Не вызывай для справочного вопроса или пока даты не уточнены. Причина необязательна.
        """
        # One mutation per turn; identity and retry key are never chosen by the model.
        return json.dumps(gateway.call("create_time_off_request", {
            "start_date": start_date, "end_date": end_date, "reason": reason,
            "idempotency_key": "time-off:" + turn.run_id,
        }, token), ensure_ascii=False)

    tools = [search_docs]
    if settings.mode == "go":
        tools.extend([get_request_types, list_my_requests, create_time_off_request])
    if settings.history_search_enabled:
        tools.append(search_history)
    by_name = {t.name: t for t in tools}
    llm = model.bind_tools(tools)
    tool_cost = units([convert_to_openai_tool(t) for t in tools])

    def prepare(state):
        before = turn.summary.through_sequence
        summary, history, memories = context.compact(
            user, turn, state["history"], state["messages"], state["memories"], store, tool_cost)
        if summary.through_sequence != before:
            get_stream_writer()({"type": "status", "text": "Сохранил резюме старой части разговора"})
        return {"history": history, "memories": memories}

    def call_model(state):
        step = state["steps"] + 1
        key = f"assistant:{step}"
        get_stream_writer()({"type": "message_start", "message_id": key})
        answer = llm.invoke(build_input(
            user, turn.summary, state["history"], state["messages"], state["memories"],
            history_search_enabled=settings.history_search_enabled))
        # Сохранение до сообщения о завершении. При повторной записи key уникален в run.
        stored = store.append(turn, key, role="assistant", content=answer.content,
                              tool_calls=answer.tool_calls, status="completed")
        saved.append(stored)
        get_stream_writer()({"type": "message_done", "message_id": key,
                             "stored_message_id": stored.id, "content": answer.content})
        return {"messages": [*state["messages"], answer], "steps": step}

    def route(state):
        if not state["messages"][-1].tool_calls:
            return END
        return "tools"

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
    graph.add_node("prepare", prepare)
    graph.add_node("call_llm", call_model)
    graph.add_node("tools", call_tools)
    graph.add_node("limit", limit)
    graph.add_edge(START, "prepare")
    graph.add_edge("prepare", "call_llm")
    graph.add_conditional_edges("call_llm", route, {"tools": "tools", END: END})
    graph.add_conditional_edges("tools", after_tools, {"limit": "limit", "prepare": "prepare"})
    graph.add_edge("limit", END)
    return graph.compile()
