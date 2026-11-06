import 'htmx.org';
import { createEditor } from './editor.js';
import { enhanceSelect } from './combobox.js';

const nonce = document.querySelector('meta[name="csp-nonce"]')?.content;

const store = {
  get(key) { try { return localStorage.getItem(key); } catch { return null; } },
  set(key, value) { try { localStorage.setItem(key, value); } catch { /* storage unavailable */ } },
  remove(key) { try { localStorage.removeItem(key); } catch { /* storage unavailable */ } },
};

const session = {
  take(key) {
    try {
      const v = sessionStorage.getItem(key);
      sessionStorage.removeItem(key);
      return v;
    } catch { return null; }
  },
  set(key, value) { try { sessionStorage.setItem(key, value); } catch { /* storage unavailable */ } },
};

// ---- Local history: pastes created in this browser, with their deletion tokens ----

const HISTORY_KEY = 'snippit:history';

const history = {
  all() {
    try {
      const list = JSON.parse(store.get(HISTORY_KEY) ?? '[]');
      return Array.isArray(list) ? list : [];
    } catch { return []; }
  },
  save(list) { store.set(HISTORY_KEY, JSON.stringify(list.slice(0, 200))); },
  get(id) { return this.all().find((e) => e.id === id); },
  add(entry) { this.save([entry, ...this.all().filter((e) => e.id !== entry.id)]); },
  remove(id) { this.save(this.all().filter((e) => e.id !== id)); },
  // Drops entries whose paste has expired; the server has removed them too.
  prune() {
    const now = Date.now() / 1000;
    const list = this.all();
    const live = list.filter((e) => !e.expires || e.expires > now);
    if (live.length !== list.length) this.save(live);
    return live;
  },
};

function recordCreated() {
  const el = document.getElementById('created');
  if (!el?.dataset.token) return;
  const d = el.dataset;
  history.add({
    id: d.id,
    title: d.title,
    language: d.language,
    created: Number(d.created),
    expires: d.expires ? Number(d.expires) : null,
    token: d.token,
  });
  toast('Paste created', 'success');
}

// ---- Shared helpers ----

export function toast(message, kind = 'info') {
  const region = document.getElementById('toasts');
  if (!region) return;
  const el = document.createElement('div');
  el.className = `toast toast-${kind}`;
  el.textContent = message;
  region.append(el);
  requestAnimationFrame(() => el.classList.add('is-visible'));
  setTimeout(() => {
    el.classList.remove('is-visible');
    setTimeout(() => el.remove(), 200);
  }, 2600);
}

async function copyText(text, message) {
  try {
    await navigator.clipboard.writeText(text);
    toast(message, 'success');
  } catch {
    toast('Copy failed. Select the text and copy it manually.', 'error');
  }
}

function formatBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
function relative(unixSeconds) {
  const diff = unixSeconds - Date.now() / 1000;
  const units = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]];
  for (const [unit, secs] of units) {
    if (Math.abs(diff) >= secs) return rtf.format(Math.round(diff / secs), unit);
  }
  return rtf.format(0, 'minute');
}

/**
 * Deletes a paste with the stored token, or asks for the delete key.
 * Resolves true when the paste is gone.
 */
async function deletePaste(id, { token, allowKey }) {
  const send = async (headers) => {
    const res = await fetch(`/api/pastes/${id}`, { method: 'DELETE', headers });
    if (res.status === 204 || res.status === 404 || res.status === 410) return null;
    const body = await res.json().catch(() => null);
    return body?.error?.message ?? 'Something went wrong.';
  };

  const dialog = document.getElementById('delete-dialog');
  if (!dialog) {
    // History page: no dialog markup, token is always present.
    if (!confirm('Delete this paste permanently? This cannot be undone.')) return false;
    const err = await send({ 'X-Delete-Token': token }).catch(() => 'Network error.');
    if (err) toast(err, 'error');
    return !err;
  }

  const form = dialog.querySelector('form');
  const keyField = dialog.querySelector('#delete-key-field');
  const keyInput = dialog.querySelector('#delete-key-input');
  const errorEl = dialog.querySelector('#delete-error');
  const confirmBtn = dialog.querySelector('#delete-confirm');
  const needKey = !token && allowKey;
  keyField.hidden = !needKey;
  keyInput.required = needKey;
  keyInput.value = '';
  errorEl.textContent = '';
  dialog.showModal();
  (needKey ? keyInput : confirmBtn).focus();

  return new Promise((resolve) => {
    const onSubmit = async (e) => {
      e.preventDefault();
      if (needKey && !keyInput.value) {
        errorEl.textContent = 'Enter the delete key.';
        keyInput.focus();
        return;
      }
      confirmBtn.disabled = true;
      const headers = needKey ? { 'X-Delete-Key': keyInput.value } : { 'X-Delete-Token': token };
      const err = await send(headers).catch(() => 'Network error. Please try again.');
      confirmBtn.disabled = false;
      if (err) {
        errorEl.textContent = err;
        if (needKey) keyInput.select();
        return;
      }
      cleanup();
      dialog.close();
      resolve(true);
    };
    const onClose = () => { cleanup(); resolve(false); };
    const cleanup = () => {
      form.removeEventListener('submit', onSubmit);
      dialog.removeEventListener('close', onClose);
    };
    form.addEventListener('submit', onSubmit);
    dialog.addEventListener('close', onClose);
  });
}

