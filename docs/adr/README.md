# Architecture Decision Records

Каждый файл — одно принятое архитектурное решение: контекст, варианты, выбор и его цена.

ADR не переписываются задним числом. Если решение отменено — заводится новый ADR со статусом `Superseded by ADR-XXX`, а старый остаётся как история.

| # | Решение | Статус |
|---|---|---|
| [001](001-no-headless-cms.md) | Отказ от готовой CMS в пользу собственной админки | Accepted |
| [002](002-dynamic-fields-and-dictionaries.md) | Гибкость каталога: реестр полей, справочники, JSONB | Accepted |
| [003](003-modular-monolith.md) | Модульный монолит на Go вместо микросервисов | Accepted |
| [004](004-rabbitmq-and-outbox.md) | RabbitMQ как очередь задач + transactional outbox | Accepted |
| [005](005-go-library-stack.md) | Набор Go-библиотек и контракт-first через oapi-codegen | Accepted |
