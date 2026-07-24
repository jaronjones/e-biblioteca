(function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  const img = document.getElementById('cbz-page');
  const prev = document.getElementById('cbz-prev');
  const next = document.getElementById('cbz-next');
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');
  let page = (prog.position && typeof prog.position.page === 'number') ? prog.position.page : 0;
  let maxKnown = page;

  function show() {
    img.src = `/read/${bookId}/pages/${page}`;
    if (status) status.textContent = `Page ${page + 1}`;
    fetch(`/progress/${bookId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        percent: Math.min(99, (page + 1) * 2),
        position: { page },
        status: 'reading',
      }),
    }).catch(() => {});
  }

  img.onerror = function () {
    if (page > 0) {
      page -= 1;
      maxKnown = page;
      show();
    }
  };
  img.onload = function () {
    if (page > maxKnown) maxKnown = page;
  };

  prev.addEventListener('click', () => { if (page > 0) { page -= 1; show(); } });
  next.addEventListener('click', () => { page += 1; show(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowRight' || e.key === ' ') { e.preventDefault(); page += 1; show(); }
    if (e.key === 'ArrowLeft') { e.preventDefault(); if (page > 0) { page -= 1; show(); } }
  });
  show();
})();
