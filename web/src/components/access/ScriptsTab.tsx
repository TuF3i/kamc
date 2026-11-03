import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Empty,
  Input,
  Modal,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import {
  CaretRightOutlined,
  DeleteOutlined,
  FileAddOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons';
import { deleteScript, getScript, listScripts, putScript, runScript } from '@/api/users';
import type { UserScript } from '@/api/types';

const DEFAULT_TEMPLATE = `-- 登录期映射脚本：把 SSO/LDAP 的 claims 映射到平台用户与策略。
-- 可用全局：claims = { sub, email, source, groups = {...}, extra = {...} }
-- 可用 API：kamc.upsert_user(username, nickname, groups_csv)
--           kamc.upsert_group(name, desc)
--           kamc.list_users() / kamc.list_groups()
--           kamc.add_policy(sub, dom, obj, act) / kamc.has_policy(...)
--           kamc.remove_policy(...) / kamc.list_policies()

-- 示例：dev-team 组的成员自动获得 default 命名空间的读权限
for _, group in ipairs(claims.groups) do
  if group == 'dev-team' then
    kamc.add_policy('user:' .. claims.sub, 'prod-bj', 'default/*', 'get|list|watch')
  end
end
`;

interface EditorState {
  name: string;
  isNew: boolean;
}

/** 映射脚本 Tab：登录期 Lua 脚本管理（保存即语法校验，支持 dry-run）。 */
export function ScriptsTab() {
  const { message } = App.useApp();
  const [scripts, setScripts] = useState<UserScript[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [content, setContent] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState<string | null>(null);

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
    setEditor({ name: '', isNew: true });
    setContent(DEFAULT_TEMPLATE);
    setEnabled(true);
    setRunResult(null);
  };

  const openEdit = async (sc: UserScript) => {
    try {
      const full = await getScript(sc.name);
      setEditor({ name: sc.name, isNew: false });
      setContent(full.content);
      setEnabled(full.enabled);
      setRunResult(null);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载脚本失败');
    }
  };

  const save = async () => {
    if (!editor) return;
    const name = editor.name.trim();
    if (!name) {
      message.warning('请输入脚本名称');
      return;
    }
    setSaving(true);
    try {
      await putScript(name, content, enabled);
      message.success(`脚本 ${name} 已保存`);
      setEditor({ name, isNew: false });
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败（Lua 语法错误？）');
    } finally {
      setSaving(false);
    }
  };

  const execute = async (name: string, dryRun: boolean) => {
    setRunning(true);
    setRunResult(null);
    try {
      const result = await runScript(name, dryRun);
      const lines = result.actions.map((a) => `${a.api}(${a.args.join(', ')})`);
      setRunResult(
        lines.length ? lines.join('\n') : `执行成功，无动作${dryRun ? '（dry-run）' : ''}`,
      );
      message.success('执行成功');
      if (!dryRun) await load();
    } catch (err) {
      setRunResult(err instanceof Error ? err.message : String(err));
      message.error('执行失败');
    } finally {
      setRunning(false);
    }
  };

  const remove = async (sc: UserScript) => {
    try {
      await deleteScript(sc.name);
      message.success(`脚本 ${sc.name} 已删除`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败');
    }
  };

  const columns: TableColumnsType<UserScript> = [
    {
      title: '脚本名',
      dataIndex: 'name',
      key: 'name',
      width: 240,
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
      width: 280,
      render: (_, sc) => (
        <Space size={0}>
          <Button type="link" size="small" onClick={() => void openEdit(sc)}>编辑</Button>
          <Button
            type="link"
            size="small"
            icon={<PlayCircleOutlined />}
            loading={running}
            onClick={() => void execute(sc.name, true)}
          >
            Dry-run
          </Button>
          <Button
            type="link"
            size="small"
            danger
            icon={<CaretRightOutlined />}
            loading={running}
            onClick={() => void execute(sc.name, false)}
          >
            执行
          </Button>
          <Button type="link" size="small" danger icon={<DeleteOutlined />} onClick={() => void remove(sc)}>
            删除
          </Button>
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
        <Button type="primary" icon={<FileAddOutlined />} onClick={openCreate}>
          新建脚本
        </Button>
      </Space>
      <Table<UserScript>
        rowKey="name"
        columns={columns}
        dataSource={scripts}
        loading={loading}
        size="middle"
        pagination={false}
        locale={{ emptyText: <Empty description="暂无映射脚本" /> }}
      />

      <Modal
        title={editor?.isNew ? '新建映射脚本' : `编辑脚本 · ${editor?.name ?? ''}`}
        open={editor !== null}
        width={860}
        onCancel={() => setEditor(null)}
        footer={
          <Space>
            <Switch checkedChildren="启用" unCheckedChildren="禁用" checked={enabled} onChange={setEnabled} />
            <Button onClick={() => setEditor(null)}>取消</Button>
            <Button type="primary" loading={saving} onClick={() => void save()}>
              保存
            </Button>
          </Space>
        }
      >
        {editor?.isNew ? (
          <Input
            placeholder="脚本名称（如 dev-team-policy）"
            value={editor.name}
            onChange={(e) => setEditor({ ...editor, name: e.target.value })}
            className="mono"
            style={{ marginBottom: 12 }}
            maxLength={128}
          />
        ) : null}
        <Input.TextArea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={18}
          className="mono lua-editor"
          spellCheck={false}
        />
        {runResult ? (
          <Alert
            style={{ marginTop: 12 }}
            type="info"
            showIcon={running}
            icon={running ? <Spin size="small" /> : undefined}
            message="执行结果"
            description={<pre className="code-block" style={{ maxHeight: 220, margin: 0 }}>{runResult}</pre>}
          />
        ) : null}
      </Modal>
    </div>
  );
}
