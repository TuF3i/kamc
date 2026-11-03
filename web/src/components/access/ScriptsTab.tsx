import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Divider,
  Drawer,
  Form,
  Input,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { PlayCircleOutlined, SaveOutlined } from '@ant-design/icons';
import CodeMirror from '@uiw/react-codemirror';
import { StreamLanguage } from '@codemirror/language';
import { lua } from '@codemirror/legacy-modes/mode/lua';
import {
  deleteScript,
  getScript,
  listScripts,
  putScript,
  runScript,
  type ScriptRunResult,
} from '@/api/users';
import type { UserScript } from '@/api/types';

const DEFAULT_TEMPLATE = `-- 登录期映射脚本：把 SSO/LDAP 的 claims 映射到平台用户与策略。
-- 可用全局：claims = { sub, email, source, groups = {...}, extra = {...} }
-- 可用 API：kamc.upsert_user(username, nickname, groups_csv)
--           kamc.upsert_group(name, desc)
--           kamc.list_users() / kamc.list_groups()
--           kamc.add_policy(sub, dom, obj, act) / kamc.has_policy(...)
--           kamc.remove_policy(...) / kamc.list_policies()

-- 示例：dev-team 组的成员自动获得 prod-bj 集群 default 命名空间的读权限
for _, group in ipairs(claims.groups) do
  if group == 'dev-team' then
    kamc.add_policy('user:' .. claims.sub, 'prod-bj', 'default/*', 'get|list|watch')
  end
end
`;

interface Draft {
  /** 编辑中的脚本名（新建为空） */
  originName: string;
  name: string;
  content: string;
  enabled: boolean;
}

/** 测试面板的执行结果展示 */
function RunResult({ result, error }: { result: ScriptRunResult | null; error: string | null }) {
  if (error) {
    return (
      <Alert type="error" showIcon message="执行失败" description={<pre className="code-block" style={{ margin: 0, maxHeight: 180, overflow: 'auto' }}>{error}</pre>} />
    );
  }
  if (!result) return null;
  return (
    <Alert
      type="success"
      showIcon
      message={`执行成功${result.dry_run ? '（dry-run，未落库）' : '（已执行）'} · ${result.actions.length} 个动作`}
      description={
        result.actions.length ? (
          <Table
            rowKey={(r) => r.api + JSON.stringify(r.args)}
            dataSource={result.actions}
            size="small"
            pagination={false}
            columns={[
              { title: 'API', dataIndex: 'api', width: 140, render: (v: string) => <code className="mono">{v}</code> },
              { title: '参数', dataIndex: 'args', render: (args: string[]) => <code className="mono" style={{ fontSize: 12 }}>{args.join(', ') || '—'}</code> },
            ]}
          />
        ) : (
          '无动作'
        )
      }
    />
  );
}

