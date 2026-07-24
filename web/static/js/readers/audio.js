(function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  const audio = document.getElementById('audio-player');
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');

  audio.addEventListener('loadedmetadata', () => {
    if (prog.position && typeof prog.position.seconds === 'number') {
      audio.currentTime = prog.position.seconds;
    }
  });

  function save() {
    if (!audio.duration) return;
    const percent = (audio.currentTime / audio.duration) * 100;
    fetch(`/progress/${bookId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        percent,
        position: { seconds: audio.currentTime },
        status: percent >= 98 ? 'finished' : 'reading',
      }),
    }).catch(() => {});
    if (status) status.textContent = `${Math.round(percent)}%`;
  }

  setInterval(save, 5000);
  audio.addEventListener('pause', save);
  window.addEventListener('beforeunload', save);
})();
