# open.auramusic.pro

Сайт ссылок «Поделиться» Belet Music: `https://open.auramusic.pro/s/{16 символов}`.

1. Отдаёт файлы связки (`apple-app-site-association`, `assetlinks.json`): если приложение стоит, iOS и Android открывают ссылку сразу в нём.
2. Отдаёт мессенджерам `og:`-теги и карточку 1200×630.
3. Показывает страницу тем, у кого приложения нет: обложка, название, кнопки магазинов, на компьютере QR.

Сайт музыку не играет и о пользователях ничего не знает. По коду он спрашивает бэкенд `GET /api/v1/internal/deeplink?code=…` (токен сайта, только с сервера) и показывает витрину.

Решения владельца — `.claude/memory/owner-decisions.md`. Задание целиком: `~/Desktop/сайт-open-auramusic-промт.md`.

## Команды

```bash
export PATH="$HOME/sdk/go1.27.3/bin:$HOME/go/bin:$PATH"   # Go стоит в ~/sdk, не через brew
make lint          # gofmt -l, go vet (+ staticcheck, если установлен)
make test          # go test -race -count=1 ./...
make dev           # заглушка бэкенда :3399 + сайт :3310, MODE=mock
make build-linux   # dist/bm-open для сервера
```

## Структура

```
cmd/bm-open/          входная точка: serve (по умолчанию), mock, stats -days N, -version
internal/config/      env и .env (окружение главнее файла), проверка обязательных полей
internal/sharecode/   проверка кода: ровно 16 символов [0-9A-Za-z]
internal/lang/        язык страницы: tm/ru, разбор Accept-Language
internal/backend/     клиент internal/deeplink: без повторов и редиректов, семафор, терпимый разбор ответа
internal/preview/     кэш превью: TTL по исходу, один запрос на код (singleflight), устаревшее при сбое
internal/lru/         LRU с ограничением по «весу» (записи или байты)
internal/mockbackend/ заглушка бэкенда: фикстуры testdata/mock/{code}.json, сгенерированные обложки
internal/imaging/     обложка, фон, карточка 1200×630, QR; скачивание обложек только с IMAGE_HOSTS
internal/stats/       счётчики по часам в DATA_DIR/stats.jsonl, маяк POST /e, отчёт для команды stats
internal/site/        HTTP: страница /s/{code} (page.go), картинки и QR (images.go), тексты tm/ru (texts.go),
                      платформа и боты по UA (agent.go), статика с хешем в имени, файлы связки
assets/               всё, что вшито в бинарник (embed.go): static/, templates/, wellknown/, fonts-ttf/, raster/
testdata/mock/        фикстуры заглушки: имя файла = код; ключи "_status" и "_delay" управляют ответом
testdata/out/         карточки для проверки глазами: IMAGING_OUT=1 go test -run WriteCards ./internal/imaging/
deploy/               pm2, nginx, release.sh
```

## Правила

- Коммиты на русском, первая строка — суть, без `type(scope):`. Коммитить и пушить в `origin main` самому после зелёных `make lint` и `make test`.
- **Никаких упоминаний ИИ** в коммитах, коде, комментариях и README. Без `Co-Authored-By` и «Generated with».
- Комментарии в коде на английском и объясняют «почему», а не пересказывают код. README и тексты для людей — на русском, тексты страницы — на туркменском и русском.
- Токен сайта никогда не попадает в git, логи, HTML, тесты и чат. В репо только `.env.example`.
- Адрес сайта берётся из `PUBLIC_ORIGIN`, никогда из заголовка `Host`.
- Из запроса берётся только код (и `lang` из двух значений). Код проверяется до любой работы; плохой код — 404 без запроса к бэкенду.
- Чужие репо (`ios/`, `android/`, `android_tv/`, `web/`, `landing/`, бэкенд) не трогать. Ресурсы из них можно копировать.
- Выкладку делает владелец. По SSH не ходить.
- Память проекта — `.claude/memory/` (`MEMORY.md` + один факт на файл), коммитится вместе с работой.
