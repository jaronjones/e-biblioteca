(async function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  const el = document.getElementById('epub-viewer');
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');

  // /stream/{id} has no .epub extension; without openAs epub.js treats it as a directory
  // and requests META-INF/container.xml under /stream/ (404, blank viewer).
  const book = ePub(`/stream/${bookId}`, { openAs: 'epub' });
  const rendition = book.renderTo(el, { width: '100%', height: '100%', flow: 'paginated' });
  await book.ready;
  if (prog.position && prog.position.cfi) {
    await rendition.display(prog.position.cfi);
  } else {
    await rendition.display();
  }

  async function save() {
    try {
      const loc = rendition.currentLocation();
      const cfi = loc && loc.start && loc.start.cfi;
      let percent = 0;
      if (book.locations && book.locations.length()) {
        percent = book.locations.percentageFromCfi(cfi) * 100;
      }
      await fetch(`/progress/${bookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ percent, position: { cfi }, status: percent >= 98 ? 'finished' : 'reading' }),
      });
      if (status) status.textContent = `${Math.round(percent)}%`;
    } catch (_) {}
  }

  try {
    await book.locations.generate(1024);
  } catch (_) {}

  rendition.on('relocated', () => { save(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowRight') rendition.next();
    if (e.key === 'ArrowLeft') rendition.prev();
  });
  window.addEventListener('beforeunload', save);
})();
