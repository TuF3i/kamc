import { ErrorBoundary as SolidErrorBoundary, type JSX } from 'solid-js';
import { Button, Result } from '@/lib/widgets';

export function ErrorBoundary(props: { children: JSX.Element }) {
  return (
    <SolidErrorBoundary
      fallback={(error) => {
        console.error('[kamc] render error', error);
        return (
          <div class="full-center">
            <Result
              status="500"
              title="页面渲染出错"
              subTitle={error instanceof Error ? error.message : String(error)}
              extra={
                <Button type="primary" onClick={() => window.location.reload()}>
                  刷新页面
                </Button>
              }
            />
          </div>
        );
      }}
    >
      {props.children}
    </SolidErrorBoundary>
  );
}
