"""Явная демонстрация контракта Go в RAM. Не используется в режиме go."""
import copy
import re
import threading
import time
from uuid import uuid4
from chat.models import User, AgentError, Summary, Message, Turn, MemoryHit, storage_message

class DemoGateway:
    def whoami(self, token):
        if token not in ("demo-alice", "demo-bob"):
            raise AgentError("identity_unverified", 401)
        uid = 1 if token == "demo-alice" else 2
        return User(id=uid, full_name="Аня" if uid == 1 else "Борис",
                    role="intern", team_id=uid, team="Учебная команда")

    def search_docs(self, query, token):
        self.whoami(token)
        return {"found": True, "fragments": [{"source": "demo://git-access", "content":
            "ДЕМО, не регламент компании: для восстановления доступа к Git обратитесь к наставнику."}]}

class MemoryBackend:
    def __init__(self):
        self.lock = threading.RLock()
        self.chats = {}
        self.next_chat = self.next_message = 1

    def bind(self, user, token):
        return MemoryStore(self, user)

class MemoryStore:
    def __init__(self, backend, user):
        self.backend, self.user = backend, user

    def chat(self, cid):
        chat = self.backend.chats.get(cid)
        if not chat or chat["user_id"] != self.user.id:
            raise AgentError("conversation_not_found", 404)
        return chat

    def create_conversation(self, title):
        with self.backend.lock:
            cid = self.backend.next_chat
            self.backend.next_chat += 1
            self.backend.chats[cid] = dict(id=cid, user_id=self.user.id, title=title,
                messages=[], summary=Summary(), runs={}, clients={}, chunks=[])
            return {"id": cid, "title": title}

    def conversations(self):
        with self.backend.lock:
            return {"conversations": [{"id": c["id"], "title": c["title"]}
                for c in self.backend.chats.values() if c["user_id"] == self.user.id]}

    def messages(self, cid, before=None):
        with self.backend.lock:
            rows = [m for m in self.chat(cid)["messages"] if before is None or m.sequence_number < before]
            page = rows[-50:]
            return {"messages": [m.model_dump() for m in page],
                    "next_before": page[0].sequence_number if len(rows) > 50 else None}

    def begin(self, cid, content, client_id):
        with self.backend.lock:
            c = self.chat(cid)
            for run in c["runs"].values():
                if run["status"] == "running" and run["expires"] < time.monotonic():
                    run["status"] = "interrupted"
            if client_id in c["clients"]:
                raise AgentError("duplicate_message", 409)
            if any(r["status"] == "running" for r in c["runs"].values()):
                raise AgentError("conversation_busy", 409)
            run_id = str(uuid4())
            c["runs"][run_id] = {"status": "running", "expires": time.monotonic() + 1800, "keys": {}, "indexed": False}
            c["clients"][client_id] = run_id
            turn = Turn(run_id=run_id, conversation_id=cid, user_id=self.user.id,
                        summary=copy.deepcopy(c["summary"]), history=[], user_message=Message(
                            id=1, sequence_number=1, run_id=run_id, role="user", payload={}))
            history = [m for m in c["messages"] if m.sequence_number > c["summary"].through_sequence
                       and c["runs"][m.run_id]["status"] == "completed"]
            turn.history = copy.deepcopy(history)
            turn.user_message = self.append(turn, "user", role="user", content=content)
            return turn

    def append(self, turn, key, **message):
        with self.backend.lock:
            c = self.chat(turn.conversation_id)
            run = c["runs"][turn.run_id]
            if key in run["keys"]:
                return copy.deepcopy(run["keys"][key])
            if run["status"] != "running":
                raise AgentError("run_closed", 409)
            row = Message(id=self.backend.next_message, sequence_number=len(c["messages"]) + 1,
                          run_id=turn.run_id, **storage_message(message))
            self.backend.next_message += 1
            c["messages"].append(row)
            run["keys"][key] = row
            return copy.deepcopy(row)

    def save_summary(self, turn, summary):
        with self.backend.lock:
            c = self.chat(turn.conversation_id)
            if c["summary"].version != turn.summary.version:
                raise AgentError("summary_conflict", 409)
            c["summary"] = summary.model_copy(update={"version": summary.version + 1})
            return copy.deepcopy(c["summary"])

    def finish(self, turn, status):
        with self.backend.lock:
            run = self.chat(turn.conversation_id)["runs"][turn.run_id]
            if run["status"] == "running":
                run["status"] = status
            return {"status": run["status"]}

    def index_run(self, turn):
        # Demo не вычисляет векторы. Имитируем успешную индексацию Go.
        with self.backend.lock:
            c = self.chat(turn.conversation_id)
            run = c["runs"][turn.run_id]
            if run["status"] != "completed":
                raise AgentError("run_not_completed", 409)
            run["indexed"] = True
            return {"status": "indexed"}

    def reindex(self, cid):
        with self.backend.lock:
            c = self.chat(cid)
            pending = [r for r in c["runs"].values() if r["status"] == "completed" and not r["indexed"]]
            for run in pending:
                run["indexed"] = True
            return {"indexed": len(pending), "failed": 0}

    def search(self, query):
        words = set(re.findall(r"\w+", query.lower()))
        scored = []
        with self.backend.lock:
            for c in self.backend.chats.values():
                if c["user_id"] != self.user.id:
                    continue
                # Ищем исходные сообщения даже до построения индекса.
                completed = [m for m in c["messages"] if c["runs"][m.run_id]["status"] == "completed"]
                for i, m in enumerate(completed):
                    score = len(words & set(re.findall(r"\w+", str(m.content).lower())))
                    if not score:
                        continue
                    nearby = completed[max(0, i - 1):i + 2]
                    hit = MemoryHit(user_id=self.user.id, conversation_id=c["id"],
                        start_sequence=nearby[0].sequence_number, end_sequence=nearby[-1].sequence_number,
                        content="\n".join(f"{x.role}: {x.content}" for x in nearby))
                    scored.append((score, hit))
        return [hit for _, hit in sorted(scored, key=lambda x: x[0], reverse=True)[:5]]

