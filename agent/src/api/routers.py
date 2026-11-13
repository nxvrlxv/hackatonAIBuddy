"""Внутренний API агента. Go проксирует SSE, но не отдаёт credential браузеру."""
import secrets
from pathlib import Path
from flask import Blueprint, Response, current_app, jsonify, request, render_template_string
from pydantic import BaseModel, Field, ValidationError, field_validator
from chat.models import AgentError

router = Blueprint("api", __name__)

class ChatRequest(BaseModel):
    conversation_id: int = Field(gt=0)
    client_message_id: str = Field(min_length=1, max_length=128)
    content: str = Field(min_length=1, max_length=12000)
    @field_validator("content", "client_message_id")
    @classmethod
    def nonempty(cls, value):
        value = value.strip()
        if not value:
            raise ValueError("Пустое значение")
        return value

class CreateConversation(BaseModel):
    title: str = Field(default="Новый чат", max_length=200)

def credentials():
    settings = current_app.extensions["settings"]
    if settings.mode == "go" and not secrets.compare_digest(
        request.headers.get("X-Agent-Key", ""), settings.service_key
    ):
        raise AgentError("service_unauthorized", 403)
    auth = request.headers.get("Authorization", "")
    if not auth.startswith("Bearer ") or not auth[7:].strip():
        raise AgentError("missing_token", 401)
    return auth[7:].strip()

def service():
    return current_app.extensions["chat_service"]

@router.app_errorhandler(AgentError)
def agent_error(exc):
    return jsonify(error=exc.code), exc.status

@router.app_errorhandler(ValidationError)
def validation_error(exc):
    return jsonify(detail=exc.errors(include_context=False, include_input=False)), 422

@router.get("/")
def demo():
    if current_app.extensions["settings"].mode != "demo":
        return jsonify(service="ai-buddy", mode="go", endpoint="/internal/chat", demo="/demo")
    return demo_page()

@router.get("/demo")
def demo_page():
    settings = current_app.extensions["settings"]
    return render_template_string(Path(__file__).with_name("demo.html").read_text(), demo_config={
        "mode": settings.mode, "public_go_url": settings.public_go_url,
        "history_search_enabled": settings.history_search_enabled,
        "summary_enabled": settings.summary_enabled,
    })

@router.get("/healthz")
def health():
    return jsonify(status="ok", mode=current_app.extensions["settings"].mode)

@router.post("/internal/conversations")
def create_conversation():
    token = credentials()
    body = CreateConversation.model_validate(request.get_json())
    _, store = service().identify(token)
    return jsonify(store.create_conversation(body.title)), 201

@router.get("/internal/conversations")
def conversations():
    _, store = service().identify(credentials())
    return jsonify(store.conversations())

@router.get("/internal/conversations/<int:cid>/messages")
def messages(cid):
    _, store = service().identify(credentials())
    before = request.args.get("before")
    if before is not None and (not before.isdigit() or int(before) < 1):
        raise AgentError("invalid_cursor", 422)
    return jsonify(store.messages(cid, int(before) if before else None))

@router.post("/internal/chat")
def chat():
    token = credentials()
    body = ChatRequest.model_validate(request.get_json())
    prepared = service().prepare(token, body.conversation_id, body.content, body.client_message_id)
    response = Response(prepared.events(), mimetype="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})
    # Закрываем run, даже если клиент не начал читать генератор.
    response.call_on_close(prepared.close)
    return response

@router.post("/internal/conversations/<int:cid>/reindex")
def reindex(cid):
    # Go сам выбирает неиндексированные ходы и вычисляет эмбеддинги.
    _, store = service().identify(credentials())
    if not current_app.extensions["settings"].history_search_enabled:
        raise AgentError("history_search_disabled", 409)
    result = store.reindex(cid)
    return jsonify(result), (200 if result["failed"] == 0 else 503)
