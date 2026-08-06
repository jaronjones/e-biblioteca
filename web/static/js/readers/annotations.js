/* Shared annotation API, panel, toolbar, and edit dialog for all readers. */
(function (global) {
  const COLORS = ['yellow', 'green', 'blue', 'pink', 'purple'];

  function qs(sel, root) { return (root || document).querySelector(sel); }
  function qsa(sel, root) { return Array.from((root || document).querySelectorAll(sel)); }

  function escapeHtml(s) {
    return String(s || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  async function api(method, url, body) {
    const opts = { method, headers: {} };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    if (res.status === 204) return null;
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) { data = text; }
    if (!res.ok) {
      const msg = (data && data.error) || text || res.statusText;
      throw new Error(msg || ('HTTP ' + res.status));
    }
    return data;
  }

  function toast(msg) {
    let el = qs('#ann-toast');
    if (!el) {
      el = document.createElement('div');
      el.id = 'ann-toast';
      el.className = 'ann-toast';
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(el._t);
    el._t = setTimeout(() => { el.hidden = true; }, 3200);
  }

  function createController(opts) {
    const root = opts.root || qs('.reader-page');
    if (!root) return null;
    const bookId = root.dataset.bookId;
    const format = (root.dataset.format || '').toLowerCase();
    let items = [];
    let filterKind = '';
    let pendingSelection = null;
    let editing = null;

    const panel = qs('#ann-panel');
    const listEl = qs('#ann-list');
    const toolbar = qs('#ann-toolbar');
    const dialog = qs('#ann-edit-dialog');

    async function load() {
      items = await api('GET', `/api/books/${bookId}/annotations`) || [];
      renderList();
      if (opts.onReload) opts.onReload(items);
      return items;
    }

    function renderList() {
      if (!listEl) return;
      const filtered = items.filter((a) => !filterKind || a.kind === filterKind);
      if (!filtered.length) {
        listEl.innerHTML = '<p class="muted ann-empty">No annotations yet.</p>';
        return;
      }
      listEl.innerHTML = filtered.map((a) => {
        const color = a.color || '';
        const quote = a.quote ? `<div class="ann-item-quote">${escapeHtml(a.quote)}</div>` : '';
        const note = a.note ? `<div class="ann-item-note">${escapeHtml(a.note)}</div>` : '';
        const tags = (a.tags || []).map((t) => `<span class="chip">${escapeHtml(t)}</span>`).join('');
        return `<button type="button" class="ann-item ann-color-${escapeHtml(color)}" data-id="${a.id}">
          <div class="ann-item-head"><span class="ann-kind">${escapeHtml(a.kind)}</span>${color ? `<span class="ann-color-dot ann-color-${escapeHtml(color)}"></span>` : ''}</div>
          ${quote}${note}
          ${tags ? `<div class="chip-row">${tags}</div>` : ''}
        </button>`;
      }).join('');
      qsa('.ann-item', listEl).forEach((btn) => {
        btn.addEventListener('click', () => {
          const ann = items.find((x) => String(x.id) === btn.dataset.id);
          if (!ann) return;
          if (opts.onJump) {
            Promise.resolve(opts.onJump(ann)).catch(() => toast('Location no longer found; note kept'));
          }
        });
        btn.addEventListener('contextmenu', (e) => {
          e.preventDefault();
          const ann = items.find((x) => String(x.id) === btn.dataset.id);
          if (ann) openEdit(ann);
        });
        btn.addEventListener('dblclick', () => {
          const ann = items.find((x) => String(x.id) === btn.dataset.id);
          if (ann) openEdit(ann);
        });
      });
    }

    function setPanelOpen(open) {
      if (!panel) return;
      panel.hidden = !open;
    }

    function showToolbar(x, y, selection) {
      pendingSelection = selection;
      if (!toolbar) return;
      toolbar.hidden = false;
      const rect = toolbar.getBoundingClientRect();
      let left = x - rect.width / 2;
      let top = y - rect.height - 10;
      if (left < 8) left = 8;
      if (top < 8) top = y + 12;
      toolbar.style.left = left + 'px';
      toolbar.style.top = top + 'px';
    }

    function hideToolbar() {
      if (toolbar) toolbar.hidden = true;
      pendingSelection = null;
    }

    async function create(payload) {
      const ann = await api('POST', `/api/books/${bookId}/annotations`, payload);
      items.push(ann);
      items.sort((a, b) => String(a.sort_key).localeCompare(String(b.sort_key)) || (a.created_at > b.created_at ? 1 : -1));
      renderList();
      if (opts.onReload) opts.onReload(items);
      return ann;
    }

    async function update(id, patch) {
      const ann = await api('PATCH', `/api/annotations/${id}`, patch);
      const i = items.findIndex((x) => x.id === id);
      if (i >= 0) items[i] = ann;
      renderList();
      if (opts.onReload) opts.onReload(items);
      return ann;
    }

    async function remove(id) {
      await api('DELETE', `/api/annotations/${id}`);
      items = items.filter((x) => x.id !== id);
      renderList();
      if (opts.onReload) opts.onReload(items);
    }

    function openEdit(ann) {
      editing = ann;
      if (!dialog) return;
      qs('#ann-edit-title').textContent = ann.kind;
      qs('#ann-edit-quote').textContent = ann.quote || '';
      qs('#ann-edit-note').value = ann.note || '';
      qs('#ann-edit-tags').value = (ann.tags || []).join(', ');
      qs('#ann-edit-color').value = ann.color || '';
      if (typeof dialog.showModal === 'function') dialog.showModal();
    }

    function wireChrome() {
      qs('#ann-panel-toggle')?.addEventListener('click', () => setPanelOpen(panel?.hidden));
      qs('#ann-panel-close')?.addEventListener('click', () => setPanelOpen(false));
      qs('#ann-pin-btn')?.addEventListener('click', async () => {
        try {
          if (!opts.getBookmarkPayload) return toast('Pin not available');
          const payload = await opts.getBookmarkPayload();
          if (!payload) return;
          await create(payload);
          toast('Pinned');
          setPanelOpen(true);
        } catch (e) {
          toast(e.message || 'Failed to pin');
        }
      });
      qsa('#ann-filters .chip').forEach((chip) => {
        chip.addEventListener('click', () => {
          qsa('#ann-filters .chip').forEach((c) => c.classList.remove('active'));
          chip.classList.add('active');
          filterKind = chip.dataset.kind || '';
          renderList();
        });
      });
      qsa('#ann-palette .ann-swatch').forEach((sw) => {
        sw.addEventListener('click', async () => {
          if (!pendingSelection) return;
          try {
            const payload = await opts.buildHighlight(pendingSelection, sw.dataset.color, '');
            await create(payload);
            hideToolbar();
            window.getSelection()?.removeAllRanges();
            toast('Highlighted');
          } catch (e) {
            toast(e.message || 'Failed to highlight');
          }
        });
      });
      qs('#ann-add-note')?.addEventListener('click', async () => {
        if (!pendingSelection) return;
        const note = window.prompt('Note');
        if (note === null) return;
        try {
          const payload = await opts.buildHighlight(pendingSelection, 'yellow', note || '');
          if (note) payload.kind = payload.kind || 'highlight';
          payload.note = note || '';
          await create(payload);
          hideToolbar();
          window.getSelection()?.removeAllRanges();
        } catch (e) {
          toast(e.message || 'Failed');
        }
      });
      qs('#ann-add-pin')?.addEventListener('click', async () => {
        try {
          const payload = await opts.getBookmarkPayload();
          if (pendingSelection && opts.buildHighlight) {
            // prefer selection-based pin when present
          }
          await create(payload);
          hideToolbar();
          toast('Pinned');
        } catch (e) {
          toast(e.message || 'Failed');
        }
      });
      qs('#ann-help-btn')?.addEventListener('click', () => {
        toast('a: panel · b: pin · Esc: close · dbl-click list item: edit');
      });
      document.addEventListener('keydown', (e) => {
        if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable)) return;
        if (e.key === 'Escape') {
          hideToolbar();
          setPanelOpen(false);
          dialog?.open && dialog.close();
        }
        if (e.key === 'a' || e.key === 'A') {
          e.preventDefault();
          setPanelOpen(panel?.hidden);
        }
        if (e.key === 'b' || e.key === 'B') {
          e.preventDefault();
          qs('#ann-pin-btn')?.click();
        }
      });
      document.addEventListener('mousedown', (e) => {
        if (toolbar && !toolbar.hidden && !toolbar.contains(e.target)) {
          // keep open until selection cleared; hide if click outside without selection
          setTimeout(() => {
            const sel = window.getSelection();
            if (!sel || sel.isCollapsed) hideToolbar();
          }, 0);
        }
      });

      const form = qs('#ann-edit-form');
      form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const submitter = e.submitter;
        if (submitter && submitter.value === 'cancel') {
          dialog.close();
          editing = null;
          return;
        }
        if (!editing) return;
        try {
          const tags = qs('#ann-edit-tags').value.split(',').map((t) => t.trim()).filter(Boolean);
          await update(editing.id, {
            note: qs('#ann-edit-note').value,
            tags,
            color: qs('#ann-edit-color').value || null,
          });
          dialog.close();
          editing = null;
          toast('Saved');
        } catch (err) {
          toast(err.message || 'Save failed');
        }
      });
      qs('#ann-edit-delete')?.addEventListener('click', async () => {
        if (!editing) return;
        if (editing.note && !window.confirm('Delete this annotation?')) return;
        try {
          await remove(editing.id);
          dialog.close();
          editing = null;
          toast('Deleted');
        } catch (err) {
          toast(err.message || 'Delete failed');
        }
      });
    }

    async function deepLink() {
      const params = new URLSearchParams(location.search);
      const id = params.get('annotation');
      if (!id) return;
      await load();
      const ann = items.find((x) => String(x.id) === String(id));
      if (!ann) {
        toast('Annotation not found');
        return;
      }
      setPanelOpen(true);
      if (opts.onJump) {
        try {
          await opts.onJump(ann);
        } catch (_) {
          toast('Location no longer found; note kept');
        }
      }
    }

    wireChrome();

    return {
      bookId,
      format,
      load,
      create,
      update,
      remove,
      items: () => items,
      showToolbar,
      hideToolbar,
      openEdit,
      setPanelOpen,
      toast,
      deepLink,
      COLORS,
    };
  }

  global.EBAnnotations = { createController, escapeHtml, toast, COLORS };
})(window);
