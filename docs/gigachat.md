# GigaChat для диалогового агента

Меняется только Python LLM. Go продолжает считать эмбеддинги через Ollama / qwen3-embedding:0.6b;
переиндексация не требуется. Ollama остаётся в compose для RAG.

В agent/.env:
```dotenv
LLM_PROVIDER=gigachat
GIGACHAT_MODEL=GigaChat-2
GIGACHAT_CREDENTIALS=<ключ авторизации из кабинета>
GIGACHAT_SCOPE=GIGACHAT_API_PERS
GIGACHAT_BASE_URL=https://api.giga.chat/v1
```

Также поддерживается существующее имя GIGACHAT_API_TOKEN как alias КЛЮЧА АВТОРИЗАЦИИ.
Это не короткоживущий access token. Приоритет у GIGACHAT_CREDENTIALS.
Для другого типа аккаунта установите соответствующий scope (B2B/CORP).
Секрет хранится только в локальном env, не в БД или промпте.

## Токен
create_model в agent/src/clients/llm.py передаёт credentials официальному SDK.
SDK получает OAuth access token при первом запросе, кеширует его и обновляет перед истечением
(30 минут, с запасом). Поэтому отдельный поток/cron/ручной refresh не нужен.
Кеш относится к экземпляру клиента в процессе; каждый worker получает собственный клиент.
Это проверено по установленному gigachat 0.2.4 (проверка expires_at и token_expiry_buffer_ms).
Срок действия ключа авторизации и квота аккаунта управляются провайдером.

## TLS
Проверка TLS всегда включена. Публичный корневой сертификат из официальной документации
включён в Python-пакет: clients/certs/russian_trusted_root_ca.pem.
Источник: https://gu-st.ru/content/lending/russian_trusted_root_ca_pem.crt
Он используется только клиентом GigaChat, системное хранилище доверия не меняется.
Для ротации можно указать GIGACHAT_CA_BUNDLE_FILE с путём к новому PEM bundle.
В контейнере путь должен существовать: при внешнем сертификате добавьте read-only volume.

## Запуск
Из корня: docker compose up -d --build --no-deps agent
Compose уже читает agent/.env; ключ не нужно копировать в корневой .env.
При запуске Flask локально экспортируйте переменные в окружение процесса.
Для отката: LLM_PROVIDER=ollama и прежние OLLAMA_MODEL/OLLAMA_URL, затем пересоздайте agent.
Остальные сервисы не нужно перенастраивать.

## Контекст и инструменты
Все системные инструкции, профиль и резюме объединяются в одно первое SystemMessage:
GigaChat отвергает несколько system-сообщений.
Официальная обёртка преобразует LangChain tool calls и ToolMessage в протокол GigaChat.
streaming=True сохраняет передачу токенов через существующий SSE-код.
CONTEXT_TOKENS остаётся бюджетом приложения; это не настройка размера окна на стороне GigaChat.
При включении облачного LLM сформированный контекст (профиль, сообщения и результаты tools)
передаётся в GigaChat; JWT Go и ключ авторизации в него не входят.

Документация:
- https://developers.sber.ru/docs/ru/gigachat/certificates
- https://developers.sber.ru/docs/ru/gigachat/api/reference/rest/post-token
- https://github.com/ai-forever/langchain-gigachat
