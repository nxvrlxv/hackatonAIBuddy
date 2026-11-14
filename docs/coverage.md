# Отчёт о покрытии кода тестами

Требование кейса: покрытие строк прикладного кода больше 80%. В расчёт не входят зависимости, сгенерированный код и сами тесты.

Отчёт снят 14.11.2026.

## Итог

| Часть | Язык | Тестов | Покрытие строк | Покрытие ветвлений | Код, на котором снято |
|---|---|---|---|---|---|
| Интерфейс | TypeScript | 105, все пройдены | **99,1%** (562 из 567) | 95,8% | ветка `test/frontend-components`, коммит `28e7f82` |
| Агент | Python | 50, все пройдены | **87%** (764 из 876) | не считалось | `main`, коммит `2885d5e` |
| Бэкенд | Go | | заполняется после прогона `make cover` | | `main` |

## Как воспроизвести

| Часть | Команда | Где отчёт |
|---|---|---|
| Интерфейс | `cd frontend && npm ci && npm run coverage` | таблица в терминале, HTML в `frontend/coverage/index.html` |
| Агент | `cd agent && uv run --with pytest --with pytest-cov pytest --cov=src --cov-report=term tests/` | таблица в терминале |
| Бэкенд | `make cover` из корня или `cd backend && go test ./... -coverprofile=cover.out && go tool cover -func=cover.out` | итог в последней строке; HTML: `go tool cover -html=cover.out` |

Без локального Go тесты бэкенда запускаются в контейнере:

```bash
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.27.1 sh -c "go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1"
```

Интеграционные тесты хранилища чатов (`backend/internal/postgres/buddy/chat_integration_test.go`) работают с настоящим PostgreSQL. Они запускаются, если задана переменная `DATABASE_URL` и применены миграции; без неё пропускаются, а остальные тесты используют `pgxmock`.

## Интерфейс по файлам

Покрытие считается по всем файлам `frontend/src` (`coverage.include` в `frontend/vite.config.ts`), а не только по тем, что загрузили тесты. Тесты экранов работают в jsdom через Testing Library, сеть в них подменена заглушкой, бэкенд не нужен.

| Файл | Строки | Покрытие строк | Ветвления |
|---|---|---|---|
| `src/App.tsx` | 35 из 35 | 100% | 100% |
| `src/Chat.tsx` | 100 из 102 | 98,0% | 94,0% |
| `src/Help.tsx` | 9 из 9 | 100% | 100% |
| `src/Home.tsx` | 50 из 50 | 100% | 95,6% |
| `src/Interns.tsx` | 17 из 17 | 100% | 96,2% |
| `src/Login.tsx` | 18 из 18 | 100% | 91,7% |
| `src/Markdown.tsx` | 4 из 4 | 100% | 100% |
| `src/Reviews.tsx` | 59 из 59 | 100% | 96,2% |
| `src/Shell.tsx` | 21 из 21 | 100% | 100% |
| `src/loginScale.ts` | 13 из 13 | 100% | 75,0% |
| `src/main.tsx` | 0 из 1 | 0% | |
| `src/ui.tsx` | 8 из 8 | 100% | 100% |
| `src/api/client.ts` | 182 из 184 | 98,9% | 96,6% |
| `src/api/mock.ts` | 23 из 23 | 100% | 95,5% |
| `src/api/sse.ts` | 23 из 23 | 100% | 92,9% |

- `main.tsx` — одна строка монтирования приложения в страницу, её проверяет запуск в браузере.
- Две строки `Chat.tsx` (фокус поля после подстановки заготовки) выполняются в следующем кадре браузера, поэтому покрытие файла колеблется между 98% и 100% от прогона к прогону.

Что проверяют тесты: [specs/07-frontend.md](specs/07-frontend.md#критерии-приёмки).

## Агент по файлам

| Файл | Операторов | Не покрыто | Покрытие |
|---|---|---|---|
| `src/agent/graph.py` | 251 | 17 | 93% |
| `src/api/routers.py` | 79 | 9 | 89% |
| `src/chat/models.py` | 59 | 2 | 97% |
| `src/chat/service.py` | 104 | 6 | 94% |
| `src/clients/go.py` | 65 | 19 | 71% |
| `src/clients/mcp.py` | 45 | 24 | 47% |
| `src/config.py` | 27 | 15 | 44% |
| `src/demo/backend.py` | 119 | 6 | 95% |
| `src/main.py` | 27 | 2 | 93% |
| `src/memory/context.py` | 93 | 12 | 87% |
| `src/memory/search.py` | 7 | 0 | 100% |
| **Итого** | **876** | **112** | **87%** |

Пустые `__init__.py` в таблицу не включены. Ниже всего покрыты сетевые клиенты (`clients/mcp.py`, `clients/go.py`) и чтение настроек (`config.py`). Логика хода, промпт и защита от ложной отметки покрыты на 87–94%.

Что проверяют тесты: [specs/00-agent.md](specs/00-agent.md#критерии-приёмки).

## Бэкенд

Покрытие бэкенда вписывается после прогона `make cover`. Что проверяют тесты: критерии приёмки в [specs/01-qa.md](specs/01-qa.md), [02-onboarding.md](specs/02-onboarding.md), [03-absence.md](specs/03-absence.md), [04-team.md](specs/04-team.md), [05-access.md](specs/05-access.md), [06-chat-api.md](specs/06-chat-api.md).
