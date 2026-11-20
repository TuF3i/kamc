import { Show, createSignal, onMount } from 'solid-js';
import {
  deleteScript,
  getScript,
  listScripts,
  putScript,
  runScript,
  type ScriptRunResult,
} from '@/api/users';
import type { UserScript } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Divider,
  Drawer,
  Field,
  Input,
  Switch,
  Table,
  Tag,
  confirm,
  type TableColumnsType,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import { LuaEditor } from '@/components/LuaEditor';
import { PlayCircleOutlined, SaveOutlined, SpinnerIcon } from '@/lib/icons';

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
function RunResult(props: { result: ScriptRunResult | null; error: string | null }) {
  return (
    <Show when={!props.error} fallback={
      <Alert
        type="error"
        message="执行失败"
        description={<pre class="code-block m-0 max-h-[180px] overflow-auto">{props.error}</pre>}
      />
    }>
      <Show when={props.result}>
        {(result) => (
          <Alert
            type="success"
            message={`执行成功${result().dry_run ? '（dry-run，未落库）' : '（已执行）'} · ${result().actions.length} 个动作`}
            description={
              result().actions.length ? (
                <Table
                  rowKey={(record) => record.api + JSON.stringify(record.args)}
                  dataSource={result().actions}
                  pagination={false}
                  columns={[
                    {
                      title: 'API',
                      dataIndex: 'api',
                      width: 140,
                      render: (value) => <code class="mono">{String(value)}</code>,
                    },
                    {
                      title: '参数',
                      dataIndex: 'args',
                      render: (value) => (
                        <code class="mono text-[12px]">
                          {(value as string[]).join(', ') || '—'}
                        </code>
                      ),
                    },
                  ]}
                />
              ) : (
                '无动作'
              )
            }
          />
        )}
      </Show>
    </Show>
  );
}

