import {
  For,
  Show,
  createMemo,
  createSignal,
  onMount,
} from 'solid-js';
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
import { toast } from '@/lib/storage/toast';
import {
  Avatar,
  Button,
  Card,
  Empty,
  Field,
  Input,
  Modal,
  Popconfirm,
  Select,
  Switch,
  Table,
  Tag,
  TextArea,
  type TableColumnsType,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  TeamOutlined,
  UserAddOutlined,
  UserOutlined,
} from '@/lib/icons';

/** 组 Tab：组 CRUD + 成员管理（成员关系存于用户表 groups 字段）。 */
export function GroupsTab() {
  const [groups, setGroups] = createSignal<GroupDTO[]>([]);
  const [users, setUsers] = createSignal<PlatformUserDTO[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [createOpen, setCreateOpen] = createSignal(false);
  const [creating, setCreating] = createSignal(false);
  const [editing, setEditing] = createSignal<GroupDTO | null>(null);
  const [editSaving, setEditSaving] = createSignal(false);
  const [memberBusy, setMemberBusy] = createSignal(false);
  const [memberToAdd, setMemberToAdd] = createSignal<string | undefined>(undefined);

  const createGroupForm = createForm({ name: '', desc: '', is_admin: false });
  const editForm = createForm({ name: '', desc: '', is_admin: false });

  const rules = {
    name: [
      { required: true, message: '请输入组名' },
      { max: 128, message: '组名最长 128 字符' },
    ],
  };

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [groupList, userList] = await Promise.all([listGroups(), listUsers()]);
      setGroups(groupList);
      setUsers(userList);
      // 编辑弹窗打开时同步刷新其数据（成员变更后保持最新）
      setEditing((prev) =>
        prev ? (groupList.find((group) => group.name === prev.name) ?? null) : prev,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载组列表失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const submitCreate = async () => {
    if (!createGroupForm.validate(rules)) return;
    setCreating(true);
    try {
      const name = String(createGroupForm.values.name).trim();
      await createGroup(
        name,
        String(createGroupForm.values.desc).trim() || '',
        createGroupForm.values.is_admin === true,
      );
      toast.success(`组 ${name} 已创建`);
      setCreateOpen(false);
      createGroupForm.reset();
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建组失败');
    } finally {
      setCreating(false);
    }
  };

  const openEdit = (group: GroupDTO) => {
    setEditing(group);
    setMemberToAdd(undefined);
    editForm.reset({
      name: group.name,
      desc: group.desc,
      is_admin: group.is_admin,
    });
  };

  const submitEdit = async () => {
    const current = editing();
    if (!current) return;
    if (!editForm.validate(rules)) return;
    setEditSaving(true);
    try {
      const updated = await updateGroup(current.name, {
        name: String(editForm.values.name).trim(),
        desc: String(editForm.values.desc).trim() || '',
        is_admin: editForm.values.is_admin === true,
      });
      toast.success(`组 ${updated.name} 已更新`);
      setEditing(null);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '更新组失败');
    } finally {
      setEditSaving(false);
    }
  };

  const addMember = async () => {
    const current = editing();
    const username = memberToAdd();
    if (!current || !username) return;
    setMemberBusy(true);
    try {
      await addGroupMember(current.name, username);
      toast.success(`${username} 已加入组`);
      setMemberToAdd(undefined);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '添加成员失败');
    } finally {
      setMemberBusy(false);
    }
  };

  const removeMember = async (username: string) => {
    const current = editing();
    if (!current) return;
    setMemberBusy(true);
    try {
      await removeGroupMember(current.name, username);
      toast.success(`${username} 已移出组`);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '移除成员失败');
    } finally {
      setMemberBusy(false);
    }
  };

  const remove = async (group: GroupDTO) => {
    try {
      await deleteGroup(group.name);
      toast.success(`组 ${group.name} 已删除`);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除组失败');
    }
  };

  // 候选成员：不在当前组内的用户
  const candidateUsers = createMemo(() => {
    const current = editing();
    if (!current) return [];
    const inGroup = new Set(current.members);
    return users().filter((user) => !inGroup.has(user.username));
  });

  const columns: TableColumnsType<GroupDTO> = [
    {
      title: '组名',
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (value, group) => (
        <span class="inline-flex items-center gap-2">
          <TeamOutlined class="h-4 w-4 text-primary" />
          <code class="mono strong">{String(value)}</code>
          <Show when={group.is_admin}>
            <Tag level="gold">平台管理员</Tag>
          </Show>
        </span>
      ),
    },
    {
      title: '描述',
      dataIndex: 'desc',
      ellipsis: true,
      render: (value) => String(value ?? '') || <span class="muted">—</span>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 100,
      render: (value) => {
        const source = String(value);
        return (
          <Tag level={source === 'ldap' ? 'info' : source === 'local' ? 'neutral' : 'primary'}>
            {source === 'ldap' ? 'LDAP' : source === 'local' ? '手动' : 'SSO'}
          </Tag>
        );
      },
    },
    {
      title: '成员',
      key: 'members',
      render: (_value, group) =>
        group.members.length ? (
          <span class="flex flex-wrap gap-1">
            <For each={group.members}>
              {(member) => (
                <Tag icon={<UserOutlined class="h-3 w-3" />}>{member}</Tag>
              )}
            </For>
          </span>
        ) : (
          <span class="muted">无成员</span>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 170,
      render: (_value, group) => (
        <div class="table-actions">
          <Button
            type="link"
            size="small"
            icon={<EditOutlined class="h-3.5 w-3.5" />}
            onClick={() => openEdit(group)}
          >
            编辑
          </Button>
          <Popconfirm
            title={`删除组「${group.name}」？`}
            description="用户归属关系不受影响；引用该组的策略需另行清理。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(group)}
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

  return (
    <div>
      <Show when={error()}>
        <p class="mb-3 text-sm text-error">{error()}</p>
      </Show>
      <div class="mb-3">
        <Button
          type="primary"
          icon={<PlusOutlined class="h-4 w-4" />}
          onClick={() => {
            createGroupForm.reset();
            setCreateOpen(true);
          }}
        >
          创建组
        </Button>
      </div>
      <Card class="soft-card" contentClass="p-0">
        <Table<GroupDTO>
          rowKey="name"
          columns={columns}
          dataSource={groups()}
          loading={loading()}
          pagination={{ pageSize: 10, showTotal: (total) => `共 ${total} 个组` }}
          locale={{
            emptyText: <Empty description="暂无组；SSO/LDAP 登录的组会自动同步" class="border-0" />,
          }}
        />
      </Card>

      <Modal
        open={createOpen()}
        title="创建组"
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
              创建
            </Button>
          </>
        }
      >
        <div class="form-stack mt-3">
          <Field label="组名" required error={createGroupForm.errors.name}>
            <Input
              value={String(createGroupForm.values.name)}
              onValueChange={(value) => createGroupForm.setValue('name', value)}
              placeholder="dev-team"
              autocomplete="off"
              status={createGroupForm.errors.name ? 'error' : undefined}
            />
          </Field>
          <Field label="描述">
            <TextArea
              value={String(createGroupForm.values.desc)}
              onValueChange={(value) => createGroupForm.setValue('desc', value)}
              placeholder="可选"
              rows={3}
              maxLength={500}
            />
          </Field>
          <Field label="平台管理员" extra="成员下次登录后生效">
            <Switch
              checked={createGroupForm.values.is_admin === true}
              onChange={(checked) => createGroupForm.setValue('is_admin', checked)}
            />
          </Field>
        </div>
      </Modal>

      <Modal
        open={editing() !== null}
        title={editing() ? `编辑组 · ${editing()!.name}` : '编辑组'}
        onClose={() => setEditing(null)}
        width={640}
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
          {(group) => (
            <>
              <div class="form-stack mt-3">
                <Field
                  label="组名"
                  required
                  error={editForm.errors.name}
                  extra={
                    group().source === 'local'
                      ? '重命名会同步用户组字段和 org:<旧名> 策略。'
                      : '外部组由 IdP 管理，组名不可修改。'
                  }
                >
                  <Input
                    value={String(editForm.values.name)}
                    onValueChange={(value) => editForm.setValue('name', value)}
                    class="mono"
                    autocomplete="off"
                    disabled={group().source !== 'local'}
                    status={editForm.errors.name ? 'error' : undefined}
                  />
                </Field>
                <Field label="描述">
                  <TextArea
                    value={String(editForm.values.desc)}
                    onValueChange={(value) => editForm.setValue('desc', value)}
                    rows={2}
                    maxLength={500}
                  />
                </Field>
                <Field label="平台管理员" extra="成员下次登录后生效">
                  <Switch
                    checked={editForm.values.is_admin === true}
                    onChange={(checked) => editForm.setValue('is_admin', checked)}
                  />
                </Field>
              </div>

              <div class="mt-4 font-bold">成员（{group().members.length}）</div>
              <div class="my-2 grid gap-1.5">
                <Show
                  when={group().members.length > 0}
                  fallback={<span class="text-[13px] text-layer-content/50">暂无成员</span>}
                >
                  <For each={group().members}>
                    {(member) => {
                      const user = () =>
                        users().find((item) => item.username === member);
                      return (
                        <div class="flex items-center gap-2 rounded-lg border border-layer-content/10 px-2 py-1">
                          <Avatar size={24} src={user()?.avatar_url || undefined}>
                            {(user()?.nickname || member).slice(0, 1).toUpperCase()}
                          </Avatar>
                          <span class="font-bold">{user()?.nickname || member}</span>
                          <code class="mono text-[12px] text-layer-content/50">{member}</code>
                          <span class="flex-1" />
                          <Show when={group().source === 'local'}>
                            <Button
                              type="link"
                              size="small"
                              danger
                              loading={memberBusy()}
                              onClick={() => void removeMember(member)}
                            >
                              移出
                            </Button>
                          </Show>
                        </div>
                      );
                    }}
                  </For>
                </Show>
              </div>

              <Show
                when={group().source === 'local'}
                fallback={
                  <span class="text-[13px] text-layer-content/50">
                    该组来自 {group().source === 'ldap' ? 'LDAP' : 'SSO'}，成员由登录同步，平台侧只读。
                  </span>
                }
              >
                <div class="flex items-center gap-2">
                  <Select
                    class="flex-1"
                    showSearch
                    allowClear
                    placeholder="选择要添加的用户"
                    value={memberToAdd()}
                    onChange={(value) =>
                      setMemberToAdd(value === undefined ? undefined : String(value))
                    }
                    optionFilterProp="label"
                    options={candidateUsers().map((user) => ({
                      value: user.username,
                      label: `${user.nickname || user.username}（${user.username}）`,
                    }))}
                  />
                  <Button
                    type="primary"
                    icon={<UserAddOutlined class="h-4 w-4" />}
                    loading={memberBusy()}
                    disabled={!memberToAdd()}
                    onClick={() => void addMember()}
                  >
                    添加用户
                  </Button>
                </div>
              </Show>
            </>
          )}
        </Show>
      </Modal>
    </div>
  );
}
