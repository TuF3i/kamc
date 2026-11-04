import { useCallback, useEffect, useMemo, useState } from 'react';
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
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  TeamOutlined,
  UserAddOutlined,
  UserOutlined,
} from '@ant-design/icons';
import {
  addGroupMember,
  createGroup,
  deleteGroup,
  listGroups,
  listUsers,
  removeGroupMember,
  updateGroup,
} from '@/api/users';
import type { GroupDTO, PlatformUserDTO } from '@/api/types';

/** 组 Tab：组 CRUD + 成员管理（成员关系存于用户表 groups 字段）。 */
export function GroupsTab() {
  const { message } = App.useApp();
  const [groups, setGroups] = useState<GroupDTO[]>([]);
  const [users, setUsers] = useState<PlatformUserDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form] = Form.useForm<{ name: string; desc: string; is_admin: boolean }>();

  const [editing, setEditing] = useState<GroupDTO | null>(null);
  const [editSaving, setEditSaving] = useState(false);
  const [memberBusy, setMemberBusy] = useState(false);
  const [memberToAdd, setMemberToAdd] = useState<string>();
  const [editForm] = Form.useForm<{ name: string; desc: string; is_admin: boolean }>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [groupList, userList] = await Promise.all([listGroups(), listUsers()]);
      setGroups(groupList);
      setUsers(userList);
      // 编辑弹窗打开时同步刷新其数据（成员变更后保持最新）
      setEditing((prev) => (prev ? groupList.find((g) => g.name === prev.name) ?? null : prev));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载组列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const submitCreate = async () => {
    const values = await form.validateFields();
    setCreating(true);
    try {
      await createGroup(values.name.trim(), values.desc?.trim() || '', values.is_admin === true);
      message.success(`组 ${values.name.trim()} 已创建`);
      setCreateOpen(false);
      form.resetFields();
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建组失败');
    } finally {
      setCreating(false);
    }
  };

  const openEdit = (g: GroupDTO) => {
    setEditing(g);
    setMemberToAdd(undefined);
    editForm.setFieldsValue({ name: g.name, desc: g.desc, is_admin: g.is_admin });
  };

  const submitEdit = async () => {
    if (!editing) return;
    const values = await editForm.validateFields();
    setEditSaving(true);
    try {
      const updated = await updateGroup(editing.name, {
        name: values.name.trim(),
        desc: values.desc?.trim() || '',
        is_admin: values.is_admin === true,
      });
      message.success(`组 ${updated.name} 已更新`);
      setEditing(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '更新组失败');
    } finally {
      setEditSaving(false);
    }
  };

  const addMember = async () => {
    if (!editing || !memberToAdd) return;
    setMemberBusy(true);
    try {
      await addGroupMember(editing.name, memberToAdd);
      message.success(`${memberToAdd} 已加入组`);
      setMemberToAdd(undefined);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加成员失败');
    } finally {
      setMemberBusy(false);
    }
  };

  const removeMember = async (username: string) => {
    if (!editing) return;
    setMemberBusy(true);
    try {
      await removeGroupMember(editing.name, username);
      message.success(`${username} 已移出组`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '移除成员失败');
    } finally {
      setMemberBusy(false);
    }
  };

  const remove = async (g: GroupDTO) => {
    try {
      await deleteGroup(g.name);
      message.success(`组 ${g.name} 已删除`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除组失败');
    }
  };

  // 候选成员：不在当前组内的用户
  const candidateUsers = useMemo(() => {
    if (!editing) return [];
    const inGroup = new Set(editing.members);
    return users.filter((u) => !inGroup.has(u.username));
  }, [editing, users]);

  const columns: TableColumnsType<GroupDTO> = [
    {
      title: '组名',
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (name: string, g) => (
        <Space size={8}>
          <TeamOutlined style={{ color: '#7cb3ff' }} />
          <code className="mono strong">{name}</code>
          {g.is_admin ? <Tag color="gold">平台管理员</Tag> : null}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'desc',
      ellipsis: true,
      render: (desc: string) => desc || <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 100,
      render: (source: string) => (
        <Tag color={source === 'ldap' ? 'geekblue' : source === 'local' ? 'default' : 'purple'}>
          {source === 'ldap' ? 'LDAP' : source === 'local' ? '手动' : 'SSO'}
        </Tag>
      ),
    },
    {
      title: '成员',
      key: 'members',
      render: (_, g) =>
        g.members.length ? (
          <Space size={[4, 4]} wrap>
            {g.members.map((m) => (
              <Tag key={m} icon={<UserOutlined />}>{m}</Tag>
            ))}
          </Space>
        ) : (
          <Typography.Text type="secondary">无成员</Typography.Text>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 170,
      render: (_, g) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(g)}>
            编辑
          </Button>
          <Popconfirm
            title={`删除组「${g.name}」？`}
            description="用户归属关系不受影响；引用该组的策略需另行清理。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => void remove(g)}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {error ? <Typography.Paragraph type="danger">{error}</Typography.Paragraph> : null}
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          创建组
        </Button>
      </Space>
      <Card className="soft-card" styles={{ body: { padding: 0 } }}>
        <Table<GroupDTO>
          rowKey="name"
          columns={columns}
          dataSource={groups}
          loading={loading}
          size="middle"
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个组` }}
          locale={{ emptyText: <Empty description="暂无组；SSO/LDAP 登录的组会自动同步" /> }}
        />
      </Card>

      <Modal
        title="创建组"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void submitCreate()}
        okText="创建"
        cancelText="取消"
        confirmLoading={creating}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }} preserve={false}>
          <Form.Item
            name="name"
            label="组名"
            rules={[
              { required: true, message: '请输入组名' },
              { max: 128, message: '组名最长 128 字符' },
            ]}
          >
            <Input placeholder="dev-team" autoComplete="off" />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea placeholder="可选" rows={3} maxLength={500} />
          </Form.Item>
          <Form.Item name="is_admin" label="平台管理员" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editing ? `编辑组 · ${editing.name}` : '编辑组'}
        open={editing !== null}
        onCancel={() => setEditing(null)}
        onOk={() => void submitEdit()}
        okText="保存"
        cancelText="取消"
        confirmLoading={editSaving}
        destroyOnHidden
        width={640}
      >
        {editing ? (
          <>
            <Form form={editForm} layout="vertical" style={{ marginTop: 12 }}>
              <Form.Item
                name="name"
                label="组名"
                rules={[
                  { required: true, message: '请输入组名' },
                  { max: 128, message: '组名最长 128 字符' },
                ]}
                extra="重命名会同步所有用户的组字段；引用旧组名的策略（org:<旧名>）需手动调整。"
              >
                <Input className="mono" autoComplete="off" />
              </Form.Item>
              <Form.Item name="desc" label="描述">
                <Input.TextArea rows={2} maxLength={500} />
              </Form.Item>
              <Form.Item name="is_admin" label="平台管理员" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Form>

            <Typography.Text strong>成员（{editing.members.length}）</Typography.Text>
            <div style={{ margin: '8px 0 12px', display: 'grid', gap: 6 }}>
              {editing.members.length === 0 ? (
                <Typography.Text type="secondary">暂无成员</Typography.Text>
              ) : (
                editing.members.map((m) => {
                  const user = users.find((u) => u.username === m);
                  return (
                    <div
                      key={m}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        padding: '4px 8px',
                        border: '1px solid var(--kamc-border-soft)',
                        borderRadius: 8,
                      }}
                    >
                      {user?.avatar_url ? (
                        <Avatar size={24} src={user.avatar_url} />
                      ) : (
                        <Avatar size={24} className="user-avatar">
                          {(user?.nickname || m).slice(0, 1).toUpperCase()}
                        </Avatar>
                      )}
                      <span className="strong">{user?.nickname || m}</span>
                      <code className="mono" style={{ fontSize: 12, color: 'var(--kamc-muted)' }}>{m}</code>
                      <span style={{ flex: 1 }} />
                      <Button
                        type="link"
                        size="small"
                        danger
                        loading={memberBusy}
                        onClick={() => void removeMember(m)}
                      >
                        移出
                      </Button>
                    </div>
                  );
                })
              )}
            </div>

            <Space.Compact style={{ width: '100%' }}>
              <Select
                style={{ flex: 1 }}
                showSearch
                allowClear
                placeholder="选择要添加的用户"
                value={memberToAdd}
                onChange={setMemberToAdd}
                optionFilterProp="label"
                options={candidateUsers.map((u) => ({
                  value: u.username,
                  label: `${u.nickname || u.username}（${u.username}）`,
                }))}
              />
              <Button
                type="primary"
                icon={<UserAddOutlined />}
                loading={memberBusy}
                disabled={!memberToAdd}
                onClick={() => void addMember()}
              >
                添加用户
              </Button>
            </Space.Compact>
          </>
        ) : null}
      </Modal>
    </div>
  );
}
