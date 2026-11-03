import { useCallback, useEffect, useState } from 'react';
import {
  App,
  Avatar,
  Button,
  Card,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  type TableColumnsType,
} from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons';
import { createUser, deleteUser, listUsers, updateUser, uploadAvatar } from '@/api/users';
import type { PlatformUserDTO, UserSource } from '@/api/types';

const SOURCE_META: Record<UserSource, { label: string; color: string }> = {
  sso: { label: 'SSO', color: 'purple' },
  ldap: { label: 'LDAP', color: 'geekblue' },
  local: { label: '内置', color: 'default' },
};

function SourceTag({ source }: { source: UserSource }) {
  const meta = SOURCE_META[source] ?? SOURCE_META.local;
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

interface EditState {
  user: PlatformUserDTO;
}

/** 用户 Tab：平台用户目录（SSO/LDAP 登录自动同步，可编辑昵称/邮箱/组）。 */
export function UsersTab() {
  const { message, modal } = App.useApp();
  const [users, setUsers] = useState<PlatformUserDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState<EditState | null>(null);
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm<{ username: string; nickname: string; email: string; avatar_url: string }>();
  const [form] = Form.useForm<{ nickname: string; email: string; avatar_url: string }>();
  const [uploading, setUploading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setUsers(await listUsers());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载用户列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const submitCreate = async () => {
    const values = await createForm.validateFields();
    setSaving(true);
    try {
      await createUser({
        username: values.username.trim(),
        nickname: values.nickname?.trim() || undefined,
        email: values.email?.trim() || undefined,
        avatar_url: values.avatar_url?.trim() || undefined,
      });
      message.success(`用户 ${values.username.trim()} 已创建`);
      setCreateOpen(false);
      createForm.resetFields();
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建用户失败');
    } finally {
      setSaving(false);
    }
  };

  const openEdit = (user: PlatformUserDTO) => {
    setEditing({ user });
    form.setFieldsValue({
      nickname: user.nickname,
      email: user.email,
      avatar_url: user.avatar_url,
    });
  };

  const submitEdit = async () => {
    if (!editing) return;
    const values = await form.validateFields();
    setSaving(true);
    try {
      await updateUser(editing.user.username, {
        nickname: values.nickname?.trim() || undefined,
        email: values.email?.trim() || undefined,
        avatar_url: values.avatar_url?.trim() || '',
      });
      message.success(`用户 ${editing.user.username} 已更新`);
      setEditing(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '更新用户失败');
    } finally {
      setSaving(false);
    }
  };

  const remove = (user: PlatformUserDTO) => {
    modal.confirm({
      title: `删除用户「${user.username}」？`,
      content: '仅删除平台用户记录；该用户已签发的 Token 不会失效，可经踢出功能吊销。SSO/LDAP 用户下次登录会自动重建。',
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteUser(user.username);
          message.success(`用户 ${user.username} 已删除`);
          await load();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除用户失败');
        }
      },
    });
  };

  const filtered = users.filter((u) => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return true;
    return [u.username, u.nickname, u.email, u.groups.join(' ')].some((v) =>
      v.toLowerCase().includes(keyword),
    );
  });

  const columns: TableColumnsType<PlatformUserDTO> = [
    {
      title: '用户',
      key: 'username',
      render: (_, u) => (
        <Space size={10}>
          {u.avatar_url ? (
            <Avatar size={30} src={u.avatar_url} />
          ) : (
            <Avatar size={30} className="user-avatar">{(u.nickname || u.username).slice(0, 1).toUpperCase()}</Avatar>
          )}
          <div>
            <div className="strong">{u.nickname || u.username}</div>
            <code className="mono" style={{ fontSize: 12, color: 'var(--kamc-muted)' }}>{u.username}</code>
          </div>
        </Space>
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
      onFilter: (value, u) => u.source === value,
      render: (source: UserSource) => <SourceTag source={source} />,
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      ellipsis: true,
      render: (email: string) => email || <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: '组',
      key: 'groups',
      render: (_, u) =>
        u.groups.length ? (
          <Space size={[4, 4]} wrap>
            {u.groups.map((g) => (
              <Tag key={g} color="blue">{g}</Tag>
            ))}
          </Space>
        ) : (
          <Typography.Text type="secondary">—</Typography.Text>
        ),
    },
    {
      title: '最近登录',
      dataIndex: 'last_login',
      width: 170,
      render: (v?: string | null) => (v ? new Date(v).toLocaleString() : <Typography.Text type="secondary">从未</Typography.Text>),
    },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_, u) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(u)}>
            编辑
          </Button>
          <Popconfirm
            title="删除这个用户？"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(u)}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {error ? (
        <Typography.Paragraph type="danger">{error}</Typography.Paragraph>
      ) : null}
      <Space style={{ marginBottom: 12 }}>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            createForm.resetFields();
            setCreateOpen(true);
          }}
        >
          新建用户
        </Button>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索用户 / 邮箱 / 组"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: 260 }}
        />
      </Space>
      <Card className="soft-card" styles={{ body: { padding: 0 } }}>
      <Table<PlatformUserDTO>
        rowKey="uid"
        columns={columns}
        dataSource={filtered}
        loading={loading}
        size="middle"
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个用户` }}
        locale={{ emptyText: <Empty description="暂无用户；用户在 SSO/LDAP 登录后自动创建" /> }}
      />
      </Card>

      <Modal
        title="新建本地用户"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void submitCreate()}
        okText="创建"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={createForm} layout="vertical" style={{ marginTop: 12 }} preserve={false}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[
              { required: true, message: '请输入用户名' },
              { max: 128, message: '用户名最长 128 字符' },
            ]}
            extra="平台内唯一；SSO/LDAP 同名用户后续登录时会接管该账号来源。"
          >
            <Input placeholder="alice" autoComplete="off" className="mono" />
          </Form.Item>
          <Form.Item name="nickname" label="昵称">
            <Input placeholder="张三" autoComplete="off" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="alice@example.com" autoComplete="off" />
          </Form.Item>
          <Form.Item name="avatar_url" label="头像 URL">
            <Input placeholder="https://…/avatar.png" autoComplete="off" />
          </Form.Item>
        </Form>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          组归属在「组」页签的编辑弹窗中管理。
        </Typography.Paragraph>
      </Modal>

      <Modal
        title={`编辑用户 · ${editing?.user.username ?? ''}`}
        open={editing !== null}
        onCancel={() => setEditing(null)}
        onOk={() => void submitEdit()}
        okText="保存"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item label="头像">
            <Space align="center" size={12}>
              {editing?.user.avatar_url ? (
                <Avatar size={56} src={editing.user.avatar_url} />
              ) : (
                <Avatar size={56} className="user-avatar">
                  {(editing?.user.nickname || editing?.user.username || '?').slice(0, 1).toUpperCase()}
                </Avatar>
              )}
              <Upload
                accept="image/png,image/jpeg,image/webp,image/gif"
                showUploadList={false}
                beforeUpload={(file) => {
                  if (file.size > 2 * 1024 * 1024) {
                    message.error('头像文件过大（上限 2MB）');
                    return Upload.LIST_IGNORE;
                  }
                  setUploading(true);
                  void uploadAvatar(editing!.user.username, file)
                    .then((url) => {
                      form.setFieldValue('avatar_url', url);
                      setEditing((prev) =>
                        prev ? { user: { ...prev.user, avatar_url: url } } : prev,
                      );
                      message.success('头像已上传');
                    })
                    .catch((err) =>
                      message.error(err instanceof Error ? err.message : '头像上传失败'),
                    )
                    .finally(() => setUploading(false));
                  return false; // 阻止 antd 内置上传
                }}
              >
                <Button icon={<UploadOutlined />} loading={uploading} size="small">
                  上传头像
                </Button>
              </Upload>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                PNG/JPEG/WebP/GIF，≤2MB；上传后生成带 token 的平台 URL
              </Typography.Text>
            </Space>
          </Form.Item>
          <Form.Item
            name="avatar_url"
            label="头像 URL"
            extra="可直接编辑；留空则使用默认字母头像"
          >
            <Input placeholder="/avatars/…?token=… 或 https://…" autoComplete="off" />
          </Form.Item>
          <Form.Item name="nickname" label="昵称">
            <Input placeholder="显示名" autoComplete="off" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="user@example.com" autoComplete="off" />
          </Form.Item>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            组归属请到「组」页签编辑。
          </Typography.Paragraph>
        </Form>
      </Modal>
    </div>
  );
}
