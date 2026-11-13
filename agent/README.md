# AI-buddy: Flask-агент

Go отвечает за авторизацию, PostgreSQL, MCP и публичный API.
Python собирает контекст, вызывает LLM/инструменты и отдаёт SSE через Go.

Интеграция согласована с origin/feature/mcps_archi (a035415).
Запуск описан в [docker-compose.md](../docs/docker-compose.md), API заявок — в [time-off.md](../docs/time-off.md).

## Запуск

Из корня проекта создайте .env по .env.example и задайте свои секреты и модель.
Существующий .env не перезаписывайте: добавьте недостающие параметры из примера.

```bash
docker compose up --build -d api mcp-buddy agent
docker compose logs -f agent
```

Compose запускает PostgreSQL и мигратор по зависимостям. Используется единственная
миграция 0004_chat: buddy.conversations, buddy.runs, buddy.messages.
Она заменяет неприменённую 0004_agent_memory. Пятой миграции нет.

API: localhost:8080; MCP: localhost:8090/mcp; агент: localhost:8001.
В сети контейнеров используются api:8080, mcp-buddy:8090 и agent:8001.
Go и Python получают один сервисный ключ: Compose подставляет AGENT_KEY
в AGENT_SERVICE_TOKEN агента. Пользовательский JWT передаётся отдельно.

Все настройки Python читаются из окружения, без скрытых значений по умолчанию.
Корневой .env.example содержит параметры всего стенда.
agent/.env — необязательные дополнительные настройки агента; адреса сервисов и
сервисный ключ Compose задаёт явно. Не включайте отложенные возможности в go-режиме.

Ollama запускается отдельным контейнером. ollama-models скачивает
qwen3-embedding:0.6b и qwen3:4b-instruct до запуска агента и MCP.
Внутри Compose OLLAMA_DOCKER_URL=http://ollama:11434;
для локального Python OLLAMA_URL=http://localhost:11434.
Подробности, GPU и таблица URL: [Docker Compose](../docs/docker-compose.md).
Эмбеддинги корпоративных документов настраивает Go через EMBEDDED_MODEL.
Для наполнения базы корпоративных документов отдельно запускается ingest:
```bash
docker compose run --rm ingest
```

## Что работает в MVP

- whoami перед каждым запросом: только Authorization, аргументы инструмента пустые.
- Проверенный профиль заново добавляется к каждому запросу LLM.
- Полная история сохраняется в Go; последние доступные целые ходы идут в контекст.
- search_docs обращается к MCP, текст ответа передаётся пользователю по SSE.
- Повтор client_message_id и параллельный ход в одном чате дают 409.
- Go продлевает активность хода при append; пять минут без активности закрывают его
  для новых записей. Длительность отдельного внешнего вызова ограничена REQUEST_TIMEOUT.
- Python проверяет фактический статус finish и не выдаёт done для прерванного хода.

В текущем Go нет поиска по истории и сохранения резюме:
```dotenv
HISTORY_SEARCH_ENABLED=false
SUMMARY_ENABLED=false
```

Первый флаг отключает начальный поиск, инструмент search_history и индексацию.
Второй отключает генерацию/сохранение резюме. Go возвращает пустой summary,
version=0, through_sequence=0. При нехватке контекста старые целые ходы убираются
только из запроса LLM, в БД они остаются. Если текущий ход сам слишком велик —
context_too_large. Профиль не удаляется. Другие чаты модель пока не вспоминает.

Go отдаёт ограниченный хвост сообщений. Адаптер пропускает оборванный первый ход,
незавершённые пары вызова/ответа инструмента и помеченные неуспешными сообщения.
Возобновление прерванного графа не реализовано.
Включать флаги для Go можно после реализации соответствующих API; demo поддерживает оба.

## Запрос с фронтенда

1. POST /api/login с {login} выдаёт JWT тестовой учётки (учебная имитация SSO).
2. POST /api/conversations с {title} создаёт чат.
3. POST /api/chat с Authorization: Bearer ...:
```json
{"conversation_id":1,"client_message_id":"уникальный-uuid","content":"Привет"}
```
4. Go подставляет свой X-Agent-Key и проксирует SSE из /internal/chat агента.
5. GET /api/conversations/{id}/messages возвращает видимый текст истории.

События: accepted, status, message_start, text_delta, message_done, warning,
done/error. Фрагменты text_delta сразу добавляются в интерфейс.
HTTP-ошибки возможны до начала потока; после начала ошибку передаёт SSE error.
Отсутствие done не означает успешное завершение. Не повторять POST автоматически.

## Граф и файлы

Граф: START → prepare → call_llm → tools → prepare либо END.
При лимите шагов tools → limit → END.

- src/main.py: Flask и сборка зависимостей.
- src/config.py: обязательные переменные окружения и флаги.
- src/agent/graph.py: узлы и инструменты.
- src/chat/service.py: whoami, начало/завершение хода, поток.
- src/chat/models.py: внутренние модели Python.
- src/memory/context.py: контекст, ограничение истории, опциональное резюме.
- src/memory/search.py: опциональные запросы памяти к Go.
- src/clients/go.py: преобразование внутреннего формата в фактический Go API.
- src/clients/mcp.py: MCP; токен только в HTTP-заголовке.
- src/api/routers.py: внутренний HTTP API.
- src/demo/backend.py: учебное хранилище в RAM.
- src/api/demo.html: форма /demo для настоящего Go API и учебного режима.

Сообщения содержат payload JSONB с content, tool_calls, tool_call_id.
Статус обработки находится в runs. Частичные сообщения Python дополнительно
помечает payload.status=failed/interrupted. Публичное API Go пока отдаёт только
текст без этого статуса; об ошибке текущего ответа фронт узнаёт через SSE.

## Локальный demo

Скопируйте agent/.env.example в agent/.env, установите AGENT_MODE=demo и доступный
из WSL адрес Ollama. Для демонстрации памяти можно включить оба флага.
```bash
cd agent
source .venv/bin/activate
python -m pip install -e .
set -a
. ./.env
set +a
python -m flask --app main:create_app run --host 127.0.0.1 --port 8001
```

В demo главная страница содержит форму, токены demo-alice/demo-bob.
БД, whoami и поиск имитируются, диалоговая модель настоящая.
Поиск лексический, данные исчезают при перезапуске, нужен один Gunicorn worker.

## Проверки

```bash
cd agent
python -m unittest discover -s tests -v
python -m pip check
cd ../backend
go test ./...
```

Тесты агента проверяют поток, изоляцию пользователей, повторные запросы,
сохранение пар инструментов, режим без памяти, резюме в demo и Go JSON-контракт.
Оценка контекста по UTF-8-байтам консервативна, это не точный tokenizer.
При аварии процесса незаписанный частичный текст может потеряться.

Для изолированной проверки настоящих Go/MCP/PostgreSQL/Flask с тестовой LLM:
```bash
cd agent
python tests/smoke_compose.py
```
Тест собирает образы, создаёт отдельный Compose-проект без открытых портов,
проверяет поток и хранение, затем удаляет только свой стенд и его временную БД.

## Настоящий чат и RAG

Откройте http://localhost:8001/demo. В go-режиме HTML использует публичный Go API,
тестовые логины и настоящую БД. PUBLIC_GO_API_URL задаёт адрес Go для браузера.
[Проверка чата и RAG](../docs/rag-testing.md): индексация, вопросы и SQL-проверки.

## Заявки на отгул

В go-режиме доступны создание и просмотр заявок через MCP. Контракт, ограничения и тестирование: [docs/time-off.md](../docs/time-off.md).
