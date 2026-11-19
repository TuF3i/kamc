import clsx from 'clsx';
import dayjs, { type Dayjs } from 'dayjs';
import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
  type JSX,
} from 'solid-js';
import { Portal } from 'solid-js/web';
import { CalendarIcon, DismissIcon, DownOutlined, ChevronUpIcon } from '@/lib/icons';

export interface DateRangeProps {
  value?: [Dayjs | null, Dayjs | null] | null;
  onChange?: (
    value: [Dayjs, Dayjs] | null,
    strings?: [string, string] | null,
  ) => void;
  allowClear?: boolean;
  showTime?: boolean;
  placeholder?: [string, string];
  class?: string;
  style?: JSX.CSSProperties;
  disabled?: boolean;
  size?: 'small' | 'middle';
}

const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六'];

function sameDay(a: Dayjs | null | undefined, b: Dayjs): boolean {
  return Boolean(a && a.isSame(b, 'day'));
}

export function DateRange(props: DateRangeProps) {
  let trigger: HTMLDivElement | undefined;
  let panel: HTMLDivElement | undefined;

  const [open, setOpen] = createSignal(false);
  const [pos, setPos] = createSignal({ left: 0, top: 0 });
  const [view, setView] = createSignal(dayjs().startOf('month'));
  const [start, setStart] = createSignal<Dayjs | null>(null);
  const [end, setEnd] = createSignal<Dayjs | null>(null);

  createEffect(() => {
    const value = props.value;
    setStart(value?.[0] ?? null);
    setEnd(value?.[1] ?? null);
  });

  const place = () => {
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    setPos({
      left: Math.max(8, Math.min(rect.left, window.innerWidth - 302)),
      top: rect.bottom + 4,
    });
  };

  const openPanel = () => {
    if (props.disabled) return;
    if (open()) {
      setOpen(false);
      return;
    }
    place();
    setView((start() ?? dayjs()).startOf('month'));
    setOpen(true);
  };

  createEffect(() => {
    if (!open()) return;
    const onDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (trigger?.contains(target) || panel?.contains(target)) return;
      setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    const onMove = () => place();
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onMove);
    window.addEventListener('scroll', onMove, true);
    onCleanup(() => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onMove);
      window.removeEventListener('scroll', onMove, true);
    });
  });

  const days = createMemo(() => {
    const first = view().startOf('month');
    const offset = first.day();
    const startDay = first.subtract(offset, 'day');
    return Array.from({ length: 42 }, (_, index) => startDay.add(index, 'day'));
  });

  const pick = (day: Dayjs) => {
    const currentStart = start();
    const currentEnd = end();
    if (!currentStart || (currentStart && currentEnd)) {
      setStart(day);
      setEnd(null);
      return;
    }
    if (day.isBefore(currentStart, 'day')) {
      setStart(day);
      setEnd(currentStart);
      props.onChange?.([day, currentStart], [day.format('YYYY-MM-DD'), currentStart.format('YYYY-MM-DD')]);
      setOpen(false);
      return;
    }
    setEnd(day);
    props.onChange?.(
      [currentStart, day],
      [currentStart.format('YYYY-MM-DD'), day.format('YYYY-MM-DD')],
    );
    setOpen(false);
  };

  const clear = () => {
    setStart(null);
    setEnd(null);
    props.onChange?.(null, null);
    setOpen(false);
  };

  const label = (value: Dayjs | null, fallback: string) => (
    <span class="date-range-part" data-empty={value ? 'false' : 'true'}>
      {value ? value.format('YYYY-MM-DD') : fallback}
    </span>
  );

  const inRange = (day: Dayjs) => {
    const a = start();
    const b = end();
    if (!a || !b) return false;
    return !day.isBefore(a, 'day') && !day.isAfter(b, 'day');
  };

  return (
    <>
      <div
        ref={trigger}
        class={clsx('date-range', props.size === 'small' && 'input-sm', props.class)}
        style={props.style}
        data-open={open() ? 'true' : 'false'}
        data-disabled={props.disabled ? 'true' : 'false'}
        role="button"
        tabindex={props.disabled ? -1 : 0}
        onClick={openPanel}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            openPanel();
          }
        }}
      >
        <CalendarIcon class="h-4 w-4 shrink-0 text-layer-content/40" />
        {label(start(), props.placeholder?.[0] ?? '开始日期')}
        <span class="date-range-sep">→</span>
        {label(end(), props.placeholder?.[1] ?? '结束日期')}
        <Show when={props.allowClear !== false && (start() || end())}>
          <span
            class="select-clear"
            role="button"
            aria-label="清除"
            onClick={(event) => {
              event.stopPropagation();
              clear();
            }}
          >
            <DismissIcon class="h-3 w-3" />
          </span>
        </Show>
        <span class="select-chevron">
          <Show when={open()} fallback={<DownOutlined class="h-4 w-4" />}>
            <ChevronUpIcon class="h-4 w-4" />
          </Show>
        </span>
      </div>

      <Show when={open()}>
        <Portal mount={document.body}>
          <div
            ref={panel}
            class="date-popup"
            style={{ left: `${pos().left}px`, top: `${pos().top}px` }}
          >
            <div class="date-head">
              <button
                type="button"
                class="date-nav"
                aria-label="上个月"
                onClick={() => setView(view().subtract(1, 'month'))}
              >
                ‹
              </button>
              <div class="date-month">{view().format('YYYY年M月')}</div>
              <button
                type="button"
                class="date-nav"
                aria-label="下个月"
                onClick={() => setView(view().add(1, 'month'))}
              >
                ›
              </button>
            </div>
            <div class="date-week">
              <For each={WEEKDAYS}>{(day) => <span>{day}</span>}</For>
            </div>
            <div class="date-grid">
              <For each={days()}>
                {(day) => (
                  <button
                    type="button"
                    class="date-day"
                    data-outside={day.month() !== view().month() ? 'true' : 'false'}
                    data-in-range={inRange(day) ? 'true' : 'false'}
                    data-edge={
                      sameDay(start(), day) || sameDay(end(), day) ? 'true' : 'false'
                    }
                    onClick={() => pick(day)}
                  >
                    {day.date()}
                  </button>
                )}
              </For>
            </div>
          </div>
        </Portal>
      </Show>
    </>
  );
}
