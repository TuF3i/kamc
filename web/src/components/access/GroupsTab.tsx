import { useCallback, useEffect, useState } from 'react';
import {
  App,
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
  type TableColumnsType,
} from 'antd';
import { DeleteOutlined, PlusOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons';
import { createGroup, deleteGroup, listGroups } from '@/api/users';
import type { GroupDTO } from '@/api/types';

/** 组 Tab：平台组聚合视图（成员来自用户表 groups 字段）。 */
export function GroupsTab() {
  const { message } = App.useApp();
  const [groups, setGroups] = useState<GroupDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form] = Form.useForm<{ name: string; desc: string }>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setGroups(await listGroups());
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
      await createGroup(values.name.trim(), values.desc?.trim() || '');
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

  const remove = async (g: GroupDTO) => {
    try {
      await deleteGroup(g.name);
      message.success(`组 ${g.name} 已删除`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除组失败');
    }
  };

  const columns: TableColumnsType<GroupDTO> = [
    {
      title: '组名',
      dataIndex: 'name',
      key: 'name',
      width: 220,
      render: (name: string) => (
        <Space size={8}>
          <TeamOutlined style={{ color: '#7cb3ff' }} />
          <code className="mono strong">{name}</code>
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
      width: 100,
      render: (_, g) => (
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
            extra="策略主体 org:<组名> 中的组名"
          >
            <Input placeholder="dev-team" autoComplete="off" />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea placeholder="可选" rows={3} maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
