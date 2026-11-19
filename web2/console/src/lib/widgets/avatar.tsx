import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';

export interface AvatarProps {
  size?: number;
  src?: string;
  alt?: string;
  class?: string;
  children?: JSX.Element;
}

export function Avatar(props: AvatarProps) {
  const size = () => props.size ?? 30;
  const fallback = () => {
    const text = typeof props.children === 'string' ? props.children : '';
    return text.slice(0, 1).toUpperCase();
  };

  return (
    <span
      class={clsx(
        'relative inline-grid shrink-0 select-none place-items-center overflow-hidden rounded-lg bg-gradient-to-br from-primary to-info font-bold text-primary-content',
        props.class,
      )}
      style={{
        width: `${size()}px`,
        height: `${size()}px`,
        'font-size': `${Math.max(11, Math.round(size() * 0.42))}px`,
      }}
    >
      <Show when={props.src} fallback={fallback()}>
        <img
          src={props.src}
          alt={props.alt ?? ''}
          class="absolute inset-0 h-full w-full object-cover"
        />
      </Show>
    </span>
  );
}
