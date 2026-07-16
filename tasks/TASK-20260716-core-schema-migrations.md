# TASK-20260716-core-schema-migrations: Миграции core-схемы БД

**Type:** feature  
**Size:** L  
**Status:** todo
**GitHub Issue:** #3 — https://github.com/prog-time/tg-shop/issues/3

## Что делаем
Реализовать goose-миграции для всех основных таблиц из `docs/database.md` (кроме уже созданной `outbox`): categories, dictionaries, dictionary_values, field_definitions, products, product_images, media, product_facets, posts, delivery_methods, payment_methods, orders, order_items, payments, users, admin_users, roles. `api` — единственный владелец схемы.

## Пользователь получает
Как **разработчик бэкенда**, я могу **опираться на готовую схему БД**, чтобы **реализовывать доменные модули поверх реальных таблиц, а не заглушек**.

## Готово когда
- [ ] Given `docs/database.md`, when применяются миграции, then созданы все core-таблицы с типами, ключами, FK и индексами по ERD
- [ ] Given инвариант архивирования, when создаются `dictionary_values`/`field_definitions`, then есть `archived_at`/`is_deprecated`, а не физическое удаление
- [ ] Given фильтры каталога, when создаётся `products`, then есть `attributes JSONB` и таблица `product_facets` с индексами под фасетный поиск
- [ ] Given история заказов, when создаётся `order_items`, then поля-снимки (название, цена, характеристики) не ссылаются на живой каталог
- [ ] Given `docker compose up`, when стартует `api`, then все миграции применяются идемпотентно (goose Up/Down)

## Как делаем
- Одна миграция на логическую группу; типы по `docs/database.md` (bigint id, timestamptz, numeric, jsonb).
- FK с корректным `ON DELETE`; частичные/составные индексы под фасеты и выборки.
- Для каждой миграции — рабочий `-- +goose Down`.

## Файлы
- `backend/migrations/000NN_*.sql` — новые миграции
- `docs/database.md` — источник истины по схеме

## Не делаем в этой задаче
- Доменную валидацию `attributes` (в Go-модуле catalog), seed-данные (отдельная задача), ORM.

## Вопросы
- ❓ Подтвердить судейские вызовы из спеки: id `bigint`, деньги `numeric`/decimal, single-currency (валюты в модели нет).

## Связанные задачи
- Зависит от: walking skeleton
- Блокирует: auth, catalog, cart+media, orders, payment, admin

## Критерии приёмки (Definition of Done)
- [ ] Критерии из "Готово когда" выполнены и покрыты автотестами (применение миграций в CI/тесте)
- [ ] `go vet` и статический анализ (golangci-lint) зелёные
- [ ] Юнит- и интеграционные тесты (`go test ./...`) зелёные локально
- [ ] Документация (`docs/database.md`, при расхождении) синхронизирована
- [ ] Code review пройден
- [ ] Нет регрессий в смежных сценариях
