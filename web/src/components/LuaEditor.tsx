import { createEffect, onCleanup, onMount } from 'solid-js';
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { basicSetup } from 'codemirror';
import { HighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language';
import { lua } from '@codemirror/legacy-modes/mode/lua';
import {
  autocompletion,
  closeBrackets,
  snippetCompletion,
  type CompletionContext,
} from '@codemirror/autocomplete';
import { tags } from '@lezer/highlight';

/** Lua 关键字/内置 API 补全来源（静态符号 + kamc 桥接 API 片段）。 */
function luaCompletions(context: CompletionContext) {
  const word = context.matchBefore(/[\w.]+/);
  if (!word || (word.from === word.to && !context.explicit)) return null;
  const kamcApi = [
    'kamc.upsert_user(username, nickname, groups_csv)',
    'kamc.upsert_group(name, desc)',
    'kamc.list_users()',
    'kamc.list_groups()',
    'kamc.add_policy(sub, dom, obj, act)',
    'kamc.remove_policy(sub, dom, obj, act)',
    'kamc.has_policy(sub, dom, obj, act)',
    'kamc.list_policies()',
  ];
  const options = [
    // kamc 桥接 API（带 snippet 占位符，Tab 跳转）
    ...kamcApi.map((label) =>
      snippetCompletion(label, {
        label,
        type: 'function',
        detail: 'kamc 桥接',
        boost: 2,
      }),
    ),
    ...['for', 'if', 'else', 'elseif', 'end', 'while', 'do', 'then', 'function', 'local', 'return', 'in', 'ipairs', 'pairs', 'tostring', 'string', 'table', 'math', 'true', 'false', 'nil', 'and', 'or', 'not', 'break'].map((keyword) => ({
      label: keyword,
      type:
        keyword === 'true' || keyword === 'false' || keyword === 'nil'
          ? 'constant'
          : 'keyword',
    })),
    snippetCompletion('for ${i} = 1, #${items} do\n\t\nend', {
      label: 'fori',
      type: 'keyword',
      detail: '数值 for 循环',
    }),
    snippetCompletion('for _, ${v} in ipairs(${t}) do\n\t\nend', {
      label: 'forin',
      type: 'keyword',
      detail: 'ipairs 遍历',
    }),
    snippetCompletion('if ${cond} then\n\t\nend', {
      label: 'ifthen',
      type: 'keyword',
      detail: '条件块',
    }),
    snippetCompletion('local function ${name}()\n\t\nend', {
      label: 'func',
      type: 'keyword',
      detail: '函数定义',
    }),
  ];
  return { from: word.from, options, validFor: /^[\w.]*$/ };
}

const kamcHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--syntax-keyword)', fontWeight: '700' },
  { tag: [tags.string, tags.special(tags.string)], color: 'var(--syntax-string)' },
  { tag: [tags.number, tags.bool], color: 'var(--syntax-number)' },
  { tag: tags.comment, color: 'var(--syntax-comment)', fontStyle: 'italic' },
  {
    tag: [tags.function(tags.variableName), tags.definition(tags.variableName)],
    color: 'var(--syntax-function)',
  },
  { tag: tags.variableName, color: 'var(--private-color-layer-content)' },
  { tag: tags.operator, color: 'var(--private-color-layer-content)' },
]);

const editorTheme = EditorView.theme({
  '&': {
    height: '420px',
    backgroundColor: 'var(--code-bg)',
    color: 'var(--private-color-layer-content)',
    border: '1px solid var(--code-border)',
    borderRadius: '8px',
    fontSize: '13px',
  },
  '&.cm-focused': { outline: '2px solid var(--private-color-primary)', outlineOffset: '2px' },
  '.cm-content': {
    fontFamily: 'var(--font-mono)',
    caretColor: 'var(--private-color-primary)',
    padding: '8px 0',
  },
  '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-mono)' },
  '.cm-foldGutter': { display: 'none' },
  '.cm-gutters': {
    backgroundColor: 'var(--code-bg)',
    color: 'color-mix(in oklch, var(--private-color-layer-content) 35%, transparent)',
    border: 'none',
  },
  '.cm-activeLine': {
    backgroundColor:
      'color-mix(in oklch, var(--private-color-layer-content) 5%, transparent)',
  },
  '.cm-activeLineGutter': {
    backgroundColor:
      'color-mix(in oklch, var(--private-color-layer-content) 5%, transparent)',
  },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
    backgroundColor:
      'color-mix(in oklch, var(--private-color-primary) 28%, transparent)',
  },
  '.cm-tooltip': {
    backgroundColor: 'var(--private-color-layer)',
    border: '1px solid color-mix(in oklch, var(--private-color-layer-content) 15%, transparent)',
    borderRadius: '8px',
    color: 'var(--private-color-layer-content)',
  },
  '.cm-tooltip-autocomplete ul li[aria-selected]': {
    backgroundColor:
      'color-mix(in oklch, var(--private-color-primary) 20%, transparent)',
    color: 'var(--private-color-primary)',
  },
});

export interface LuaEditorProps {
  value: string;
  onChange: (value: string) => void;
}

/** CodeMirror 6 Lua 编辑器（Solid 直接集成，无 React 包装器）。 */
export function LuaEditor(props: LuaEditorProps) {
  let host: HTMLDivElement | undefined;
  let view: EditorView | undefined;

  onMount(() => {
    if (!host) return;
    const state = EditorState.create({
      doc: props.value,
      extensions: [
        basicSetup,
        StreamLanguage.define(lua),
        autocompletion({ override: [luaCompletions] }),
        closeBrackets(),
        syntaxHighlighting(kamcHighlight),
        editorTheme,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) props.onChange(update.state.doc.toString());
        }),
      ],
    });
    view = new EditorView({ state, parent: host });
    onCleanup(() => {
      view?.destroy();
      view = undefined;
    });
  });

  createEffect(() => {
    const next = props.value;
    if (!view) return;
    const current = view.state.doc.toString();
    if (next !== current) {
      view.dispatch({
        changes: { from: 0, to: current.length, insert: next },
      });
    }
  });

  return <div ref={host} class="lua-editor" />;
}
