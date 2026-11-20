import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onMount,
} from 'solid-js';
import { createPolicies, deletePolicy, listPolicies } from '@/api/access';
import { listClusterResources } from '@/api/resources';
import type { ClusterResource, PolicyDTO } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Field,
  Input,
  Modal,
  Popconfirm,
  Select,
  Table,
  Tag,
  confirm,
  type DescriptionItem,
  type TableColumnsType,
  type TableRowSelection,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import { summarizeBatch } from '@/utils/batch';
import { DeleteOutlined, EditOutlined, PlusOutlined, SearchOutlined } from '@/lib/icons';

function subjectLabel(record: PolicyDTO): string {
  if (record.subject_type === 'org') return '组';
  if (record.subject_type === 'user') return '用户';
  if (record.subject_type === 'robot') return '机器人';
  return '遗留';
}

function subjectLevel(record: PolicyDTO): 'primary' | 'info' | 'gold' | 'neutral' {
  if (record.subject_type === 'org') return 'info';
  if (record.subject_type === 'robot') return 'gold';
  if (record.subject_type === 'user') return 'primary';
  return 'neutral';
}

/** 策略是否命名空间资源：obj 以 cluster/ 开头、资源为 * 或非资源路径视为集群级 */
function isNamespaced(record: PolicyDTO): boolean {
  if (record.resource === '*') return false;
  if (record.resource.startsWith('nonresource')) return false;
  return !record.obj.startsWith('cluster/');
}

function resourceLabel(record: PolicyDTO): string {
  if (record.resource === '*') return '全部资源';
  return record.subresource ? `${record.resource}/${record.subresource}` : record.resource;
}

interface PoliciesTabProps {
  /** 主体范围：robot 仅显示机器人策略；缺省显示用户/组策略（机器人策略单独管理） */
  subjectScope?: 'robot';
  /** 仅显示该集群的策略 */
  clusterFilter?: string;
  /** 显示「创建策略」按钮 */
  onCreate?: () => void;
  /** 策略创建后递增，触发列表刷新（替代 React 的 key 重挂载） */
  refreshKey?: number;
}

