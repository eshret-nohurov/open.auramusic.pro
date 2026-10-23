---
name: backend-api
description: Внутренний API бэкенда для сайта (internal/deeplink по коду), чего не хватает и что спросить у Hakim
metadata:
  type: reference
---

Источник: `~/Downloads/share-api.md`, раздел 6 (ответ бэкенда на набросок мобильного разработчика).

- `GET {BACKEND_BASE_URL}/api/v1/internal/deeplink?code={code}`, `Authorization: Bearer <токен сайта>`. Без JWT. Бэкенд пускает только IP сервера сайта.
- Ответ: `type`, `title`, `artist_name`, `thumb_url`, `artwork_url`; у плейлистов и видео-плейлистов ещё `title_ru`/`title_tm` и `…_tm` обложки. Пустая картинка — `""`. Ответы `no-store`.
- 404 одинаковый для неизвестного, отозванного и скрытого кода.
- Старый вариант `?type=…&id=…` сайту запрещён.
- Обложки — публичные ссылки на CDN `cache.ma.st.com.tm`; сайт отдаёт их со своего адреса.

Чего нет на 23.10.2026: `docs/deeplink-api.md` (полный формат по типам — в `share-api.md` написано «прикладываем», но файла нет), токена, адресов dev/prod. Модель ответа сделана терпимой: незнакомые поля и `null` не ломают разбор.

**How to apply:** когда придёт `docs/deeplink-api.md`, сверить `internal/backend` (поля и порядок выбора обложки). См. [[owner-decisions]].
