import { Show, type JSX } from 'solid-js';

export interface ResultProps {
  status?: '404' | '500' | '403';
  title?: JSX.Element;
  subTitle?: JSX.Element;
  extra?: JSX.Element;
}

export function Result(props: ResultProps) {
  return (
    <div class="result">
      <div class="result-code">{props.status ?? '500'}</div>
      <Show when={props.title}>
        <div class="result-title">{props.title}</div>
      </Show>
      <Show when={props.subTitle}>
        <div class="result-sub">{props.subTitle}</div>
      </Show>
      <Show when={props.extra}>{props.extra}</Show>
    </div>
  );
}
