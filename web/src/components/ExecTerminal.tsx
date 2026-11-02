import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  Select,
  Space,
  Tag,
} from 'antd';
import {
  ApiOutlined,
  DisconnectOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { getPod, listNamespaces, listPods } from '@/api/dataplane';
import { getToken } from '@/api/http';
import {
  EXEC_SUBPROTOCOL,
  buildExecUrl,
  decodePayload,
  parseFrame,
  resizeFrame,
  stdinFrame,
} from '@/terminal/exec';

type ConnStatus = 'idle' | 'connecting' | 'open' | 'closed';

const STATUS_META: Record<ConnStatus, { color: string; label: string }> = {
  idle: { color: 'default', label: '未连接' },
  connecting: { color: 'processing', label: '连接中' },
  open: { color: 'success', label: '已连接' },
  closed: { color: 'error', label: '已断开' },
};

export function ExecTerminal({ clusterId }: { clusterId: string }) {
  const { message } = App.useApp();
  const hostRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [pods, setPods] = useState<string[]>([]);
  const [containers, setContainers] = useState<string[]>([]);
  const [namespace, setNamespace] = useState<string>();
  const [pod, setPod] = useState<string>();
  const [container, setContainer] = useState<string>();
  const [nsLoading, setNsLoading] = useState(false);
  const [podLoading, setPodLoading] = useState(false);
  const [status, setStatus] = useState<ConnStatus>('idle');
  const [error, setError] = useState<string | null>(null);

  const writeLine = useCallback((text: string, color = '36') => {
    termRef.current?.writeln(`\x1b[${color}m${text}\x1b[0m`);
  }, []);

  const sendFrame = useCallback((frame: Uint8Array) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(frame);
    }
  }, []);

  // 初始化 xterm
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const term = new Terminal({
      fontFamily: "'JetBrains Mono', Consolas, 'Courier New', monospace",
      fontSize: 13,
      cursorBlink: true,
      scrollback: 4000,
      theme: {
        background: '#0b1522',
        foreground: '#d7e4f3',
        cursor: '#7cb3ff',
        selectionBackground: 'rgba(124, 179, 255, 0.28)',
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);
    termRef.current = term;
    fitRef.current = fit;

    try {
      fit.fit();
    } catch {
      // 容器尚未布局时忽略
    }

    const dataDisposable = term.onData((data) => sendFrame(stdinFrame(data)));
    const resizeDisposable = term.onResize(({ cols, rows }) => {
      sendFrame(resizeFrame(cols, rows));
    });

    const observer = new ResizeObserver(() => {
      try {
        fit.fit();
      } catch {
        // ignore
      }
    });
    observer.observe(host);

    return () => {
      observer.disconnect();
      dataDisposable.dispose();
      resizeDisposable.dispose();
      wsRef.current?.close(1000, 'component unmounted');
      wsRef.current = null;
      term.dispose();
      termRef.current = null;
      fitRef.current = null;
    };
  }, [sendFrame]);

  const loadNamespaces = useCallback(async () => {
    setNsLoading(true);
    setError(null);
    try {
      const list = await listNamespaces(clusterId);
      const names = list.items.map((item) => item.metadata.name).sort((a, b) => a.localeCompare(b));
      setNamespaces(names);
      setNamespace((current) => current ?? names[0]);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载命名空间失败');
    } finally {
      setNsLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    void loadNamespaces();
  }, [loadNamespaces]);

  useEffect(() => {
    if (!namespace) {
      setPods([]);
      setPod(undefined);
      return;
    }
    let cancelled = false;
    setPodLoading(true);
    setError(null);
    setPods([]);
    setPod(undefined);
    setContainers([]);
    setContainer(undefined);
    listPods(clusterId, namespace)
      .then((list) => {
        if (cancelled) return;
        const names = list.items
          .filter((item) => item.status?.phase === 'Running')
          .map((item) => item.metadata.name)
          .sort((a, b) => a.localeCompare(b));
        setPods(names);
        setPod(names[0]);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : '加载 Pod 失败');
      })
      .finally(() => {
        if (!cancelled) setPodLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [clusterId, namespace]);

  useEffect(() => {
    if (!namespace || !pod) {
      setContainers([]);
      setContainer(undefined);
      return;
    }
    let cancelled = false;
    getPod(clusterId, namespace, pod)
      .then((detail) => {
        if (cancelled) return;
        const names = (detail.spec?.containers ?? []).map((c) => c.name);
        setContainers(names);
        setContainer(names[0]);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : '加载 Pod 详情失败');
      });
    return () => {
      cancelled = true;
    };
  }, [clusterId, namespace, pod]);

  const disconnect = useCallback(() => {
    const ws = wsRef.current;
    if (ws) {
      wsRef.current = null;
      ws.close(1000, 'user disconnect');
    }
    setStatus((prev) => (prev === 'idle' ? prev : 'closed'));
  }, []);

  const connect = () => {
    if (!namespace || !pod || !container) {
      message.warning('请先选择命名空间、Pod 与容器');
      return;
    }
    const token = getToken();
    if (!token) {
      message.error('登录状态已失效，请重新登录');
      return;
    }

    disconnect();
    const term = termRef.current;
    term?.reset();
    writeLine(`$ 连接 ${namespace}/${pod}（容器 ${container}）…`);
    setStatus('connecting');
    setError(null);

    const ws = new WebSocket(
      buildExecUrl(clusterId, namespace, pod, container, token),
      [EXEC_SUBPROTOCOL],
    );
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus('open');
      try {
        fitRef.current?.fit();
      } catch {
        // ignore
      }
      term?.focus();
      writeLine('$ 已连接（v5.channel.k8s.io）', '32');
    };

    ws.onmessage = (event: MessageEvent) => {
      if (typeof event.data === 'string') {
        term?.write(event.data);
        return;
      }
      const { channel, payload } = parseFrame(event.data as ArrayBuffer);
      if (channel === 1 || channel === 2) {
        if (payload.length > 0) term?.write(decodePayload(payload));
        return;
      }
      if (channel === 3) {
        const text = decodePayload(payload);
        let statusText = text;
        try {
          const parsed = JSON.parse(text) as {
            status?: string;
            message?: string;
            reason?: string;
            details?: { causes?: { message?: string }[] };
          };
          const exit = parsed.details?.causes?.[0]?.message;
          statusText = [parsed.status, parsed.reason, parsed.message, exit]
            .filter(Boolean)
            .join(' · ');
        } catch {
          // 保留原始文本
        }
        term?.writeln(`\r\n\x1b[31m${statusText || '连接结束'}\x1b[0m`);
      }
    };

    ws.onerror = () => {
      term?.writeln('\r\n\x1b[31m连接错误：可能是无权限、集群离线或网关中断\x1b[0m');
    };

    ws.onclose = (event: CloseEvent) => {
      wsRef.current = null;
      setStatus('closed');
      const reason = event.reason ? ` ${event.reason}` : '';
      if (event.code === 1006) {
        term?.writeln(
          `\r\n\x1b[33m连接已断开（1006）：升级被拒绝，请检查 Casbin exec 权限 / 集群隧道状态\x1b[0m${reason}`,
        );
      } else if (event.code !== 1000) {
        term?.writeln(`\r\n\x1b[33m连接已断开（${event.code}）${reason}\x1b[0m`);
      }
    };
  };

  const meta = STATUS_META[status];
  const canConnect = Boolean(namespace && pod && container) && status !== 'connecting' && status !== 'open';

  return (
    <Card className="soft-card terminal-card">
      <div className="terminal-toolbar">
        <Space wrap>
          <Select
            showSearch
            placeholder="命名空间"
            style={{ width: 190 }}
            value={namespace}
            loading={nsLoading}
            onChange={setNamespace}
            options={namespaces.map((name) => ({ value: name, label: name }))}
            optionFilterProp="label"
          />
          <Select
            showSearch
            placeholder="Pod（仅 Running）"
            style={{ width: 260 }}
            value={pod}
            loading={podLoading}
            onChange={setPod}
            options={pods.map((name) => ({ value: name, label: name }))}
            optionFilterProp="label"
            notFoundContent={podLoading ? '加载中…' : '无 Running Pod'}
          />
          <Select
            placeholder="容器"
            style={{ width: 180 }}
            value={container}
            onChange={setContainer}
            options={containers.map((name) => ({ value: name, label: name }))}
          />
          <Button icon={<ReloadOutlined />} onClick={() => void loadNamespaces()} loading={nsLoading}>
            刷新
          </Button>
        </Space>
        <Space>
          <Tag color={meta.color}>{meta.label}</Tag>
          <Button
            type="primary"
            icon={<ApiOutlined />}
            disabled={!canConnect}
            onClick={connect}
          >
            连接
          </Button>
          <Button
            icon={<DisconnectOutlined />}
            disabled={status !== 'open' && status !== 'connecting'}
            onClick={disconnect}
          >
            断开
          </Button>
        </Space>
      </div>

      {error ? (
        <Alert
          type="error"
          showIcon
          message={error}
          style={{ marginBottom: 12 }}
          action={
            <Button size="small" onClick={() => void loadNamespaces()}>
              重试
            </Button>
          }
        />
      ) : null}

      <div className="terminal-frame">
        <div ref={hostRef} className="terminal-host" />
      </div>
    </Card>
  );
}
