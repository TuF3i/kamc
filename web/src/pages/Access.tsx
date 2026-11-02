import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  App,
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import {
  DeleteOutlined,
  PlusOutlined,
  SearchOutlined,
  UserDeleteOutlined,
} from '@ant-design/icons';
import {
  addBinding,
  addPolicy,
  deleteBinding,
  deletePolicy,
  listBindings,
  listPolicies,
  revokeUser,
  type AddBindingInput,
  type AddPolicyInput,
} from '@/api/access';
import { createRole, deleteRole, listRoles } from '@/api/roles';
import { listClusters } from '@/api/clusters';
import type { BindingRow, Cluster, PolicyRow, RoleDTO } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { fmtTime } from '@/utils/format';

type TabKey = 'roles' | 'users' | 'policies';

interface RoleForm {
  name: string;
  desc?: string;
}

function isRobotSub(sub: string): boolean {
  return sub.startsWith('robot:');
}

export function Access() {
  const { message } = App.useApp();
  const [activeTab, setActiveTab] = useState<TabKey>('roles');
  const [roles, setRoles] = useState<RoleDTO[]>([]);
  const [bindings, setBindings] = useState<BindingRow[]>([]);
  const [policies, setPolicies] = useState<PolicyRow[]>([]);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [roleSearch, setRoleSearch] = useState('');
  const [userSearch, setUserSearch] = useState('');
  const [policySearch, setPolicySearch] = useState('');
  const [revokeSub, setRevokeSub] = useState<string>();

  const [roleOpen, setRoleOpen] = useState(false);
  const [bindingOpen, setBindingOpen] = useState(false);
  const [policyOpen, setPolicyOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [revoking, setRevoking] = useState(false);

  const [roleForm] = Form.useForm<RoleForm>();
  const [bindingForm] = Form.useForm<AddBindingInput>();
  const [policyForm] = Form.useForm<AddPolicyInput>();

  const loadAll = async () => {
    setLoading(true);
    setError(null);
    try {
      const [roleList, bindingList, policyList, clusterList] = await Promise.all([
        listRoles(),
        listBindings(),
        listPolicies(),
        listClusters(),
      ]);
      setRoles(roleList);
      setBindings(bindingList);
      setPolicies(policyList);
      setClusters(clusterList);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载权限数据失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const roleOptions = useMemo(
    () => roles.map((r) => ({ value: r.name, label: `${r.name}${r.desc ? ` · ${r.desc}` : ''}` })),
    [roles],
  );

  const clusterOptions = useMemo(
    () => clusters.map((c) => ({ value: c.id, label: `${c.id} · ${c.name}` })),
    [clusters],
  );

  const userOptions = useMemo(() => {
    const users = new Set<string>();
    for (const row of bindings) {
      if (!isRobotSub(row[0])) users.add(row[0]);
    }
    return Array.from(users)
      .sort((a, b) => a.localeCompare(b))
      .map((u) => ({ value: u, label: u }));
  }, [bindings]);

  const visibleBindings = useMemo(() => {
    const keyword = userSearch.trim().toLowerCase();
    return bindings
      .filter((row) => !isRobotSub(row[0]))
      .filter((row) => !keyword || row.some((v) => v.toLowerCase().includes(keyword)));
  }, [bindings, userSearch]);

  const visibleRoles = useMemo(() => {
    const keyword = roleSearch.trim().toLowerCase();
    if (!keyword) return roles;
    return roles.filter((r) =>
      [r.name, r.desc].some((v) => v.toLowerCase().includes(keyword)),
    );
  }, [roles, roleSearch]);

  const visiblePolicies = useMemo(() => {
    const keyword = policySearch.trim().toLowerCase();
    if (!keyword) return policies;
    return policies.filter((row) => row.some((v) => v.toLowerCase().includes(keyword)));
  }, [policies, policySearch]);

  const submitRole = async (values: RoleForm) => {
    setSaving(true);
    try {
      await createRole({ name: values.name.trim(), desc: values.desc?.trim() || undefined });
      message.success(`角色 ${values.name.trim()} 已创建`);
      setRoleOpen(false);
      roleForm.resetFields();
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建角色失败');
    } finally {
      setSaving(false);
    }
  };

  const removeRole = async (role: RoleDTO) => {
    try {
      await deleteRole(role.name);
      message.success(`角色 ${role.name} 已删除`);
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除角色失败');
    }
  };

  const submitBinding = async (values: AddBindingInput) => {
    setSaving(true);
    try {
      await addBinding({
        user: values.user.trim(),
        role: values.role.trim(),
        dom: values.dom.trim(),
      });
      message.success('用户绑定已添加');
      setBindingOpen(false);
      bindingForm.resetFields();
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加绑定失败');
    } finally {
      setSaving(false);
    }
  };

  const submitPolicy = async (values: AddPolicyInput) => {
    setSaving(true);
    try {
      await addPolicy({
        sub: values.sub.trim(),
        dom: values.dom.trim(),
        obj: values.obj.trim(),
        act: values.act.trim(),
      });
      message.success('策略已添加');
      setPolicyOpen(false);
      policyForm.resetFields();
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加策略失败');
    } finally {
      setSaving(false);
    }
  };

  const removeBinding = async (row: BindingRow) => {
    try {
      await deleteBinding({ user: row[0], role: row[1], dom: row[2] });
      message.success('用户绑定已删除');
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除绑定失败');
    }
  };

  const removePolicy = async (row: PolicyRow) => {
    try {
      await deletePolicy({ sub: row[0], dom: row[1], obj: row[2], act: row[3] });
      message.success('策略已删除');
      await loadAll();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除策略失败');
    }
  };

  const submitRevoke = () => {
    if (!revokeSub) {
      message.warning('请选择要踢出的用户');
      return;
    }
    Modal.confirm({
      title: `踢出用户「${revokeSub}」？`,
      content: '该用户此前签发的全部 Token 将立即失效；同秒内重新登录获得的新 Token 不受影响。',
      okText: '立即吊销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setRevoking(true);
        try {
          await revokeUser(revokeSub);
          message.success(`用户 ${revokeSub} 的 Token 已全部吊销`);
          setRevokeSub(undefined);
        } catch (err) {
          message.error(err instanceof Error ? err.message : '踢出用户失败');
        } finally {
          setRevoking(false);
        }
      },
    });
  };

  const roleColumns: TableColumnsType<RoleDTO> = [
    {
      title: '角色',
      dataIndex: 'name',
      key: 'name',
      width: 240,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (name: string) => <code className="mono strong">{name}</code>,
    },
    {
      title: '描述',
      dataIndex: 'desc',
      key: 'desc',
      render: (desc: string) => desc || <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: '策略数',
      dataIndex: 'policy_count',
      key: 'policy_count',
      width: 100,
      align: 'right',
      sorter: (a, b) => a.policy_count - b.policy_count,
    },
    {
      title: '绑定数',
      dataIndex: 'binding_count',
      key: 'binding_count',
      width: 100,
      align: 'right',
      sorter: (a, b) => a.binding_count - b.binding_count,
    },
    {
      title: '来源',
      dataIndex: 'managed',
      key: 'managed',
      width: 110,
      render: (managed: boolean) =>
        managed ? <Tag color="blue">受管</Tag> : <Tag>派生</Tag>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (value?: string) => (value ? fmtTime(value) : '—'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 190,
      render: (_, record) => (
        <Space size={0}>
          <Button
            type="link"
            size="small"
            onClick={() => {
              setPolicySearch(record.name);
              setActiveTab('policies');
            }}
          >
            查看策略
          </Button>
          {record.managed ? (
            <Popconfirm
              title={`删除角色「${record.name}」？`}
              description="仍被用户绑定或策略引用时无法删除。"
              okText="删除"
              cancelText="取消"
              okButtonProps={{ danger: true }}
              onConfirm={() => void removeRole(record)}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          ) : null}
        </Space>
      ),
    },
  ];

  const bindingColumns: TableColumnsType<BindingRow> = [
    {
      title: '用户',
      key: 'user',
      width: 220,
      render: (_, row) => <code className="mono strong">{row[0]}</code>,
    },
    {
      title: '角色',
      key: 'role',
      width: 220,
      render: (_, row) => <Tag color="blue">{row[1]}</Tag>,
    },
    {
      title: '集群',
      key: 'dom',
      width: 220,
      render: (_, row) => <code className="mono">{row[2]}</code>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, row) => (
        <Popconfirm
          title="删除这条用户绑定？"
          description={`${row[0]} → ${row[1]} @ ${row[2]}`}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => void removeBinding(row)}
        >
          <Button type="link" size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  const policyColumns: TableColumnsType<PolicyRow> = [
    {
      title: '角色',
      key: 'sub',
      width: 200,
      render: (_, row) => <Tag color="blue">{row[0]}</Tag>,
    },
    {
      title: '集群',
      key: 'dom',
      width: 160,
      render: (_, row) => <code className="mono">{row[1]}</code>,
    },
    {
      title: '资源',
      key: 'obj',
      render: (_, row) => <code className="mono">{row[2]}</code>,
    },
    {
      title: '动词',
      key: 'act',
      width: 220,
      render: (_, row) => <code className="mono">{row[3]}</code>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, row) => (
        <Popconfirm
          title="删除这条策略？"
          description={row.join(' · ')}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => void removePolicy(row)}
        >
          <Button type="link" size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  const rolesTab = (
    <Card
      className="soft-card"
      title={`角色（${roles.length}）`}
      extra={
        <Space>
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜索角色"
            value={roleSearch}
            onChange={(e) => setRoleSearch(e.target.value)}
            style={{ width: 220 }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              roleForm.resetFields();
              setRoleOpen(true);
            }}
          >
            新建角色
          </Button>
        </Space>
      }
    >
      <Table<RoleDTO>
        rowKey="name"
        columns={roleColumns}
        dataSource={visibleRoles}
        loading={loading}
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个角色` }}
        scroll={{ x: 1000 }}
      />
    </Card>
  );

  const usersTab = (
    <div className="tab-stack">
      <Card
        className="soft-card"
        title={`用户绑定（${visibleBindings.length}）`}
        extra={
          <Space>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="搜索用户 / 角色 / 集群"
              value={userSearch}
              onChange={(e) => setUserSearch(e.target.value)}
              style={{ width: 260 }}
            />
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => {
                bindingForm.resetFields();
                setBindingOpen(true);
              }}
            >
              添加用户
            </Button>
          </Space>
        }
      >
        <Table<BindingRow>
          rowKey={(row) => row.join('|')}
          columns={bindingColumns}
          dataSource={visibleBindings}
          loading={loading}
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条绑定` }}
          scroll={{ x: 760 }}
        />
      </Card>

      <Card title="踢出用户" className="soft-card">
        <Space wrap>
          <Select
            showSearch
            allowClear
            placeholder="选择用户"
            style={{ width: 300 }}
            value={revokeSub}
            onChange={setRevokeSub}
            options={userOptions}
            optionFilterProp="label"
            notFoundContent="暂无绑定用户"
          />
          <Button
            danger
            type="primary"
            icon={<UserDeleteOutlined />}
            loading={revoking}
            disabled={!revokeSub}
            onClick={submitRevoke}
          >
            立即吊销全部 Token
          </Button>
        </Space>
      </Card>
    </div>
  );

  const policiesTab = (
    <Card
      className="soft-card"
      title={`策略（${policies.length}）`}
      extra={
        <Space>
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜索角色 / 集群 / 资源 / 动词"
            value={policySearch}
            onChange={(e) => setPolicySearch(e.target.value)}
            style={{ width: 300 }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              policyForm.resetFields();
              setPolicyOpen(true);
            }}
          >
            添加策略
          </Button>
        </Space>
      }
    >
      <Table<PolicyRow>
        rowKey={(row) => row.join('|')}
        columns={policyColumns}
        dataSource={visiblePolicies}
        loading={loading}
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条策略` }}
        scroll={{ x: 900 }}
      />
    </Card>
  );

  return (
    <div>
      <PageHeader
        title="权限管理"
        subtitle="先创建角色，再为角色配置策略，并把用户绑定到角色。"
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="权限数据加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void loadAll()}>
              重试
            </Button>
          }
        />
      ) : null}

      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as TabKey)}
        items={[
          { key: 'roles', label: `角色（${roles.length}）`, children: rolesTab },
          { key: 'users', label: '用户', children: usersTab },
          { key: 'policies', label: `策略（${policies.length}）`, children: policiesTab },
        ]}
      />

      <Modal
        title="新建角色"
        open={roleOpen}
        onCancel={() => setRoleOpen(false)}
        onOk={() => roleForm.submit()}
        okText="创建"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form<RoleForm>
          form={roleForm}
          layout="vertical"
          initialValues={{ name: 'role:' }}
          onFinish={submitRole}
          preserve={false}
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="name"
            label="角色名"
            rules={[{ required: true, message: '请输入角色名' }]}
          >
            <Input placeholder="role:developer" className="mono" autoComplete="off" />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea placeholder="可选，说明角色用途" rows={3} maxLength={512} showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="添加用户绑定"
        open={bindingOpen}
        onCancel={() => setBindingOpen(false)}
        onOk={() => bindingForm.submit()}
        okText="添加"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form<AddBindingInput>
          form={bindingForm}
          layout="vertical"
          onFinish={submitBinding}
          preserve={false}
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="user"
            label="用户"
            rules={[{ required: true, message: '请输入用户 sub' }]}
            extra="Authentik 用户名 / JWT sub"
          >
            <Input placeholder="alice" className="mono" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="role"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
          >
            <AutoComplete
              options={roleOptions}
              placeholder="选择或输入角色"
              filterOption={(input, option) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
            />
          </Form.Item>
          <Form.Item
            name="dom"
            label="集群"
            rules={[{ required: true, message: '请选择集群' }]}
          >
            <Select
              showSearch
              placeholder="选择集群"
              options={clusterOptions}
              optionFilterProp="label"
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="添加角色策略"
        open={policyOpen}
        onCancel={() => setPolicyOpen(false)}
        onOk={() => policyForm.submit()}
        okText="添加"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form<AddPolicyInput>
          form={policyForm}
          layout="vertical"
          initialValues={{ dom: '*' }}
          onFinish={submitPolicy}
          preserve={false}
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="sub"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
          >
            <AutoComplete
              options={roleOptions}
              placeholder="选择或输入角色"
              filterOption={(input, option) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
            />
          </Form.Item>
          <Form.Item
            name="dom"
            label="集群"
            rules={[{ required: true, message: '请输入集群 ID' }]}
            extra="* 表示全部集群"
          >
            <AutoComplete
              options={[{ value: '*', label: '*（全部集群）' }, ...clusterOptions]}
              placeholder="prod-bj 或 *"
            />
          </Form.Item>
          <Form.Item
            name="obj"
            label="资源"
            rules={[{ required: true, message: '请输入资源' }]}
            extra="dev/pods、dev/pods/exec、cluster/nodes、nonresource/healthz，支持 *"
          >
            <Input placeholder="dev/pods" className="mono" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="act"
            label="动词"
            rules={[{ required: true, message: '请输入动词' }]}
            extra="K8s verb 正则片段：get|list|watch"
          >
            <Input placeholder="get|list|watch" className="mono" autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
