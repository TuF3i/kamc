import { Button, Result } from 'antd';
import { isRouteErrorResponse, useRouteError } from 'react-router-dom';

/** 路由级错误边界：懒加载模块失败、页面渲染异常时给出可恢复入口。 */
export function RouteError() {
  const error = useRouteError();
  const detail = isRouteErrorResponse(error)
    ? `${error.status} ${error.statusText}`
    : error instanceof Error
      ? error.message
      : '未知错误';

  return (
    <div className="full-center">
      <Result
        status="500"
        title="页面加载失败"
        subTitle={detail}
        extra={
          <Button type="primary" onClick={() => window.location.reload()}>
            重新加载
          </Button>
        }
      />
    </div>
  );
}
