"""Проверяет личность, запускает ход и организует сохранение/поток. SQL здесь нет."""
import json
from contextlib import closing
from langchain_core.messages import AIMessageChunk
from chat.models import AgentError
from agent.graph import build_graph
from memory.context import ContextManager
from memory.search import HistoryMemory

def sse_event(name, data):
    return f"event: {name}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"

class PreparedTurn:
    def __init__(self, service, user, token, store, turn):
        self.service, self.user, self.token = service, user, token
        self.store, self.turn = store, turn
        self.finished = False
        self.partial, self.pending_key = "", None

    def finish(self, status):
        if not self.finished:
            self.store.finish(self.turn, status)
            self.finished = True

    def close(self):
        if self.finished:
            return
        try:
            if self.partial and self.pending_key:
                self.store.append(self.turn, self.pending_key, role="assistant",
                                  content=self.partial, status="interrupted")
            self.finish("interrupted")
        except Exception:
            # Go должен освобождать зависшие run по lease; нельзя обещать фиксацию при сетевом сбое.
            pass

    def events(self):
        s = self.service
        memory = HistoryMemory(self.store)
        saved = [self.turn.user_message]
        try:
            yield sse_event("accepted", {"run_id": self.turn.run_id,
                                        "user_message_id": self.turn.user_message.id})
            yield sse_event("status", {"text": "Пользователь проверен. Восстанавливаю контекст"})
            try:
                memories = memory.search(str(self.turn.user_message.content))
            except Exception:
                memories = []
                yield sse_event("warning", {"code": "history_search_unavailable"})
            graph = build_graph(s.model, ContextManager(s.settings, s.summary_model),
                s.settings, self.user, self.turn, self.store, s.gateway, self.token, memory, saved)
            initial = {
                "messages": [self.turn.user_message.as_langchain()],
                "history": self.turn.history, "memories": memories, "steps": 0,
            }
            with closing(graph.stream(initial, stream_mode=["messages", "custom"],
                         version="v2", config={"recursion_limit": s.settings.max_steps * 4 + 10})) as parts:
                for part in parts:
                    if part["type"] == "custom":
                        event = dict(part["data"])
                        kind = event.pop("type")
                        if kind == "message_start":
                            self.pending_key, self.partial = event["message_id"], ""
                        elif kind == "message_done":
                            final = event.pop("content", "")
                            if not isinstance(final, str):
                                final = "".join(b.get("text", "") for b in final
                                    if isinstance(b, dict) and b.get("type") == "text")
                            # Некоторые модели возвращают только целый ответ.
                            if final and not self.partial:
                                yield sse_event("text_delta", {"text": final, "message_id": self.pending_key})
                            self.pending_key, self.partial = None, ""
                        yield sse_event(kind, event)
                    elif part["type"] == "messages":
                        chunk, meta = part["data"]
                        if meta.get("langgraph_node") != "call_llm" or "memory_summary" in meta.get("tags", []):
                            continue
                        if not isinstance(chunk, AIMessageChunk) or self.pending_key is None:
                            continue
                        content = chunk.content
                        text = content if isinstance(content, str) else "".join(
                            b.get("text", "") for b in content if isinstance(b, dict) and b.get("type") == "text")
                        if text:
                            self.partial += text
                            yield sse_event("text_delta", {"text": text, "message_id": self.pending_key})
            # Сначала фиксируем ход. Go индексирует только завершённые ходы.
            self.finish("completed")
            try:
                memory.index(self.turn)
            except Exception:
                yield sse_event("warning", {"code": "index_pending"})
            yield sse_event("done", {"run_id": self.turn.run_id})
        except GeneratorExit:
            raise
        except Exception as exc:
            code = exc.code if isinstance(exc, AgentError) else "agent_failed"
            try:
                if self.partial and self.pending_key:
                    self.store.append(self.turn, self.pending_key, role="assistant",
                                      content=self.partial, status="failed")
                self.finish("failed")
            except Exception:
                code = "storage_unavailable"
            yield sse_event("error", {"code": code})
        finally:
            self.close()
            self.token = ""  # не сохраняем credential в долгоживущем объекте

class ChatService:
    def __init__(self, settings, model, gateway, store_factory, summary_model=None):
        self.settings, self.model, self.gateway = settings, model, gateway
        self.store_factory = store_factory
        self.summary_model = summary_model or model

    def identify(self, token):
        # Обязательный первый вызов. Модель не может пропустить его или заменить токен.
        user = self.gateway.whoami(token)
        return user, self.store_factory(user, token)

    def prepare(self, token, cid, content, client_id):
        user, store = self.identify(token)
        turn = store.begin(cid, content, client_id)
        if turn.user_id != user.id:
            raise AgentError("owner_mismatch", 403)
        return PreparedTurn(self, user, token, store, turn)
