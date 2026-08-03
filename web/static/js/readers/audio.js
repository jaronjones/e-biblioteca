(function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  const audio = document.getElementById('audio-player');
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');

  function fmtTime(sec) {
    sec = Math.max(0, Math.floor(sec || 0));
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
    return `${m}:${String(s).padStart(2, '0')}`;
  }

  const ctrl = window.EBAnnotations.createController({
    root,
    getBookmarkPayload: async () => {
      const seconds = audio.currentTime || 0;
      return {
        kind: 'bookmark',
        color: '',
        quote: fmtTime(seconds),
        note: '',
        tags: [],
        sort_key: String(Math.floor(seconds * 1000)).padStart(12, '0'),
        anchor: { scheme: 'audio', seconds },
      };
    },
    onJump: async (ann) => {
      let anchor = ann.anchor || {};
      if (typeof anchor === 'string') {
        try { anchor = JSON.parse(anchor); } catch (_) { throw new Error('bad anchor'); }
      }
      if (typeof anchor.seconds !== 'number') throw new Error('no time');
      audio.currentTime = anchor.seconds;
      audio.play().catch(() => {});
    },
  });

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
    if (status) status.textContent = `${Math.round(percent)}% · ${fmtTime(audio.currentTime)}`;
  }

  setInterval(save, 5000);
  audio.addEventListener('pause', save);
  window.addEventListener('beforeunload', save);

  ctrl.load().then(() => ctrl.deepLink());
})();
