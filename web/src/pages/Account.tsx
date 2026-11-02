import { useEffect, useState } from 'react';
import { Card, Descriptions, Space, Tag } from 'antd';
import { useAuthStore } from '@/stores/auth';
import { PageHeader } from '@/components/PageHeader';
import { fmtRemaining, fmtTime, remainingSeconds } from '@/utils/format';

export function Account() {
  const claims = useAuthStore((s) => s.claims);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [, setTick] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => setTick((v) => v + 1), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const secondsLeft = remainingSeconds(claims?.exp);

  return (
    <div>
      <PageHeader
        title="个人中心"
        subtitle="当前登录身份的详细信息。"
        tags={isAdmin ? <Tag color="blue">管理员</Tag> : <Tag>普通用户</Tag>}
      />

      <Card className="soft-card">
        <div className="profile-hero">
          <div className="profile-avatar">{(claims?.sub ?? '?').slice(0, 1).toUpperCase()}</div>
          <div>
            <div className="profile-name">{claims?.sub ?? '—'}</div>
            <div className="profile-sub">{claims?.email || '未设置邮箱'}</div>
          </div>
        </div>

        <Descriptions column={{ xs: 1, md: 2 }} size="middle" bordered>
          <Descriptions.Item label="用户 sub">
            <code className="mono strong">{claims?.sub ?? '—'}</code>
          </Descriptions.Item>
          <Descriptions.Item label="邮箱">{claims?.email || '—'}</Descriptions.Item>
          <Descriptions.Item label="组" span={2}>
            <Space size={[4, 4]} wrap>
              {(claims?.groups ?? []).length > 0
                ? claims?.groups?.map((g) => <Tag key={g}>{g}</Tag>)
                : '—'}
            </Space>
          </Descriptions.Item>
          <Descriptions.Item label="签发者 (iss)">
            <code className="mono">{claims?.iss ?? '—'}</code>
          </Descriptions.Item>
          <Descriptions.Item label="受众 (aud)">
            <code className="mono">{claims?.aud?.join(', ') || '—'}</code>
          </Descriptions.Item>
          <Descriptions.Item label="签发时间">
            {fmtTime(claims ? new Date(claims.iat * 1000).toISOString() : null)}
          </Descriptions.Item>
          <Descriptions.Item label="过期时间">
            {fmtTime(claims ? new Date(claims.exp * 1000).toISOString() : null)}
            <Tag
              color={secondsLeft <= 0 ? 'error' : secondsLeft < 600 ? 'warning' : 'success'}
              style={{ marginInlineStart: 8 }}
            >
              {secondsLeft <= 0 ? '已过期' : `剩余 ${fmtRemaining(claims?.exp)}`}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="JWT ID">
            <code className="mono">{claims?.jti ?? '—'}</code>
          </Descriptions.Item>
          <Descriptions.Item label="吊销 epoch">{claims?.am_ep ?? 0}</Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}