/** 映射脚本 Tab：列表 + 右侧 Drawer 编辑器（Lua 高亮 / 测试执行 / 改名）。 */
export function ScriptsTab() {
  const { message, modal } = App.useApp();
  const [scripts, setScripts] = useState<UserScript[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState<ScriptRunResult | null>(null);
  const [runError, setRunError] = useState<string | null>(null);
  const [form] = Form.useForm<{ sub: string; email: string; groups: string; source: string }>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setScripts(await listScripts(true));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载脚本列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openCreate = () => {
    setDraft({ originName: '', name: '', content: DEFAULT_TEMPLATE, enabled: true });
    form.resetFields();
    setRunResult(null);
    setRunError(null);
    setDrawerOpen(true);
  };

  const openEdit = async (sc: UserScript) => {
    try {
      const full = await getScript(sc.name);
      setDraft({ originName: sc.name, name: sc.name, content: full.content, enabled: full.enabled });
      form.resetFields();
      setRunResult(null);
      setRunError(null);
      setDrawerOpen(true);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载脚本失败');
    }
  };

  const save = async () => {
    if (!draft) return;
    const name = draft.name.trim();
    if (!name) {
      message.warning('请输入脚本名称');
      return;
    }
    setSaving(true);
    try {
      // 改名：保存新名字后删除旧记录
      await putScript(name, draft.content, draft.enabled);
      if (draft.originName && draft.originName !== name) {
        await deleteScript(draft.originName);
      }
      message.success(`脚本 ${name} 已保存`);
      setDraft({ ...draft, originName: name });
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败（Lua 语法错误？）');
    } finally {
      setSaving(false);
    }
  };

  const execute = async (dryRun: boolean) => {
    if (!draft) return;
    const name = draft.name.trim();
    if (!name) {
      message.warning('请先填写脚本名称并保存');
      return;
    }
    // 未保存的修改先落库（执行读取的是服务端内容）
    try {
      await putScript(name, draft.content, draft.enabled);
      if (draft.originName && draft.originName !== name) {
        await deleteScript(draft.originName);
        setDraft({ ...draft, originName: name });
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败，无法执行');
      return;
    }

    const values = await form.validateFields().catch(() => null);
    if (!values) return;
    setRunning(true);
    setRunResult(null);
    setRunError(null);
    try {
      const result = await runScript(name, dryRun, {
        sub: values.sub?.trim() || undefined,
        email: values.email?.trim() || undefined,
        groups: values.groups
          ? values.groups.split(',').map((g) => g.trim()).filter(Boolean)
          : undefined,
        source: values.source || undefined,
      });
      setRunResult(result);
      if (!dryRun) await load();
    } catch (err) {
      setRunError(err instanceof Error ? err.message : String(err));
    } finally {
      setRunning(false);
    }
  };

  const remove = (sc: UserScript) => {
    modal.confirm({
      title: `删除脚本「${sc.name}」？`,
      content: '删除后登录链路不再执行该脚本。',
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteScript(sc.name);
          message.success(`脚本 ${sc.name} 已删除`);
          await load();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除失败');
        }
      },
    });
  };

  const columns: {
    title: string;
    dataIndex?: string;
    key: string;
    width?: number;
    render: (value: any, record: UserScript, index: number) => any;
  }[] = [
    {
      title: '脚本名',
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (name: string) => <code className="mono strong">{name}</code>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 110,
      render: (enabled: boolean) =>
        enabled ? <Tag color="success">启用</Tag> : <Tag>已禁用</Tag>,
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (v: string) => new Date(v).toLocaleString(),
    },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_, sc: UserScript) => (
        <Space size={0}>
          <Button type="link" size="small" onClick={() => void openEdit(sc)}>编辑</Button>
          <Button type="link" size="small" danger onClick={() => remove(sc)}>删除</Button>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {error ? <Typography.Paragraph type="danger">{error}</Typography.Paragraph> : null}
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        message="脚本在每次 SSO/LDAP 登录与 Token 兑换时按名称顺序执行（fail-closed）：脚本出错将拒绝签发 Token。"
      />
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" onClick={openCreate}>
          新建脚本
        </Button>
      </Space>
      <Table
        rowKey="name"
        columns={columns}
        dataSource={scripts}
        loading={loading}
        size="middle"
        pagination={false}
        locale={{ emptyText: '暂无映射脚本' }}
      />

      <Drawer
        title={draft?.originName ? `编辑脚本 · ${draft.originName}` : '新建映射脚本'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={860}
        destroyOnHidden
        footer={
          <Space style={{ float: 'right' }}>
            <Switch checkedChildren="启用" unCheckedChildren="禁用" checked={draft?.enabled ?? true} onChange={(v) => setDraft((d) => (d ? { ...d, enabled: v } : d))} />
            <Button onClick={() => setDrawerOpen(false)}>关闭</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={() => void save()}>
              保存
            </Button>
          </Space>
        }
      >
        {draft ? (
          <div className="script-drawer">
            <Space.Compact style={{ width: '100%', marginBottom: 12 }}>
              <Input
                addonBefore="脚本名称"
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="如 dev-team-policy"
                className="mono"
                maxLength={128}
              />
            </Space.Compact>

            <CodeMirror
              value={draft.content}
              height="420px"
              extensions={[StreamLanguage.define(lua)]}
              theme="dark"
              basicSetup={{ foldGutter: false, autocompletion: true, highlightActiveLine: true }}
              onChange={(value) => setDraft((d) => (d ? { ...d, content: value } : d))}
              style={{ fontSize: 13, border: '1px solid var(--kamc-border)', borderRadius: 8 }}
            />

            <Divider plain>测试执行</Divider>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
              注入模拟 claims 执行脚本（服务端 dry-run 不落库）；保存后亦可去掉 dry-run 真实执行。
            </Typography.Paragraph>
            <Form form={form} layout="inline" style={{ rowGap: 8 }}>
              <Form.Item name="sub" label="sub" initialValue="">
                <Input placeholder="alice" style={{ width: 140 }} />
              </Form.Item>
              <Form.Item name="email" label="email" initialValue="">
                <Input placeholder="alice@example.com" style={{ width: 200 }} />
              </Form.Item>
              <Form.Item name="groups" label="组" initialValue="">
                <Input placeholder="dev-team, qa-team" style={{ width: 220 }} />
              </Form.Item>
              <Form.Item name="source" label="来源" initialValue="sso">
                <Input style={{ width: 110 }} placeholder="sso / ldap" />
              </Form.Item>
              <Form.Item>
                <Space>
                  <Button icon={<PlayCircleOutlined />} loading={running} onClick={() => void execute(true)}>
                    Dry-run
                  </Button>
                  <Button danger loading={running} onClick={() => void execute(false)}>
                    真实执行
                  </Button>
                </Space>
              </Form.Item>
            </Form>
            {running ? <Spin size="small" style={{ marginTop: 12 }} /> : null}
            <div style={{ marginTop: 12 }}>
              <RunResult result={runResult} error={runError} />
            </div>
          </div>
        ) : null}
      </Drawer>
    </div>
  );
}
