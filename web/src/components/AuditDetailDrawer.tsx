import { type ReactNode } from 'react';
import { Descriptions, Drawer, Space, Spin, Tag, Typography } from 'antd';
import type { AuditEvent } from '@/api/types';
import { CopyButton } from '@/components/CopyButton';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';

/** 地理信息展示：国家 · 地区 · 城市（空 = —） */
function GeoText({ record }: { record: AuditEvent }) {
  const parts = [record.geo_country, record.geo_region, record.geo_city].filter(Boolean);
  return parts.length > 0 ? <span>{parts.join(' · ')}</span> : <Typography.Text type="secondary">—</Typography.Text>;
}

function metaItems(record: AuditEvent) {
  return [
    <Descriptions.Item label="事件 ID" key="id">{record.id}</Descriptions.Item>,
    <Descriptions.Item label="事件" key="event"><code className="mono">{record.event || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="用户" key="user"><code className="mono">{record.user || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="组" key="groups">
      {record.groups ? record.groups.split(',').map((g) => <Tag key={g}>{g}</Tag>) : '—'}
    </Descriptions.Item>,
    <Descriptions.Item label="对端 IP" key="ip"><code className="mono">{record.source_ip || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="X-Forwarded-For" key="xff">
      <code className="mono">{record.forwarded_for || '—'}</code>
    </Descriptions.Item>,
    <Descriptions.Item label="地理信息" key="geo"><GeoText record={record} /></Descriptions.Item>,
    <Descriptions.Item label="User-Agent" key="ua">
      <span style={{ wordBreak: 'break-all' }}>{record.user_agent || '—'}</span>
    </Descriptions.Item>,
    <Descriptions.Item label="方法" key="method">{record.method || '—'}</Descriptions.Item>,
    <Descriptions.Item label="耗时" key="lat">
      {record.latency_ms ? `${record.latency_ms} ms` : '—'}
    </Descriptions.Item>,
  ];
}

/** JSON 行内语法高亮：key / 字符串 / 数字 / 布尔 分色 */
const JSON_TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;

function highlightJSONLine(line: string, rowKey: number): ReactNode {
  const nodes: ReactNode[] = [];
  let last = 0;
  let seq = 0;
  JSON_TOKEN.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = JSON_TOKEN.exec(line)) !== null) {
    if (m.index > last) nodes.push(line.slice(last, m.index));
    if (m[1] !== undefined) {
      const isKey = m[2] !== undefined;
      nodes.push(
        <span key={`s${rowKey}-${seq++}`} className={isKey ? 'json-key' : 'json-string'}>
          {m[1]}
        </span>,
      );
      if (isKey) nodes.push(m[2]);
    } else if (m[3] !== undefined) {
      nodes.push(<span key={`b${rowKey}-${seq++}`} className="json-bool">{m[3]}</span>);
    } else {
      nodes.push(<span key={`n${rowKey}-${seq++}`} className="json-number">{m[4]}</span>);
    }
    last = JSON_TOKEN.lastIndex;
  }
  if (last < line.length) nodes.push(line.slice(last));
  return <div key={rowKey}>{nodes.length ? nodes : ' '}</div>;
}

/** 完整 HTTP 报文高亮：起始行 / 头名值 / JSON body 分别着色。 */
function HighlightedDump({ text }: { text: string }) {
  const sep = text.indexOf('\r\n\r\n');
  const head = sep >= 0 ? text.slice(0, sep) : text;
  const body = sep >= 0 ? text.slice(sep + 4) : '';
  const headLines = head.split(/\r?\n/);
  return (
    <>
      {headLines.map((line, i) => {
        if (i === 0) {
          return <div key={`h${i}`} className="http-start">{line || ' '}</div>;
        }
        const idx = line.indexOf(':');
        if (idx > 0) {
          return (
            <div key={`h${i}`} className="http-header">
              <span className="http-header-key">{line.slice(0, idx)}</span>
              <span className="http-header-value">{line.slice(idx)}</span>
            </div>
          );
        }
        return <div key={`h${i}`} className={line.startsWith('…') ? 'dim' : undefined}>{line || ' '}</div>;
      })}
      {sep >= 0
        ? body.split(/\r?\n/).map((line, i) =>
            line.startsWith('(') || line.startsWith('…') ? (
              <div key={`j${i}`} className="dim">{line || ' '}</div>
            ) : (
              highlightJSONLine(line, i)
            ),
          )
        : null}
    </>
  );
}

function AuditBody({ title, body }: { title: string; body?: string }) {
  if (!body) {
    return (
      <div style={{ marginTop: 12, color: 'var(--kamc-muted)', fontSize: 12 }}>
        {title}：（空）
      </div>
    );
  }
  return (
    <div style={{ marginTop: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <Typography.Text type="secondary">
          {title}（{body.length} 字符）
        </Typography.Text>
        <CopyButton text={body} label="复制" size="small" />
      </div>
      <pre className="code-block audit-body">
        <HighlightedDump text={body} />
      </pre>
    </div>
  );
}

/** 事件详情抽屉：从右侧滑出，展示完整对端上下文与原始 HTTP 报文。 */
export function AuditDetailDrawer({
  event,
  loading = false,
  onClose,
}: {
  event: AuditEvent | null;
  loading?: boolean;
  onClose: () => void;
}) {
  const isCluster = event?.kind === 'cluster';
  return (
    <Drawer
      open={event !== null}
      onClose={onClose}
      width={780}
      title={
        event ? (
          <Space size={8} wrap>
            <Tag color={isCluster ? 'geekblue' : 'blue'}>{isCluster ? '集群操作' : '平台事件'}</Tag>
            <code className="mono strong">{event.event || '事件详情'}</code>
            <AuditResultTag allowed={event.allowed} />
          </Space>
        ) : (
          '事件详情'
        )
      }
      styles={{ body: { padding: 20 } }}
      destroyOnHidden
    >
      <Spin spinning={loading}>
      {event ? (
        <>
          <Descriptions size="small" column={1} bordered>
            {metaItems(event)}
            <Descriptions.Item label="集群">
              {event.cluster ? <code className="mono">{event.cluster}</code> : '—'}
            </Descriptions.Item>
            <Descriptions.Item label="动作 / 鉴权对象">
              {event.verb ? <Tag color="blue">{event.verb}</Tag> : null}
              {event.object ? (
                <code className="mono" style={{ marginInlineStart: event.verb ? 8 : 0 }}>
                  {event.object}
                </code>
              ) : !event.verb ? (
                '—'
              ) : null}
            </Descriptions.Item>
            <Descriptions.Item label="HTTP 状态">
              {event.status_code ? <Tag>{event.status_code}</Tag> : '—'}
            </Descriptions.Item>
            <Descriptions.Item label="路径">
              <code className="mono" style={{ wordBreak: 'break-all' }}>
                {event.path || '—'}
              </code>
            </Descriptions.Item>
            <Descriptions.Item label="结果说明">
              <ReasonTag reason={event.reason} />
            </Descriptions.Item>
          </Descriptions>
          {isCluster || event.request_body ? (
            <AuditBody title="请求报文" body={event.request_body} />
          ) : null}
          {isCluster || event.response_body ? (
            <AuditBody title="响应报文" body={event.response_body} />
          ) : null}
        </>
      ) : null}
      </Spin>
    </Drawer>
  );
}
