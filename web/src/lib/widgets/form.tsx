import { createStore, reconcile } from 'solid-js/store';
import { Show, type JSX } from 'solid-js';
import clsx from 'clsx';

export interface Rule {
  required?: boolean;
  /** 校验失败提示；validator 自带文案时可省略 */
  message?: string;
  max?: number;
  pattern?: RegExp;
  /** 自定义校验：返回错误文案（undefined = 通过） */
  validator?: (value: unknown) => string | undefined;
}

export type RuleMap = Record<string, Rule[]>;

function runRule(rule: Rule, value: unknown): string | undefined {
  if (rule.validator) return rule.validator(value);
  const empty =
    value === undefined ||
    value === null ||
    value === '' ||
    (Array.isArray(value) && value.length === 0);
  if (rule.required && empty) return rule.message ?? '该项为必填';
  if (rule.max !== undefined && typeof value === 'string' && value.length > rule.max) {
    return rule.message;
  }
  if (rule.pattern && typeof value === 'string' && value && !rule.pattern.test(value)) {
    return rule.message;
  }
  return undefined;
}

/** 轻量表单状态：受控值 + 逐字段校验，文案由调用方逐字传入。 */
export function createForm<T extends Record<string, unknown>>(initial: T) {
  const [values, setValues] = createStore<T>({ ...initial });
  const [errors, setErrors] = createStore<Record<string, string | undefined>>({});

  const setValue = <K extends keyof T>(key: K, value: T[K]) => {
    setValues((prev) => ({ ...prev, [key]: value }) as T);
    setErrors(String(key), undefined);
  };

  const patch = (next: Partial<T>) => {
    setValues(reconcile({ ...values, ...next } as T));
  };

  const validate = (rules: RuleMap): boolean => {
    let ok = true;
    for (const [key, list] of Object.entries(rules)) {
      for (const rule of list) {
        const message = runRule(rule, values[key]);
        if (message) {
          setErrors(key, message);
          ok = false;
          break;
        }
        setErrors(key, undefined);
      }
    }
    return ok;
  };

  const reset = (next?: T) => {
    setValues(reconcile({ ...(next ?? initial) } as T));
    setErrors(reconcile({}));
  };

  return { values, errors, setValue, patch, validate, reset };
}

export interface FieldProps {
  label?: JSX.Element;
  required?: boolean;
  extra?: JSX.Element;
  error?: string;
  for?: string;
  class?: string;
  children: JSX.Element;
}

export function Field(props: FieldProps) {
  return (
    <div class={clsx('field', props.class)}>
      <Show when={props.label}>
        <label class={clsx('field-label', props.required && 'field-required')} for={props.for}>
          {props.label}
        </label>
      </Show>
      {props.children}
      <Show when={props.extra}>
        <div class="field-extra">{props.extra}</div>
      </Show>
      <Show when={props.error}>
        <div class="field-error">{props.error}</div>
      </Show>
    </div>
  );
}
