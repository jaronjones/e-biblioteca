---
name: testing-e-biblioteca
description: Boot e-biblioteca locally with docker compose, seed sample books, and verify the readers, progress tracking, covers and CSRF end-to-end. Use when testing or reviewing changes to readers, scanning, metadata/covers, progress, auth/CSRF or OPDS.
---

# Testing e-biblioteca end-to-end

Server-rendered Go app (chi + templ + htmx + scs sessions on Postgres). Nearly all interesting
behaviour is browser-side glue (readers, progress POSTs, CSRF), so **shell checks are not enough** —
every reader bug found so far passed `go build`, `go vet` and `gofmt` and was only visible in a browser.

## Devin Secrets Needed

None. Everything runs locally against a throwaway Postgres container.

## 1. Boot the app

```bash
cd ~/repos/e-biblioteca
cp -n .env.example .env          # defaults are fine locally
docker compose up -d --build     # Postgres 16 + app on http://localhost:8080
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/healthz   # expect 200
docker compose logs -f app       # keep this open: every request is logged with its status code
```

The access log is the primary oracle for these tests — assert on status codes there rather than
guessing from the UI.

Static checks (Go lives at `/usr/local/go/bin`, templ at `~/go/bin`):

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Do **not** run `templ generate` unless your local templ matches the version in `go.mod` — an older
generator rewrites every `*_templ.go` and produces a huge phantom diff. Check the version banner at
the top of a generated file instead.

## 2. Seed sample books

```bash
python3 .agents/skills/testing-e-biblioteca/make_fixtures.py books/test
```

That writes `Long Novel.epub`, `Sample Novel.epub`, `Sample Report.pdf`, `Sample Comic.cbz` and
(if ffmpeg is present) `Sample Audiobook.mp3`. The fixtures are deliberately shaped to expose bugs:

- **CBZ pages are stored out of natural order** (`page10`, `page2`, `page1`) in distinct solid
  colours, so page-ordering and cover-selection bugs are visible at a glance: blue = `page1`,
  green = `page2`, red = `page10`.
- **Two EPUBs.** `Long Novel` is long enough to paginate (needed for progress); `Sample Novel` fits
  a single page, which catches readers that only save progress on relocation.

`./books` is mounted at `/books`. Library paths must resolve under `BOOKS_DIR`; absolute paths
outside it and any `..` are rejected, so use a bare subdirectory name like `test`.

Every scan re-runs extraction and rewrites the cover for existing books too
(`scanner.go` calls `metadata.Extract` → `UpsertBook` → `saveCover` per file), so after changing
extraction code just rebuild the image and rescan — no need to recreate the library. Rebuild first:
`docker compose up -d --build`, otherwise you are scanning with the old binary.

## 3. Drive setup/login/scan from the shell (also proves CSRF works without JS)

Forms render a hidden `csrf_token` server-side, so plain curl works and simultaneously exercises the
no-JavaScript path:

```bash
tok(){ grep -o 'name="csrf_token" value="[a-f0-9]*"' page.html | head -1 | sed 's/.*value="//;s/"//'; }
curl -s -c c.jar -b c.jar http://localhost:8080/setup -o page.html; T=$(tok)
curl -s -b c.jar -c c.jar -o /dev/null -w '%{http_code}\n' -X POST \
  -d "username=admin&display_name=Admin&password=supersecret1" http://localhost:8080/setup   # expect 403
curl -s -b c.jar -c c.jar -o /dev/null -w '%{http_code}\n' -X POST \
  -d "csrf_token=$T&username=admin&display_name=Admin&password=supersecret1" http://localhost:8080/setup  # expect 303
```

Same pattern for `POST /libraries` (fields `name`, `path`) and `POST /libraries/{id}/scan`; the app
log prints `scanned library N: M books`. The missing-token 403 plus present-token 303 is the
assertion *pair* — a happy-path-only check cannot tell whether CSRF is enforced at all.

