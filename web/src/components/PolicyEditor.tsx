import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Drawer,
  Radio,
  Select,
  Space,
  Tag,
  Typography,
} from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { createPolicies } from '@/api/access';
import { listOrgs, listUsers } from '@/api/directory';
import { listClusterResources } from '@/api/resources';
import type {
  Cluster,
  ClusterResource,
  DirectoryOption,
  PolicyRuleInput,
  Robot,
} from '@/api/types';

interface PolicyEditorProps {
  open: boolean;
  clusters: Cluster[];
  robots: Robot[];
  initialSubjectType?: 'org' | 'user';
  initialSubject?: string;
  onClose: () => void;
  onCreated: () => void;
}

type SubjectType = 'org' | 'user';

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

function newRule(key: number): RuleDraft {
  return { key, resourceKey: undefined, namespace: '*', verbs: [] };
}

export function PolicyEditor({
  open,
  clusters,
  robots,
  initialSubjectType,
  initialSubject,
  onClose,
  onCreated,
}: PolicyEditorProps) {
  const { message } = App.useApp();
  const [subjectType, setSubjectType] = useState<SubjectType>('user');
  const [subject, setSubject] = useState<string>();
  const [cluster, setCluster] = useState<string>();
  const [directoryOptions, setDirectoryOptions] = useState<DirectoryOption[]>([]);
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const [directoryError, setDirectoryError] = useState<string | null>(null);
  const [resources, setResources] = useState<ClusterResource[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [resourceLoading, setResourceLoading] = useState(false);
  const [resourceError, setResourceError] = useState<string | null>(null);
  const [rules, setRules] = useState<RuleDraft[]>([newRule(1)]);
  const [submitting, setSubmitting] = useState(false);
  const ruleSeq = useRef(2);
  const searchTimer = useRef<number>();

  const robotOptions = useMemo<DirectoryOption[]>(
    () => robots.map((r) => ({ value: r.sub, label: `机器人 · ${r.name}（${r.sub}）` })),
    [robots],
  );

  const loadDirectory = async (type: SubjectType, query: string) => {
    setDirectoryLoading(true);
    setDirectoryError(null);
    try {
      const options = type === 'org' ? await listOrgs(query) : await listUsers(query);
      const extra =
        type === 'user'
          ? robotOptions.filter(
              (o) =>
                !query ||
                o.label.toLowerCase().includes(query.toLowerCase()) ||
                o.value.toLowerCase().includes(query.toLowerCase()),
            )
          : [];
      const merged = new Map<string, DirectoryOption>();
      for (const option of [...options, ...extra]) {
        merged.set(option.value, option);
      }
      setDirectoryOptions(Array.from(merged.values()));
    } catch (err) {
      setDirectoryError(err instanceof Error ? err.message : 'Authentik 目录加载失败');
      setDirectoryOptions(type === 'user' ? robotOptions : []);
    } finally {
      setDirectoryLoading(false);
    }
  };

  useEffect(() => {
    if (!open) return;
    const type = initialSubjectType ?? 'user';
    setSubjectType(type);
    setSubject(initialSubject);
    setCluster(undefined);
    setResources([]);
    setNamespaces([]);
    setRules([newRule(1)]);
    ruleSeq.current = 2;
    setResourceError(null);
    void loadDirectory(type, '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialSubjectType, initialSubject]);

  const onSubjectSearch = (query: string) => {
    if (searchTimer.current) window.clearTimeout(searchTimer.current);
    searchTimer.current = window.setTimeout(() => {
      void loadDirectory(subjectType, query);
    }, 300);
  };

  const onSubjectTypeChange = (type: SubjectType) => {
    setSubjectType(type);
    setSubject(undefined);
    setDirectoryOptions([]);
    void loadDirectory(type, '');
  };

  const resourceMap = useMemo(() => {
    const map = new Map<string, ClusterResource>();
    for (const resource of resources) {
      map.set(resourceKey(resource), resource);
    }
    return map;
  }, [resources]);

  const resourceOptions = useMemo(() => {
    const groups = new Map<string, { value: string; label: string }[]>();
    for (const resource of resources) {
      const groupLabel = resource.group ? `${resource.group}/${resource.version}` : resource.version;
      const label = `${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''} · ${resource.kind}`;
      const list = groups.get(groupLabel) ?? [];
      list.push({ value: resourceKey(resource), label });
      groups.set(groupLabel, list);
    }
    return [
      { label: '通配', options: [{ value: '*', label: '*（全部资源）' }] },
      ...Array.from(groups.entries()).map(([label, options]) => ({ label, options })),
    ];
  }, [resources]);

  const loadResources = async (clusterID: string) => {
    setCluster(clusterID);
    setResources([]);
    setNamespaces([]);
    setResourceError(null);
    setRules([newRule(ruleSeq.current++)]);
    if (!clusterID) return;
    setResourceLoading(true);
    try {
      const data = await listClusterResources(clusterID);
      setResources(data.resources);
      setNamespaces(data.namespaces);
      if (data.partial) {
        message.warning('部分 API 组的资源发现失败，资源列表可能不完整');
      }
    } catch (err) {
      setResourceError(err instanceof Error ? err.message : '集群资源加载失败');
    } finally {
      setResourceLoading(false);
    }
  };

  const updateRule = (key: number, patch: Partial<RuleDraft>) => {
    setRules((prev) => prev.map((rule) => (rule.key === key ? { ...rule, ...patch } : rule)));
  };

  const onResourceChange = (key: number, value: string) => {
    if (value === '*') {
      updateRule(key, { resourceKey: value, namespace: '', verbs: ['*'] });
      return;
    }
    const resource = resourceMap.get(value);
    updateRule(key, {
      resourceKey: value,
      namespace: resource?.namespaced ? '*' : '',
      verbs: [],
    });
  };

  const selectedResource = (rule: RuleDraft): ClusterResource | undefined =>
    rule.resourceKey && rule.resourceKey !== '*' ? resourceMap.get(rule.resourceKey) : undefined;

  const verbsOptions = (rule: RuleDraft) => {
    const resource = selectedResource(rule);
    if (!resource) {
      return [{ value: '*', label: '*（全部动作）' }];
    }
    return [
      { value: '*', label: '*（全部动作）' },
      ...resource.verbs.map((verb) => ({ value: verb, label: verb })),
    ];
  };

  const onVerbsChange = (key: number, values: string[]) => {
    updateRule(key, { verbs: values.includes('*') ? ['*'] : values });
  };

  const addRule = () => {
    setRules((prev) => [...prev, newRule(ruleSeq.current++)]);
  };

  const removeRule = (key: number) => {
    setRules((prev) => (prev.length === 1 ? prev : prev.filter((rule) => rule.key !== key)));
  };

  const previewRows = useMemo<PreviewRow[]>(() => {
    if (!subject || !cluster) return [];
    const sub = `${subjectType}:${subject}`;
    const rows: PreviewRow[] = [];
    for (const rule of rules) {
      if (!rule.resourceKey || rule.verbs.length === 0) continue;
      if (rule.resourceKey === '*') {
        rows.push({ sub, dom: cluster, obj: '*', act: '.*' });
        continue;
      }
      const resource = resourceMap.get(rule.resourceKey);
      if (!resource) continue;
      const act = rule.verbs.includes('*') ? '.*' : rule.verbs.join('|');
      if (resource.namespaced) {
        const namespace = rule.namespace || '*';
        const obj = `${namespace}/${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''}`;
        rows.push({ sub, dom: cluster, obj, act });
        if (namespace === '*' && !resource.subresource) {
          rows.push({ sub, dom: cluster, obj: `cluster/${resource.resource}`, act });
        }
      } else {
        const obj = `cluster/${resource.resource}${resource.subresource ? `/${resource.subresource}` : ''}`;
        rows.push({ sub, dom: cluster, obj, act });
        if (resource.resource === 'namespaces' && !resource.subresource) {
          rows.push({ sub, dom: cluster, obj: '*/namespaces', act });
        }
      }
    }
    return rows;
  }, [subject, subjectType, cluster, rules, resourceMap]);

  const hasWildcard = rules.some((rule) => rule.resourceKey === '*');

  const submit = async () => {
    if (!subject) {
      message.warning('请选择组织或用户');
      return;
    }
    if (!cluster) {
      message.warning('请选择集群');
      return;
    }
    const payloadRules: PolicyRuleInput[] = [];
    for (const rule of rules) {
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
      const resource = resourceMap.get(rule.resourceKey);
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
      message.warning('请至少添加一条资源与动作');
      return;
    }
    setSubmitting(true);
    try {
      const result = await createPolicies({
        subject_type: subjectType,
        subject,
        cluster,
        rules: payloadRules,
      });
      message.success(`已创建 ${result.created} 条策略`);
      onCreated();
      onClose();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建策略失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Drawer
      title="创建策略"
      open={open}
      onClose={onClose}
      width={860}
      destroyOnHidden
      className="policy-drawer"
      footer={
        <div className="policy-drawer-footer">
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={submitting} onClick={() => void submit()}>
            创建策略
          </Button>
        </div>
      }
    >
      <div className="policy-builder">
        <section className="policy-section">
          <div className="policy-section-title">
            <span className="policy-step">1</span>
            <span>选择匹配主体</span>
          </div>
          <Radio.Group
            value={subjectType}
            optionType="button"
            buttonStyle="solid"
            onChange={(e) => onSubjectTypeChange(e.target.value as SubjectType)}
            options={[
              { value: 'user', label: '用户' },
              { value: 'org', label: '组织' },
            ]}
          />
          <Select
            showSearch
            allowClear
            className="policy-subject-select"
            placeholder={subjectType === 'org' ? '选择 Authentik 组织' : '选择 Authentik 用户或机器人'}
            value={subject}
            loading={directoryLoading}
            filterOption={false}
            onSearch={onSubjectSearch}
            onChange={setSubject}
            options={directoryOptions}
            notFoundContent={directoryLoading ? '加载中…' : '没有匹配项'}
          />
          {directoryError ? (
            <Alert
              type="warning"
              showIcon
              message="Authentik 目录不可用"
              description={directoryError}
            />
          ) : null}
        </section>

        <section className="policy-section">
          <div className="policy-section-title">
            <span className="policy-step">2</span>
            <span>选择集群</span>
          </div>
          <Select
            showSearch
            className="policy-cluster-select"
            placeholder="选择目标集群"
            value={cluster}
            onChange={(value) => void loadResources(value)}
            options={clusters.map((item) => ({
              value: item.id,
              label: `${item.id} · ${item.name}`,
            }))}
            optionFilterProp="label"
          />
          {resourceError ? (
            <Alert type="error" showIcon message="集群资源加载失败" description={resourceError} />
          ) : null}
        </section>

        <section className="policy-section">
          <div className="policy-section-title">
            <span className="policy-step">3</span>
            <span>添加允许的资源与动作</span>
          </div>
          <div className="policy-rule-list">
            {rules.map((rule, index) => {
              const resource = selectedResource(rule);
              const isWildcard = rule.resourceKey === '*';
              return (
                <div className="policy-rule-row" key={rule.key}>
                  <div className="policy-rule-index">{index + 1}</div>
                  <div className="policy-rule-field">
                    <span className="policy-rule-label">资源</span>
                    <Select
                      showSearch
                      placeholder="选择资源"
                      value={rule.resourceKey}
                      loading={resourceLoading}
                      disabled={!cluster}
                      options={resourceOptions}
                      optionFilterProp="label"
                      onChange={(value) => onResourceChange(rule.key, value)}
                    />
                  </div>
                  {resource?.namespaced ? (
                    <div className="policy-rule-field">
                      <span className="policy-rule-label">命名空间</span>
                      <Select
                        showSearch
                        value={rule.namespace}
                        options={[
                          { value: '*', label: '*（全部命名空间）' },
                          ...namespaces.map((name) => ({ value: name, label: name })),
                        ]}
                        optionFilterProp="label"
                        onChange={(value) => updateRule(rule.key, { namespace: value })}
                      />
                    </div>
                  ) : null}
                  <div className="policy-rule-field policy-rule-verbs">
                    <span className="policy-rule-label">动作</span>
                    <Select
                      mode="multiple"
                      placeholder="选择动作"
                      value={rule.verbs}
                      disabled={!rule.resourceKey || isWildcard}
                      options={verbsOptions(rule)}
                      maxTagCount="responsive"
                      onChange={(values) => onVerbsChange(rule.key, values)}
                    />
                  </div>
                  <Button
                    type="text"
                    danger
                    icon={<DeleteOutlined />}
                    disabled={rules.length === 1}
                    onClick={() => removeRule(rule.key)}
                  />
                </div>
              );
            })}
          </div>
          <Button
            type="dashed"
            block
            icon={<PlusOutlined />}
            disabled={!cluster || resourceLoading}
            onClick={addRule}
          >
            添加一条资源规则
          </Button>
          {hasWildcard ? (
            <Alert
              type="warning"
              showIcon
              message="已选择全部资源：动作将自动为全部动作（*）"
            />
          ) : null}
        </section>

        <section className="policy-section">
          <div className="policy-section-title">
            <span className="policy-step">4</span>
            <span>通配与预览</span>
          </div>
          <Typography.Text type="secondary">
            资源选择 <code className="mono">*</code> 时自动授予全部动作；以下为最终策略行。
          </Typography.Text>
          <pre className="policy-preview">
            {previewRows.length > 0
              ? previewRows.map((row) => `p, ${row.sub}, ${row.dom}, ${row.obj}, ${row.act}`).join('\n')
              : '请先选择主体、集群与至少一条资源规则'}
          </pre>
          {previewRows.length > 0 ? (
            <Space size={[6, 6]} wrap>
              <Tag color="blue">{previewRows.length} 条策略</Tag>
              {hasWildcard ? <Tag color="gold">包含全部权限</Tag> : null}
            </Space>
          ) : null}
        </section>
      </div>
    </Drawer>
  );
}
