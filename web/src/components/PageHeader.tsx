import type { ReactNode } from 'react';
import { Space, Typography } from 'antd';

interface PageHeaderProps {
  title: ReactNode;
  subtitle?: ReactNode;
  tags?: ReactNode;
  extra?: ReactNode;
}

/** 页面标题区：标题 + 可选标签 + 副标题 + 右侧操作。 */
export function PageHeader({ title, subtitle, tags, extra }: PageHeaderProps) {
  return (
    <div className="page-header">
      <div className="page-header-main">
        <Space align="center" size={10} wrap>
          <Typography.Title level={3} className="page-title">
            {title}
          </Typography.Title>
          {tags}
        </Space>
        {subtitle ? <div className="page-subtitle">{subtitle}</div> : null}
      </div>
      {extra ? <div className="page-header-extra">{extra}</div> : null}
    </div>
  );
}
