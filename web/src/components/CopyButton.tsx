import { useState } from 'react';
import { App, Button } from 'antd';
import { CheckOutlined, CopyOutlined } from '@ant-design/icons';

interface CopyButtonProps {
  text: string;
  size?: 'small' | 'middle' | 'large';
  label?: string;
  type?: 'text' | 'default' | 'primary';
}

/** 复制到剪贴板，Clipboard API 不可用时回退到 textarea。 */
export function CopyButton({ text, size = 'small', label, type = 'text' }: CopyButtonProps) {
  const { message } = App.useApp();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
      } else {
        const area = document.createElement('textarea');
        area.value = text;
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.appendChild(area);
        area.select();
        document.execCommand('copy');
        area.remove();
      }
      setCopied(true);
      message.success('已复制到剪贴板');
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      message.error('复制失败，请手动选择文本');
    }
  };

  return (
    <Button
      size={size}
      type={type}
      icon={copied ? <CheckOutlined /> : <CopyOutlined />}
      onClick={copy}
    >
      {label ?? '复制'}
    </Button>
  );
}
