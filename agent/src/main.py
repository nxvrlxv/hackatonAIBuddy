"""Flask-приложение. В go-режиме БД и публичный API принадлежат Go."""
from flask import Flask
from langchain_ollama import ChatOllama
from config import Settings
from chat.service import ChatService
from clients.go import GoStore
from clients.mcp import MCPGateway
from api.routers import router

def create_app(settings=None, model=None, gateway=None, store_factory=None, summary_model=None):
    settings = settings or Settings.from_env()
    if settings.mode == "go" and not settings.service_key:
        raise ValueError("AGENT_SERVICE_TOKEN обязателен")
    app = Flask(__name__)
    app.config["MAX_CONTENT_LENGTH"] = 64 * 1024
    if model is None:
        model = ChatOllama(model=settings.model, base_url=settings.ollama_url,
            temperature=0, reasoning=False, num_predict=settings.output_tokens,
            num_ctx=settings.context_tokens, client_kwargs={"timeout": settings.timeout})
    if settings.mode == "demo":
        from demo.backend import DemoGateway, MemoryBackend
        backend = MemoryBackend()
        gateway = gateway or DemoGateway()
        store_factory = store_factory or backend.bind
        app.extensions["demo_backend"] = backend
    else:
        gateway = gateway or MCPGateway(settings)
        store_factory = store_factory or (lambda user, token: GoStore(settings, user, token))
    app.extensions["settings"] = settings
    app.extensions["chat_service"] = ChatService(settings, model, gateway, store_factory, summary_model)
    app.register_blueprint(router)
    return app
