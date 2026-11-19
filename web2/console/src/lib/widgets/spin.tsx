import clsx from 'clsx';
import { SpinnerIcon } from '@/lib/icons';

export interface SpinProps {
  tip?: string;
  size?: 'small' | 'large';
}

export function Spin(props: SpinProps) {
  return (
    <div class="flex flex-col items-center justify-center gap-2 text-sm text-layer-content/60">
      <SpinnerIcon
        class={clsx('animate-spin', props.size === 'large' ? 'h-6 w-6' : 'h-4 w-4')}
      />
      {props.tip ? <span>{props.tip}</span> : null}
    </div>
  );
}

export function FullScreenLoading(props: { tip?: string }) {
  return (
    <div class="full-center">
      <Spin size="large" tip={props.tip ?? '正在加载…'} />
    </div>
  );
}

export function FullScreenBoot(props: { tip?: string }) {
  return (
    <div class="full-screen-loading">
      <SpinnerIcon class="h-7 w-7 animate-spin text-primary" />
      <span>{props.tip ?? '正在加载…'}</span>
    </div>
  );
}
