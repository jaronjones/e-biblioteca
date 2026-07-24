(function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  // iframe-based PDF viewer; mark as reading once opened
  fetch(`/progress/${bookId}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ percent: 1, position: { page: 1 }, status: 'reading' }),
  }).catch(() => {});
})();
