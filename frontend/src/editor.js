import { Compartment, EditorState } from '@codemirror/state';
import {
  EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter,
  highlightSpecialChars, drawSelection, placeholder as placeholderExt,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
import {
  HighlightStyle, StreamLanguage, bracketMatching, foldGutter, foldKeymap, indentOnInput, syntaxHighlighting,
} from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

const legacy = (load, name) => () => load().then((m) => StreamLanguage.define(m[name]));

// Language packs load on demand so the initial bundle stays small.
const loaders = {
  javascript: () => import('@codemirror/lang-javascript').then((m) => m.javascript()),
  typescript: () => import('@codemirror/lang-javascript').then((m) => m.javascript({ typescript: true })),
  html: () => import('@codemirror/lang-html').then((m) => m.html()),
  css: () => import('@codemirror/lang-css').then((m) => m.css()),
  json: () => import('@codemirror/lang-json').then((m) => m.json()),
  xml: () => import('@codemirror/lang-xml').then((m) => m.xml()),
  python: () => import('@codemirror/lang-python').then((m) => m.python()),
  php: () => import('@codemirror/lang-php').then((m) => m.php()),
  go: () => import('@codemirror/lang-go').then((m) => m.go()),
  rust: () => import('@codemirror/lang-rust').then((m) => m.rust()),
  java: () => import('@codemirror/lang-java').then((m) => m.java()),
  c: () => import('@codemirror/lang-cpp').then((m) => m.cpp()),
  cpp: () => import('@codemirror/lang-cpp').then((m) => m.cpp()),
  csharp: legacy(() => import('@codemirror/legacy-modes/mode/clike'), 'csharp'),
  bash: legacy(() => import('@codemirror/legacy-modes/mode/shell'), 'shell'),
  sql: () => import('@codemirror/lang-sql').then((m) => m.sql()),
  yaml: () => import('@codemirror/lang-yaml').then((m) => m.yaml()),
  markdown: () => import('@codemirror/lang-markdown').then((m) => m.markdown()),
  dockerfile: legacy(() => import('@codemirror/legacy-modes/mode/dockerfile'), 'dockerFile'),
  nginx: legacy(() => import('@codemirror/legacy-modes/mode/nginx'), 'nginx'),
};

export async function loadLanguage(id) {
  const load = loaders[id];
  if (!load) return [];
  try {
    return await load();
  } catch {
    return []; // highlighting is an enhancement; plain text is always fine
  }
}

const theme = EditorView.theme({
  '&': { color: 'var(--color-fg)', backgroundColor: 'var(--color-bg)', fontSize: 'var(--code-size)' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.65' },
  '.cm-content': { caretColor: 'var(--color-fg)', padding: '10px 0' },
  '.cm-line': { padding: '0 16px 0 8px' },
  '&.cm-focused': { outline: 'none' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-fg)', borderLeftWidth: '2px' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
    { backgroundColor: 'rgba(9, 105, 218, 0.16)' },
  '.cm-activeLine': { backgroundColor: 'rgba(234, 238, 242, 0.55)' },
  '.cm-gutters': { backgroundColor: 'var(--color-surface)', color: 'var(--color-muted)', border: 'none', borderRight: '1px solid var(--color-border)' },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 12px 0 14px', minWidth: '3ch' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent', color: 'var(--color-fg)' },
  '.cm-foldGutter .cm-gutterElement': { padding: '0 4px', color: 'var(--color-muted)' },
  '.cm-matchingBracket': { backgroundColor: 'rgba(21, 127, 91, 0.14)', outline: '1px solid rgba(21, 127, 91, 0.45)' },
  '.cm-selectionMatch': { backgroundColor: 'rgba(212, 167, 44, 0.2)' },
  '.cm-searchMatch': { backgroundColor: 'rgba(212, 167, 44, 0.3)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'rgba(212, 167, 44, 0.55)' },
  '.cm-placeholder': { color: 'var(--color-muted)' },
  '.cm-panels': { backgroundColor: 'var(--color-surface)', color: 'var(--color-fg)', borderColor: 'var(--color-border)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--color-border)' },
  '.cm-panel input, .cm-panel button': { fontFamily: 'var(--font-sans)', fontSize: '13px' },
  '.cm-textfield': { backgroundColor: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: '6px', color: 'var(--color-fg)' },
  '.cm-button': { backgroundImage: 'none', backgroundColor: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: '6px', color: 'var(--color-fg)' },
  '.cm-tooltip': { backgroundColor: 'var(--color-elevated)', border: '1px solid var(--color-border)', borderRadius: '6px' },
}, { dark: false });

// GitHub-light inspired; every color is at least 4.5:1 on white.
const highlight = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.moduleKeyword, t.operatorKeyword, t.definitionKeyword, t.modifier], color: '#CF222E' },
  { tag: [t.string, t.special(t.string), t.character, t.attributeValue], color: '#0A3069' },
  { tag: [t.number, t.bool, t.null, t.atom, t.escape, t.color, t.unit], color: '#0550AE' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: '#656D76', fontStyle: 'italic' },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.definition(t.function(t.variableName)), t.macroName], color: '#8250DF' },
  { tag: [t.typeName, t.className, t.namespace, t.standard(t.typeName)], color: '#953800' },
  { tag: [t.tagName, t.angleBracket], color: '#116329' },
  { tag: [t.attributeName, t.labelName], color: '#0550AE' },
  { tag: [t.propertyName, t.definition(t.propertyName)], color: '#0550AE' },
  { tag: [t.regexp, t.special(t.variableName), t.self], color: '#116329' },
  { tag: [t.operator, t.punctuation, t.separator, t.bracket, t.derefOperator], color: '#1F2328' },
  { tag: [t.meta, t.processingInstruction, t.documentMeta, t.annotation], color: '#57606A' },
  { tag: t.heading, color: '#0550AE', fontWeight: '650' },
  { tag: t.link, color: '#0969DA', textDecoration: 'underline' },
  { tag: t.url, color: '#0969DA' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: t.strong, fontWeight: '650' },
  { tag: t.strikethrough, textDecoration: 'line-through' },
  { tag: [t.inserted], color: '#116329' },
  { tag: [t.deleted, t.invalid], color: '#82071E' },
]);

