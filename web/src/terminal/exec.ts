/** exec WebSocket 协议封装（Kubernetes v5.channel.k8s.io）。 */

export const EXEC_SUBPROTOCOL = 'v5.channel.k8s.io';

/** 优先 bash；有 script 时用它争取 tty；否则回退 sh。 */
export const SHELL_SCRIPT =
  'TERM=xterm-256color; export TERM; [ -x /bin/bash ] && ( [ -x /usr/bin/script ] && /usr/bin/script -q -c "/bin/bash" /dev/null || exec /bin/bash ) || exec /bin/sh';

/** Kubernetes exec 的 command 是 argv 数组，需显式通过 /bin/sh -c 执行引导脚本。 */
export const SHELL_ARGV = ['/bin/sh', '-c', SHELL_SCRIPT];

export function buildExecUrl(
  clusterId: string,
  namespace: string,
  pod: string,
  container: string,
  token: string,
): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const params = new URLSearchParams();
  if (container) params.set('container', container);
  for (const arg of SHELL_ARGV) params.append('command', arg);
  params.set('stdin', '1');
  params.set('stdout', '1');
  params.set('stderr', '1');
  params.set('tty', '1');
  // access_token 仅对 WebSocket 升级请求有效（后端 middleware/jwt.go）
  params.set('access_token', token);

  return (
    `${protocol}//${window.location.host}/proxy/${encodeURIComponent(clusterId)}` +
    `/api/v1/namespaces/${encodeURIComponent(namespace)}` +
    `/pods/${encodeURIComponent(pod)}/exec?${params.toString()}`
  );
}

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8');

export function stdinFrame(text: string): Uint8Array {
  const payload = encoder.encode(text);
  const frame = new Uint8Array(payload.length + 1);
  frame[0] = 0;
  frame.set(payload, 1);
  return frame;
}

export function resizeFrame(cols: number, rows: number): Uint8Array {
  // 注意：Go TerminalSize 没有 json tag，字段名必须大写 Width/Height
  const payload = encoder.encode(JSON.stringify({ Width: cols, Height: rows }));
  const frame = new Uint8Array(payload.length + 1);
  frame[0] = 4;
  frame.set(payload, 1);
  return frame;
}

export interface ExecFrame {
  channel: number;
  payload: Uint8Array;
}

export function parseFrame(data: ArrayBuffer): ExecFrame {
  const view = new Uint8Array(data);
  return { channel: view[0] ?? 0, payload: view.slice(1) };
}

export function decodePayload(payload: Uint8Array): string {
  return decoder.decode(payload);
}
