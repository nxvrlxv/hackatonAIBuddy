"""HTTP-клиент хранилища Go. Один экземпляр привязан к одному пользователю."""
import httpx
from chat.models import AgentError, Turn, Message, Summary, MemoryHit, storage_message

class GoStore:
    def __init__(self, settings, user, token, transport=None):
        self.settings, self.user = settings, user
        self.headers = {
            "Authorization": f"Bearer {token}",
            "X-Agent-Key": settings.service_key,
        }
        self.transport = transport

    def request(self, method, path, **kwargs):
        try:
            with httpx.Client(
                base_url=self.settings.go_url, headers=self.headers,
                timeout=self.settings.timeout, transport=self.transport,
            ) as client:
                response = client.request(method, "/internal/agent" + path, **kwargs)
            if response.status_code >= 400:
                status = response.status_code if response.status_code in (401, 403, 404, 409) else 502
                raise AgentError("storage_rejected" if status != 502 else "storage_unavailable", status)
            return response.json()
        except httpx.HTTPError:
            raise AgentError("storage_unavailable") from None

    def create_conversation(self, title):
        return self.request("POST", "/conversations", json={"title": title})

    def conversations(self):
        return self.request("GET", "/conversations")

    def messages(self, cid, before=None):
        return self.request("GET", f"/conversations/{cid}/messages",
                            params={"limit": 50, **({"before": before} if before else {})})

    def decode_message(self, data, cid):
        data = dict(data)
        if data.get("conversation_id", cid) != cid:
            raise AgentError("owner_mismatch", 403)
        data["status"] = data.get("payload", {}).get("status", data.get("status", "completed"))
        return Message.model_validate(data)

    def begin(self, cid, content, client_id):
        data = self.request("POST", f"/conversations/{cid}/turns",
            json={"payload": {"content": content}, "client_message_id": client_id})
        if data.get("user_id", self.user.id) != self.user.id or data.get("conversation_id", cid) != cid:
            raise AgentError("owner_mismatch", 403)
        turn = Turn(run_id=data["run_id"], conversation_id=cid, user_id=self.user.id,
                    summary=data["summary"],
                    user_message=self.decode_message(data["user_message"], cid),
                    history=[self.decode_message(m, cid) for m in data["history"]])
        if turn.user_message.run_id != turn.run_id:
            raise AgentError("invalid_run")
        from memory.context import complete_history
        turn.history = complete_history(turn.history)
        if turn.user_id != self.user.id or turn.conversation_id != cid:
            raise AgentError("owner_mismatch", 403)
        return turn

    def append(self, turn, key, **message):
        data = storage_message(message)
        status = data.pop("status", "completed")
        if status != "completed":
            data["payload"]["status"] = status
        result = self.request(
            "POST", f"/conversations/{turn.conversation_id}/runs/{turn.run_id}/messages",
            json={"event_key": key, **data})
        stored = self.decode_message(result, turn.conversation_id)
        if stored.run_id != turn.run_id:
            raise AgentError("invalid_run")
        return stored

    def save_summary(self, turn, summary):
        return Summary.model_validate(self.request(
            "PUT", f"/conversations/{turn.conversation_id}/summary",
            json={"run_id": turn.run_id, "expected_version": turn.summary.version,
                  "text": summary.text, "through_sequence": summary.through_sequence}))

    def finish(self, turn, status):
        return self.request("POST",
            f"/conversations/{turn.conversation_id}/runs/{turn.run_id}/finish",
            json={"status": status})

    def search(self, query):
        # Вектор запроса и выбор embedding-модели полностью принадлежат Go.
        data = self.request("POST", "/memory/search", json={"query": query, "limit": 5})
        hits = [MemoryHit.model_validate(item) for item in data["hits"]]
        if any(hit.user_id != self.user.id for hit in hits):
            raise AgentError("owner_mismatch", 403)
        return hits

    def index_run(self, turn):
        return self.request("POST",
            f"/conversations/{turn.conversation_id}/runs/{turn.run_id}/index", json={})

    def reindex(self, cid):
        return self.request("POST", f"/conversations/{cid}/reindex", json={})
