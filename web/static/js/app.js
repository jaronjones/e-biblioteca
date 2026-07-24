// Global UI helpers
document.addEventListener('submit', (e) => {
  const form = e.target;
  if (form && form.dataset.confirm) {
    if (!confirm(form.dataset.confirm)) e.preventDefault();
  }
});

// Theme preview without save already handled by onchange on select
