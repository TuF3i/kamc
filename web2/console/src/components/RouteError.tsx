import { Button, Result } from '@/lib/widgets';

export function RouteError(props: { error?: unknown }) {
  const detail =
    props.error instanceof Error
      ? props.error.message
      : props.error === undefined
        ? '未知错误'
        : String(props.error);

  return (
    <div class="full-center">
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
