import { For, Show, type JSX } from 'solid-js';
import type { AuditEvent } from '@/api/types';
import { Descriptions, Drawer, Tag, type DescriptionItem } from '@/lib/widgets';
import { CopyButton } from '@/components/CopyButton';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';
import { SpinnerIcon } from '@/lib/icons';

/** 地理信息展示：国家 · 地区 · 城市（空 = —） */
function GeoText(props: { record: AuditEvent }) {
  const parts = [props.record.geo_country, props.record.geo_region, props.record.geo_city].filter(
    Boolean,
  );
  return parts.length > 0 ? (
    <span>{parts.join(' · ')}</span>
  ) : (
    <span class="muted">—</span>
  );
}

/** JSON 行内语法高亮：key / 字符串 / 数字 / 布尔 分色 */
const JSON_TOKEN =
  /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;

function highlightJSONLine(line: string): JSX.Element {
  const nodes: JSX.Element[] = [];
  let last = 0;
  JSON_TOKEN.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = JSON_TOKEN.exec(line)) !== null) {
    if (match.index > last) nodes.push(line.slice(last, match.index));
    if (match[1] !== undefined) {
      const isKey = match[2] !== undefined;
      nodes.push(<span class={isKey ? 'json-key' : 'json-string'}>{match[1]}</span>);
      if (isKey) nodes.push(match[2]);
    } else if (match[3] !== undefined) {
      nodes.push(<span class="json-bool">{match[3]}</span>);
    } else {
      nodes.push(<span class="json-number">{match[4]}</span>);
    }
    last = JSON_TOKEN.lastIndex;
  }
  if (last < line.length) nodes.push(line.slice(last));
  return <div>{nodes.length ? nodes : ' '}</div>;
}

/** 完整 HTTP 报文高亮：起始行 / 头名值 / JSON body 分别着色。 */
function HighlightedDump(props: { text: string }) {
  const sep = () => props.text.indexOf('\r\n\r\n');
  const head = () => (sep() >= 0 ? props.text.slice(0, sep()) : props.text);
  const body = () => (sep() >= 0 ? props.text.slice(sep() + 4) : '');
  const headLines = () => head().split(/\r?\n/);

  return (
    <>
      <For each={headLines()}>
        {(line, index) => {
          if (index() === 0) {
            return <div class="http-start">{line || ' '}</div>;
          }
          const colon = line.indexOf(':');
          if (colon > 0) {
            return (
              <div class="http-header">
                <span class="http-header-key">{line.slice(0, colon)}</span>
                <span class="http-header-value">{line.slice(colon)}</span>
              </div>
            );
          }
          return <div class={line.startsWith('…') ? 'dim' : undefined}>{line || ' '}</div>;
        }}
      </For>
      <Show when={sep() >= 0}>
        <For each={body().split(/\r?\n/)}>
          {(line) =>
            line.startsWith('(') || line.startsWith('…') ? (
              <div class="dim">{line || ' '}</div>
            ) : (
              highlightJSONLine(line)
            )
          }
        </For>
      </Show>
    </>
  );
}

function AuditBody(props: { title: string; body?: string }) {
  return (
    <Show
      when={props.body}
      fallback={
        <div class="mt-3 text-xs text-layer-content/50">{props.title}：（空）</div>
      }
    >
      {(body) => (
        <div class="mt-3">
          <div class="mb-1 flex items-center gap-2">
            <span class="text-xs text-layer-content/50">
              {props.title}（{body().length} 字符）
            </span>
            <CopyButton text={body()} label="复制" size="small" />
          </div>
          <pre class="code-block audit-body">
            <HighlightedDump text={body()} />
          </pre>
        </div>
      )}
    </Show>
  );
}

/** 事件详情抽屉：从右侧滑出，展示完整对端上下文与原始 HTTP 报文。 */
export function AuditDetailDrawer(props: {
  event: AuditEvent | null;
  loading?: boolean;
  onClose: () => void;
}) {
  const isCluster = () => props.event?.kind === 'cluster';

  const items = (): DescriptionItem[] => {
    const record = props.event;
    if (!record) return [];
    return [
      { label: '事件 ID', value: String(record.id) },
      {
        label: '事件',
        value: <code class="mono">{record.event || '—'}</code>,
      },
      {
        label: '用户',
        value: <code class="mono">{record.user || '—'}</code>,
      },
      {
        label: '组',
        value: record.groups ? (
          <span class="flex flex-wrap gap-1">
            <For each={record.groups.split(',')}>{(group) => <Tag>{group}</Tag>}</For>
          </span>
        ) : (
          '—'
        ),
      },
      {
        label: '对端 IP',
        value: <code class="mono">{record.source_ip || '—'}</code>,
      },
      {
        label: 'X-Forwarded-For',
        value: <code class="mono">{record.forwarded_for || '—'}</code>,
      },
      { label: '地理信息', value: <GeoText record={record} /> },
      {
        label: 'User-Agent',
        value: <span class="break-all">{record.user_agent || '—'}</span>,
      },
      { label: '方法', value: record.method || '—' },
      {
        label: '耗时',
        value: record.latency_ms ? `${record.latency_ms} ms` : '—',
      },
      {
        label: '集群',
        value: record.cluster ? <code class="mono">{record.cluster}</code> : '—',
      },
      {
        label: '动作 / 鉴权对象',
        value: (
          <span class="inline-flex flex-wrap items-center gap-2">
            <Show when={record.verb}>
              <Tag level="primary">{record.verb}</Tag>
            </Show>
            <Show
              when={record.object}
              fallback={<Show when={!record.verb}>—</Show>}
            >
              <code class="mono">{record.object}</code>
            </Show>
          </span>
        ),
      },
      {
        label: 'HTTP 状态',
        value: record.status_code ? <Tag>{record.status_code}</Tag> : '—',
      },
      {
        label: '路径',
        value: <code class="mono break-all">{record.path || '—'}</code>,
      },
      { label: '结果说明', value: <ReasonTag reason={record.reason} /> },
    ];
  };

  return (
    <Drawer
      open={props.event !== null}
      onClose={props.onClose}
      width={780}
      title={
        <Show
          when={props.event}
          fallback="事件详情"
        >
          {(event) => (
            <span class="inline-flex flex-wrap items-center gap-2">
              <Tag level={isCluster() ? 'info' : 'primary'}>
                {isCluster() ? '集群操作' : '平台事件'}
              </Tag>
              <code class="mono strong">{event().event || '事件详情'}</code>
              <AuditResultTag allowed={event().allowed} />
            </span>
          )}
        </Show>
      }
    >
      <Show when={props.loading}>
        <div class="mb-3 flex items-center gap-2">
          <SpinnerIcon class="h-4 w-4 animate-spin text-primary" />
        </div>
      </Show>
      <Show when={props.event}>
        {(event) => (
          <>
            <Descriptions items={items()} column={1} />
            <Show when={isCluster() || event().request_body}>
              <AuditBody title="请求报文" body={event().request_body} />
            </Show>
            <Show when={isCluster() || event().response_body}>
              <AuditBody title="响应报文" body={event().response_body} />
            </Show>
          </>
        )}
      </Show>
    </Drawer>
  );
}
