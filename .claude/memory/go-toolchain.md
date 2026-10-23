---
name: go-toolchain
description: Go на Маке стоит из официального архива в ~/sdk/go1.27.3, а не через brew; PATH в ~/.zprofile
metadata:
  type: project
---

`brew install go` 23.10.2026 не прошёл: Homebrew не смог скачать свой Ruby с ghcr.io (обрыв загрузки). Поэтому Go 1.27.3 поставлен из официального архива go.dev (sha256 сверен) в `~/sdk/go1.27.3`. В `~/.zprofile` добавлен `PATH="$HOME/sdk/go1.27.3/bin:$HOME/go/bin:$PATH"`.

`librsvg` тоже не поставился. PNG знака логотипа для карточки рендерится через headless Chrome (команда в README).

**How to apply:** в неинтерактивном shell Go не виден без `export PATH="$HOME/sdk/go1.27.3/bin:$HOME/go/bin:$PATH"`. Версия в `go.mod` = установленная. См. [[owner-decisions]].
