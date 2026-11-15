# 06. Публичный API и поток ответа для интерфейса

Код: `backend/internal/httpapi/` (`server.go`, `handlers.go`, `conversations.go`, `proxy.go`, `agent_handlers.go`), `backend/internal/chat/`, `backend/internal/postgres/buddy/chat.go`, миграция `0004_chat.up.sql`.
Тесты: `backend/internal/httpapi/*_test.go`, `backend/internal/chat/service_test.go`, `backend/internal/postgres/buddy/chat_test.go`, `backend/internal/postgres/buddy/chat_integration_test.go`.

## Цель

Дать интерфейсу один понятный API: вход, профиль, чаты, история и потоковый ответ Buddy. Простые операции не зависят от LLM и отвечают быстро. Ответ агента приходит потоком событий, поэтому интерфейс не блокируется на время работы модели.

## Ручки для интерфейса

Базовый адрес `http://localhost:8080`. Интерфейс в Docker ходит на тот же API через nginx по относительному пути `/api` на `http://localhost:3000`. Все ручки, кроме входа и `/healthz`, требуют `Authorization: Bearer <JWT>`. Ответы в JSON, ошибки в виде `{"error": "..."}`. CORS разрешает любой источник: токен идёт в заголовке, а не в cookie.

| Метод | Путь | Тело | Ответ |
|---|---|---|---|
| GET | `/healthz` | | 200 |
| POST | `/api/login` | `{"login":"sasha.intern"}` | 200 `{token, user}`; 400 пустой логин; 401 неизвестный логин |
| GET | `/api/me` | | 200 `user` |
| POST | `/api/conversations` | `{"title":"..."}` (необязательно, до 200 символов) | 201 `Conversation` |
| GET | `/api/conversations` | | 200 `{conversations: [Conversation]}`, новые первыми |
| GET | `/api/conversations/{id}/messages?limit=50&before=<seq>` | | 200 `{messages: [VisibleMessage]}`; `limit` от 1 до 200 |
| POST | `/api/chat` | `{conversation_id, client_message_id, content}` | 200 `text/event-stream` |
| GET | `/api/checklist`, PATCH `/api/checklist/tasks/{id}` | | см. [02-onboarding.md](02-onboarding.md) |
| GET | `/api/reviews`, `/api/interns`, POST `/api/reviews/{task_id}/approve`, `/api/reviews/{task_id}/reject` | | только наставник, см. [02-onboarding.md](02-onboarding.md) |
| GET | `/api/team` | | 200 `{members}`, см. [04-team.md](04-team.md) |
| GET | `/api/request-types`, `/api/requests`, POST `/api/requests/time-off` | | см. [03-absence.md](03-absence.md) |

Объекты:
```json
// user
{ "id": 3, "login": "sasha.intern", "full_name": "Саша Иванов", "role": "intern",
  "team_id": 2, "team": "Команда Авито Путешествий", "mentor": "Анна Смирнова" }
// Conversation
{ "id": 1, "title": "Как получить доступ к GitLab?", "last_sequence": 4,
  "created_at": "RFC3339", "updated_at": "RFC3339" }
// VisibleMessage
{ "id": 10, "role": "user|assistant", "content": "…", "sequence_number": 3, "created_at": "RFC3339" }
```

В истории для интерфейса остаются только сообщения `user` и `assistant` с непустым текстом. Вызовы инструментов, ответы `tool` и системные сообщения скрыты. `limit` применяется до фильтрации, поэтому видимых сообщений может прийти меньше.

## Поток ответа `POST /api/chat`

1. `api` проверяет токен и проксирует запрос агенту на `/internal/chat`, добавляя `X-Agent-Key`. Поток отдаётся без буферизации: каждый кусок отправляется клиенту сразу.
2. `client_message_id` генерирует клиент (UUID). Повтор с тем же id не создаёт новый ответ.
3. События и их данные описаны в [00-agent.md](00-agent.md#события-потока-sse). Для интерфейса важны:
   - `status`: показать, чем занят Buddy;
   - `message_start`: агент может ответить несколькими сообщениями подряд, интерфейс склеивает их через пустую строку;
   - `text_delta`: дописать текст;
   - `done`: ответ готов и сохранён;
   - `error`: показать понятную ошибку и кнопку «Повторить».
4. Если поток оборвался без `done` и без `error`, это обрыв, а не успех.

Ошибки до начала потока приходят обычным HTTP-ответом: 401 без токена, 404 чужой или несуществующий чат, 409 в чате уже идёт ход, 422 неверное тело, 502 `agent unavailable`.

## Хранение диалога

| Таблица | Что хранит |
|---|---|
| `buddy.conversations` | чаты пользователя |
| `buddy.runs` | ходы: `running`, `completed`, `failed`, `interrupted` |
| `buddy.messages` | все сообщения хода, включая вызовы инструментов, в формате OpenAI в `payload` |

Правила:
- в чате одновременно идёт не больше одного хода;
- `client_message_id` уникален в пределах чата;
- сообщение агента записывается с `event_key`, повтор с тем же ключом возвращает существующую запись;
- ход без записи дольше 5 минут считается просроченным и больше не блокирует чат.

## Внутренние ручки для агента

`/internal/agent/conversations…` нужны только агенту, требуют `X-Agent-Key` и токен пользователя. Контракт описан в комментарии к `backend/internal/httpapi/agent_handlers.go`: создание и список чатов, история с полным `payload`, начало хода (`turns`), запись сообщения хода и завершение хода (`finish`).

## Критерии приёмки

| Критерий | Тест |
|---|---|
| Вход по логину выдаёт рабочий токен, неизвестный логин даёт 401 | `handlers_test.go::TestLogin`, `TestLoginReturnsWorkingToken` |
| `/api/me` отдаёт свежий профиль | `handlers_test.go::TestMe` |
| CORS и `/healthz` работают без токена | `handlers_test.go::TestCORSAndHealth` |
| Ручки чатов требуют токен | `conversations_test.go::TestRoutingAuth` |
| Интерфейс получает только видимый текст, пустая история это `[]` | `conversations_test.go::TestListMessagesVisibleTextOnly`, `TestListMessagesEmptyIsArray` |
| Создание и список чатов, ошибки ввода | `conversations_test.go::TestConversationsCreateAndList`, `TestConversationsErrors` |
| Прокси отдаёт поток без буферизации и сохраняет базовый путь агента | `proxy_test.go::TestAgentProxyStreamsAndRewritesHeaders`, `TestAgentProxyKeepsTargetBasePath` |
| Недоступный агент даёт 502 | `proxy_test.go::TestAgentProxyAgentDown` |
| Проверка входа хода: `client_message_id`, `payload`, роли, статусы | `chat/service_test.go::TestStartTurnValidation`, `TestAppendMessage`, `TestFinishRun` |
| Один активный ход, просроченный ход освобождается, чужой чат недоступен | `chat_integration_test.go::TestChatIntegrationConcurrentTurns`, `TestChatIntegrationStaleRun`, `TestChatIntegrationForeignUser` |

Интеграционные тесты `chat_integration_test.go` работают с настоящим Postgres и пропускаются, если база не задана; остальные используют `pgxmock`.

## Вне рамок

- Отмена хода клиентом через отдельную ручку: закрытие соединения сохраняет частичный ответ со статусом `interrupted`.
- Веб-сокеты: потока SSE достаточно для одностороннего ответа.
