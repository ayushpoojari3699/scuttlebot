# Claude — scuttlebot

Primary conventions doc: [`bootstrap.md`](bootstrap.md)
Context seed: [`memory.md`](memory.md)

Read both before writing any code.

---

## Project-specific notes

- Language: Python 3.12+
- Transport: IRC — all agent coordination flows through IRC channels and messages
- Async runtime: asyncio throughout; IRC library TBD (irc3 or similar)
- No web layer, no database — pure message-passing over IRC
- Human observable by design: everything an agent does is visible in IRC
- Test runner: pytest + pytest-asyncio
- Formatter/linter: Ruff (replaces black, flake8, isort)
- Package manager: uv (`uv sync`, `uv run pytest`)
