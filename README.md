# open.auramusic.pro — сайт ссылок «Поделиться» Belet Music

Когда в приложении Belet Music нажимают «Поделиться», получается ссылка вида
`https://open.auramusic.pro/s/Xk3pQ9aB7mLzT2wR`. Этот сайт:

- показывает по ссылке страницу с обложкой, названием и кнопками: на iPhone —
  App Store (и баннер Safari «Открыть»), на Android — «Belet Music-de açmak» и
  Google Play, на компьютере — QR-код и оба магазина;
- отдаёт мессенджерам (Telegram, WhatsApp, iMessage, Facebook…) карточку
  превью 1200×630 с обложкой и туркменским названием;
- раздаёт файлы связки, по которым iOS и Android открывают такие ссылки сразу
  в приложении (`apple-app-site-association`, `assetlinks.json`);
- считает открытия и нажатия без личных данных.

Это один бинарник `bm-open` на Go: шаблоны, стили, шрифты и картинки вшиты в
него. Данные о треке, альбоме, плейлисте, артисте или видео сайт берёт у
бэкенда по коду ссылки (`GET /api/v1/internal/deeplink?code=…` с токеном
сайта). Больше ни к чему, кроме CDN обложек, он не ходит.

## Адреса

| Путь | Что отдаёт |
|---|---|
| `/s/{code}` и `/s/{code}/` | страница ссылки; код — ровно 16 символов `[0-9A-Za-z]`, регистр важен |
| `/s/{code}?lang=tm\|ru` | ставит язык в cookie и возвращает на `/s/{code}` |
| `/i/{code}/cover.jpg`, `bg.jpg`, `og.jpg` | обложка 640 px, размытый фон, карточка для мессенджеров |
| `/qr/{code}.svg` | QR-код ссылки для компьютера |
| `/.well-known/apple-app-site-association`, `/apple-app-site-association`, `/.well-known/assetlinks.json` | файлы связки |
| `/` | 302 на `https://auramusic.pro/tm` или `/ru` |
| `/healthz` | `ok <версия>` |
| `POST /e` | маяк нажатий со страницы |
| всё остальное | страница «Tapylmady» с кодом 404 |

## Запуск у себя

