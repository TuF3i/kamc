import { Show, createRoot, createSignal, type JSX } from 'solid-js';
import { Portal } from 'solid-js/web';
import { Button } from './button';
import { Modal } from './modal';
import { Popover } from './popover';

export interface ConfirmOptions {
  title: JSX.Element;
  content?: JSX.Element;
  okText?: string;
  cancelText?: string;
  danger?: boolean;
  onOk?: () => void | Promise<void>;
  onCancel?: () => void;
}

interface ConfirmState extends ConfirmOptions {
  loading: boolean;
}

function makeConfirmRoot() {
  const [state, setState] = createSignal<ConfirmState | null>(null);

  const confirm = (options: ConfirmOptions) => {
    setState({ ...options, loading: false });
  };

  const close = () => setState(null);

  const ConfirmHost = () => (
    <Show when={state()}>
      {(current) => (
        <Portal mount={document.body}>
          <Modal
            open={true}
            title={current().title}
            onClose={close}
            width={440}
            footer={
              <>
                <Button
                  type="ghost"
                  size="small"
                  onClick={() => {
                    current().onCancel?.();
                    close();
                  }}
                >
                  {current().cancelText ?? '取消'}
                </Button>
                <Button
                  type="primary"
                  size="small"
                  danger={current().danger}
                  loading={current().loading}
                  onClick={async () => {
                    const onOk = current().onOk;
                    if (!onOk) {
                      close();
                      return;
                    }
                    setState((prev) => (prev ? { ...prev, loading: true } : prev));
                    try {
                      await onOk();
                      close();
                    } catch {
                      setState((prev) => (prev ? { ...prev, loading: false } : prev));
                    }
                  }}
                >
                  {current().okText ?? '确定'}
                </Button>
              </>
            }
          >
            <div class="whitespace-pre-wrap break-words text-[13px] leading-6 text-layer-content/70">
              {current().content}
            </div>
          </Modal>
        </Portal>
      )}
    </Show>
  );

  return { confirm, ConfirmHost };
}

const confirmRoot = createRoot(makeConfirmRoot);

export const confirm = confirmRoot.confirm;
export const ConfirmHost = confirmRoot.ConfirmHost;

export interface PopconfirmProps {
  title: JSX.Element;
  description?: JSX.Element;
  okText?: string;
  cancelText?: string;
  onConfirm?: () => void | Promise<void>;
  okButtonProps?: { danger?: boolean; disabled?: boolean };
  disabled?: boolean;
  children: JSX.Element;
}

export function Popconfirm(props: PopconfirmProps) {
  const [open, setOpen] = createSignal(false);
  const [loading, setLoading] = createSignal(false);

  return (
    <Popover
      open={open()}
      onOpenChange={(next) => {
        if (props.disabled) return;
        setOpen(next);
      }}
      panelClass="w-72 p-3"
      minWidth={288}
      placement="bottom-end"
      trigger={props.children}
    >
      <div class="confirm-title">{props.title}</div>
      <Show when={props.description}>
        <div class="confirm-desc">{props.description}</div>
      </Show>
      <div class="confirm-actions">
        <Button type="ghost" size="small" onClick={() => setOpen(false)}>
          {props.cancelText ?? '取消'}
        </Button>
        <Button
          type="primary"
          size="small"
          danger={props.okButtonProps?.danger}
          disabled={props.okButtonProps?.disabled}
          loading={loading()}
          onClick={async () => {
            if (!props.onConfirm) {
              setOpen(false);
              return;
            }
            setLoading(true);
            try {
              await props.onConfirm();
              setOpen(false);
            } finally {
              setLoading(false);
            }
          }}
        >
          {props.okText ?? '确定'}
        </Button>
      </div>
    </Popover>
  );
}