/** 策略列表（权限 Tab 与机器人权限 Tab 共用）。 */
export function PoliciesTab(props: PoliciesTabProps) {
  const [policies, setPolicies] = createSignal<PolicyDTO[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [search, setSearch] = createSignal('');
  const [selectedKeys, setSelectedKeys] = createSignal<(string | number)[]>([]);
  const [editing, setEditing] = createSignal<PolicyDTO | null>(null);
  const [editSaving, setEditSaving] = createSignal(false);
  const [resources, setResources] = createSignal<ClusterResource[]>([]);
  const [namespaces, setNamespaces] = createSignal<string[]>([]);

  const editForm = createForm({ namespace: '*', verbs: [] as string[] });

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setPolicies(await listPolicies());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载策略失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  createEffect(() => {
    if (props.refreshKey) void load();
  });

  const openEdit = (record: PolicyDTO) => {
    setEditing(record);
    const namespaced = isNamespaced(record);
    editForm.reset({
      namespace: namespaced ? record.namespace : '*',
      verbs: record.verbs,
    });
    void listClusterResources(record.cluster)
      .then((res) => {
        setResources(res.resources);
        setNamespaces(res.namespaces);
      })
      .catch(() => {
        setResources([]);
        setNamespaces([]);
      });
  };

  const filtered = createMemo(() => {
    let list = policies();
    if (props.subjectScope === 'robot') {
      list = list.filter((policy) => policy.subject_type === 'robot');
    } else {
      // 机器人策略只在机器人页签维护
      list = list.filter((policy) => policy.subject_type !== 'robot');
    }
    if (props.clusterFilter) {
      list = list.filter((policy) => policy.dom === props.clusterFilter);
    }
    const keyword = search().trim().toLowerCase();
    if (!keyword) return list;
    return list.filter((record) =>
      [
        record.subject,
        record.cluster,
        record.namespace,
        record.resource,
        record.subresource ?? '',
        record.verbs.join(' '),
      ].some((value) => value.toLowerCase().includes(keyword)),
    );
  });

  const policyKey = (record: PolicyDTO) =>
    `${record.sub}|${record.dom}|${record.obj}|${record.act}`;

  const policyByKey = createMemo(() => {
    const map = new Map<string, PolicyDTO>();
    for (const record of filtered()) map.set(policyKey(record), record);
    return map;
  });

  const selectedPolicies = createMemo(() =>
    selectedKeys()
      .map((key) => policyByKey().get(String(key)))
      .filter((record): record is PolicyDTO => Boolean(record)),
  );

  const remove = async (record: PolicyDTO) => {
    try {
      await deletePolicy({
        sub: record.sub,
        dom: record.dom,
        obj: record.obj,
        act: record.act,
      });
      toast.success('策略已删除');
      setSelectedKeys((keys) => keys.filter((key) => key !== policyKey(record)));
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除策略失败');
    }
  };

  const submitEdit = async () => {
    const current = editing();
    if (!current) return;
    if (!editForm.validate({ verbs: [{ required: true, message: '至少选择一个动作' }] })) {
      return;
    }
    const namespaced = isNamespaced(current);
    const subjectType =
      current.subject_type === 'org'
        ? 'org'
        : current.subject_type === 'robot'
          ? 'robot'
          : 'user';
    setEditSaving(true);
    try {
      // 编辑语义：删除旧四元组 + 按新规则重建（保持主体/集群不变）
      await deletePolicy({
        sub: current.sub,
        dom: current.dom,
        obj: current.obj,
        act: current.act,
      });
      await createPolicies({
        subject_type: subjectType,
        subject: current.subject,
        cluster: current.cluster,
        rules: [
          {
            namespace: namespaced ? String(editForm.values.namespace) : '*',
            resource: current.resource,
            subresource: current.subresource,
            namespaced,
            verbs: current.resource === '*' ? ['*'] : (editForm.values.verbs as string[]),
          },
        ],
      });
      toast.success('策略已更新');
      setEditing(null);
      await load();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : '更新策略失败（旧策略可能已删除，请刷新核对）',
      );
      await load();
    } finally {
      setEditSaving(false);
    }
  };

  const batchRemove = () => {
    const targets = selectedPolicies();
    if (targets.length === 0) return;
    confirm({
      title: `批量删除 ${targets.length} 条策略？`,
      content: '删除立即生效，对应主体马上失去这些资源的访问权限。',
      okText: '批量删除',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(
            targets.map((record) =>
              deletePolicy({
                sub: record.sub,
                dom: record.dom,
                obj: record.obj,
                act: record.act,
              }),
            ),
          ),
        );
        if (outcome.failed === 0) {
          toast.success(`已删除 ${outcome.succeeded} 条策略`);
        } else {
          toast.error(
            `已删除 ${outcome.succeeded} 条策略，${outcome.failed} 条失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection = (): TableRowSelection<PolicyDTO> => ({
    selectedRowKeys: selectedKeys(),
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  });

  const columns: TableColumnsType<PolicyDTO> = [
    {
      title: '主体',
      key: 'subject',
      width: 260,
      render: (_value, record) => (
        <span class="inline-flex items-center gap-1.5">
          <Tag level={subjectLevel(record)}>{subjectLabel(record)}</Tag>
          <code class="mono strong">{record.subject}</code>
        </span>
      ),
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 180,
      render: (value) => <code class="mono">{String(value)}</code>,
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
      width: 140,
      render: (value) => {
        const text = String(value ?? '');
        return text === '*' ? (
          <Tag>全部</Tag>
        ) : text ? (
          <code class="mono">{text}</code>
        ) : (
          <span class="muted">—</span>
        );
      },
    },
    {
      title: '资源',
      key: 'resource',
      width: 220,
      render: (_value, record) =>
        record.resource === '*' ? (
          <Tag level="gold">全部资源</Tag>
        ) : (
          <code class="mono">{resourceLabel(record)}</code>
        ),
    },
    {
      title: '动作',
      key: 'verbs',
      render: (_value, record) =>
        record.verbs.includes('*') ? (
          <Tag level="gold">全部动作</Tag>
        ) : (
          <span class="flex flex-wrap gap-1">
            <For each={record.verbs}>{(verb) => <Tag>{verb}</Tag>}</For>
          </span>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_value, record) => (
        <div class="table-actions">
          <Button
            type="link"
            size="small"
            icon={<EditOutlined class="h-3.5 w-3.5" />}
            onClick={() => openEdit(record)}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除这条策略？"
            description={`${record.sub} · ${record.dom} · ${record.obj} · ${record.act}`}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(record)}
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

  const editingItems = (): DescriptionItem[] => {
    const record = editing();
    if (!record) return [];
    return [
      { label: '主体', value: <code class="mono">{record.sub}</code> },
      { label: '集群', value: <code class="mono">{record.cluster}</code> },
      { label: '资源', value: <code class="mono">{resourceLabel(record)}</code> },
    ];
  };

  const verbOptions = () => {
    const record = editing();
    if (!record) return [];
    const found = resources().find(
      (resource) =>
        resource.resource === record.resource &&
        (resource.subresource ?? '') === (record.subresource ?? ''),
    );
    const verbs = found?.verbs ?? [
      'get',
      'list',
      'watch',
      'create',
      'update',
      'patch',
      'delete',
    ];
    return verbs.map((verb) => ({ value: verb, label: verb }));
  };

  const namespaceOptions = () => {
    const record = editing();
    if (!record) return [{ value: '*', label: '*（全部命名空间）' }];
    return [
      { value: '*', label: '*（全部命名空间）' },
      ...Array.from(new Set([record.namespace, ...namespaces()]))
        .filter((namespace) => namespace && namespace !== '*')
        .sort()
        .map((namespace) => ({ value: namespace, label: namespace })),
    ];
  };

  return (
    <div>
      <Show when={error()}>
        <Alert
          type="error"
          message="策略加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      </Show>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Show when={props.onCreate}>
          <Button
            type="primary"
            icon={<PlusOutlined class="h-4 w-4" />}
            onClick={props.onCreate}
          >
            创建策略
          </Button>
        </Show>
        <Input
          prefix={<SearchOutlined class="h-4 w-4" />}
          placeholder="搜索主体 / 集群 / 资源 / 动作"
          value={search()}
          onValueChange={setSearch}
          class="w-[260px]"
        />
      </div>
      <Card class="soft-card" contentClass="p-0">
        <Show when={selectedKeys().length > 0}>
          <div class="batch-bar">
            <div class="flex items-center gap-3">
              <div>
                已选择 <span class="batch-count">{selectedKeys().length}</span> 条策略
              </div>
              <Button type="link" size="small" onClick={() => setSelectedKeys([])}>
                清空
              </Button>
            </div>
            <Button
              danger
              size="small"
              icon={<DeleteOutlined class="h-3.5 w-3.5" />}
              onClick={batchRemove}
            >
              批量删除
            </Button>
          </div>
        </Show>
        <Table<PolicyDTO>
          rowKey={(record) => policyKey(record)}
          columns={columns}
          dataSource={filtered()}
          loading={loading()}
          rowSelection={rowSelection()}
          pagination={{ pageSize: 10, showTotal: (total) => `共 ${total} 条策略` }}
          scroll={{ x: 1100 }}
          locale={{
            emptyText: (
              <div class="px-4 py-8 text-center text-sm text-layer-content/50">
                尚未配置策略。创建策略后，组织或用户才能访问目标集群资源。
              </div>
            ),
          }}
        />
      </Card>

      <Modal
        open={editing() !== null}
        title="编辑策略"
        onClose={() => setEditing(null)}
        width={560}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setEditing(null)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              loading={editSaving()}
              onClick={() => void submitEdit()}
            >
              保存
            </Button>
          </>
        }
      >
        <Show when={editing()}>
          {(record) => (
            <>
              <Alert
                type="info"
                message="主体、集群与资源不可修改；保存时会删除旧策略并按新命名空间/动作重建"
                class="mb-4"
              />
              <Descriptions items={editingItems()} column={1} class="mb-4" />
              <div class="flex flex-col gap-4">
                <Field
                  label="命名空间"
                  extra={
                    isNamespaced(record())
                      ? '切换命名空间会重建为对应作用域的规则'
                      : '集群级资源无需命名空间'
                  }
                >
                  <Select
                    value={String(editForm.values.namespace)}
                    disabled={!isNamespaced(record()) || record().resource === '*'}
                    options={namespaceOptions()}
                    onChange={(value) =>
                      editForm.setValue('namespace', String(value ?? '*'))
                    }
                  />
                </Field>
                <Field
                  label="动作"
                  extra={
                    record().resource === '*'
                      ? '资源为 * 时动作固定为全部权限'
                      : '可自定义（回车添加）'
                  }
                  error={editForm.errors.verbs}
                >
                  <Select
                    mode="tags"
                    value={editForm.values.verbs as string[]}
                    disabled={record().resource === '*'}
                    placeholder="选择或输入动作"
                    options={verbOptions()}
                    onChange={(values) =>
                      editForm.setValue(
                        'verbs',
                        Array.isArray(values) ? values.map(String) : [],
                      )
                    }
                  />
                </Field>
              </div>
            </>
          )}
        </Show>
      </Modal>
    </div>
  );
}
