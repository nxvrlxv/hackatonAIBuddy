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

    def begin(self, cid, content, client_id):
        turn = Turn.model_validate(self.request("POST", f"/conversations/{cid}/turns",
            json={"content": content, "client_message_id": client_id}))
        if turn.user_id != self.user.id or turn.conversation_id != cid:
            raise AgentError("owner_mismatch", 403)
        return turn

    def append(self, turn, key, **message):
        return Message.model_validate(self.request(
            "POST", f"/conversations/{turn.conversation_id}/runs/{turn.run_id}/messages",
            json={"event_key": key, **storage_message(message)}))

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
