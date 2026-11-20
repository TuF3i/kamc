import {
  For,
  Show,
  createMemo,
  createSignal,
  onMount,
} from 'solid-js';
import {
  createUser,
  deleteUser,
  listUsers,
  updateUser,
  uploadAvatar,
} from '@/api/users';
import { auth } from '@/stores/auth';
import type { PlatformUserDTO, UserSource } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Avatar,
  Button,
  Card,
  Empty,
  Field,
  Input,
  Modal,
  Popconfirm,
  Table,
  Tag,
  Upload,
  confirm,
  type TableColumnsType,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  SearchOutlined,
  UploadOutlined,
} from '@/lib/icons';

const SOURCE_META: Record<UserSource, { label: string; level: 'info' | 'primary' | 'neutral' }> = {
  sso: { label: 'SSO', level: 'info' },
  ldap: { label: 'LDAP', level: 'primary' },
  local: { label: '内置', level: 'neutral' },
};

function SourceTag(props: { source: UserSource }) {
  const meta = SOURCE_META[props.source] ?? SOURCE_META.local;
  return <Tag level={meta.level}>{meta.label}</Tag>;
}

/** 用户 Tab：平台用户目录（SSO/LDAP 登录自动同步，可编辑昵称/邮箱/组）。 */
export function UsersTab() {
  const [users, setUsers] = createSignal<PlatformUserDTO[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [search, setSearch] = createSignal('');
  const [editing, setEditing] = createSignal<PlatformUserDTO | null>(null);
  const [saving, setSaving] = createSignal(false);
  const [createOpen, setCreateOpen] = createSignal(false);
  const [uploading, setUploading] = createSignal(false);

  const createUserForm = createForm({
    username: '',
    nickname: '',
    email: '',
    avatar_url: '',
  });
  const editForm = createForm({
    nickname: '',
    email: '',
    avatar_url: '',
  });

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setUsers(await listUsers());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载用户列表失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const submitCreate = async () => {
    if (
      !createUserForm.validate({
        username: [
          { required: true, message: '请输入用户名' },
          { max: 128, message: '用户名最长 128 字符' },
        ],
      })
    ) {
      return;
    }
    setSaving(true);
    try {
      const username = String(createUserForm.values.username).trim();
      await createUser({
        username,
        nickname: String(createUserForm.values.nickname).trim() || undefined,
        email: String(createUserForm.values.email).trim() || undefined,
        avatar_url: String(createUserForm.values.avatar_url).trim() || undefined,
      });
      toast.success(`用户 ${username} 已创建`);
      setCreateOpen(false);
      createUserForm.reset();
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建用户失败');
    } finally {
      setSaving(false);
    }
  };

  const openEdit = (user: PlatformUserDTO) => {
    setEditing(user);
    editForm.reset({
      nickname: user.nickname,
      email: user.email,
      avatar_url: user.avatar_url,
    });
  };

  const submitEdit = async () => {
    const current = editing();
    if (!current) return;
    setSaving(true);
    try {
      await updateUser(current.username, {
        nickname: String(editForm.values.nickname).trim() || undefined,
        email: String(editForm.values.email).trim() || undefined,
        avatar_url: String(editForm.values.avatar_url).trim() || '',
      });
      toast.success(`用户 ${current.username} 已更新`);
      if (current.username === auth.claims()?.sub) void auth.refreshProfile();
      setEditing(null);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '更新用户失败');
    } finally {
      setSaving(false);
    }
  };

  const remove = (user: PlatformUserDTO) => {
    confirm({
      title: `删除用户「${user.username}」？`,
      content:
        '仅删除平台用户记录；该用户已签发的 Token 不会失效，可经踢出功能吊销。SSO/LDAP 用户下次登录会自动重建。',
      okText: '删除',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteUser(user.username);
          toast.success(`用户 ${user.username} 已删除`);
          await load();
        } catch (err) {
          toast.error(err instanceof Error ? err.message : '删除用户失败');
        }
      },
    });
  };

  const filtered = createMemo(() => {
    const keyword = search().trim().toLowerCase();
    if (!keyword) return users();
    return users().filter((user) =>
      [user.username, user.nickname, user.email, user.groups.join(' ')].some((value) =>
        value.toLowerCase().includes(keyword),
      ),
    );
  });

  const columns: TableColumnsType<PlatformUserDTO> = [
    {
      title: '用户',
      key: 'username',
      render: (_value, user) => (
        <span class="inline-flex items-center gap-2.5">
          <Avatar src={user.avatar_url || undefined}>
            {(user.nickname || user.username).slice(0, 1).toUpperCase()}
          </Avatar>
          <span class="min-w-0">
            <span class="block font-bold">{user.nickname || user.username}</span>
            <code class="mono text-[12px] text-layer-content/50">{user.username}</code>
          </span>
        </span>
      ),
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 90,
      filters: [
        { text: 'SSO', value: 'sso' },
        { text: 'LDAP', value: 'ldap' },
        { text: '内置', value: 'local' },
      ],
      onFilter: (value, user) => user.source === String(value),
      render: (_value, user) => <SourceTag source={user.source} />,
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      ellipsis: true,
      render: (value) => String(value ?? '') || <span class="muted">—</span>,
    },
    {
      title: '组',
      key: 'groups',
      render: (_value, user) =>
        user.groups.length ? (
          <span class="flex flex-wrap gap-1">
            <For each={user.groups}>{(group) => <Tag level="primary">{group}</Tag>}</For>
          </span>
        ) : (
          <span class="muted">—</span>
        ),
    },
    {
      title: '最近登录',
      dataIndex: 'last_login',
      width: 170,
      render: (value) =>
        value ? new Date(String(value)).toLocaleString() : <span class="muted">从未</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_value, user) => (
        <div class="table-actions">
          <Button
            type="link"
            size="small"
            icon={<EditOutlined class="h-3.5 w-3.5" />}
            onClick={() => openEdit(user)}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除这个用户？"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(user)}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined class="h-3.5 w-3.5" />}
            >
              删除
            </Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  const uploadEditAvatar = (file: File) => {
    const current = editing();
    if (!current) return false;
    if (file.size > 2 * 1024 * 1024) {
      toast.error('头像文件过大（上限 2MB）');
      return false;
    }
    setUploading(true);
    void uploadAvatar(current.username, file)
      .then((url) => {
        editForm.setValue('avatar_url', url);
        setEditing((prev) => (prev ? { ...prev, avatar_url: url } : prev));
        if (current.username === auth.claims()?.sub) void auth.refreshProfile();
        toast.success('头像已上传');
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : '头像上传失败'))
      .finally(() => setUploading(false));
    return false;
  };

  return (
    <div>
      <Show when={error()}>
        <p class="mb-3 text-sm text-error">{error()}</p>
      </Show>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Button
          type="primary"
          icon={<PlusOutlined class="h-4 w-4" />}
          onClick={() => {
            createUserForm.reset();
            setCreateOpen(true);
          }}
        >
          新建用户
        </Button>
        <Input
          prefix={<SearchOutlined class="h-4 w-4" />}
          placeholder="搜索用户 / 邮箱 / 组"
          value={search()}
          onValueChange={setSearch}
          class="w-[260px]"
        />
      </div>
      <Card class="soft-card" contentClass="p-0">
        <Table<PlatformUserDTO>
          rowKey="uid"
          columns={columns}
          dataSource={filtered()}
          loading={loading()}
          pagination={{ pageSize: 10, showTotal: (total) => `共 ${total} 个用户` }}
          locale={{
            emptyText: <Empty description="暂无用户；用户在 SSO/LDAP 登录后自动创建" class="border-0" />,
          }}
        />
      </Card>

      <Modal
        open={createOpen()}
        title="新建本地用户"
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              loading={saving()}
              onClick={() => void submitCreate()}
            >
              创建
            </Button>
          </>
        }
      >
        <div class="form-stack mt-3">
          <Field
            label="用户名"
            required
            error={createUserForm.errors.username}
            extra="平台内唯一；SSO/LDAP 同名用户后续登录时会接管该账号来源。"
          >
            <Input
              value={String(createUserForm.values.username)}
              onValueChange={(value) => createUserForm.setValue('username', value)}
              placeholder="alice"
              autocomplete="off"
              class="mono"
              status={createUserForm.errors.username ? 'error' : undefined}
            />
          </Field>
          <Field label="昵称">
            <Input
              value={String(createUserForm.values.nickname)}
              onValueChange={(value) => createUserForm.setValue('nickname', value)}
              placeholder="张三"
              autocomplete="off"
            />
          </Field>
          <Field label="邮箱">
            <Input
              value={String(createUserForm.values.email)}
              onValueChange={(value) => createUserForm.setValue('email', value)}
              placeholder="alice@example.com"
              autocomplete="off"
            />
          </Field>
          <Field label="头像 URL">
            <Input
              value={String(createUserForm.values.avatar_url)}
              onValueChange={(value) => createUserForm.setValue('avatar_url', value)}
              placeholder="https://…/avatar.png"
              autocomplete="off"
            />
          </Field>
        </div>
        <p class="mt-3 text-[13px] text-layer-content/50">
          组归属在「组」页签的编辑弹窗中管理。
        </p>
      </Modal>

      <Modal
        open={editing() !== null}
        title={`编辑用户 · ${editing()?.username ?? ''}`}
        onClose={() => setEditing(null)}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setEditing(null)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              loading={saving()}
              onClick={() => void submitEdit()}
            >
              保存
            </Button>
          </>
        }
      >
        <Show when={editing()}>
          {(user) => (
            <div class="form-stack mt-3">
              <Field label="头像">
                <div class="flex items-center gap-3">
                  <Avatar size={56} src={user().avatar_url || undefined}>
                    {(user().nickname || user().username || '?').slice(0, 1).toUpperCase()}
                  </Avatar>
                  <Upload
                    accept="image/png,image/jpeg,image/webp,image/gif"
                    beforeUpload={uploadEditAvatar}
                    disabled={uploading()}
                  >
                    <Button
                      icon={<UploadOutlined class="h-4 w-4" />}
                      loading={uploading()}
                      size="small"
                    >
                      上传头像
                    </Button>
                  </Upload>
                </div>
              </Field>
              <Field
                label="头像 URL"
                extra="可直接编辑；留空则使用默认字母头像"
              >
                <Input
                  value={String(editForm.values.avatar_url)}
                  onValueChange={(value) => editForm.setValue('avatar_url', value)}
                  placeholder="/avatars/…?token=… 或 https://…"
                  autocomplete="off"
                />
              </Field>
              <Field label="昵称">
                <Input
                  value={String(editForm.values.nickname)}
                  onValueChange={(value) => editForm.setValue('nickname', value)}
                  placeholder="显示名"
                  autocomplete="off"
                />
              </Field>
              <Field label="邮箱">
                <Input
                  value={String(editForm.values.email)}
                  onValueChange={(value) => editForm.setValue('email', value)}
                  placeholder="user@example.com"
                  autocomplete="off"
                />
              </Field>
            </div>
          )}
        </Show>
        <p class="mt-3 text-[13px] text-layer-content/50">组归属请到「组」页签编辑。</p>
      </Modal>
    </div>
  );
}
