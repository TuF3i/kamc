import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { PageHeader } from '@/components/PageHeader';
import { SafetyCertificateOutlined } from '@ant-design/icons';
import { GroupsTab } from '@/components/access/GroupsTab';
import { PoliciesTab } from '@/components/access/PoliciesTab';
import { ScriptsTab } from '@/components/access/ScriptsTab';
import { UsersTab } from '@/components/access/UsersTab';
import { PolicyEditor } from '@/components/PolicyEditor';
import { listClusters } from '@/api/clusters';
import { listRobots } from '@/api/robots';
import type { Cluster, Robot } from '@/api/types';

type AccessTab = 'users' | 'groups' | 'policies' | 'scripts';

const TABS: { key: AccessTab; label: string }[] = [
  { key: 'users', label: '用户' },
  { key: 'groups', label: '组' },
  { key: 'policies', label: '权限' },
  { key: 'scripts', label: '映射脚本' },
];

export function Access() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [tab, setTab] = useState<AccessTab>((searchParams.get('tab') as AccessTab) || 'users');
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [robots, setRobots] = useState<Robot[]>([]);
  const [editorOpen, setEditorOpen] = useState(false);
  const [initialSubjectType, setInitialSubjectType] = useState<'org' | 'user'>('user');
  const [initialSubject, setInitialSubject] = useState<string>();

  useEffect(() => {
    void Promise.all([listClusters(), listRobots()])
      .then(([clusterList, robotList]) => {
        setClusters(clusterList);
        setRobots(robotList);
      })
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

  const switchTab = (key: AccessTab) => {
    setTab(key);
  };

  return (
    <div>
      <PageHeader
        title="权限管理"
        icon={<SafetyCertificateOutlined />}
        subtitle="平台用户目录、SSO/LDAP 组、集群策略与登录期映射脚本。"
        extra={
          <div className="subtab-bar" role="tablist" aria-label="权限子页签">
            {TABS.map((t) => (
              <button
                key={t.key}
                type="button"
                role="tab"
                aria-selected={tab === t.key}
                className={`subtab${tab === t.key ? ' active' : ''}`}
                onClick={() => switchTab(t.key)}
              >
                {t.label}
              </button>
            ))}
          </div>
        }
      />

      {tab === 'users' ? <UsersTab /> : null}
      {tab === 'groups' ? <GroupsTab /> : null}
      {tab === 'policies' ? (
        <div className="tab-stack">
          <PoliciesTab />
          <PolicyEditor
            open={editorOpen}
            clusters={clusters}
            robots={robots}
            initialSubjectType={initialSubjectType}
            initialSubject={initialSubject}
            onClose={() => setEditorOpen(false)}
            onCreated={() => undefined}
          />
        </div>
      ) : null}
      {tab === 'scripts' ? <ScriptsTab /> : null}
    </div>
  );
}
