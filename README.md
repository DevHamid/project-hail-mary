# Project Hail Mary 🚀

A privacy-first, FOSS, Markdown-native notes app inspired by UseMemos.
Single static binary. SQLite under the hood, Markdown on screen.

## Stack

- **Backend:** Go — single static binary, no CGO, no runtime deps.
- **Frontend:** Single page, Tailwind CSS + Marked.js (CDN). No build step.
- **Storage:** SQLite via `modernc.org/sqlite` (pure Go).
- **Port:** 4815

## Features

- Quick capture (Ctrl/Cmd optional, one textbox)
- Markdown rendering (headings, bold, lists, code, links)
- `#tags` — typed inline, extracted, filterable in sidebar
- Tag rename (renames in one click, notes update live)
- Edit & delete notes
- Auto-persists to SQLite — restart-safe

## How to run

### Docker (recommended)

```bash
git clone https://github.com/DevHamid/project-hail-mary
cd project-hail-mary
docker compose up --build -d
```

Open http://localhost:4815

Data lives in `./data/notes.db`.

### Plain Go

```bash
go run main.go
```

DB file: `notes.db` in working dir. Set `DB_PATH` and `PORT` env to override.

## API

| Method | Path | Body | Purpose |
|---|---|---|---|
| POST | `/api/notes` | `{"content":"..."}` | Create note |
| GET | `/api/notes` | — | List (optional `?tag=foo`) |
| PUT | `/api/notes` | `{"id":1,"content":"..."}` | Update |
| DELETE | `/api/notes?id=1` | — | Delete |
| GET | `/api/tags` | — | Tag counts `{tag: n}` |
| PUT | `/api/tags` | `{"old":"a","new":"b"}` | Rename tag |

## Roadmap

- [ ] Simple auth (login/session)
- [ ] Full-text search (SQLite FTS5)
- [ ] PWA manifest + favicon
- [ ] Note move / reorder
- [ ] Export to `.md`

## License

MIT — do whatever you want, attribution appreciated.
