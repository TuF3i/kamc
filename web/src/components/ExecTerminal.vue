<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import Banner from '@/components/Banner.vue';
import { apiRaw, getToken } from '@/api/http';
import { buildExecUrl, EXEC_SUBPROTOCOL, parseFrame, resizeFrame, stdinFrame } from '@/terminal/exec';
import { useGrowlStore } from '@/stores/growl';

// Rancher ContainerShell 交互的等价实现：选命名空间/Pod → 直连 /proxy exec WS
const props = defineProps<{ clusterId: string }>();
const growl = useGrowlStore();

const namespaces = ref<{ label: string; value: string }[]>([]);
const pods = ref<{ label: string; value: string; phase: string }[]>([]);
const containers = ref<{ label: string; value: string }[]>([]);

const ns = ref('');
const pod = ref('');
const container = ref('');

const terminalHost = ref<HTMLDivElement | null>(null);
const loadErr = ref('');
const connState = ref<'idle' | 'connecting' | 'open' | 'closed'>('idle');

let term: Terminal | null = null;
let fit: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObs: ResizeObserver | null = null;

async function loadNamespaces(): Promise<void> {
  loadErr.value = '';
  try {
    const resp = await apiRaw(`/proxy/${encodeURIComponent(props.clusterId)}/api/v1/namespaces?limit=500`);
    const body = (await resp.json()) as { items?: { metadata?: { name?: string } }[] };
    namespaces.value = (body.items ?? [])
      .map((it) => it.metadata?.name)
      .filter((n): n is string => !!n)
      .sort()
      .map((n) => ({ label: n, value: n }));
    if (!namespaces.value.length) {
      loadErr.value = '目标集群未返回任何命名空间';
    }
  } catch (e) {
    loadErr.value = `命名空间列表获取失败：${e instanceof Error ? e.message : String(e)}（需要 list namespaces 权限且集群在线）`;
  }
}

watch(ns, async (v) => {
  pod.value = '';
  containers.value = [];
  if (!v) return;
  try {
    const resp = await apiRaw(
      `/proxy/${encodeURIComponent(props.clusterId)}/api/v1/namespaces/${encodeURIComponent(v)}/pods?limit=500`,
    );
    const body = (await resp.json()) as {
      items?: { metadata?: { name?: string }; status?: { phase?: string }; spec?: { containers?: { name?: string }[] } }[];
    };
    pods.value = (body.items ?? [])
      .filter((it) => it.status?.phase === 'Running' && it.metadata?.name)
      .map((it) => ({ label: `${it.metadata!.name}`, value: it.metadata!.name!, phase: it.status?.phase ?? '' }))
      .sort((a, b) => a.value.localeCompare(b.value));
    if (!pods.value.length) loadErr.value = '该命名空间下没有 Running 状态的 Pod';
  } catch (e) {
    loadErr.value = `Pod 列表获取失败：${e instanceof Error ? e.message : String(e)}`;
  }
});

watch(pod, async (v) => {
  container.value = '';
  if (!v || !ns.value) return;
  try {
    const resp = await apiRaw(
      `/proxy/${encodeURIComponent(props.clusterId)}/api/v1/namespaces/${encodeURIComponent(ns.value)}/pods/${encodeURIComponent(v)}`,
    );
    const body = (await resp.json()) as { spec?: { containers?: { name?: string }[] } };
    containers.value = (body.spec?.containers ?? [])
      .map((c) => c.name)
      .filter((n): n is string => !!n)
      .map((n) => ({ label: n, value: n }));
    container.value = containers.value[0]?.value ?? '';
  } catch (e) {
    loadErr.value = `容器列表获取失败：${e instanceof Error ? e.message : String(e)}`;
  }
});

function ensureTerm(): Terminal {
  if (term) return term;
  term = new Terminal({
    fontFamily: "'JetBrains Mono', Consolas, monospace",
    fontSize: 13,
    cursorBlink: true,
    theme: { background: '#141b24', foreground: '#d6e2ee' },
    scrollback: 4000,
  });
  fit = new FitAddon();
  term.loadAddon(fit);
  term.open(terminalHost.value!);
  nextTick(() => fit?.fit());
  resizeObs = new ResizeObserver(() => fit?.fit());
  resizeObs.observe(terminalHost.value!);
  term.onResize(({ cols, rows }) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(resizeFrame(cols, rows));
  });
  term.onData((data) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(stdinFrame(data));
  });
  return term;
}

