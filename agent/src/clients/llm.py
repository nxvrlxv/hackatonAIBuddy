"""Выбор только диалоговой модели. Эмбеддинги остаются на Go/Ollama."""
from pathlib import Path

def create_model(settings):
    if settings.llm_provider == "gigachat":
        from langchain_gigachat import GigaChat
        # credentials, not access_token: SDK obtains/refreshes OAuth tokens itself.
        ca = settings.gigachat_ca_bundle or str(Path(__file__).with_name("certs") / "russian_trusted_root_ca.pem")
        if not Path(ca).is_file():
            raise ValueError("Не найден GIGACHAT_CA_BUNDLE_FILE")
        return GigaChat(
            credentials=settings.gigachat_credentials,
            scope=settings.gigachat_scope, model=settings.model,
            base_url=settings.gigachat_base_url,
            ca_bundle_file=ca, verify_ssl_certs=True,
            temperature=0, max_tokens=settings.output_tokens,
            timeout=settings.timeout, streaming=True,
        )
    from langchain_ollama import ChatOllama
    return ChatOllama(model=settings.model, base_url=settings.ollama_url,
        temperature=0, reasoning=False, num_predict=settings.output_tokens,
        num_ctx=settings.context_tokens, client_kwargs={"timeout": settings.timeout})
