(async function () {
  const root = document.querySelector('.reader-page');
  if (!root) return;
  const bookId = root.dataset.bookId;
  const el = document.getElementById('epub-viewer');
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');

  if (typeof ePub === 'undefined') {
    if (status) status.textContent = 'EPUB library failed to load';
    console.error('ePub is not defined — check /static/vendor/epub.min.js');
    return;
  }
  if (!el) {
    console.error('epub-viewer element missing');
    return;
  }

  // epub.js measures the container; ensure non-zero box before renderTo
  function ensureSize() {
    const w = el.clientWidth || el.parentElement?.clientWidth || window.innerWidth;
    const h = el.clientHeight || el.parentElement?.clientHeight || Math.max(320, window.innerHeight - 80);
    if (el.clientHeight < 40) {
      el.style.height = h + 'px';
    }
    return { width: w, height: Math.max(h, 320) };
  }
  const size = ensureSize();

  const book = ePub(`/stream/${bookId}`);
  const rendition = book.renderTo(el, {
    width: size.width,
    height: size.height,
    flow: 'paginated',
    resizeOnOrientationChange: true,
  });
  try {
    await book.ready;
  } catch (err) {
    if (status) status.textContent = 'Failed to open EPUB';
    console.error(err);
    return;
  }
  if (prog.position && prog.position.cfi) {
    await rendition.display(prog.position.cfi);
  } else {
    await rendition.display();
  }

  // Keep iframe sized when layout settles (panel open, window resize)
  function resizeRendition() {
    const s = ensureSize();
    try { rendition.resize(s.width, s.height); } catch (_) {}
  }
  window.addEventListener('resize', resizeRendition);
  requestAnimationFrame(resizeRendition);

  try {
    await book.locations.generate(1024);
  } catch (_) {}

  let painted = new Set();

  function colorStyle(color) {
    const map = {
      yellow: 'rgba(250, 204, 21, 0.4)',
      green: 'rgba(74, 222, 128, 0.4)',
      blue: 'rgba(56, 189, 248, 0.4)',
      pink: 'rgba(244, 114, 182, 0.4)',
      purple: 'rgba(167, 139, 250, 0.4)',
    };
    return map[color] || map.yellow;
  }

  function clearHighlights() {
    painted.forEach((id) => {
      try { rendition.annotations.remove(id, 'highlight'); } catch (_) {}
    });
    painted = new Set();
  }

  function paintAnnotations(items) {
    clearHighlights();
    (items || []).forEach((ann) => {
      if (ann.kind !== 'highlight' && !ann.quote) {
        // still try if cfi range present
      }
      let anchor = ann.anchor || {};
      if (typeof anchor === 'string') {
        try { anchor = JSON.parse(anchor); } catch (_) { anchor = {}; }
      }
      if (anchor.scheme !== 'epubcfi' || !anchor.cfi) return;
      const cfi = anchor.cfi_end ? `epubcfi(${String(anchor.cfi).replace(/^epubcfi\(/, '').replace(/\)$/, '')},${String(anchor.cfi_end).replace(/^epubcfi\(/, '').replace(/\)$/, '')})` : anchor.cfi;
      // Prefer range if both start/end stored separately in anchor.cfi as full range already
      const rangeCfi = anchor.cfi;
      const color = ann.color || 'yellow';
      try {
        const id = String(ann.id);
        rendition.annotations.highlight(
          rangeCfi,
          {},
          (e) => {
            ctrl.openEdit(ann);
          },
          id,
          { fill: colorStyle(color), 'fill-opacity': '0.45', 'mix-blend-mode': 'multiply' },
        );
        painted.add(id);
      } catch (_) {
        // CFI may not resolve in current spine item yet
      }
    });
  }

  let ctrl = null;
  if (window.EBAnnotations && window.EBAnnotations.createController) {
    ctrl = window.EBAnnotations.createController({
      root,
      onReload: paintAnnotations,
      getBookmarkPayload: async () => {
        const loc = rendition.currentLocation();
        const cfi = loc && loc.start && loc.start.cfi;
        if (!cfi) throw new Error('No location');
        let sortKey = '';
        try {
          if (book.locations && book.locations.length()) {
            sortKey = String(book.locations.percentageFromCfi(cfi)).padStart(12, '0');
          }
        } catch (_) {}
        return {
          kind: 'bookmark',
          color: '',
          quote: '',
          note: '',
          tags: [],
          sort_key: sortKey,
          anchor: { scheme: 'epubcfi', cfi },
        };
      },
      buildHighlight: async (sel, color, note) => {
        let sortKey = '';
        try {
          if (book.locations && book.locations.length() && sel.cfi) {
            sortKey = String(book.locations.percentageFromCfi(sel.cfi)).padStart(12, '0');
          }
        } catch (_) {}
        return {
          kind: 'highlight',
          color: color || 'yellow',
          quote: sel.text || '',
          note: note || '',
          tags: [],
          sort_key: sortKey,
          anchor: {
            scheme: 'epubcfi',
            cfi: sel.cfi,
            cfi_end: sel.cfiEnd || undefined,
          },
        };
      },
      onJump: async (ann) => {
        let anchor = ann.anchor || {};
        if (typeof anchor === 'string') {
          try { anchor = JSON.parse(anchor); } catch (_) { anchor = {}; }
        }
        if (!anchor.cfi) throw new Error('missing cfi');
        await rendition.display(anchor.cfi);
      },
    });

    rendition.on('selected', (cfiRange, contents) => {
      try {
        const range = contents.range(cfiRange);
        const text = range ? String(range.toString() || '').trim() : '';
        if (!text) return;
        const rects = range.getClientRects();
        const r = rects[0] || range.getBoundingClientRect();
        const iframe = contents.document.defaultView.frameElement;
        const ir = iframe ? iframe.getBoundingClientRect() : { left: 0, top: 0 };
        ctrl.showToolbar(ir.left + r.left + r.width / 2, ir.top + r.top, {
          cfi: cfiRange,
          text,
        });
      } catch (_) {}
    });
  } else {
    console.warn('annotations UI unavailable');
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

  rendition.on('relocated', () => {
    save();
    if (ctrl) paintAnnotations(ctrl.items());
  });
  document.addEventListener('keydown', (e) => {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA')) return;
    if (e.key === 'ArrowRight') rendition.next();
    if (e.key === 'ArrowLeft') rendition.prev();
  });
  window.addEventListener('beforeunload', save);

  if (ctrl) {
    await ctrl.load();
    await ctrl.deepLink();
  }
})();
