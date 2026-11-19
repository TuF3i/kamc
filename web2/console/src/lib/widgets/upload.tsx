import clsx from 'clsx';
import { type JSX } from 'solid-js';

export interface UploadProps {
  accept?: string;
  disabled?: boolean;
  beforeUpload?: (file: File) => boolean | void | Promise<boolean | void>;
  class?: string;
  children?: JSX.Element;
}

/** 隐藏 file input + 自定义触发内容；beforeUpload 返回 false 时不自动上传。 */
export function Upload(props: UploadProps) {
  let input: HTMLInputElement | undefined;

  const pick = () => {
    if (!props.disabled) input?.click();
  };

  return (
    <span class={clsx('inline-flex', props.class)}>
      <input
        ref={input}
        type="file"
        class="hidden"
        accept={props.accept}
        disabled={props.disabled}
        onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          event.currentTarget.value = '';
          if (file) void props.beforeUpload?.(file);
        }}
      />
      <span
        role="button"
        tabindex={props.disabled ? -1 : 0}
        class={clsx(props.disabled && 'cursor-not-allowed opacity-50')}
        onClick={pick}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            pick();
          }
        }}
      >
        {props.children}
      </span>
    </span>
  );
}
