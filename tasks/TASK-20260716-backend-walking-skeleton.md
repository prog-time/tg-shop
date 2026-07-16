# TASK-20260716-backend-walking-skeleton: Walking skeleton бэкенда

**Type:** feature  
**Size:** M  
**Status:** done
**GitHub Issue:** #2 — https://github.com/prog-time/tg-shop/issues/2

## Что делаем
Собрать «скелет, который ходит»: один Go-модуль, три entrypoint-а (`api`/`bot`/`worker`), конфиг из ENV, структурный лог, health/readiness/metrics, подключения к Postgres (с раннером миграций), Redis, RabbitMQ, и `Dockerfile` с таргетами под compose. Цель — доказать весь инфраструктурный плюмбинг до доменной логики.

## Пользователь получает
Как **разработчик**, я могу **поднять весь стек одной командой и получить работающие сервисы с health-эндпоинтами**, чтобы **строить доменные модули на готовом каркасе, а не на пустом месте**.

## Готово когда
- [x] Given `docker compose up`, when собираются образы `api`/`bot`/`worker`, then сборка проходит (таргеты Dockerfile существуют)
- [x] Given старт `api`, when он поднимается, then применяются goose-миграции и он слушает `:8080`
- [x] Given работающие сервисы, when дёргается `/readyz`, then api→postgres+redis, worker→postgres+rabbitmq отвечают `ok`

## Как делаем
- `internal/{config,logging,httpx,postgres,redis,rabbit}` + `cmd/{api,bot,worker}` + `migrations/`.
- Библиотеки по ADR-005 (chi, pgx, goose, go-redis, amqp091, prometheus).
- Первая миграция `00001_outbox` по `docs/database.md`.

## Файлы
- `backend/**` — каркас модуля, три entrypoint-а, Dockerfile, миграции
- `docs/adr/005-go-library-stack.md` — решение по стеку

## Не делаем в этой задаче
- Доменную логику, кодогенерацию из OpenAPI, автотесты — приходят с модулями.

## Вопросы
- ✅ Реализовано и проверено сборкой + запуском (см. коммиты Фазы 0).

## Связанные задачи
- Зависит от: —
- Блокирует: миграции core-схемы, oapi-codegen + роутинг

## Критерии приёмки (Definition of Done)
- [x] Критерии из "Готово когда" выполнены (сборка, запуск, миграции, health)
- [x] `go vet` и `gofmt` зелёные
- [ ] Юнит-тесты — отложены до появления доменной логики
- [x] Документация (ADR-005, backend/README, CLAUDE.md) обновлена
- [ ] Code review — при заведении в репозиторий
- [x] Нет регрессий (greenfield)
