// Turns a native <select> into a searchable listbox (WAI-ARIA combobox pattern).
// The <select> stays in the form as the source of truth.
let uid = 0;

export function enhanceSelect(select, { onChange } = {}) {
  const id = `cb-${++uid}`;
  const options = [...select.options].map((o) => ({ value: o.value, label: o.textContent.trim() }));
  const labelEl = select.id ? document.querySelector(`label[for="${select.id}"]`) : null;

  const root = document.createElement('div');
  root.className = 'combobox';

  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'combobox-button';
  button.setAttribute('aria-haspopup', 'listbox');
  button.setAttribute('aria-expanded', 'false');
  if (labelEl) {
    labelEl.id ||= `${id}-label`;
    button.setAttribute('aria-labelledby', `${labelEl.id} ${id}-value`);
    labelEl.htmlFor = `${id}-button`;
  }
  button.id = `${id}-button`;
  const valueEl = document.createElement('span');
  valueEl.id = `${id}-value`;
  button.append(valueEl, chevron());

  const popup = document.createElement('div');
  popup.className = 'combobox-popup';
  popup.hidden = true;

  const search = document.createElement('input');
  search.type = 'text';
  search.className = 'combobox-search';
  search.placeholder = 'Search languages…';
  search.autocomplete = 'off';
  search.spellcheck = false;
  search.setAttribute('role', 'combobox');
  search.setAttribute('aria-autocomplete', 'list');
  search.setAttribute('aria-expanded', 'true');
  search.setAttribute('aria-controls', `${id}-list`);
  search.setAttribute('aria-label', 'Search languages');

  const list = document.createElement('ul');
  list.className = 'combobox-list';
  list.id = `${id}-list`;
  list.setAttribute('role', 'listbox');
  if (labelEl) list.setAttribute('aria-labelledby', labelEl.id);

  popup.append(search, list);
  root.append(button, popup);
  select.after(root);
  select.hidden = true;
  select.tabIndex = -1;

  let visible = [];
  let active = 0;

  const sync = () => {
    valueEl.textContent = select.selectedOptions[0]?.textContent.trim() ?? '';
  };

  const renderList = () => {
    const q = search.value.trim().toLowerCase();
    visible = options.filter((o) => !q || o.label.toLowerCase().includes(q) || o.value.includes(q));
    list.replaceChildren(...visible.map((o, i) => {
      const li = document.createElement('li');
      li.id = `${id}-opt-${i}`;
      li.setAttribute('role', 'option');
      li.className = 'combobox-option';
      li.textContent = o.label;
      li.setAttribute('aria-selected', String(o.value === select.value));
      li.addEventListener('mousedown', (e) => e.preventDefault());
      li.addEventListener('click', () => choose(o.value));
      return li;
    }));
    if (!visible.length) {
      const li = document.createElement('li');
      li.className = 'combobox-empty';
      li.textContent = 'No matching language';
      list.append(li);
    }
    setActive(Math.max(0, visible.findIndex((o) => o.value === select.value)));
  };

  const setActive = (i) => {
    active = Math.min(Math.max(i, 0), visible.length - 1);
    [...list.children].forEach((li, j) => li.classList.toggle('is-active', j === active));
    const el = list.children[active];
    if (el && visible.length) {
      search.setAttribute('aria-activedescendant', el.id);
      el.scrollIntoView({ block: 'nearest' });
    } else {
      search.removeAttribute('aria-activedescendant');
    }
  };

  const open = () => {
    popup.hidden = false;
    button.setAttribute('aria-expanded', 'true');
    search.value = '';
    renderList();
    search.focus();
  };

  const close = (focusButton = true) => {
    if (popup.hidden) return;
    popup.hidden = true;
    button.setAttribute('aria-expanded', 'false');
    if (focusButton) button.focus();
  };

  const choose = (value) => {
    if (select.value !== value) {
      select.value = value;
      select.dispatchEvent(new Event('change', { bubbles: true }));
    }
    sync();
    close();
  };

  button.addEventListener('click', () => (popup.hidden ? open() : close()));
  button.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      open();
    }
  });
  search.addEventListener('input', renderList);
  search.addEventListener('keydown', (e) => {
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); setActive(active + 1); break;
      case 'ArrowUp': e.preventDefault(); setActive(active - 1); break;
      case 'Home': e.preventDefault(); setActive(0); break;
      case 'End': e.preventDefault(); setActive(visible.length - 1); break;
      case 'Enter':
        e.preventDefault();
        if (visible[active]) choose(visible[active].value);
        break;
      case 'Escape': e.preventDefault(); close(); break;
      case 'Tab': close(false); break;
    }
  });
  document.addEventListener('mousedown', (e) => {
    if (!root.contains(e.target)) close(false);
  });
  select.addEventListener('change', () => {
    sync();
    onChange?.(select.value);
  });
  sync();
}

function chevron() {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 16 16');
  svg.setAttribute('aria-hidden', 'true');
  svg.classList.add('combobox-chevron');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute('d', 'M4 6l4 4 4-4');
  svg.append(path);
  return svg;
}
