import { useCallback, useEffect, useState } from 'react';
import {
  App,
  Avatar,
  Button,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import { DeleteOutlined, EditOutlined, SearchOutlined } from '@ant-design/icons';
import { deleteUser, listUsers, updateUser } from '@/api/users';
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
  const [form] = Form.useForm<{ nickname: string; email: string; groups: string }>();

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

  const openEdit = (user: PlatformUserDTO) => {
    setEditing({ user });
    form.setFieldsValue({
      nickname: user.nickname,
      email: user.email,
      groups: user.groups.join(', '),
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
        groups: values.groups
          ? values.groups.split(',').map((g) => g.trim()).filter(Boolean)
          : [],
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
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索用户 / 邮箱 / 组"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: 260 }}
        />
      </Space>
      <Table<PlatformUserDTO>
        rowKey="uid"
        columns={columns}
        dataSource={filtered}
        loading={loading}
        size="middle"
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个用户` }}
        locale={{ emptyText: <Empty description="暂无用户；用户在 SSO/LDAP 登录后自动创建" /> }}
      />

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
          <Form.Item name="nickname" label="昵称">
            <Input placeholder="显示名" autoComplete="off" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="user@example.com" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="groups"
            label="组"
            extra="逗号分隔；登录同步的组会在下次登录时恢复"
          >
            <Input placeholder="dev-team, qa-team" autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
