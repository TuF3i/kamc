import clsx from 'clsx';
import { Show, createSignal, splitProps, type JSX } from 'solid-js';
import { DismissIcon, EyeOffIcon, EyeOutlined } from '@/lib/icons';

export interface InputProps
  extends Omit<
    JSX.InputHTMLAttributes<HTMLInputElement>,
    'size' | 'prefix' | 'onInput' | 'onChange'
  > {
  value?: string;
  size?: 'small' | 'middle';
  status?: 'error';
  prefix?: JSX.Element;
  allowClear?: boolean;
  onValueChange?: (value: string) => void;
  onInput?: (event: InputEvent & { currentTarget: HTMLInputElement }) => void;
  onChange?: (event: Event & { currentTarget: HTMLInputElement }) => void;
}

function inputClass(props: {
  size?: 'small' | 'middle';
  status?: 'error';
  prefix?: JSX.Element;
  class?: string;
}): string {
  return clsx(
    'input',
    props.size === 'small' && 'input-sm',
    props.status === 'error' && 'input-error',
    props.prefix && 'input-has-prefix',
    props.class,
  );
}

export function Input(props: InputProps) {
  const [local, rest] = splitProps(props, [
    'value',
    'size',
    'status',
    'prefix',
    'class',
    'allowClear',
    'onValueChange',
    'onInput',
    'onChange',
  ]);

  const showClear = () => local.allowClear && (local.value ?? '') !== '';

  const control = (
    <input
      {...rest}
      class={inputClass({ ...local, class: clsx(local.class, showClear() && 'input-has-action') })}
      value={local.value ?? ''}
      onInput={(event) => {
        local.onValueChange?.(event.currentTarget.value);
        local.onInput?.(event);
      }}
      onChange={(event) => {
        local.onChange?.(event);
      }}
    />
  );

  return (
    <Show when={local.prefix || showClear()} fallback={control}>
      <div class={clsx('input-wrap', local.class)}>
        <Show when={local.prefix}>
          <span class="input-prefix">{local.prefix}</span>
        </Show>
        {control}
        <Show when={showClear()}>
          <span class="input-action">
            <button
              type="button"
              class="icon-btn h-6 w-6"
              aria-label="清除"
              onClick={() => local.onValueChange?.('')}
            >
              <DismissIcon class="h-3 w-3" />
            </button>
          </span>
        </Show>
      </div>
    </Show>
  );
}

export function PasswordInput(props: InputProps) {
  const [visible, setVisible] = createSignal(false);
  const [local, rest] = splitProps(props, [
    'value',
    'size',
    'status',
    'prefix',
    'class',
    'onValueChange',
    'onInput',
    'onChange',
  ]);

  return (
    <div class={clsx('input-wrap', local.class)}>
      <Show when={local.prefix}>
        <span class="input-prefix">{local.prefix}</span>
      </Show>
      <input
        {...rest}
        type={visible() ? 'text' : 'password'}
        class={clsx(inputClass(local), 'input-has-action')}
        value={local.value ?? ''}
        onInput={(event) => {
          local.onValueChange?.(event.currentTarget.value);
          local.onInput?.(event);
        }}
        onChange={(event) => {
          local.onChange?.(event);
        }}
      />
      <span class="input-action">
        <button
          type="button"
          class="icon-btn"
          aria-label={visible() ? '隐藏密码' : '显示密码'}
          onClick={() => setVisible((value) => !value)}
        >
          <Show when={visible()} fallback={<EyeOutlined class="h-4 w-4" />}>
            <EyeOffIcon class="h-4 w-4" />
          </Show>
        </button>
      </span>
    </div>
  );
}

export interface TextAreaProps
  extends Omit<JSX.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onInput' | 'onChange'> {
  value?: string;
  status?: 'error';
  showCount?: boolean;
  onValueChange?: (value: string) => void;
  onInput?: (event: InputEvent & { currentTarget: HTMLTextAreaElement }) => void;
  onChange?: (event: Event & { currentTarget: HTMLTextAreaElement }) => void;
}

export function TextArea(props: TextAreaProps) {
  const [local, rest] = splitProps(props, [
    'value',
    'status',
    'class',
    'showCount',
    'onValueChange',
    'onInput',
    'onChange',
  ]);

  const area = (
    <textarea
      {...rest}
      class={clsx(
        'input',
        local.status === 'error' && 'input-error',
        local.showCount && 'pb-6',
        local.class,
      )}
      value={local.value ?? ''}
      onInput={(event) => {
        local.onValueChange?.(event.currentTarget.value);
        local.onInput?.(event);
      }}
      onChange={(event) => {
        local.onChange?.(event);
      }}
    />
  );

  return (
    <Show when={local.showCount} fallback={area}>
      <div class="relative w-full">
        {area}
        <span class="pointer-events-none absolute bottom-1.5 right-3 text-[11px] text-layer-content/40">
          {(local.value ?? '').length} / {rest.maxLength ?? 0}
        </span>
      </div>
    </Show>
  );
}
