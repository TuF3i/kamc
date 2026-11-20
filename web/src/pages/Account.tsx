import { For, Show, createSignal, onCleanup, onMount } from 'solid-js';
import { uploadAvatarSelf } from '@/api/users';
import { changePassword } from '@/api/auth';
import { ApiError } from '@/api/http';
import { auth } from '@/stores/auth';
import { toast } from '@/lib/storage/toast';
import { fmtRemaining, fmtTime, remainingSeconds } from '@/utils/format';
import {
  Button,
  Card,
  Descriptions,
  Field,
  PasswordInput,
  Tag,
  Upload,
  type DescriptionItem,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import { PageHeader } from '@/components/PageHeader';
import { UploadOutlined } from '@/lib/icons';

/** 后端业务码 → 中文提示（与 errcode 对应）。 */
function passwordChangeError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 40012:
        return '当前密码不正确';
      case 40013:
        return 'SSO/LDAP 账号的密码由身份源管理，请在对应平台修改';
      case 40014:
        return '当前账号未设置密码，无法修改';
      case 10001:
        return '新密码长度需为 8-72 个字符';
    }
  }
  return err instanceof Error ? err.message : '修改密码失败';
}

interface PasswordForm extends Record<string, unknown> {
  old_password: string;
  new_password: string;
  confirm: string;
}

export default function Account() {
  const [tick, setTick] = createSignal(0);
  const [uploading, setUploading] = createSignal(false);

  const pwdForm = createForm<PasswordForm>({
    old_password: '',
    new_password: '',
    confirm: '',
  });
  const [pwdSaving, setPwdSaving] = createSignal(false);

  const submitPassword = async () => {
    if (
      !pwdForm.validate({
        old_password: [{ required: true, message: '请输入当前密码' }],
        new_password: [
          { required: true, message: '请输入新密码' },
          {
            validator: (value) => {
              const text = String(value ?? '');
              if (text && (text.length < 8 || text.length > 72)) {
                return '新密码长度需为 8-72 个字符';
              }
              return undefined;
            },
          },
        ],
        confirm: [
          { required: true, message: '请再次输入新密码' },
          {
            validator: (value) =>
              String(value ?? '') !== String(pwdForm.values.new_password)
                ? '两次输入的新密码不一致'
                : undefined,
          },
        ],
      })
    ) {
      return;
    }
    setPwdSaving(true);
    try {
      await changePassword(
        String(pwdForm.values.old_password),
        String(pwdForm.values.new_password),
      );
      toast.success('密码已修改，请重新登录');
      // 改密成功后后端已吊销全部 JWT：回到登录页并给出专用提示。
      try {
        sessionStorage.setItem('kamc:password_changed', '1');
      } catch {
        // 隐私模式忽略
      }
      auth.clearLocal();
      window.location.assign('/#/login?password_changed=1');
    } catch (err) {
      toast.error(passwordChangeError(err));
    } finally {
      setPwdSaving(false);
    }
  };

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

      <Show when={auth.profile()?.password_set}>
        <Card title="修改密码" class="soft-card mt-4">
          <div class="max-w-md">
            <div class="mb-4 text-[13px] leading-6 text-layer-content/50">
              修改成功后当前登录将立即失效，所有已登录设备需使用新密码重新登录。
            </div>
            <div class="flex flex-col gap-4">
              <Field label="当前密码" required error={pwdForm.errors.old_password}>
                <PasswordInput
                  value={String(pwdForm.values.old_password)}
                  onValueChange={(value) => pwdForm.setValue('old_password', value)}
                  placeholder="请输入当前密码"
                  autocomplete="current-password"
                  status={pwdForm.errors.old_password ? 'error' : undefined}
                />
              </Field>
              <Field label="新密码" required error={pwdForm.errors.new_password}>
                <PasswordInput
                  value={String(pwdForm.values.new_password)}
                  onValueChange={(value) => pwdForm.setValue('new_password', value)}
                  placeholder="8-72 个字符"
                  autocomplete="new-password"
                  status={pwdForm.errors.new_password ? 'error' : undefined}
                />
              </Field>
              <Field label="确认新密码" required error={pwdForm.errors.confirm}>
                <PasswordInput
                  value={String(pwdForm.values.confirm)}
                  onValueChange={(value) => pwdForm.setValue('confirm', value)}
                  placeholder="再次输入新密码"
                  autocomplete="new-password"
                  status={pwdForm.errors.confirm ? 'error' : undefined}
                />
              </Field>
              <div>
                <Button
                  type="primary"
                  loading={pwdSaving()}
                  onClick={() => void submitPassword()}
                >
                  修改密码
                </Button>
              </div>
            </div>
          </div>
        </Card>
      </Show>
    </div>
  );
}