function connect(): void {
  if (!ns.value || !pod.value || !container.value) {
    growl.warning('请先选择命名空间、Pod 与容器');
    return;
  }
  disconnect();
  connState.value = 'connecting';
  const t = ensureTerm();
  t.reset();
  t.writeln(`\x1b[36m$ 连接 ${ns.value}/${pod.value}（容器 ${container.value}）…\x1b[0m`);

  ws = new WebSocket(buildExecUrl({ clusterId: props.clusterId, namespace: ns.value, pod: pod.value, container: container.value, token: getToken() }), [
    EXEC_SUBPROTOCOL,
  ]);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    connState.value = 'open';
    t.writeln('\x1b[32m已连接。\x1b[0m');
    fit?.fit();
  };
  ws.onmessage = (ev) => {
    if (!(ev.data instanceof ArrayBuffer)) return;
    const frame = parseFrame(ev.data);
    if (!frame) return;
    if (frame.channel === 'error') {
      t.writeln(`\x1b[31m${frame.text.trim() || 'exec 被拒绝（可能缺少 pods/exec 权限）'}\x1b[0m`);
      return;
    }
    // tty 模式下 stderr 也走输出流，直接写
    t.write(frame.text);
  };
  ws.onclose = (ev) => {
    connState.value = 'closed';
    const why = ev.reason ? `：${ev.reason}` : ev.code === 1006 ? '（升级被拒绝：无权限 / 集群离线 / 网关中断）' : `（code ${ev.code}）`;
    t.writeln(`\r\n\x1b[33m连接已关闭${why}\x1b[0m`);
  };
  ws.onerror = () => {
    t.writeln('\r\n\x1b[31m连接错误\x1b[0m');
  };
}

function disconnect(): void {
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
  connState.value = 'idle';
}

onBeforeUnmount(() => {
  disconnect();
  resizeObs?.disconnect();
  term?.dispose();
  term = null;
});

onMounted(loadNamespaces);
</script>

<template>
  <div>
    <div class="terminal-pickers">
      <div class="picker">
        <div class="picker-label">命名空间</div>
        <el-select v-model="ns" placeholder="选择…" style="width: 100%">
          <el-option v-for="o in namespaces" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
      </div>
      <div class="picker">
        <div class="picker-label">Pod（仅 Running）</div>
        <el-select v-model="pod" placeholder="选择…" style="width: 100%">
          <el-option v-for="o in pods" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
      </div>
      <div class="picker">
        <div class="picker-label">容器</div>
        <el-select v-model="container" placeholder="选择…" :disabled="!containers.length" style="width: 100%">
          <el-option v-for="o in containers" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
      </div>
      <div class="picker picker-actions">
        <el-button type="primary" :disabled="connState === 'open' || connState === 'connecting'" @click="connect">
          {{ connState === 'connecting' ? '连接中…' : connState === 'open' ? '已连接' : '连接' }}
        </el-button>
        <el-button v-if="connState === 'open'" @click="disconnect">断开</el-button>
      </div>
    </div>

    <Banner v-if="loadErr" type="warning" closable @close="loadErr = ''">{{ loadErr }}</Banner>

    <div class="terminal-frame">
      <div ref="terminalHost" class="terminal-host" />
    </div>
    <p class="terminal-hint">
      数据面直连平台 <code class="mono">/proxy/{{ clusterId }}</code>（v4.channel.k8s.io），
      经 Casbin pods/exec 鉴权后注入平台身份；审计记录创建事件。
    </p>
  </div>
</template>

<style lang="scss" scoped>
.terminal-pickers {
  display: flex;
  gap: 14px;
  margin-bottom: 12px;
  flex-wrap: wrap;
  align-items: flex-end;
}

.picker {
  width: 220px;
}

.picker-label {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 5px;
  color: $text;
}

.picker-actions {
  width: auto;
  padding-bottom: 1px;
}

.terminal-frame {
  background: #141b24;
  border-radius: 8px;
  padding: 6px;
  min-height: 420px;
}

.terminal-host {
  height: 460px;
  width: 100%;
}

.terminal-hint {
  color: $text-muted;
  font-size: 12px;
  margin-top: 8px;
}
</style>
