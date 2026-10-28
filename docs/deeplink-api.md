# Внутренний API: данные для превью диплинка

Для нашего второго бэкенда, который обслуживает диплинки (сайт
`open.auramusic.pro`), — не для мобильного приложения. По коду из ссылки
«Поделиться» или по типу и id отдаёт то, что нужно для превью ссылки на трек,
альбом, артиста, плейлист, видео или видео-плейлист: название, исполнителя и
обложки (например, для `og:title` и `og:image`).

## Запрос

По коду из ссылки `https://open.auramusic.pro/s/{code}`:

```
GET /api/v1/internal/deeplink?code=Xk3pQ9aB7mLzT2wR
Authorization: Bearer <токен>
```

Или по типу и id:

```
GET /api/v1/internal/deeplink?type=track&id=123
Authorization: Bearer <токен>
```

| Параметр | Значение |
|---|---|
| `code` | код из ссылки: 16 символов `0-9`, `A-Z`, `a-z`, регистр важен |
| `type` | `track`, `album`, `artist`, `playlist`, `video` или `video_playlist` — строчными |
| `id` | id записи, целое число больше нуля |

Передаётся либо `code`, либо `type` и `id` вместе. JWT пользователя и
заголовок `music-app-unique-user-key` не нужны.

```bash
curl -H "Authorization: Bearer $TOKEN" "https://<api-host>/api/v1/internal/deeplink?code=Xk3pQ9aB7mLzT2wR"
```

Сайт может отсекать мусор сам, до запроса: код — ровно 16 символов из
`0-9A-Za-z`. Всё остальное бэкенд всё равно не найдёт.

## Ответ 200

В каждом ответе есть `type` — тип элемента (`track`, `album`, `artist`,
`playlist`, `video`, `video_playlist`): по коду он заранее не известен.
Остальные поля зависят от типа.

Трек:

```json
{
  "type": "track",
  "title": "Ýüregim",
  "artist_name": "Artist A, Artist B",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/..."
}
```

Альбом:

```json
{
  "type": "album",
  "title": "Album",
  "artist_name": "Artist A",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/..."
}
```

Артист:

```json
{
  "type": "artist",
  "title": "Artist A",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/..."
}
```

Плейлист:

```json
{
  "type": "playlist",
  "title_tm": "Täze aýdymlar",
  "title_ru": "Новые песни",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/...",
  "thumb_url_tm": "https://cache.ma.st.com.tm/...",
  "artwork_url_tm": "https://cache.ma.st.com.tm/..."
}
```

Видео:

```json
{
  "type": "video",
  "title": "Klip",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/..."
}
```

Видео-плейлист — те же поля, что у плейлиста:

```json
{
  "type": "video_playlist",
  "title_tm": "Täze klipler",
  "title_ru": "Новые клипы",
  "thumb_url": "https://cache.ma.st.com.tm/...",
  "artwork_url": "https://cache.ma.st.com.tm/...",
  "thumb_url_tm": "https://cache.ma.st.com.tm/...",
  "artwork_url_tm": "https://cache.ma.st.com.tm/..."
}
```

- Язык параметром не передаётся: у плейлиста и видео-плейлиста приходят оба. `thumb_url` и
  `artwork_url` — русская обложка, `thumb_url_tm` и `artwork_url_tm` —
  туркменская. У старых плейлистов туркменской обложки может не быть — тогда
  берите русскую.
- `thumb_url` — уменьшенная картинка, `artwork_url` — полноразмерная.
- У видео исполнителя нет — только название и обложки.
- Видео и видео-плейлисты отдаются для любой страны: в приложении вне
  Туркменистана видны только национальные, но превью смотрят серверы
  мессенджеров, а не слушатель. Откроется ли видео — решает приложение.
- `artist_name` — все исполнители через запятую, в том же порядке, что в приложении.
- Обложки те же, что видит пользователь в приложении: у трека без своей
  обложки — обложка артиста или альбома, у альбома без своей — обложка артиста.
- Ссылки — на CDN, как в приложении. Нет картинки — пустая строка, `null` не бывает.

## Ошибки

Тело: `{"error": "Not Found", "message": "..."}`.

| Код | Когда |
|---|---|
| `400` | `type` не из списка, `id` не положительное число, или вместе с `code` передали `type`/`id` |
| `401` | нет токена или он не совпал |
| `403` | запрос пришёл не с адреса из `allowed_ips` |
| `404` | записи нет или она скрыта: трек удалён, трек, альбом, артист, плейлист, видео или видео-плейлист выключен в админке, трек или видео помечены как explicit. По `code` — ещё и когда кода нет, он отозван или это не код: ответ всегда один и тот же |
| `500` | сбой базы — можно повторить позже |

## Настройка

Go-сервис, `configs/config.toml` (после правки — рестарт):

```toml
[internal_api]
token = "..."                    # openssl rand -hex 32; пустой — API закрыт
allowed_ips = ["203.0.113.10"]   # IP или подсети; пустой список — только токен
```

Второй бэкенд — тот же токен в своём `.env`.

Если сервис не пускает свой сервер с ответом `403`, в логе Go будет строка
`remote=... x-real-ip=... x-forwarded-for=...` — по ней видно, какой адрес
вписать в `allowed_ips`. Адрес берётся из заголовков, которые ставит nginx;
подставить разрешённый адрес своим заголовком клиент не сможет.

Ответы идут с `Cache-Control: no-store`. Если nginx настроен этот заголовок
игнорировать (`proxy_ignore_headers`), путь `/api/v1/internal/` нужно исключить
из кэша: закэшированный ответ отдался бы и без токена.
