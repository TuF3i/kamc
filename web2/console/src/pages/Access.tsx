import { createSignal, onMount } from 'solid-js';
import { useSearchParams } from '@solidjs/router';
import { listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { GroupsTab } from '@/components/access/GroupsTab';
import { PoliciesTab } from '@/components/access/PoliciesTab';
import { ScriptsTab } from '@/components/access/ScriptsTab';
import { UsersTab } from '@/components/access/UsersTab';
import { PolicyEditor } from '@/components/PolicyEditor';
import { Tabs } from '@/lib/widgets';

type AccessTab = 'users' | 'groups' | 'policies' | 'scripts';

function param(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] ?? '';
  return value ?? '';
}

export default function Access() {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialTab = param(searchParams.tab) as AccessTab;
  const [tab, setTab] = createSignal<AccessTab>(
    ['users', 'groups', 'policies', 'scripts'].includes(initialTab) ? initialTab : 'users',
  );
  const [clusters, setClusters] = createSignal<Cluster[]>([]);
  const [editorOpen, setEditorOpen] = createSignal(false);
  const [initialSubjectType, setInitialSubjectType] = createSignal<'org' | 'user'>('user');
  const [initialSubject, setInitialSubject] = createSignal<string | undefined>(undefined);
  const [policiesKey, setPoliciesKey] = createSignal(0);

  onMount(() => {
    void listClusters()
      .then(setClusters)
      .catch(() => undefined);

    const subjectType = param(searchParams.subjectType);
    const subject = param(searchParams.subject);
    if (subject) {
      setInitialSubjectType(subjectType === 'org' ? 'org' : 'user');
      setInitialSubject(subject);
      setEditorOpen(true);
      setSearchParams({}, { replace: true });
    }
  });

  return (
    <div>
      <PageHeader
        title="权限管理"
        subtitle="管理平台用户、组、集群访问策略与登录期映射脚本"
      />

      <Tabs
        activeKey={tab()}
        onChange={(key) => setTab(key as AccessTab)}
        items={[
          { key: 'users', label: '用户', children: () => <UsersTab /> },
          { key: 'groups', label: '组', children: () => <GroupsTab /> },
          {
            key: 'policies',
            label: '权限',
            children: () => (
              <div class="tab-stack">
                <PoliciesTab
                  refreshKey={policiesKey()}
                  onCreate={() => {
                    setInitialSubjectType('user');
                    setInitialSubject(undefined);
                    setEditorOpen(true);
                  }}
                />
                <PolicyEditor
                  open={editorOpen()}
                  clusters={clusters()}
                  initialSubjectType={initialSubjectType()}
                  initialSubject={initialSubject()}
                  onClose={() => setEditorOpen(false)}
                  onCreated={() => setPoliciesKey((key) => key + 1)}
                />
              </div>
            ),
          },
          { key: 'scripts', label: '映射脚本', children: () => <ScriptsTab /> },
        ]}
      />
    </div>
  );
}
