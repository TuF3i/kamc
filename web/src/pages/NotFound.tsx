import { useNavigate } from '@solidjs/router';
import { Button, Result } from '@/lib/widgets';

export default function NotFound() {
  const navigate = useNavigate();
  return (
    <Result
      status="404"
      title="页面不存在"
      subTitle="你访问的页面可能已被移除，或地址输入有误。"
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          返回概览
        </Button>
      }
    />
  );
}
