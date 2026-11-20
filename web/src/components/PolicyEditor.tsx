import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
} from 'solid-js';
import { createPolicies } from '@/api/access';
import { listGroups, listUsers } from '@/api/users';
import { listClusterResources } from '@/api/resources';
import type {
  Cluster,
  ClusterResource,
  DirectoryOption,
  PolicyRuleInput,
  Robot,
} from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Drawer,
  Segmented,
  Select,
  Tag,
  type SelectOption,
} from '@/lib/widgets';
import { DeleteOutlined, PlusOutlined } from '@/lib/icons';

interface PolicyEditorProps {
  open: boolean;
  clusters: Cluster[];
  /** 机器人主体候选（机器人页使用；标准权限页不传） */
  robots?: Robot[];
  /** 仅允许机器人主体：机器人页的独立权限入口 */
  robotOnly?: boolean;
  initialSubjectType?: SubjectType;
  initialSubject?: string;
  onClose: () => void;
  onCreated: () => void;
}

type SubjectType = 'org' | 'user' | 'robot';

interface RuleDraft {
  key: number;
  resourceKey?: string;
  namespace: string;
  verbs: string[];
}

interface PreviewRow {
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

function resourceKey(resource: ClusterResource): string {
  return `${resource.group}|${resource.version}|${resource.resource}|${resource.subresource ?? ''}`;
}

/** 非资源路径（/version、/api 等发现接口）的伪资源 key：直接作为 Casbin obj 存储。 */
const NONRESOURCE_KEY = 'nonresource/*';

function newRule(key: number): RuleDraft {
  return { key, resourceKey: undefined, namespace: '*', verbs: [] };
}

export function PolicyEditor(props: PolicyEditorProps) {
  const [subjectType, setSubjectType] = createSignal<SubjectType>(
    props.robotOnly ? 'robot' : 'user',
  );
  const [subject, setSubject] = createSignal<string | undefined>(undefined);
  const [cluster, setCluster] = createSignal<string | undefined>(undefined);
  const [directoryOptions, setDirectoryOptions] = createSignal<DirectoryOption[]>([]);
  const [directoryLoading, setDirectoryLoading] = createSignal(false);
  const [directoryError, setDirectoryError] = createSignal<string | null>(null);
  const [resources, setResources] = createSignal<ClusterResource[]>([]);
  const [namespaces, setNamespaces] = createSignal<string[]>([]);
  const [resourceLoading, setResourceLoading] = createSignal(false);
  const [resourceError, setResourceError] = createSignal<string | null>(null);
  const [rules, setRules] = createSignal<RuleDraft[]>([newRule(1)]);
  const [submitting, setSubmitting] = createSignal(false);

  let ruleSeq = 2;
  let searchTimer: number | undefined;

  onCleanup(() => {
    if (searchTimer) window.clearTimeout(searchTimer);
  });

  const robotOptions = createMemo<DirectoryOption[]>(() =>
    (props.robots ?? []).map((robot) => ({
      value: robot.id,
      label: `${robot.name}（${robot.sub}）`,
    })),
  );

  const loadDirectory = async (type: SubjectType, query: string) => {
    setDirectoryError(null);
    if (type === 'robot') {
      const q = query.trim().toLowerCase();
      setDirectoryOptions(
        robotOptions().filter(
          (option) =>
            !q ||
            option.label.toLowerCase().includes(q) ||
            option.value.toLowerCase().includes(q),
        ),
      );
      setDirectoryLoading(false);
      return;
    }
    setDirectoryLoading(true);
    try {
      const q = query.trim().toLowerCase();
      const options: DirectoryOption[] =
        type === 'org'
          ? (await listGroups())
              .filter(
                (group) =>
                  !q ||
                  group.name.toLowerCase().includes(q) ||
                  group.desc.toLowerCase().includes(q),
              )
              .map((group) => ({
                value: group.name,
                label: `组 · ${group.name}（${group.members.length} 人）`,
              }))
          : (await listUsers())
              .filter(
                (user) =>
                  !q ||
                  user.username.toLowerCase().includes(q) ||
                  user.nickname.toLowerCase().includes(q) ||
                  user.email.toLowerCase().includes(q),
              )
              .map((user) => ({
                value: user.username,
                label: `${user.nickname || user.username}（${user.username}）`,
              }));
      setDirectoryOptions(options);
    } catch (err) {
      setDirectoryError(err instanceof Error ? err.message : '平台用户/组加载失败');
      setDirectoryOptions([]);
    } finally {
      setDirectoryLoading(false);
    }
  };

  createEffect(() => {
    if (!props.open) return;
    const type = props.initialSubjectType ?? (props.robotOnly ? 'robot' : 'user');
    setSubjectType(type);
    setSubject(props.initialSubject);
    setCluster(undefined);
    setResources([]);
    setNamespaces([]);
    setRules([newRule(1)]);
    ruleSeq = 2;
    setResourceError(null);
    void loadDirectory(type, '');
  });

  const onSubjectSearch = (query: string) => {
    if (searchTimer) window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(() => {
      void loadDirectory(subjectType(), query);
    }, 300);
  };

  const onSubjectTypeChange = (type: SubjectType) => {
    setSubjectType(type);
    setSubject(undefined);
    setDirectoryOptions([]);
    void loadDirectory(type, '');
  };

  const resourceMap = createMemo(() => {
    const map = new Map<string, ClusterResource>();
    for (const resource of resources()) map.set(resourceKey(resource), resource);
    return map;
  });

  const resourceOptions = createMemo<SelectOption[]>(() => {
    const groups = new Map<string, { value: string; label: string }[]>();
    for (const resource of resources()) {
      const groupLabel = resource.group
        ? `${resource.group}/${resource.version}`
        : resource.version;
      const label = `${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''} · ${resource.kind}`;
      const list = groups.get(groupLabel) ?? [];
      list.push({ value: resourceKey(resource), label });
      groups.set(groupLabel, list);
    }
    return [
      { label: '通配', value: '', options: [{ value: '*', label: '*（全部资源）' }] },
      {
        label: '非资源路径',
        value: '',
        options: [{ value: NONRESOURCE_KEY, label: 'nonresource/*（版本与发现接口）' }],
      },
      ...Array.from(groups.entries()).map(([label, options]) => ({ label, value: '', options })),
    ];
  });

  const loadResources = async (clusterID: string | undefined) => {
    setCluster(clusterID);
    setResources([]);
    setNamespaces([]);
    setResourceError(null);
    setRules([newRule(ruleSeq++)]);
    if (!clusterID) return;
    setResourceLoading(true);
    try {
      const data = await listClusterResources(clusterID);
      setResources(data.resources);
      setNamespaces(data.namespaces);
      if (data.partial) {
        toast.warning('部分 API 组的资源发现失败，资源列表可能不完整');
      }
    } catch (err) {
      setResourceError(err instanceof Error ? err.message : '集群资源加载失败');
    } finally {
      setResourceLoading(false);
    }
  };

  const updateRule = (key: number, patch: Partial<RuleDraft>) => {
    setRules((prev) =>
      prev.map((rule) => (rule.key === key ? { ...rule, ...patch } : rule)),
    );
  };

  const onResourceChange = (key: number, value: string) => {
    if (value === '*') {
      updateRule(key, { resourceKey: value, namespace: '', verbs: ['*'] });
      return;
    }
    if (value === NONRESOURCE_KEY) {
      updateRule(key, { resourceKey: value, namespace: '', verbs: [] });
      return;
    }
    const resource = resourceMap().get(value);
    updateRule(key, {
      resourceKey: value,
      namespace: resource?.namespaced ? '*' : '',
      verbs: [],
    });
  };

  const selectedResource = (rule: RuleDraft): ClusterResource | undefined =>
    rule.resourceKey && rule.resourceKey !== '*' && rule.resourceKey !== NONRESOURCE_KEY
      ? resourceMap().get(rule.resourceKey)
      : undefined;

  const verbsOptions = (rule: RuleDraft) => {
    if (rule.resourceKey === NONRESOURCE_KEY) {
      return [
        { value: '*', label: '*（全部动作）' },
        { value: 'get', label: 'get' },
      ];
    }
    const resource = selectedResource(rule);
    if (!resource) return [{ value: '*', label: '*（全部动作）' }];
    return [
      { value: '*', label: '*（全部动作）' },
      ...resource.verbs.map((verb) => ({ value: verb, label: verb })),
    ];
  };

  const onVerbsChange = (key: number, values: string[]) => {
    updateRule(key, { verbs: values.includes('*') ? ['*'] : values });
  };

  const addRule = () => setRules((prev) => [...prev, newRule(ruleSeq++)]);

  const removeRule = (key: number) => {
    setRules((prev) =>
      prev.length === 1 ? prev : prev.filter((rule) => rule.key !== key),
    );
  };

  const previewRows = createMemo<PreviewRow[]>(() => {
    const currentSubject = subject();
    const currentCluster = cluster();
    if (!currentSubject || !currentCluster) return [];
    const sub = `${subjectType()}:${currentSubject}`;
    const rows: PreviewRow[] = [];
    for (const rule of rules()) {
      if (!rule.resourceKey || rule.verbs.length === 0) continue;
      if (rule.resourceKey === '*') {
        rows.push({ sub, dom: currentCluster, obj: '*', act: '.*' });
        continue;
      }
      const act = rule.verbs.includes('*') ? '.*' : rule.verbs.join('|');
      if (rule.resourceKey === NONRESOURCE_KEY) {
        rows.push({ sub, dom: currentCluster, obj: NONRESOURCE_KEY, act });
        continue;
      }
      const resource = resourceMap().get(rule.resourceKey);
      if (!resource) continue;
      if (resource.namespaced) {
        const namespace = rule.namespace || '*';
        const obj = `${namespace}/${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''}`;
        rows.push({ sub, dom: currentCluster, obj, act });
        if (namespace === '*' && !resource.subresource) {
          rows.push({ sub, dom: currentCluster, obj: `cluster/${resource.resource}`, act });
        }
      } else {
        const obj = `cluster/${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''}`;
        rows.push({ sub, dom: currentCluster, obj, act });
        if (resource.resource === 'namespaces' && !resource.subresource) {
          rows.push({ sub, dom: currentCluster, obj: '*/namespaces', act });
        }
      }
    }
    return rows;
  });

  const hasWildcard = createMemo(() =>
    rules().some((rule) => rule.resourceKey === '*'),
  );

  const submit = async () => {
    const currentSubject = subject();
    if (!currentSubject) {
      toast.warning('请选择主体');
      return;
    }
    const currentCluster = cluster();
    if (!currentCluster) {
      toast.warning('请选择集群');
      return;
    }
    const payloadRules: PolicyRuleInput[] = [];
    for (const rule of rules()) {
      if (!rule.resourceKey || rule.verbs.length === 0) continue;
      if (rule.resourceKey === '*') {
        payloadRules.push({
          namespace: '',
          resource: '*',
          namespaced: false,
          verbs: ['*'],
        });
        continue;
      }
      if (rule.resourceKey === NONRESOURCE_KEY) {
        payloadRules.push({
          namespace: '',
          resource: NONRESOURCE_KEY,
          namespaced: false,
          verbs: rule.verbs,
        });
        continue;
      }
      const resource = resourceMap().get(rule.resourceKey);
      if (!resource) continue;
      payloadRules.push({
        namespace: resource.namespaced ? rule.namespace || '*' : '',
        resource: resource.resource,
        subresource: resource.subresource,
        namespaced: resource.namespaced,
        verbs: rule.verbs,
      });
    }
    if (payloadRules.length === 0) {
      toast.warning('请至少添加一条资源与动作');
      return;
    }
    setSubmitting(true);
    try {
      const result = await createPolicies({
        subject_type: subjectType(),
        subject: currentSubject,
        cluster: currentCluster,
        rules: payloadRules,
      });
      toast.success(`已创建 ${result.created} 条策略`);
      props.onCreated();
      props.onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建策略失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Drawer
      title={props.robotOnly ? '配置机器人策略' : '创建策略'}
      open={props.open}
      onClose={props.onClose}
      width={860}
      class="policy-drawer"
      footer={
        <div class="policy-drawer-footer">
          <Button size="small" onClick={props.onClose}>
            取消
          </Button>
          <Button
            type="primary"
            size="small"
            loading={submitting()}
            onClick={() => void submit()}
          >
            创建策略
          </Button>
        </div>
      }
    >
      <div class="policy-builder">
        <section class="policy-section">
          <div class="policy-section-title">
            <span class="policy-step">1</span>
            <span>选择匹配主体</span>
          </div>
          <Show
            when={!props.robotOnly}
            fallback={
              <div class="flex flex-wrap items-center gap-2">
                <Tag level="info">机器人</Tag>
                <span class="text-[13px] text-layer-content/50">
                  机器人主体由平台签发，仅使用自身策略
                </span>
              </div>
            }
          >
            <Segmented
              class="self-start"
              value={subjectType()}
              options={[
                { value: 'user', label: '用户' },
                { value: 'org', label: '组' },
              ]}
              onChange={(value) => onSubjectTypeChange(value as SubjectType)}
            />
          </Show>
          <Select
            showSearch
            allowClear
            class="policy-subject-select"
            placeholder={
              subjectType() === 'org'
                ? '选择组'
                : subjectType() === 'robot'
                  ? '选择机器人'
                  : '选择用户'
            }
            value={subject()}
            loading={directoryLoading()}
            filterOption={false}
            onSearch={onSubjectSearch}
            onChange={(value) => setSubject(value === undefined ? undefined : String(value))}
            options={directoryOptions()}
            notFoundContent={directoryLoading() ? '加载中…' : '没有匹配项'}
          />
          <Show when={directoryError()}>
            <Alert
              type="warning"
              message="用户/组目录不可用"
              description={directoryError() ?? undefined}
            />
          </Show>
        </section>

        <section class="policy-section">
          <div class="policy-section-title">
            <span class="policy-step">2</span>
            <span>选择集群</span>
          </div>
          <Select
            showSearch
            class="policy-cluster-select"
            placeholder="选择目标集群"
            value={cluster()}
            onChange={(value) =>
              void loadResources(value === undefined ? undefined : String(value))
            }
            options={props.clusters.map((item) => ({
              value: item.id,
              label: `${item.id} · ${item.name}`,
            }))}
            optionFilterProp="label"
          />
          <Show when={resourceError()}>
            <Alert
              type="error"
              message="集群资源加载失败"
              description={resourceError() ?? undefined}
            />
          </Show>
        </section>

        <section class="policy-section">
          <div class="policy-section-title">
            <span class="policy-step">3</span>
            <span>添加允许的资源与动作</span>
          </div>
          <div class="policy-rule-list">
            <For each={rules()}>
              {(rule, index) => {
                const resource = () => selectedResource(rule);
                const isWildcard = () => rule.resourceKey === '*';
                return (
                  <div class="policy-rule-row">
                    <div class="policy-rule-index">{index() + 1}</div>
                    <div class="policy-rule-field">
                      <span class="policy-rule-label">资源</span>
                      <Select
                        showSearch
                        placeholder="选择资源"
                        value={rule.resourceKey}
                        loading={resourceLoading()}
                        disabled={!cluster()}
                        options={resourceOptions()}
                        optionFilterProp="label"
                        onChange={(value) =>
                          onResourceChange(rule.key, String(value ?? ''))
                        }
                      />
                    </div>
                    <Show when={resource()?.namespaced}>
                      <div class="policy-rule-field">
                        <span class="policy-rule-label">命名空间</span>
                        <Select
                          showSearch
                          value={rule.namespace}
                          options={[
                            { value: '*', label: '*（全部命名空间）' },
                            ...namespaces().map((name) => ({ value: name, label: name })),
                          ]}
                          optionFilterProp="label"
                          onChange={(value) =>
                            updateRule(rule.key, { namespace: String(value ?? '') })
                          }
                        />
                      </div>
                    </Show>
                    <div class="policy-rule-field policy-rule-verbs">
                      <span class="policy-rule-label">动作</span>
                      <Select
                        mode="multiple"
                        placeholder="选择动作"
                        value={rule.verbs}
                        disabled={!rule.resourceKey || isWildcard()}
                        options={verbsOptions(rule)}
                        maxTagCount="responsive"
                        onChange={(values) =>
                          onVerbsChange(rule.key, Array.isArray(values) ? values.map(String) : [])
                        }
                      />
                    </div>
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined class="h-4 w-4" />}
                      disabled={rules().length === 1}
                      onClick={() => removeRule(rule.key)}
                    />
                  </div>
                );
              }}
            </For>
          </div>
          <Button
            type="dashed"
            block
            icon={<PlusOutlined class="h-4 w-4" />}
            disabled={!cluster() || resourceLoading()}
            onClick={addRule}
          >
            添加一条资源规则
          </Button>
          <Show when={hasWildcard()}>
            <Alert
              type="warning"
              message="已选择全部资源：动作将自动为全部动作（*）"
            />
          </Show>
        </section>

        <section class="policy-section">
          <div class="policy-section-title">
            <span class="policy-step">4</span>
            <span>通配与预览</span>
          </div>
          <div class="text-[13px] text-layer-content/50">
            资源选择 <code class="mono">*</code> 时自动授予全部动作；以下为最终策略行。
          </div>
          <pre class="policy-preview">
            {previewRows().length > 0
              ? previewRows()
                  .map((row) => `p, ${row.sub}, ${row.dom}, ${row.obj}, ${row.act}`)
                  .join('\n')
              : '请先选择主体、集群与至少一条资源规则'}
          </pre>
          <Show when={previewRows().length > 0}>
            <div class="flex flex-wrap gap-1.5">
              <Tag level="primary">{previewRows().length} 条策略</Tag>
              <Show when={hasWildcard()}>
                <Tag level="gold">包含全部权限</Tag>
              </Show>
            </div>
          </Show>
        </section>
      </div>
    </Drawer>
  );
}