// ---- Home: create paste ----

function initHome() {
  const form = document.getElementById('paste-form');
  const textarea = document.getElementById('content');
  const mount = document.getElementById('editor');
  const language = document.getElementById('language');
  const counter = document.getElementById('size-counter');
  const submit = document.getElementById('submit');
  const errorBox = document.getElementById('form-error');
  const maxBytes = Number(form.dataset.maxBytes);
  const encoder = new TextEncoder();

  const updateCounter = (text) => {
    const size = encoder.encode(text).length;
    counter.textContent = `${formatBytes(size)} / ${formatBytes(maxBytes)}`;
    counter.classList.toggle('is-over', size > maxBytes);
    return size;
  };

  const editor = createEditor(mount, {
    doc: textarea.value,
    language: language.value === 'auto' ? 'plaintext' : language.value,
    placeholder: 'Paste your code here...',
    label: 'Paste content',
    nonce,
    onChange: (state) => {
      textarea.value = state.doc.toString();
      updateCounter(textarea.value);
    },
  });
  textarea.hidden = true;
  mount.hidden = false;
  updateCounter(textarea.value);
  editor.view.focus();

  enhanceSelect(language, {
    onChange: (value) => editor.setLanguage(value === 'auto' ? 'plaintext' : value),
  });

  const validate = (e) => {
    const text = editor.view.state.doc.toString();
    textarea.value = text;
    let message = '';
    if (!text.trim()) message = 'Paste some content first.';
    else if (updateCounter(text) > maxBytes) message = `Content exceeds the ${formatBytes(maxBytes)} limit.`;
    if (message) {
      e.preventDefault();
      errorBox.textContent = message;
      editor.view.focus();
    }
  };
  form.addEventListener('htmx:confirm', validate);
  form.addEventListener('submit', validate);
  form.addEventListener('htmx:beforeRequest', () => { errorBox.textContent = ''; });
  form.addEventListener('htmx:sendError', () => { errorBox.textContent = 'Network error. Please try again.'; });

  // Ctrl/Cmd+Enter submits from anywhere in the form, including the editor.
  form.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !submit.disabled) {
      e.preventDefault();
      form.requestSubmit(submit);
    }
  });

  if (session.take('snippit:deleted')) toast('Paste deleted', 'success');
}

// ---- View: paste viewer ----

