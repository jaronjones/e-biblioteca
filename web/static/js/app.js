// Global UI helpers
document.addEventListener('submit', (e) => {
  const form = e.target;
  if (form && form.dataset.confirm) {
    if (!confirm(form.dataset.confirm)) e.preventDefault();
  }
});

(function csrfBootstrap() {
  function token() {
    const meta = document.querySelector('meta[name="csrf-token"]');
    return meta && meta.content ? meta.content : '';
  }

  // Inject CSRF token into POST forms from <meta name="csrf-token">.
  function ensureField(form) {
    if (!form || (form.method || 'get').toLowerCase() !== 'post') return;
    const t = token();
    if (!t) return;
    let input = form.querySelector('input[name="csrf_token"]');
    if (!input) {
      input = document.createElement('input');
      input.type = 'hidden';
      input.name = 'csrf_token';
      form.appendChild(input);
    }
    input.value = t;
  }

  document.querySelectorAll('form').forEach(ensureField);
  document.addEventListener('submit', (e) => ensureField(e.target), true);

  // Attach token to fetch/XHR/htmx POSTs (readers, progress, etc.).
  const origFetch = window.fetch.bind(window);
  window.fetch = function (input, init) {
    init = init ? { ...init } : {};
    const method = (init.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
      const t = token();
      if (t) {
        const headers = new Headers(init.headers || {});
        if (!headers.has('X-CSRF-Token')) {
          headers.set('X-CSRF-Token', t);
        }
        init.headers = headers;
      }
    }
    return origFetch(input, init);
  };

  document.body.addEventListener('htmx:configRequest', (e) => {
    const t = token();
    if (t) e.detail.headers['X-CSRF-Token'] = t;
  });
})();
