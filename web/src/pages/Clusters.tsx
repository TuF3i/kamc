import { Show, createMemo, createSignal, onMount } from 'solid-js';
import { useNavigate } from '@solidjs/router';
import { createCluster, deleteCluster, listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Field,
  Input,
  Modal,
  Table,
  TextArea,
  Tooltip,
  confirm,
  type TableColumnsType,
  type TableRowSelection,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';
import { summarizeBatch } from '@/utils/batch';
import { DeleteOutlined, PlusOutlined, SearchOutlined } from '@/lib/icons';

interface CreateFormValues extends Record<string, unknown> {
  id: string;
  name: string;
  desc: string;
}

export default function Clusters() {
  const navigate = useNavigate();
  const [clusters, setClusters] = createSignal<Cluster[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [search, setSearch] = createSignal('');
  const [selectedKeys, setSelectedKeys] = createSignal<(string | number)[]>([]);
  const [createOpen, setCreateOpen] = createSignal(false);
  const [creating, setCreating] = createSignal(false);
  const [deleteTarget, setDeleteTarget] = createSignal<Cluster | null>(null);
  const [deleteConfirm, setDeleteConfirm] = createSignal('');
  const [deleting, setDeleting] = createSignal(false);

  const form = createForm<CreateFormValues>({ id: '', name: '', desc: '' });
  const rules = {
    id: [
      { required: true, message: '请输入集群 ID' },
      { max: 64, message: '集群 ID 最长 64 个字符' },
    ],
    name: [{ required: true, message: '请输入集群名称' }],
  };

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setClusters(await listClusters());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载集群列表失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const filtered = createMemo(() => {
    const keyword = search().trim().toLowerCase();
    if (!keyword) return clusters();
    return clusters().filter((cluster) =>
      [cluster.id, cluster.name, cluster.desc].some((value) =>
        value.toLowerCase().includes(keyword),
      ),
    );
  });

  const openCreate = () => {
    form.reset();
    setCreateOpen(true);
  };

  const submitCreate = async () => {
    if (!form.validate(rules)) return;
    setCreating(true);
    try {
      await createCluster({
        id: String(form.values.id).trim(),
        name: String(form.values.name).trim(),
        desc: String(form.values.desc).trim() || undefined,
      });
      toast.success(`集群 ${String(form.values.id).trim()} 已注册`);
      setCreateOpen(false);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '注册集群失败');
    } finally {
      setCreating(false);
    }
  };

  const openDelete = (cluster: Cluster) => {
    setDeleteTarget(cluster);
    setDeleteConfirm('');
  };

  const submitDelete = async () => {
    const target = deleteTarget();
    if (!target) return;
    setDeleting(true);
    try {
      await deleteCluster(target.id);
      toast.success(`集群 ${target.id} 已注销`);
      setSelectedKeys((keys) => keys.filter((key) => key !== target.id));
      setDeleteTarget(null);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '注销集群失败');
    } finally {
      setDeleting(false);
    }
  };

  const selectedClusters = createMemo(() =>
    clusters().filter((cluster) => selectedKeys().includes(cluster.id)),
  );

  const batchDelete = () => {
    const targets = selectedClusters();
    if (targets.length === 0) return;
    confirm({
      title: `批量注销 ${targets.length} 个集群？`,
      content:
        '仅删除集群记录，该操作不可撤销；已签发的 Agent Token、Casbin 策略不会自动清理，需要另行处理。',
      okText: '批量注销',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(targets.map((cluster) => deleteCluster(cluster.id))),
        );
        if (outcome.failed === 0) {
          toast.success(`已注销 ${outcome.succeeded} 个集群`);
        } else {
          toast.error(
            `已注销 ${outcome.succeeded} 个集群，${outcome.failed} 个失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection = (): TableRowSelection<Cluster> => ({
    selectedRowKeys: selectedKeys(),
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  });

  const columns: TableColumnsType<Cluster> = [
    {
      title: '集群 ID',
      dataIndex: 'id',
      key: 'id',
      width: 150,
      sorter: (a, b) => a.id.localeCompare(b.id),
      render: (value) => {
        const id = String(value);
        return (
          <Tooltip title={id}>
            <code class="mono strong">
              {id.length > 18 ? `${id.slice(0, 15)}…` : id}
            </code>
          </Tooltip>
        );
      },
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      sorter: (a, b) => a.name.localeCompare(b.name, 'zh-CN'),
    },
    {
      title: '描述',
      dataIndex: 'desc',
      key: 'desc',
      ellipsis: true,
      render: (value) => {
        const desc = String(value ?? '');
        return desc || <span class="muted">—</span>;
      },
    },
    {
      title: '接入状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      filters: [
        { text: '在线', value: 'online' },
        { text: '待接入', value: 'pending' },
        { text: '离线', value: 'offline' },
      ],
      onFilter: (value, record) => record.status === String(value),
      render: (_value, record) => <ClusterStatusTag status={record.status} />,
    },
    {
      title: '注册时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      sorter: (a, b) => a.created_at.localeCompare(b.created_at),
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_value, record) => (
        <div class="table-actions" onClick={(event) => event.stopPropagation()}>
          <Button
            type="link"
            size="small"
            onClick={() => navigate(`/clusters/${encodeURIComponent(record.id)}`)}
          >
            详情
          </Button>
          <Button
            type="link"
            size="small"
            danger
            icon={<DeleteOutlined class="h-3.5 w-3.5" />}
            onClick={() => openDelete(record)}
          >
            注销
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="集群"
        subtitle="注册与管理目标 Kubernetes 集群"
        extra={
          <div class="flex flex-wrap items-center gap-2">
            <Input
              prefix={<SearchOutlined class="h-4 w-4" />}
              placeholder="搜索 ID / 名称 / 描述"
              value={search()}
              onValueChange={setSearch}
              class="w-60"
            />
            <Button
              type="primary"
              icon={<PlusOutlined class="h-4 w-4" />}
              onClick={openCreate}
            >
              注册集群
            </Button>
          </div>
        }
      />

      <Show when={error()}>
        <Alert
          type="error"
          message="集群列表加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      </Show>

      <Card class="soft-card" contentClass="p-0">
        <Show when={selectedKeys().length > 0}>
          <div class="batch-bar">
            <div class="flex items-center gap-3">
              <div>
                已选择 <span class="batch-count">{selectedKeys().length}</span> 个集群
              </div>
              <Button type="link" size="small" onClick={() => setSelectedKeys([])}>
                清空
              </Button>
            </div>
            <Button
              danger
              size="small"
              icon={<DeleteOutlined class="h-3.5 w-3.5" />}
              onClick={batchDelete}
            >
              批量注销
            </Button>
          </div>
        </Show>
        <Table<Cluster>
          rowKey="id"
          columns={columns}
          dataSource={filtered()}
          loading={loading()}
          rowSelection={rowSelection()}
          onRow={(record) => ({
            onClick: () => navigate(`/clusters/${encodeURIComponent(record.id)}`),
          })}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50],
            showTotal: (total) => `共 ${total} 个集群`,
          }}
          scroll={{ x: 900 }}
        />
      </Card>

      <Modal
        open={createOpen()}
        title="注册集群"
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              loading={creating()}
              onClick={() => void submitCreate()}
            >
              注册
            </Button>
          </>
        }
      >
        <div class="form-stack mt-4">
          <Field
            label="集群 ID"
            required
            error={form.errors.id}
            extra="唯一标识，同时作为 Casbin dom（如 prod-bj）；注册后不可修改。"
          >
            <Input
              value={String(form.values.id)}
              onValueChange={(value) => form.setValue('id', value)}
              placeholder="prod-bj"
              class="mono"
              autocomplete="off"
              status={form.errors.id ? 'error' : undefined}
            />
          </Field>
          <Field label="名称" required error={form.errors.name}>
            <Input
              value={String(form.values.name)}
              onValueChange={(value) => form.setValue('name', value)}
              placeholder="北京生产集群"
              autocomplete="off"
              status={form.errors.name ? 'error' : undefined}
            />
          </Field>
          <Field label="描述">
            <TextArea
              value={String(form.values.desc)}
              onValueChange={(value) => form.setValue('desc', value)}
              placeholder="可选，说明集群用途"
              rows={3}
              maxLength={500}
              showCount
            />
          </Field>
        </div>
      </Modal>

      <Modal
        open={deleteTarget() !== null}
        title="注销集群"
        onClose={() => setDeleteTarget(null)}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setDeleteTarget(null)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              danger
              loading={deleting()}
              disabled={deleteConfirm() !== deleteTarget()?.id}
              onClick={() => void submitDelete()}
            >
              确认注销
            </Button>
          </>
        }
      >
        <Alert
          type="warning"
          message="该操作不可撤销"
          description="仅删除集群记录。已签发的 Agent Token、Casbin 策略与绑定不会自动清理，需要另行处理。"
          class="mb-4"
        />
        <p class="text-[13px]">
          请输入集群 ID <code class="mono strong">{deleteTarget()?.id}</code> 以确认：
        </p>
        <Input
          value={deleteConfirm()}
          onValueChange={setDeleteConfirm}
          placeholder={deleteTarget()?.id}
          class="mono"
          autocomplete="off"
        />
      </Modal>
    </div>
  );
}