Нужен Go той версии, что в `go.mod` (1.27.3). На Маке архив с
[go.dev/dl](https://go.dev/dl/) распакован в `~/sdk/go1.27.3`, в `~/.zprofile`:

```bash
export PATH="$HOME/sdk/go1.27.3/bin:$HOME/go/bin:$PATH"
```

Запуск:

```bash
make dev
```

Поднимаются два процесса: заглушка бэкенда на `127.0.0.1:3399` с фикстурами
из `testdata/mock/` и сам сайт на `127.0.0.1:3310` в режиме `MODE=mock`.
Токен и `.env` для этого не нужны. Открывайте, например:

- http://127.0.0.1:3310/s/TrackCyrillic001 — трек;
- http://127.0.0.1:3310/s/PlaylistTmRu0001 — плейлист с разными названиями на tm и ru;
- http://127.0.0.1:3310/s/ArtistNavai00001 — артист (круглая обложка);
- http://127.0.0.1:3310/s/VideoClip0000001 — видео (16:9);
- http://127.0.0.1:3310/s/NotFound00000404 — «не найдено»;
- http://127.0.0.1:3310/s/ServerError00500 — сбой бэкенда;
- http://127.0.0.1:3310/s/LongTitle0000001 — очень длинные название и артист.

Имя файла фикстуры — это код. Ключи `_status` и `_delay` в фикстуре задают код
ответа и задержку заглушки. Платформу удобно менять подменой User-Agent в
инструментах разработчика браузера (режим устройства iPhone или Android).

## Проверка

```bash
make lint   # gofmt, go vet, staticcheck (если установлен)
make test   # все тесты с -race
```

Тесты покрывают: проверку кода, язык и cookie, платформы и ботов превью,
мета-теги по всем типам, экранирование текста из бэкенда, правила путей,
три состояния страницы, кэширование картинок, файлы связки, статистику,
защиту скачивания обложек (только разрешённые хосты, https, без чужих
редиректов, ограничения размера), читаемость QR, отсутствие токена сайта в
ответах и логах, один запрос к бэкенду на десятки одновременных открытий.

Карточки для проверки глазами:

```bash
IMAGING_OUT=1 go test -run WriteCards ./internal/imaging/
open testdata/out
```

## Настройки (`.env`)

Все переменные с пояснениями — в `.env.example`. Окружение главнее файла.

| Переменная | По умолчанию | Зачем |
|---|---|---|
| `MODE` | `prod` | `prod` — настоящий бэкенд; `mock` — заглушка для разработки |
| `LISTEN_ADDR` | `127.0.0.1:3310` | только localhost, снаружи — через nginx |
| `PUBLIC_ORIGIN` | `https://open.auramusic.pro` | из него строятся все абсолютные ссылки (og, QR, баннер); заголовок `Host` не используется |
| `MAIN_SITE_URL` | `https://auramusic.pro` | куда ведёт `/` |
| `BACKEND_BASE_URL` | — | корень API бэкенда без `/api/v1`, разный для dev и прод |
| `SITE_TOKEN` | — | токен сайта; секрет: не коммитить, не писать в чат |
| `IMAGE_HOSTS` | `*.ma.st.com.tm` | с каких хостов можно скачивать обложки; `*.домен` — любой поддомен на порту 443 (обложки треков, альбомов, артистов и плейлистов лежат на `storage2.`, видео — на `cache.`) |
| `PREVIEW_TTL`, `NOT_FOUND_TTL`, `FAIL_TTL`, `STALE_MAX` | `1h`, `10m`, `15s`, `24h` | кэш ответов бэкенда; при сбое до суток отдаётся последнее известное превью |
| `BACKEND_TIMEOUT`, `BACKEND_MAX_INFLIGHT` | `2500ms`, `16` | мессенджеры ждут превью 3–5 с |
| `IMAGE_CACHE_MB` | `128` | память под готовые картинки |
| `DATA_DIR` | `./data` | где лежит `stats.jsonl` |
| `TRUST_PROXY` | `true` | брать адрес посетителя из `X-Real-IP` (только для лимита маяков, нигде не хранится) |
| `LOG_LEVEL` | `info` | логи в JSON, без IP, User-Agent и токена |

## Выкладка

Выкладывает владелец. Сервер тот же, где `auramusic.pro` (`95.85.127.85`, `/opt/bm-web`, nginx + pm2 под root). Первая выкладка — 28.10.2026.

### Схема на сервере

```
/opt/bm-open/                         git clone этого репо
/opt/bm-open/.env                     chmod 600, владелец — пользователь pm2
/opt/bm-open/releases/<UTC>/bm-open   собранные бинарники, хранятся последние 5
/opt/bm-open/live -> releases/<UTC>   текущий
/opt/bm-open/data/stats.jsonl         статистика
```

### Шаги по порядку

1. DevOps: DNS и сертификат (текст ниже). Бэкенд (Hakim): токен сайта и allowlist IP (текст ниже).
2. На сервере:
   ```bash
   git clone https://gitlab.st.com.tm/eshret/open.auramusic.pro.git /opt/bm-open   # спросит логин и пароль GitLab
   cd /opt/bm-open
   cp .env.example .env && chmod 600 .env   # вписать BACKEND_BASE_URL и SITE_TOKEN
   mkdir -p data
   ```
   Код на сервер идёт с GitLab. GitHub — его копия: с Мака `git push origin main` обновляет обе.
3. Поставить Go через Nexus (ниже) или взять готовый бинарник с Мака.
4. Первая выкладка: `deploy/release.sh`. В первый раз скрипт сам сделает
   `pm2 start deploy/ecosystem.config.cjs && pm2 save`. Чтобы pm2 поднимался
   после перезагрузки сервера, один раз выполнить `pm2 startup` (если для
   `bm-web` это уже сделано, повторять не нужно).
5. nginx: положить `deploy/nginx/open.auramusic.pro.conf` в
   `/etc/nginx/sites-available/` (и ссылку в `sites-enabled/`) или в
   `/etc/nginx/conf.d/`, создать `sudo mkdir -p /var/cache/nginx/bm-open`,
   поправить пути сертификата, затем `sudo nginx -t && sudo systemctl reload nginx`.
6. Проверка после выкладки (чек-лист ниже).

### Go на сервере

Сборка идёт на сервере: `deploy/release.sh` делает `git pull` и `go build`.

С сервера закрыты `proxy.golang.org` и `sum.golang.org`: вместо них отвечает сертификат `cklk.st.com.tm`, истёкший 27.07.2026. Поэтому Go и библиотеки берутся через Nexus `https://nexus.telecom.tm/repository/go-proxy`. Системный Go 1.21.3 не трогаем: другие проекты на сервере могут от него зависеть. Go 1.27.3 лежит рядом, в `/usr/local/go1.27.3`. По ссылке `/usr/local/bin/go1.27.3` старый `go` находит его сам: этого требует строка `go 1.27.3` в `go.mod`. Сам скачать новый Go старый не может, потому что проверяет его только через `sum.golang.org`.

Так сделано 28.10.2026, под root:

```bash
cd /tmp && curl -fsSL -o go1.27.3.zip https://nexus.telecom.tm/repository/go-proxy/golang.org/toolchain/@v/v0.0.1-go1.27.3.linux-amd64.zip
# сумма того же файла с proxy.golang.org, снята с другой машины
echo "af4c06478303fdd1bfb335fab86c7ccb9b12e529044a946a16a1c4bcc779f8d7  go1.27.3.zip" | sha256sum -c
python3 -m zipfile -e go1.27.3.zip /tmp/gotc
mv /tmp/gotc/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64 /usr/local/go1.27.3
chmod +x /usr/local/go1.27.3/bin/* /usr/local/go1.27.3/pkg/tool/linux_amd64/*
ln -sfn /usr/local/go1.27.3/bin/go /usr/local/bin/go1.27.3
rm -rf /tmp/gotc /tmp/go1.27.3.zip
cd /opt/bm-open && go version      # go1.27.3
go env -w GOPROXY=https://nexus.telecom.tm/repository/go-proxy   # только после go version: до этого go в /opt/bm-open не запустится
go mod download
```

Новая версия Go в `go.mod` ставится так же, со своим номером и суммой. Сумму нужно снять с `proxy.golang.org` на машине, откуда он открывается.

**Без Go на сервере:** собрать на Маке и привезти бинарник.

```bash
make build-linux               # dist/bm-open для amd64; для arm64: make build-linux ARCH=arm64
scp dist/bm-open server:/tmp/bm-open
ssh server 'cd /opt/bm-open && git pull --ff-only && deploy/release.sh install /tmp/bm-open'
```

### release.sh

```bash
deploy/release.sh                  # git pull, сборка, переключение, перезапуск, проверка
deploy/release.sh install <файл>   # то же с готовым бинарником
deploy/release.sh rollback         # вернуть предыдущий релиз
```

После переключения скрипт ждёт до 15 с, пока `/healthz` ответит версией нового
релиза. Не ответил — `live` возвращается на прошлый релиз, pm2 перезапускается,
скрипт выходит с ошибкой. Хранятся последние 5 релизов, текущий не удаляется.

### pm2 и nginx

- `deploy/ecosystem.config.cjs`: процесс `bm-open`, запуск `live/bm-open serve`
  из `/opt/bm-open`, перезапуск при 300 МБ, 10 с на корректную остановку
  (сайт дописывает статистику текущего часа).
- `deploy/nginx/open.auramusic.pro.conf`: https с HSTS (без `includeSubDomains`),
  gzip, лимит 10 запросов в секунду с адреса (ответ 429), кэш картинок и QR на
  диске до 2 ГБ. Страницы nginx не кэширует: они зависят от устройства и языка.
  Файлы связки идут без лимита и без редиректов.
- IP посетителей сайт не пишет. Журнал nginx — настройка сервера. Для `/i/`,
  `/qr/` и `/static/` в конфиге есть закомментированный `access_log off`:
  в этих строках нечего смотреть.

## Статистика

Раз в час и при остановке процесса сайт дописывает строку в
`/opt/bm-open/data/stats.jsonl`: открытия по типу, платформе и языку,
«не найдено», сбои бэкенда, просмотры ботами превью, нажатия на кнопки,
попадания в кэш. IP, User-Agent, cookies и коды ссылок не хранятся. Файл растёт
примерно на 1 КБ в час (около 9 МБ в год), ротация не нужна.

```bash
cd /opt/bm-open && live/bm-open stats -days 7
```

Печатает таблицу по дням: открытия по типам, нажатия App Store, Google Play и
«Открыть», доля «не найдено» и сбоев, боты.

## Проверка после выкладки

1. Снаружи:
   ```bash
   curl -sI https://open.auramusic.pro/.well-known/apple-app-site-association   # 200, application/json, без Location
   curl -sI https://open.auramusic.pro/.well-known/assetlinks.json              # 200, application/json
   curl -s  https://app-site-association.cdn-apple.com/a/v1/open.auramusic.pro  # копия Apple (может обновиться через часы)
   curl -s "https://digitalassetlinks.googleapis.com/v1/statements:list?source.web.site=https://open.auramusic.pro&relation=delegate_permission/common.handle_all_urls"
   curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" https://open.auramusic.pro/   # 302 → https://auramusic.pro/tm
   curl -sI https://open.auramusic.pro/s/<настоящий_код>                         # 200
   ```
   Настоящий код — из приложения: «Поделиться» → «Скопировать».
2. Telegram: отправить ссылку себе в «Избранное» — карточка с обложкой и
   туркменским названием. Если показалась старая или пустая, обновить через
   бота `@WebpageBot`.
3. WhatsApp, iMessage, Instagram Direct — карточка есть.
4. Facebook Sharing Debugger (`developers.facebook.com/tools/debug`) — без ошибок `og:`.
5. iPhone без приложения: Safari → страница, баннер сверху ведёт в App Store, бейдж работает.
6. iPhone с приложением (DEV-сборка с `applinks:open.auramusic.pro` или будущий релиз):
   - сначала переустановить приложение (iOS перечитает AASA);
   - ссылка из Telegram и из «Заметок» открывает приложение сразу;
   - в Safari долгое нажатие на ссылку → «Открыть в Belet Music».

   Диагностика: «Настройки → Для разработчиков → Universal Links → Диагностика».
7. Android (после задачи 3.3 в Android-приложении):
   ```bash
   adb shell pm verify-app-links --re-verify com.sanlytilsimat.musicapp.dev.debug
   adb shell pm get-app-links com.sanlytilsimat.musicapp.dev.debug          # open.auramusic.pro: verified
   adb shell am start -a android.intent.action.VIEW -d "https://open.auramusic.pro/s/<код>"
   ```
   Кнопка «Belet Music-de açmak» в Chrome открывает приложение (у боевого приложения).
8. Компьютер: страница с QR. Скан камерой iPhone открывает приложение на этом элементе.
9. Статистика: через час `live/bm-open stats -days 1` показывает открытия.

## Что пока работает не до конца (не в этом репо)

- **Android, кнопка «Belet Music-de açmak».** Ведёт в приложение только когда
  Android-приложение научится ссылкам `/s/` (задача 3.3: `intent-filter` на
  `https://open.auramusic.pro/s/`). До этого кнопка открывает Google Play, там у
  установленного приложения есть «Открыть», и оно открывается на главной.
  Во встроенных браузерах (Instagram, Facebook) `intent://` может не сработать,
  поэтому бейдж Google Play остаётся всегда.
- **iPhone, баннер Safari «Открыть».** Передаёт приложению адрес ссылки
  (`app-argument`). Приём этого адреса (`onOpenURL` → тот же разбор, что у
  Universal Links) делается отдельно в репо `ios`. До этого «Открыть» открывает
  приложение на главной. Своей кнопки «Открыть» на iPhone нет: ссылка на тот же
  домен приложение не откроет, а своей схемы у приложения нет.

## Что попросить у DevOps и у бэкенда

### DevOps

> Нужно для сайта ссылок «Поделиться» `open.auramusic.pro`. Он работает на сервере сайта `auramusic.pro` (`/opt/bm-web`), IP: `<IP сервера>`.
> 1. **DNS:** A-запись `open.auramusic.pro` → `<IP>`. AAAA — только если у сервера есть IPv6 и nginx слушает его. На 28.10.2026 сделано: домен ведёт на `95.85.127.85`. Но и он, и `auramusic.pro` отдают по https сертификат `cklk.st.com.tm`, истёкший 27.07.2026.
> 2. **Сертификат** Let's Encrypt на `open.auramusic.pro` с автопродлением. Если на `auramusic.pro` ещё стоит тот же чужой просроченный — его тоже.
> 3. **Доступ из-за границы** к порту 443 этого сервера. Его проверяют Apple (через свой CDN, иначе ссылки не открывают приложение на iPhone) и Google (App Links на Android). Ещё оттуда приходят Telegram, WhatsApp и Facebook за превью.
> 4. **Исходящий доступ** с этого сервера к API бэкенда (dev и прод) и к CDN `storage2.ma.st.com.tm` и `cache.ma.st.com.tm`.
> 5. Для справки: доступен ли CDN `*.ma.st.com.tm` из-за границы. Сайт всё равно отдаёт картинки сам.

### Бэкенд (Hakim)

Ответ пришёл 28.10.2026, формат по типам — [`docs/deeplink-api.md`](docs/deeplink-api.md).

- `BACKEND_BASE_URL` тот же, что у приложения: dev — `devUrl`, прод — `baseUrl` из `android/config/secure-env.local.properties` (без `/api/v1`). 28.10.2026 `internal/deeplink` на проде уже работал, сайт смотрит в прод. Dev (`pn2`) отвечает на тот же токен `401 «Неверный токен»`.
- Токен один для dev и прод, бэкенд передаёт его лично. Срока жизни нет; при смене предупредят заранее: поменять `SITE_TOKEN` в `.env` и перезапустить сайт.
- IP `95.85.127.85` добавляют в allowlist на dev и прод. IP проверяется раньше токена: чужой IP — `403`, неверный токен — `401`. Лимита запросов на токен нет.
- При старте сайт спрашивает код `0000000000000000` и пишет в лог `backend reachable` (бэкенд ответил `404`). Если вместо этого `403`, отправить бэкенду время этой строки из `pm2 logs bm-open`: по нему они найдут, с какого адреса пришёл запрос.
- Строка `cover fetch failed` с `host not allowed` значит, что бэкенд прислал обложку с хоста не из `IMAGE_HOSTS`. Хост из этой строки отправить бэкенду: это старая ссылка, они её исправят.

Ещё ждём: настоящие коды для проверки и ответ, почему dev не принимает токен (на прод это не влияет).

## Устройство кода

```
cmd/bm-open/          входная точка: serve, mock, stats, -version
internal/config/      настройки из окружения и .env
internal/sharecode/   проверка кода ссылки
internal/lang/        язык страницы (tm/ru), разбор Accept-Language
internal/backend/     клиент бэкенда: без повторов и редиректов, ограничение одновременных запросов
internal/preview/     кэш превью: один запрос на код, устаревший ответ при сбое
internal/lru/         LRU с ограничением по размеру
internal/imaging/     обложка, фон, карточка 1200×630, QR; скачивание обложек
internal/stats/       счётчики, маяк /e, отчёт
internal/site/        HTTP: страница, картинки, статика, файлы связки
internal/mockbackend/ заглушка бэкенда для разработки и тестов
assets/               всё, что вшито в бинарник
deploy/               pm2, nginx, release.sh
testdata/mock/        фикстуры заглушки
```
