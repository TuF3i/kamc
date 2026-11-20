import { Show, type JSX } from 'solid-js';

export interface PageHeaderProps {
  title: JSX.Element;
  subtitle?: JSX.Element;
  tags?: JSX.Element;
  extra?: JSX.Element;
  icon?: JSX.Element;
}

/** 页面标题区：标题 + 可选标签 + 副标题 + 右侧操作（标题默认不加图标）。 */
export function PageHeader(props: PageHeaderProps) {
  return (
    <div class="page-header">
      <div class="page-header-main">
        <div class="flex flex-wrap items-center gap-2.5">
          <Show when={props.icon}>
            <span class="page-title-icon">{props.icon}</span>
          </Show>
          <h1 class="page-title">{props.title}</h1>
          {props.tags}
        </div>
        <Show when={props.subtitle}>
          <div class="page-subtitle">{props.subtitle}</div>
        </Show>
      </div>
      <Show when={props.extra}>
        <div class="page-header-extra">{props.extra}</div>
      </Show>
    </div>
  );
}