Then log the browser in once before recording, so the recording starts on the actual feature.

## 4. Verify the readers in a real browser (the part that keeps breaking)

Reader pages are `/read/{id}`; `epub.js`, `pdf.js`, `cbz.js`, `audio.js` all read progress from a
`<script id="progress-data" type="application/json">` block and POST to `/progress/{id}`.

Assertions that actually discriminate:

- Content is visible: chapter text for EPUB, a page image for CBZ, the PDF in its iframe.
- `#reader-status` in the header is non-empty (`Page N` for CBZ, a percentage otherwise). Empty
  usually means the reader IIFE threw before doing anything.
- `POST /progress/{id}` returns **204**, not 403. A 403 means the CSRF token wasn't attached —
  usually a script-ordering problem, since `app.js` (which wraps `window.fetch`) is `defer`red in
  `<head>` and non-deferred body scripts would run before it.
- CBZ images change colour in natural order (blue → green → red).
- After leaving the reader, the book detail page shows `N% · reading` — that proves the write
  reached Postgres, not merely that a request was sent.
- Zero uncaught exceptions and no 4xx on the reader page.

Gotchas that cost real time here:

- Arrow-key paging in the EPUB reader only works while focus is **outside** the epub.js iframe (the
  `keydown` listener is on the parent document). Click the reader header bar, not the text, before
  pressing arrows.
- `rendition.on('relocated', …)` is registered after the first `display()`, so no progress is saved
  until the reader relocates — with a single-page book, none is ever saved. Use `Long Novel` when
  testing progress.

Reader bugs that have recurred, worth checking first when a reader looks dead:

- Progress JSON emitted as literal templ source instead of JSON (`<script>` bodies are raw text in
  templ; `@templ.JSONScript(...)` is the correct construct).
- The JSON `<script>` rendered *after* the reader `<script>` tags, so
  `getElementById('progress-data')` is `null` at parse time.
- `ePub('/stream/{id}')` with no `.epub` extension: epub.js then treats the URL as an unpacked EPUB
  directory and fetches `/stream/META-INF/container.xml` (404), leaving a blank viewer. The fix is
  `{ openAs: 'epub' }`; the tell is that 404 in the log.

### Reading console + network without opening devtools

Devtools clutter a recording. Attach to the already-running Chrome over CDP instead (works headed,
invisible in the video):

```python
import json, urllib.request, websocket   # pip install websocket-client
tabs = json.load(urllib.request.urlopen("http://localhost:29229/json"))
t = [x for x in tabs if x["type"] == "page"][0]
ws = websocket.create_connection(t["webSocketDebuggerUrl"], suppress_origin=True)  # 403 without suppress_origin
```

Enable `Runtime`/`Log`/`Network`, reload, and collect `Runtime.exceptionThrown`, `Log.entryAdded`
and `Network.responseReceived`. Note `google-chrome --headless --dump-dom` may silently produce
nothing in this environment; CDP against the running browser is the reliable path.

Use CDP only for observation and one-off root-cause probes (e.g. re-opening a book with different
options to confirm a diagnosis). Do the actual flow with real clicks so the recording is watchable.

## 5. Known gaps / harder to test locally

- OPDS (`/opds`, `/opds/catalog`, `/opds/cover/{id}`, `/opds/download/{id}`) uses HTTP basic auth
  with `opds_users`; needs a client or an authenticated fetch, and the linked app user's
  `CanDownload` applies.
- BookDrop import and metadata lookup need outbound Open Library / Google Books access; cover
  downloads are restricted to an https host allowlist, so arbitrary cover URLs are rejected by design.
- Permission/ownership behaviour (shelf IDOR, `CanDownload`, login lockout) needs a second
  non-admin user created under Settings.

## Cleanup

`docker compose down -v` removes the Postgres volume; `./data`, `./books` and `./bookdrop` are host
directories, so delete any generated sample books there.