/**
 * Creates a CodeMirror view.
 * Returns { view, setLanguage(id), setWrap(bool) }.
 */
export function createEditor(parent, { doc = '', language = 'plaintext', readOnly = false, wrap = false, placeholder = '', label = 'Code', nonce, onChange } = {}) {
  const langConf = new Compartment();
  const wrapConf = new Compartment();
  const extensions = [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    foldGutter({ openText: '▾', closedText: '▸' }),
    drawSelection(),
    bracketMatching(),
    highlightSelectionMatches(),
    syntaxHighlighting(highlight),
    theme,
    langConf.of([]),
    wrapConf.of(wrap ? EditorView.lineWrapping : []),
    EditorView.contentAttributes.of({ 'aria-label': label }),
  ];
  if (nonce) extensions.push(EditorView.cspNonce.of(nonce));
  if (readOnly) {
    extensions.push(EditorState.readOnly.of(true), keymap.of([...searchKeymap, ...foldKeymap, ...defaultKeymap]));
  } else {
    extensions.push(
      history(),
      indentOnInput(),
      closeBrackets(),
      highlightActiveLine(),
      EditorState.tabSize.of(4),
      keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...historyKeymap, ...searchKeymap, ...foldKeymap, indentWithTab]),
    );
    if (placeholder) extensions.push(placeholderExt(placeholder));
    if (onChange) {
      extensions.push(EditorView.updateListener.of((u) => { if (u.docChanged) onChange(u.state); }));
    }
  }

  const view = new EditorView({ parent, state: EditorState.create({ doc, extensions }) });
  let langToken = 0;
  const setLanguage = async (id) => {
    const token = ++langToken;
    const support = await loadLanguage(id);
    if (token === langToken) view.dispatch({ effects: langConf.reconfigure(support) });
  };
  const setWrap = (on) => view.dispatch({ effects: wrapConf.reconfigure(on ? EditorView.lineWrapping : []) });
  setLanguage(language);
  return { view, setLanguage, setWrap };
}
