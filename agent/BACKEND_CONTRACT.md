# Действующий контракт Go ↔ Python

Согласован с feature/mcps_archi, коммит a035415. Go API, whoami, хранение чатов
и SSE-прокси реализованы. Поиск по истории и сохранение резюме отложены.

## Авторизация и whoami

Python → Go: Authorization: Bearer <JWT>, X-Agent-Key: <service-secret>.
Python → MCP: Authorization: Bearer <JWT>.
whoami вызывается программно до чтения истории/LLM с аргументами {}.
Токен никогда не передаётся аргументом инструмента или в контекст модели.

whoami возвращает structuredContent:
```json
{"id":3,"full_name":"Саша Иванов","role":"intern","team_id":2,"team":"Команда","mentor":"Анна"}
```
MCP search_docs принимает {"question":"..."}, пользователь/команда определяются
на сервере из проверенного токена.

## Хранение

Единственная четвёртая миграция: backend/migrations/0004_chat.*.sql.
Таблицы: buddy.conversations (включая поля будущего резюме),
buddy.runs (включая status, last_activity_at), buddy.messages (payload JSONB).
Прежней 0004_agent_memory и пятой миграции быть не должно.

Message:
```json
{
  "id":10,"conversation_id":1,"run_id":"uuid","sequence_number":2,
  "event_key":"assistant:1","role":"assistant",
  "payload":{"content":"Ответ","tool_calls":[]},
  "created_at":"2026-11-13T00:00:00Z"
}
```

Ответ инструмента хранит payload.tool_call_id, соответствующий id в tool_calls.
Отдельной колонки tool_call_id нет. Статус принадлежит run.
Для частично полученного текста Python добавляет payload.status=failed/interrupted.

Все следующие пути начинаются с /internal/agent:

| Метод и путь | Тело → результат |
|---|---|
| POST /conversations | {title} → Conversation, 201 |
| GET /conversations | → {conversations:[...]} |
| GET /conversations/{id}/messages?limit=50&before=N | → {messages:[Message]} |
| POST /conversations/{id}/turns | {client_message_id,payload:{content}} → Turn, 201 |
| POST /conversations/{id}/runs/{run_id}/messages | {event_key,role,payload} → Message, 201/200 |
| POST /conversations/{id}/runs/{run_id}/finish | {status} → Run |

Turn не содержит верхнеуровневые user_id/conversation_id:
```json
{
  "run_id":"uuid",
  "user_message":{"id":1,"conversation_id":1,"run_id":"uuid","sequence_number":1,
    "event_key":"user","role":"user","payload":{"content":"Вопрос"},
    "created_at":"2026-11-13T00:00:00Z"},
  "history":[],
  "summary":{"text":"","version":0,"through_sequence":0}
}
```

Адаптер Python использует уже проверенного пользователя и запрошенный conversation_id.
Go проверяет владение во всех операциях. History не включает новое сообщение.
Выдача истории ограничена числом сообщений; Python пропускает оборванные ходы.
Для публичной/внутренней пагинации следующий before вычисляется по минимальному
sequence_number сырой страницы; отдельного next_before текущий Go не возвращает.

Run:
```json
{"run_id":"uuid","conversation_id":1,"status":"completed","finished_at":"2026-11-13T00:00:00Z"}
```

Повтор client_message_id → 409. Один running на чат.
Append идемпотентен по (run_id,event_key), каждый новый append продлевает активность.
После пяти минут без активности новые записи запрещены.
Finish идемпотентен и возвращает фактический статус, не переписывая завершённый.
Python выдаёт done только если Go подтвердил completed.

## Отложенная память

В текущем Go:
```dotenv
HISTORY_SEARCH_ENABLED=false
SUMMARY_ENABLED=false
```

При false Python не вызывает API памяти/резюме; search_history не связывается с LLM.
В контекст идут профиль, последние целые ходы текущего чата и текущий запрос.
Старые сообщения исключаются из контекста по бюджету, но остаются в БД.

Будущий контракт (НЕ реализован в Go; не включать флаги до реализации):
- POST /memory/search: {query,limit} → {hits:[{user_id,conversation_id,
  start_sequence,end_sequence,content,created_at?}]}.
- POST /conversations/{id}/runs/{run_id}/index: {} → {status:"indexed"}.
- POST /conversations/{id}/reindex: {} → {indexed:N,failed:N}.
- PUT /conversations/{id}/summary: {run_id,expected_version,text,through_sequence}
  → {text,version,through_sequence}; конфликт версии → 409.

Go самостоятельно нарезает историю и вычисляет embeddings документов/запросов,
ограничивает поиск пользователем до top-k, возвращает исходные фрагменты.
Python не знает модель эмбеддингов и не передаёт векторы.
Резюме генерирует Python; Go сохраняет текст/границу атомарно.
Таблицы поискового индекса истории в текущей 0004_chat нет.

## Фронтенд и Compose

Публичный API Go:
POST /api/login, GET /api/me, POST/GET /api/conversations,
GET /api/conversations/{id}/messages, POST /api/chat.

POST /api/chat проксируется на http://agent:8001/internal/chat без буферизации,
входящий X-Agent-Key заменяется серверным. Публичная история содержит только
видимый текст, а внутренняя — полный payload для восстановления инструментов.

Тело чата: {conversation_id,client_message_id,content}.
SSE: accepted, status, message_start, text_delta, message_done, warning, done/error.

Compose содержит postgres, migrator, api, mcp-buddy, agent, отдельный ingest.
Адреса: api:8080, mcp-buddy:8090/mcp, agent:8001.
Корневой .env задаёт AGENT_KEY; Compose передаёт его Python как AGENT_SERVICE_TOKEN.
agent/.env необязателен. Остальные параметры перечислены в .env.example.
