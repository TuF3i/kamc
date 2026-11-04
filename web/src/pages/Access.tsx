import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Tabs } from 'antd';
import { PageHeader } from '@/components/PageHeader';
import { GroupsTab } from '@/components/access/GroupsTab';
import { PoliciesTab } from '@/components/access/PoliciesTab';
import { ScriptsTab } from '@/components/access/ScriptsTab';
import { UsersTab } from '@/components/access/UsersTab';
import { PolicyEditor } from '@/components/PolicyEditor';
import { listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';

type AccessTab = 'users' | 'groups' | 'policies' | 'scripts';

export function Access() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [tab, setTab] = useState<AccessTab>((searchParams.get('tab') as AccessTab) || 'users');
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [editorOpen, setEditorOpen] = useState(false);
  const [initialSubjectType, setInitialSubjectType] = useState<'org' | 'user'>('user');
  const [initialSubject, setInitialSubject] = useState<string>();
  const [policiesKey, setPoliciesKey] = useState(0);

  useEffect(() => {
    void listClusters()
      .then(setClusters)
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    const subjectType = searchParams.get('subjectType');
    const subject = searchParams.get('subject');
    if (subject) {
      setInitialSubjectType(subjectType === 'org' ? 'org' : 'user');
      setInitialSubject(subject);
      setEditorOpen(true);
      setSearchParams({}, { replace: true });
    }
    // 只在首次进入时读取预填参数
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div>
      <PageHeader
        title="权限管理"
        subtitle="平台用户目录、SSO/LDAP 组、集群策略与登录期映射脚本。"
      />

      <Tabs
        activeKey={tab}
        onChange={(key) => setTab(key as AccessTab)}
        items={[
          { key: 'users', label: '用户', children: <UsersTab /> },
          { key: 'groups', label: '组', children: <GroupsTab /> },
          {
            key: 'policies',
            label: '权限',
            children: (
              <div className="tab-stack">
                <PoliciesTab
                  key={policiesKey}
                  onCreate={() => {
                    setInitialSubjectType('user');
                    setInitialSubject(undefined);
                    setEditorOpen(true);
                  }}
                />
                <PolicyEditor
                  open={editorOpen}
                  clusters={clusters}
                  initialSubjectType={initialSubjectType}
                  initialSubject={initialSubject}
                  onClose={() => setEditorOpen(false)}
                  onCreated={() => setPoliciesKey((k) => k + 1)}
                />
              </div>
            ),
          },
          { key: 'scripts', label: '映射脚本', children: <ScriptsTab /> },
        ]}
      />
    </div>
  );
}
