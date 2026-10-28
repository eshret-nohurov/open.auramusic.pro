---
name: server
description: Сервер сайта 95.85.127.85: root и pm2, клон с GitLab, Go 1.27.3 рядом с системным через Nexus, закрытые адреса, сертификат
metadata:
  type: reference
---

Сервер `clients`, 95.85.127.85 (`auramusic.pro`, `bm-web`, `bm-payment` и другие). Владелец заходит `ssh eshret@st.com.tm` и работает под root. pm2 тоже под root.

- Сайт в `/opt/bm-open`, клон с GitLab по https: при `git pull` (и в `deploy/release.sh`) спрашивает логин и пароль. `.env` вставлен через буфер обмена из локального `.env.server`. pm2-процесс `bm-open` поднят 28.10.2026.
- С сервера закрыты `proxy.golang.org` и `sum.golang.org`: отвечает сертификат `cklk.st.com.tm`, истёкший 2026-07-27. Обходить проверку TLS нельзя.
- Go: системный 1.21.3 не трогаем. Go 1.27.3 лежит в `/usr/local/go1.27.3`, ссылка `/usr/local/bin/go1.27.3`. Старый go сам переключается на него по `go.mod`. Скачан через Nexus `https://nexus.telecom.tm/repository/go-proxy`, sha256 сверен с proxy.golang.org. `go env -w GOPROXY=` Nexus для root.
- Go 1.21.3 при скачивании toolchain всегда сверяет его через sum.golang.org (GONOSUMDB не помогает) и не читает `go env -w`. Поэтому toolchain ставится вручную, как описано в README «Go на сервере».
- Домен, сертификат и nginx подключает DevOps. 28.10 DNS уже ведёт на этот сервер, а nginx на 443 отдаёт просроченный `cklk.st.com.tm`, в том числе для `auramusic.pro`.

**How to apply:** новая версия сайта — `cd /opt/bm-open && deploy/release.sh`. Новая версия Go в `go.mod` — заново по README. См. [[project-status]], [[backend-api]].
