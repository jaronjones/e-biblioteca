(async function () {
  const root = document.querySelector('.reader-page');
  if (!root || !window.pdfjsLib) {
    // fallback: no pdf.js loaded
    return;
  }
  const bookId = root.dataset.bookId;
  const status = document.getElementById('reader-status');
  const prog = JSON.parse(document.getElementById('progress-data').textContent || '{}');
  const canvas = document.getElementById('pdf-canvas');
  const textLayerDiv = document.getElementById('pdf-text-layer');
  const hlLayer = document.getElementById('pdf-hl-layer');
  const pageLabel = document.getElementById('pdf-page-label');
  if (!canvas) return;

  pdfjsLib.GlobalWorkerOptions.workerSrc =
    'https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.11.174/pdf.worker.min.js';

  let pdfDoc = null;
  let pageNum = (prog.position && typeof prog.position.page === 'number')
    ? Math.max(1, prog.position.page)
    : 1;
  let rendering = false;
  let pending = null;
  let viewport = null;

  async function save() {
    if (!pdfDoc) return;
    const percent = (pageNum / pdfDoc.numPages) * 100;
    try {
      await fetch(`/progress/${bookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          percent,
          position: { page: pageNum },
          status: percent >= 98 ? 'finished' : 'reading',
        }),
      });
    } catch (_) {}
    if (status) status.textContent = `${Math.round(percent)}%`;
    if (pageLabel) pageLabel.textContent = `Page ${pageNum} / ${pdfDoc.numPages}`;
  }

  function paintHighlights(items) {
    if (!hlLayer || !viewport) return;
    hlLayer.innerHTML = '';
    (items || []).forEach((ann) => {
      let anchor = ann.anchor || {};
      if (typeof anchor === 'string') {
        try { anchor = JSON.parse(anchor); } catch (_) { return; }
      }
      if (anchor.scheme !== 'pdf' || Number(anchor.page) !== pageNum) return;
      const rects = anchor.rects || [];
      rects.forEach((r) => {
        const div = document.createElement('div');
        div.className = `pdf-hl ann-color-${ann.color || 'yellow'}`;
        // stored as PDF viewport coords at render scale 1-relative fractions of page
        div.style.left = (r.x * viewport.width) + 'px';
        div.style.top = (r.y * viewport.height) + 'px';
        div.style.width = (r.w * viewport.width) + 'px';
        div.style.height = (r.h * viewport.height) + 'px';
        div.title = ann.note || ann.quote || '';
        div.addEventListener('click', () => ctrl.openEdit(ann));
        hlLayer.appendChild(div);
      });
    });
  }

  async function renderPage(num) {
    rendering = true;
    const page = await pdfDoc.getPage(num);
    const wrap = document.getElementById('pdf-canvas-wrap');
    const maxW = wrap ? wrap.clientWidth : 800;
    const unscaled = page.getViewport({ scale: 1 });
    const scale = Math.min(2, maxW / unscaled.width);
    viewport = page.getViewport({ scale });
    const ctx = canvas.getContext('2d');
    canvas.height = viewport.height;
    canvas.width = viewport.width;
    await page.render({ canvasContext: ctx, viewport }).promise;

    // text layer for selection
    if (textLayerDiv && pdfjsLib.renderTextLayer) {
      textLayerDiv.innerHTML = '';
      textLayerDiv.style.width = viewport.width + 'px';
      textLayerDiv.style.height = viewport.height + 'px';
      const textContent = await page.getTextContent();
      await pdfjsLib.renderTextLayer({
        textContentSource: textContent,
        container: textLayerDiv,
        viewport,
        textDivs: [],
      }).promise;
    } else if (textLayerDiv) {
      textLayerDiv.innerHTML = '';
      textLayerDiv.style.width = viewport.width + 'px';
      textLayerDiv.style.height = viewport.height + 'px';
      const textContent = await page.getTextContent();
      // manual minimal text layer
      textContent.items.forEach((item) => {
        const tx = pdfjsLib.Util.transform(viewport.transform, item.transform);
        const div = document.createElement('span');
        div.textContent = item.str;
        div.style.position = 'absolute';
        div.style.left = tx[4] + 'px';
        div.style.top = (tx[5] - item.height * scale) + 'px';
        div.style.fontSize = (item.height * scale) + 'px';
        div.style.fontFamily = 'sans-serif';
        div.style.transformOrigin = '0% 0%';
        div.style.whiteSpace = 'pre';
        div.style.color = 'transparent';
        textLayerDiv.appendChild(div);
      });
    }

    rendering = false;
    await save();
    paintHighlights(ctrl.items());
    if (pending !== null) {
      const p = pending;
      pending = null;
      queueRender(p);
    }
  }

  function queueRender(num) {
    if (rendering) {
      pending = num;
      return;
    }
    renderPage(num);
  }

  function go(delta) {
    if (!pdfDoc) return;
    const next = pageNum + delta;
    if (next < 1 || next > pdfDoc.numPages) return;
    pageNum = next;
    queueRender(pageNum);
  }

  document.getElementById('pdf-prev')?.addEventListener('click', () => go(-1));
  document.getElementById('pdf-next')?.addEventListener('click', () => go(1));
  document.addEventListener('keydown', (e) => {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA')) return;
    if (e.key === 'ArrowRight' || e.key === 'PageDown') { e.preventDefault(); go(1); }
    if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); go(-1); }
  });

  const ctrl = window.EBAnnotations.createController({
    root,
    onReload: paintHighlights,
    getBookmarkPayload: async () => ({
      kind: 'bookmark',
      color: '',
      quote: '',
      note: '',
      tags: [],
      sort_key: String(pageNum).padStart(8, '0'),
      anchor: { scheme: 'pdf', page: pageNum },
    }),
    buildHighlight: async (sel, color, note) => ({
      kind: 'highlight',
      color: color || 'yellow',
      quote: sel.text || '',
      note: note || '',
      tags: [],
      sort_key: String(pageNum).padStart(8, '0'),
      anchor: {
        scheme: 'pdf',
        page: pageNum,
        rects: sel.rects || [],
      },
    }),
    onJump: async (ann) => {
      let anchor = ann.anchor || {};
      if (typeof anchor === 'string') {
        try { anchor = JSON.parse(anchor); } catch (_) { throw new Error('bad anchor'); }
      }
      const p = Number(anchor.page) || 1;
      pageNum = p;
      await renderPage(pageNum);
    },
  });

  // selection on text layer
  document.addEventListener('mouseup', () => {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || !textLayerDiv || !textLayerDiv.contains(sel.anchorNode)) return;
    const text = String(sel.toString() || '').trim();
    if (!text || !viewport) return;
    const range = sel.getRangeAt(0);
    const wrapRect = document.getElementById('pdf-canvas-wrap').getBoundingClientRect();
    const clientRects = Array.from(range.getClientRects());
    const rects = clientRects.map((r) => ({
      x: (r.left - wrapRect.left) / viewport.width,
      y: (r.top - wrapRect.top) / viewport.height,
      w: r.width / viewport.width,
      h: r.height / viewport.height,
    })).filter((r) => r.w > 0 && r.h > 0);
    const first = clientRects[0];
    if (!first) return;
    ctrl.showToolbar(first.left + first.width / 2, first.top, { text, rects });
  });

  pdfDoc = await pdfjsLib.getDocument(`/stream/${bookId}`).promise;
  if (pageNum > pdfDoc.numPages) pageNum = 1;
  await renderPage(pageNum);
  await ctrl.load();
  await ctrl.deepLink();
  window.addEventListener('resize', () => queueRender(pageNum));
})();
