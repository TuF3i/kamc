// 浏览器 exec 终端的 WS 协议封装。
// 平台 /proxy 对 WS 升级按子协议分流：v4/v5.channel.k8s.io → StreamTranslatorHandler
// （翻译为上游 SPDY，与 kubectl exec 同构）。帧格式（v4 二进制模式）：
//   首字节 = 流号（0 stdin / 1 stdout / 2 stderr / 3 error / 4 resize），其余为载荷；
//   resize 帧载荷为 JSON {"Width":cols,"Height":rows}。

export const EXEC_SUBPROTOCOL = 'v4.channel.k8s.io';

const CH_STDIN = 0;
const CH_STDOUT = 1;
const CH_STDERR = 2;
const CH_ERROR = 3;
const CH_RESIZE = 4;

/** 与 Rancher ContainerShell 相同的 shell 引导命令：优先 bash（经 script 争取 tty），回退 sh */
export const SHELL_COMMAND = [
  'TERM=xterm-256color; export TERM; [ -x /bin/bash ] && ([ -x /usr/bin/script ] && /usr/bin/script -q -c "/bin/bash" /dev/null || exec /bin/bash) || exec /bin/sh',
];

export interface ExecTarget {
  clusterId: string;
  namespace: string;
  pod: string;
  container: string;
  /** 平台 JWT（浏览器 WS 无法携带 Authorization 头，经 access_token 查询参数传递） */
  token: string;
}

export function buildExecUrl(t: ExecTarget): string {
  const q = new URLSearchParams();
  q.set('container', t.container);
  for (const c of SHELL_COMMAND) q.append('command', c);
  q.set('stdin', '1');
  q.set('stdout', '1');
  q.set('stderr', '1');
  q.set('tty', '1');
  q.set('access_token', t.token);
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/proxy/${encodeURIComponent(t.clusterId)}/api/v1/namespaces/${encodeURIComponent(
    t.namespace,
  )}/pods/${encodeURIComponent(t.pod)}/exec?${q.toString()}`;
}

const encoder = new TextEncoder();

/** 分配确定以 ArrayBuffer 为底的三元组帧（ws.send 的 BufferSource 类型要求） */
function alloc(n: number): Uint8Array<ArrayBuffer> {
  return new Uint8Array(new ArrayBuffer(n));
}

export function stdinFrame(text: string): Uint8Array<ArrayBuffer> {
  const payload = encoder.encode(text);
  const frame = alloc(payload.length + 1);
  frame[0] = CH_STDIN;
  frame.set(payload, 1);
  return frame;
}

export function resizeFrame(cols: number, rows: number): Uint8Array<ArrayBuffer> {
  const payload = encoder.encode(JSON.stringify({ Width: cols, Height: rows }));
  const frame = alloc(payload.length + 1);
  frame[0] = CH_RESIZE;
  frame.set(payload, 1);
  return frame;
}

export type Channel = 'stdout' | 'stderr' | 'error';

/** 拆分入站二进制帧；stdin/resize 帧不应出现（防御性忽略） */
export function parseFrame(buf: ArrayBuffer): { channel: Channel; text: string } | null {
  const view = new Uint8Array(buf);
  if (view.length < 1) return null;
  const ch = view[0];
  const text = new TextDecoder().decode(view.subarray(1));
  switch (ch) {
    case CH_STDOUT:
      return { channel: 'stdout', text };
    case CH_STDERR:
      return { channel: 'stderr', text };
    case CH_ERROR:
      return { channel: 'error', text };
    default:
      return null;
  }
}
