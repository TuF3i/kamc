import { For, Show, createSignal, onCleanup, onMount } from 'solid-js';
import { uploadAvatarSelf } from '@/api/users';
import { auth } from '@/stores/auth';
import { toast } from '@/lib/storage/toast';
import { fmtRemaining, fmtTime, remainingSeconds } from '@/utils/format';
import {
  Button,
  Card,
  Descriptions,
  Tag,
  Upload,
  type DescriptionItem,
} from '@/lib/widgets';
import { PageHeader } from '@/components/PageHeader';
import { UploadOutlined } from '@/lib/icons';

export default function Account() {
  const [tick, setTick] = createSignal(0);
  const [uploading, setUploading] = createSignal(false);

  onMount(() => {
    const timer = window.setInterval(() => setTick((value) => value + 1), 1000);
    onCleanup(() => window.clearInterval(timer));
  });

  const secondsLeft = () => {
    tick();
    return remainingSeconds(auth.claims()?.exp);
  };

  const uploadOwnAvatar = (file: File) => {
    if (file.size > 2 * 1024 * 1024) {
      toast.error('头像文件过大（上限 2MB）');
      return false;
    }
    setUploading(true);
    void uploadAvatarSelf(file)
      .then(async () => {
        await auth.refreshProfile();
        toast.success('头像已更新');
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : '头像上传失败'))
      .finally(() => setUploading(false));
    return false;
  };

  const ssoSub = () => {
    const value = auth.profile()?.metadata?.oidc_sub;
    return typeof value === 'string' ? value : '';
  };

  const items = (): DescriptionItem[] => {
    const claims = auth.claims();
    const profile = auth.profile();
    const left = secondsLeft();
    return [
      {
        label: '用户名',
        value: <code class="mono strong">{claims?.sub ?? '—'}</code>,
      },
      { label: '昵称', value: profile?.nickname || '—' },
      {
        label: 'SSO 标识 (sub)',
        span: 2,
        value: <code class="mono">{ssoSub() || '—'}</code>,
      },
      { label: '邮箱', value: claims?.email || '—' },
      {
        label: '组',
        span: 2,
        value: (
          <span class="flex flex-wrap gap-1">
            <Show
              when={(claims?.groups ?? []).length > 0}
              fallback={<span>—</span>}
            >
              <For each={claims?.groups ?? []}>
                {(group) => <Tag>{group}</Tag>}
              </For>
            </Show>
          </span>
        ),
      },
      {
        label: '签发者 (iss)',
        value: <code class="mono">{claims?.iss ?? '—'}</code>,
      },
      {
        label: '受众 (aud)',
        value: <code class="mono">{claims?.aud?.join(', ') || '—'}</code>,
      },
      {
        label: '签发时间',
        value: fmtTime(claims ? new Date(claims.iat * 1000).toISOString() : null),
      },
      {
        label: '过期时间',
        value: (
          <span class="inline-flex flex-wrap items-center gap-2">
            {fmtTime(claims ? new Date(claims.exp * 1000).toISOString() : null)}
            <Tag
              level={left <= 0 ? 'error' : left < 600 ? 'warning' : 'success'}
            >
              {left <= 0 ? '已过期' : `剩余 ${fmtRemaining(claims?.exp)}`}
            </Tag>
          </span>
        ),
      },
      {
        label: 'JWT ID',
        value: <code class="mono">{claims?.jti ?? '—'}</code>,
      },
      { label: '吊销 epoch', value: String(claims?.am_ep ?? 0) },
    ];
  };

  return (
    <div>
      <PageHeader
        title="个人中心"
        subtitle="当前登录身份的详细信息"
        tags={
          auth.isAdmin() ? <Tag level="primary">管理员</Tag> : <Tag>普通用户</Tag>
        }
      />

      <Card class="soft-card">
        <div class="profile-hero">
          <div class="profile-avatar">
            <Show
              when={auth.profile()?.avatar_url}
              fallback={(auth.claims()?.sub ?? '?').slice(0, 1).toUpperCase()}
            >
              <img src={auth.profile()!.avatar_url} alt="头像" />
            </Show>
          </div>
          <div>
            <div class="profile-name">
              {auth.profile()?.nickname || auth.claims()?.sub || '—'}
            </div>
            <div class="profile-sub">{auth.claims()?.email || '未设置邮箱'}</div>
            <div class="mt-2">
              <Upload
                accept="image/png,image/jpeg,image/webp,image/gif"
                beforeUpload={uploadOwnAvatar}
                disabled={uploading()}
              >
                <Button
                  size="small"
                  icon={<UploadOutlined class="h-4 w-4" />}
                  loading={uploading()}
                >
                  更换头像
                </Button>
              </Upload>
            </div>
          </div>
        </div>

        <Descriptions items={items()} column={{ xs: 1, md: 2 }} bordered />
      </Card>
    </div>
  );
}