/** 映射脚本 Tab：列表 + 右侧 Drawer 编辑器（Lua 高亮 / 测试执行 / 改名）。 */
export function ScriptsTab() {
  const [scripts, setScripts] = createSignal<UserScript[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [drawerOpen, setDrawerOpen] = createSignal(false);
  const [draft, setDraft] = createSignal<Draft | null>(null);
  const [saving, setSaving] = createSignal(false);
  const [running, setRunning] = createSignal(false);
  const [runResult, setRunResult] = createSignal<ScriptRunResult | null>(null);
  const [runError, setRunError] = createSignal<string | null>(null);

  const claimsForm = createForm({ sub: '', email: '', groups: '', source: 'sso' });

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setScripts(await listScripts(true));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载脚本列表失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const openCreate = () => {
    setDraft({ originName: '', name: '', content: DEFAULT_TEMPLATE, enabled: true });
    claimsForm.reset();
    setRunResult(null);
    setRunError(null);
    setDrawerOpen(true);
  };

  const openEdit = async (script: UserScript) => {
    try {
      const full = await getScript(script.name);
      setDraft({
        originName: script.name,
        name: script.name,
        content: full.content,
        enabled: full.enabled,
      });
      claimsForm.reset();
      setRunResult(null);
      setRunError(null);
      setDrawerOpen(true);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '加载脚本失败');
    }
  };

  const save = async () => {
    const current = draft();
    if (!current) return;
    const name = current.name.trim();
    if (!name) {
      toast.warning('请输入脚本名称');
      return;
    }
    setSaving(true);
    try {
      // 改名：保存新名字后删除旧记录
      await putScript(name, current.content, current.enabled);
      if (current.originName && current.originName !== name) {
        await deleteScript(current.originName);
      }
      toast.success(`脚本 ${name} 已保存`);
      setDraft({ ...current, originName: name });
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败（Lua 语法错误？）');
    } finally {
      setSaving(false);
    }
  };

  const execute = async (dryRun: boolean) => {
    const current = draft();
    if (!current) return;
    const name = current.name.trim();
    if (!name) {
      toast.warning('请先填写脚本名称并保存');
      return;
    }
    // 未保存的修改先落库（执行读取的是服务端内容）
    try {
      await putScript(name, current.content, current.enabled);
      if (current.originName && current.originName !== name) {
        await deleteScript(current.originName);
        setDraft({ ...current, originName: name });
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败，无法执行');
      return;
    }

    setRunning(true);
    setRunResult(null);
    setRunError(null);
    try {
      const groups = String(claimsForm.values.groups);
      const result = await runScript(name, dryRun, {
        sub: String(claimsForm.values.sub).trim() || undefined,
        email: String(claimsForm.values.email).trim() || undefined,
        groups: groups
          ? groups
              .split(',')
              .map((group) => group.trim())
              .filter(Boolean)
          : undefined,
        source: String(claimsForm.values.source) || undefined,
      });
      setRunResult(result);
      if (!dryRun) await load();
    } catch (err) {
      setRunError(err instanceof Error ? err.message : String(err));
    } finally {
      setRunning(false);
    }
  };

  const remove = (script: UserScript) => {
    confirm({
      title: `删除脚本「${script.name}」？`,
      content: '删除后登录链路不再执行该脚本。',
      okText: '删除',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteScript(script.name);
          toast.success(`脚本 ${script.name} 已删除`);
          await load();
        } catch (err) {
          toast.error(err instanceof Error ? err.message : '删除失败');
        }
      },
    });
  };

  const columns: TableColumnsType<UserScript> = [
    {
      title: '脚本名',
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (value) => <code class="mono strong">{String(value)}</code>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 110,
      render: (value) =>
        value ? <Tag level="success">启用</Tag> : <Tag>已禁用</Tag>,
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (value) => new Date(String(value)).toLocaleString(),
    },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_value, script) => (
        <div class="table-actions">
          <Button type="link" size="small" onClick={() => void openEdit(script)}>
            编辑
          </Button>
          <Button type="link" size="small" danger onClick={() => remove(script)}>
            删除
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <Show when={error()}>
        <p class="mb-3 text-sm text-error">{error()}</p>
      </Show>
      <Alert
        type="info"
        message="脚本在每次 SSO/LDAP 登录与 Token 兑换时按名称顺序执行（fail-closed）：脚本出错将拒绝签发 Token。"
        class="mb-3"
      />
      <div class="mb-3">
        <Button type="primary" onClick={openCreate}>
          新建脚本
        </Button>
      </div>
      <Card class="soft-card" contentClass="p-0">
        <Table<UserScript>
          rowKey="name"
          columns={columns}
          dataSource={scripts()}
          loading={loading()}
          pagination={false}
          locale={{ emptyText: '暂无映射脚本' }}
        />
      </Card>

      <Drawer
        open={drawerOpen()}
        title={draft()?.originName ? `编辑脚本 · ${draft()!.originName}` : '新建映射脚本'}
        onClose={() => setDrawerOpen(false)}
        width={860}
        footer={
          <div class="ml-auto flex items-center gap-2">
            <Switch
              checked={draft()?.enabled ?? true}
              checkedChildren="启用"
              unCheckedChildren="禁用"
              onChange={(checked) =>
                setDraft((current) =>
                  current ? { ...current, enabled: checked } : current,
                )
              }
            />
            <Button size="small" onClick={() => setDrawerOpen(false)}>
              关闭
            </Button>
            <Button
              type="primary"
              size="small"
              icon={<SaveOutlined class="h-4 w-4" />}
              loading={saving()}
              onClick={() => void save()}
            >
              保存
            </Button>
          </div>
        }
      >
        <Show when={draft()}>
          {(current) => (
            <div class="script-drawer">
              <div class="flex w-full">
                <span class="flex min-h-12 shrink-0 items-center whitespace-nowrap rounded-l-lg border-r border-layer-content/10 bg-layer-content/10 px-3 text-[13px] text-layer-content/60">
                  脚本名称
                </span>
                <Input
                  value={current().name}
                  onValueChange={(value) =>
                    setDraft((state) => (state ? { ...state, name: value } : state))
                  }
                  placeholder="如 dev-team-policy"
                  class="mono rounded-l-none"
                  maxLength={128}
                />
              </div>

              <div class="mt-3">
                <LuaEditor
                  value={current().content}
                  onChange={(value) =>
                    setDraft((state) => (state ? { ...state, content: value } : state))
                  }
                />
              </div>

              <Divider>测试执行</Divider>
              <p class="mb-2 text-[13px] leading-6 text-layer-content/50">
                注入模拟 claims 执行脚本（服务端 dry-run 不落库）；保存后亦可去掉 dry-run 真实执行。
              </p>
              <div class="flex flex-wrap items-end gap-3">
                <Field label="sub">
                  <Input
                    value={String(claimsForm.values.sub)}
                    onValueChange={(value) => claimsForm.setValue('sub', value)}
                    placeholder="alice"
                    class="w-[140px]"
                  />
                </Field>
                <Field label="email">
                  <Input
                    value={String(claimsForm.values.email)}
                    onValueChange={(value) => claimsForm.setValue('email', value)}
                    placeholder="alice@example.com"
                    class="w-[200px]"
                  />
                </Field>
                <Field label="组">
                  <Input
                    value={String(claimsForm.values.groups)}
                    onValueChange={(value) => claimsForm.setValue('groups', value)}
                    placeholder="dev-team, qa-team"
                    class="w-[220px]"
                  />
                </Field>
                <Field label="来源">
                  <Input
                    value={String(claimsForm.values.source)}
                    onValueChange={(value) => claimsForm.setValue('source', value)}
                    placeholder="sso / ldap"
                    class="w-[110px]"
                  />
                </Field>
                <div class="flex items-center gap-2">
                  <Button
                    icon={<PlayCircleOutlined class="h-4 w-4" />}
                    loading={running()}
                    onClick={() => void execute(true)}
                  >
                    Dry-run
                  </Button>
                  <Button danger loading={running()} onClick={() => void execute(false)}>
                    真实执行
                  </Button>
                </div>
              </div>
              <Show when={running()}>
                <div class="mt-3">
                  <SpinnerIcon class="h-4 w-4 animate-spin text-primary" />
                </div>
              </Show>
              <div class="mt-3">
                <RunResult result={runResult()} error={runError()} />
              </div>
            </div>
          )}
        </Show>
      </Drawer>
    </div>
  );
}
