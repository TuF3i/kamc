import { createSignal } from 'solid-js';
import { Button } from '@/lib/widgets';
import { toast } from '@/lib/storage/toast';
import { CheckOutlined, CopyOutlined } from '@/lib/icons';

export interface CopyButtonProps {
  text: string;
  size?: 'small' | 'middle' | 'large';
  label?: string;
  type?: 'text' | 'default' | 'primary';
}

/** 复制到剪贴板，Clipboard API 不可用时回退到 textarea。 */
export function CopyButton(props: CopyButtonProps) {
  const [copied, setCopied] = createSignal(false);

  const copy = async () => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(props.text);
      } else {
        const area = document.createElement('textarea');
        area.value = props.text;
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.appendChild(area);
        area.select();
        document.execCommand('copy');
        area.remove();
      }
      setCopied(true);
      toast.success('已复制到剪贴板');
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error('复制失败，请手动选择文本');
    }
  };

  return (
    <Button
      size={props.size ?? 'small'}
      type={props.type ?? 'text'}
      icon={copied() ? <CheckOutlined class="h-4 w-4" /> : <CopyOutlined class="h-4 w-4" />}
      onClick={() => void copy()}
    >
      {props.label ?? '复制'}
    </Button>
  );
}