function initView() {
  const main = document.getElementById('paste');
  const { id, url, language, filename } = main.dataset;
  const hasDeleteKey = main.dataset.hasDeleteKey === 'true';
  const source = document.getElementById('source');
  const mount = document.getElementById('viewer');
  const content = source.textContent;
  const wrapKey = 'snippit:wrap';
  let wrap = store.get(wrapKey) === '1';

  const editor = createEditor(mount, { doc: content, language, readOnly: true, wrap, label: 'Paste content', nonce });
  source.hidden = true;
  mount.hidden = false;

  const wrapButton = main.querySelector('[data-action="wrap"]');
  const syncWrap = () => wrapButton?.setAttribute('aria-pressed', String(wrap));
  syncWrap();

  const deleteButton = main.querySelector('[data-action="delete"]');
  if (deleteButton && (history.get(id)?.token || hasDeleteKey)) deleteButton.hidden = false;

  const blobOpen = (download) => {
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const href = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = href;
    if (download) a.download = filename;
    else a.target = '_blank';
    a.rel = 'noopener';
    a.click();
    setTimeout(() => URL.revokeObjectURL(href), 30_000);
  };

  const qrDialog = document.getElementById('qr-dialog');

  const actions = {
    copy: () => copyText(content, 'Copied to clipboard'),
    'copy-link': () => copyText(url, 'Link copied'),
    wrap: () => {
      wrap = !wrap;
      store.set(wrapKey, wrap ? '1' : '0');
      editor.setWrap(wrap);
      syncWrap();
    },
    raw: () => blobOpen(false),
    download: () => { blobOpen(true); toast('Download started', 'success'); },
    qr: () => {
      const img = qrDialog.querySelector('img');
      if (!img.src) img.src = img.dataset.src;
      qrDialog.showModal();
    },
    'close-dialog': (btn) => btn.closest('dialog').close(),
    delete: async () => {
      const deleted = await deletePaste(id, { token: history.get(id)?.token, allowKey: hasDeleteKey });
      if (!deleted) return;
      history.remove(id);
      session.set('snippit:deleted', '1');
      location.assign('/');
    },
  };

  document.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-action]');
    if (btn && actions[btn.dataset.action]) actions[btn.dataset.action](btn);
  });
  for (const d of document.querySelectorAll('dialog')) {
    d.addEventListener('click', (e) => { if (e.target === d) d.close(); });
  }

  // Markdown: Preview / Code tabs.
  const tabs = [...main.querySelectorAll('[role="tab"]')];
  const selectTab = (tab) => {
    for (const t of tabs) {
      const selected = t === tab;
      t.setAttribute('aria-selected', String(selected));
      t.tabIndex = selected ? 0 : -1;
      document.getElementById(t.getAttribute('aria-controls')).hidden = !selected;
    }
    if (wrapButton) wrapButton.hidden = tab.dataset.view !== 'code';
  };
  tabs.forEach((tab, i) => {
    tab.addEventListener('click', () => selectTab(tab));
    tab.addEventListener('keydown', (e) => {
      const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
      if (!step) return;
      const next = tabs[(i + step + tabs.length) % tabs.length];
      selectTab(next);
      next.focus();
    });
  });
  if (tabs.length) selectTab(tabs.find((t) => t.getAttribute('aria-selected') === 'true') ?? tabs[0]);
}

// ---- History page ----

function initHistory() {
  const list = document.getElementById('history-list');
  const empty = document.getElementById('history-empty');
  const clear = document.getElementById('history-clear');

  const render = () => {
    const entries = history.prune();
    empty.hidden = entries.length > 0;
    clear.hidden = entries.length === 0;
    list.replaceChildren(...entries.map((e) => {
      const row = document.createElement('div');
      row.className = 'history-row';

      const info = document.createElement('div');
      info.className = 'min-w-0 flex-1';
      const link = document.createElement('a');
      link.className = 'title block truncate';
      link.href = `/${encodeURIComponent(e.id)}`;
      link.textContent = e.title || 'Untitled paste';
      const meta = document.createElement('p');
      meta.className = 'mt-0.5 text-[12px] text-muted';
      meta.textContent = [
        e.language,
        `created ${relative(e.created)}`,
        e.expires ? `expires ${relative(e.expires)}` : 'never expires',
      ].filter(Boolean).join(' · ');
      info.append(link, meta);

      const actions = document.createElement('div');
      actions.className = 'flex gap-2';
      const del = document.createElement('button');
      del.type = 'button';
      del.className = 'btn btn-danger';
      del.textContent = 'Delete';
      del.setAttribute('aria-label', `Delete ${e.title || 'untitled paste'}`);
      del.addEventListener('click', async () => {
        if (await deletePaste(e.id, { token: e.token })) {
          history.remove(e.id);
          toast('Paste deleted', 'success');
          render();
        }
      });
      const forget = document.createElement('button');
      forget.type = 'button';
      forget.className = 'btn btn-ghost';
      forget.textContent = 'Remove from list';
      forget.addEventListener('click', () => {
        history.remove(e.id);
        render();
      });
      actions.append(del, forget);

      row.append(info, actions);
      return row;
    }));
  };

  clear.addEventListener('click', () => {
    if (!confirm('Clear history? The pastes stay online, but this browser will no longer be able to delete them unless you set a delete key.')) return;
    history.save([]);
    render();
  });
  render();
}

recordCreated();
const page = document.body.dataset.page;
if (page === 'home') initHome();
if (page === 'view') initView();
if (page === 'history') initHistory();
