<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { EditorState } from '@codemirror/state';
import { EditorView, basicSetup } from 'codemirror';
import { StreamLanguage } from '@codemirror/language';
import { lua } from '@codemirror/legacy-modes/mode/lua';

// CodeMirror 6 轻封装：Lua 语法高亮（绑定脚本编辑用），v-model 双向
const props = withDefaults(
  defineProps<{
    modelValue: string;
    language?: 'lua' | 'plain';
    height?: string;
    readonly?: boolean;
  }>(),
  { language: 'lua', height: '420px', readonly: false },
);

const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const host = ref<HTMLDivElement | null>(null);
let view: EditorView | null = null;

function createDoc(): void {
  if (!host.value) return;
  view = new EditorView({
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        props.language === 'lua' ? StreamLanguage.define(lua) : [],
        EditorView.editable.of(!props.readonly),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) emit('update:modelValue', u.state.doc.toString());
        }),
        EditorView.theme({
          '&': { fontSize: '13px' },
          '.cm-scroller': {
            fontFamily: "'JetBrains Mono', Consolas, 'Liberation Mono', monospace",
            minHeight: props.height,
          },
          '.cm-gutters': { backgroundColor: '#f7f9fb', border: 'none', color: '#9aa8b5' },
          '.cm-activeLine': { backgroundColor: 'rgba(43,125,201,0.05)' },
          '.cm-activeLineGutter': { backgroundColor: 'rgba(43,125,201,0.08)' },
          '&.cm-focused': { outline: 'none' },
          '.cm-content': { padding: '10px 0' },
        }),
      ],
    }),
    parent: host.value,
  });
}

onMounted(createDoc);

onBeforeUnmount(() => view?.destroy());

// 外部重置内容时同步（编辑器内部变更不回灌，避免光标跳动）
watch(
  () => props.modelValue,
  (v) => {
    if (view && v !== view.state.doc.toString()) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
    }
  },
);
</script>

<template>
  <div ref="host" class="code-editor" :style="{ minHeight: height }" />
</template>

<style lang="scss" scoped>
.code-editor {
  border: 1px solid $input-border;
  border-radius: 4px;
  overflow: hidden;
  background: #fff;

  &:focus-within {
    border-color: $brand;
    box-shadow: 0 0 0 3px rgba(43, 125, 201, 0.15);
  }
}
</style>
