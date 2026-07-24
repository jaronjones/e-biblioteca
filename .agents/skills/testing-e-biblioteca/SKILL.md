---
name: testing-e-biblioteca
description: Boot e-biblioteca locally with docker compose, seed sample books, and verify the readers, progress tracking and CSRF end-to-end. Use when testing or reviewing changes to readers, scanning, progress, auth/CSRF or OPDS.
---

# Testing e-biblioteca end-to-end

Server-rendered Go app (chi + templ + htmx + scs sessions on Postgres). Nearly all interesting
behaviour is browser-side glue (readers, progress POSTs, CSRF), so **shell checks are not enough** —
issues here have repeatedly been invisible to `go build`/`go vet` and only visible in a real browser.

## Devin Secrets Needed

None. Everything runs locally against a throwaway Postgres container.

## 1. Boot the app

```bash
cd ~/repos/e-biblioteca
cp -n .env.example .env          # only POSTGRES_PASSWORD matters; defaults are fine locally
docker compose up -d --build     # Postgres 16 + app on http://localhost:8080
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/healthz   # expect 200
docker compose logs -f app       # keep this handy: every request is logged with its status code
```

Static checks worth running alongside (Go lives at `/usr/local/go/bin`, templ at `~/go/bin`):

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
go build ./... && go vet ./... && gofmt -l .
```

Do **not** run `templ generate` unless your local templ matches the version in `go.mod` — an older
generator rewrites every `*_templ.go` and produces noise. Check the version banner at the top of a
generated file instead.

## 2. Seed sample books (fast, no real content needed)

Write files into `./books/<subdir>` (mounted at `/books`), then create a library whose path is that
subdir and scan it. Library paths **must** resolve under `BOOKS_DIR`; absolute paths outside it and
any `..` are rejected.

Generate samples with a short Python script (`zipfile` for EPUB/CBZ, a hand-written minimal PDF,
`ffmpeg -f lavfi -i "sine=frequency=440:duration=20" ... out.mp3` for audio). Two tips that make the
tests much stronger:

- Name CBZ pages out of order (`page10.png`, `page2.png`, `page1.png`) and give each a distinct solid
  colour — page order bugs then become visible at a glance instead of requiring pixel comparison.
- Put real values in the EPUB OPF (title/creator/publisher/ISBN) so metadata extraction is verifiable.

## 3. Drive setup/login/scan from the shell (proves CSRF works without JS)

Forms render a hidden `csrf_token` server-side, so plain curl works and simultaneously proves the
no-JavaScript path:

```bash
tok(){ grep -o 'name="csrf_token" value="[a-f0-9]*"' page.html | head -1 | sed 's/.*value="//;s/"//'; }
curl -s -c c.jar -b c.jar http://localhost:8080/setup -o page.html; T=$(tok)
curl -s -b c.jar -c c.jar -o /dev/null -w '%{http_code}\n' -X POST \
  -d "username=admin&display_name=Admin&password=supersecret1" http://localhost:8080/setup   # expect 403
curl -s -b c.jar -c c.jar -o /dev/null -w '%{http_code}\n' -X POST \
  -d "csrf_token=$T&username=admin&display_name=Admin&password=supersecret1" http://localhost:8080/setup  # expect 303
```

Repeat the same pattern for `POST /libraries` and `POST /libraries/{id}/scan`. A missing-token 403 and
a present-token 303 is the assertion pair — a test that only does the happy path can't tell whether
CSRF is enforced at all.

Then log the browser in once before recording so the recording starts on the actual feature.

## 4. Verify the readers in a real browser (the part that keeps breaking)

Reader pages are `/read/{id}`; `epub.js`, `pdf.js`, `cbz.js`, `audio.js` all read progress from a
`<script id="progress-data" type="application/json">` block and POST to `/progress/{id}`.

Assertions that actually discriminate:

- Header `#reader-status` is non-empty (`Page N` for CBZ, a percentage for epub/audio). Empty means
  the reader IIFE threw before it did anything.
- `POST /progress/{id}` returns **204** in the app log, not 403. A 403 means the CSRF token wasn't
  attached — usually a script-ordering problem, since `app.js` (which wraps `window.fetch`) is
  `defer`red in `<head>` and non-deferred body scripts would run before it.
- CBZ page images change colour in natural order (page1 → page2 → page10).
- After leaving the reader, the book detail page shows `N% · reading` — that proves the write landed
  in Postgres, not just that a request was sent.
- Zero uncaught exceptions and no 4xx on the reader page.

Things that have actually broken here more than once, worth checking first when a reader looks dead:

- Progress JSON emitted as literal templ source instead of JSON (`<script>` bodies are raw text in
  templ; `@templ.JSONScript(...)` is the correct construct).
- The JSON `<script>` rendered *after* the reader `<script>` tags, so `getElementById('progress-data')`
  is `null` at parse time.
- `ePub('/stream/{id}')` with no `.epub` extension: epub.js then treats the URL as an unpacked EPUB
  directory and fetches `/stream/META-INF/container.xml` (404), leaving a blank viewer. If you see
  that request, the fix is an explicit `{ openAs: 'epub' }` (or an `.epub`-suffixed URL / ArrayBuffer).

### Reading console + network without opening devtools

Devtools clutter a recording. Attach to the already-running Chrome over CDP instead (works headed,
invisible in the video):

```python
import json, urllib.request, websocket   # pip install websocket-client
tabs = json.load(urllib.request.urlopen("http://localhost:29229/json"))
t = [x for x in tabs if x["type"] == "page"][0]
ws = websocket.create_connection(t["webSocketDebuggerUrl"], suppress_origin=True)  # 403 without suppress_origin
```

Enable `Runtime`/`Log`/`Network`, reload, and collect `Runtime.exceptionThrown`, `Log.entryAdded` and
`Network.responseReceived`. Note `google-chrome --headless --dump-dom` may silently produce nothing in
this environment; CDP against the running browser is the reliable path.

Use CDP only for observation and one-off root-cause probes. Do the actual flow with real clicks so the
recording is watchable.

## 5. Known gaps / harder to test locally

- OPDS (`/opds`, `/opds/catalog`, `/opds/cover/{id}`, `/opds/download/{id}`) uses HTTP basic auth with
  `opds_users`; needs a client or an authenticated fetch, and the linked app user's `CanDownload`
  now applies.
- BookDrop import and metadata lookup need outbound Open Library / Google Books access; cover
  downloads are restricted to an https host allowlist, so arbitrary cover URLs will be rejected by
  design.
- Permission/ownership behaviour (shelf IDOR, `CanDownload`, login lockout) needs a second
  non-admin user created under Settings.

## Cleanup

`docker compose down -v` removes the Postgres volume; `./data`, `./books`, `./bookdrop` are host
directories, so delete any sample books you created there.
