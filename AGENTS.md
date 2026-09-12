# AGENTS.md

## Product

Telegram-бот **Aura Radar**: по контексту группового чата оценивает ауру автора сообщения (сильная / слабая / нейтральная). Сам часто ставит реакции, редко пишет в чат, ведёт рейтинг очков **внутри чата**. `/aura` реплаем либо принудительно оценивает, либо объясняет уже стоящую реакцию.

Источник правды по продукту и нарезке работ: [`docs/tz.md`](docs/tz.md).  
Этот файл — как собирать, что не ломать, куда класть код.

Если код и ТЗ расходятся — побеждает ТЗ. Продукт меняется только правкой `docs/tz.md`.

## Stack (TZ §12)

- Go 1.23+
- `gopkg.in/telebot.v4`, long polling (нет HTTP-сервера, нет открытых портов)
- `database/sql` + SQLite (`modernc.org/sqlite`); Postgres если `DATABASE_URL` postgres
- LLM: `github.com/openai/openai-go`
- тесты: `go test ./...`, без сети и без секретов

Ориентир по стилю: `Ra1ze505/goNewsBot` (telebot.v4, openai-go, godotenv). Не копировать оттуда news/MTProto.

## Layout

```
src/                 # go run ./src
prompts/judge.txt
db/migrations/
docs/tz.md
```

Пока каркаса нет — первый срез S0 из ТЗ §17 его создаёт. Не плодить Python-пакет.

## Commands (когда появится go.mod)

| Task | Command |
|------|---------|
| Download deps | `go mod download` |
| Format | `gofmt -w .` |
| Vet | `go vet ./...` |
| Tests | `go test ./...` |
| Run | `go run ./src` с `.env` |
| Docker | `docker compose up --build` |

`go test ./...` **не** требует `BOT_TOKEN`, Postgres, LLM.

## Invariants (не нарушать)

1. Автотекст редкий; автореакции частые — раздельные пороги ТЗ §8.3. Нейтраль → ничего.
2. Нет реакции → нет авто-очков и нет события (нечего объяснять).
3. `/aura` на уже сохранённое событие не пересчитывает очки.
4. Очки и контекст не пересекают `chat_id`.
5. Дешёвые фильтры авторадара **до** вызова LLM.
6. Нет секретов в git. `.env` gitignore. Секреты в чат не просить — ТЗ §13.4.
7. Пользовательские строки на русском, код на английском.
8. Не реализовывать §2.2 ТЗ. Не возвращать Python-стек.

## Secrets (TZ §13.4)

Cloud Agent не получает токены из переписки.

Человек кладёт их в [Cloud Agents → Secrets](https://cursor.com/dashboard/cloud-agents) (браузер; с телефона не приложение Cursor):

| Имя | Тип |
|---|---|
| `BOT_TOKEN` | Runtime Secret |
| `OPENAI_API_KEY` | Runtime Secret |
| `OPENAI_MODEL` | Environment Variable или текст в чате (не секрет) |
| `OPENAI_BASE_URL` | то же |

После сохранения нужен **новый** агент: секреты инжектятся только на старте. Читать `os.Getenv`. Если переменных нет — сослаться на §13.4, не просить прислать значение.

Локально у человека: gitignored `.env`. Не коммитить.

## BotFather (человек, не агент)

Privacy mode **Disable**, иначе буфера контекста не будет. Это должно быть в README.

## Slices

Работай одним срезом S0…S8 из ТЗ §17. Не тащи следующий срез «заодно», если задача не просила.
