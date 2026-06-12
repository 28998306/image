import {useEffect, useState} from "react";

interface ConfirmOptions {
  title?: string;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
}

interface ConfirmState extends ConfirmOptions {
  open: boolean;
  message: string;
}

const initialState: ConfirmState = {open: false, message: ""};

let resolver: ((value: boolean) => void) | null = null;
let pushState: ((state: ConfirmState) => void) | null = null;

/**
 * confirmDialog 弹出一个居中的确认框，返回用户是否点击「确定」。
 * 用于替代 window.confirm（WebView2 的原生弹窗会跑到窗口顶部）。
 */
export function confirmDialog(message: string, options?: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    // 若已有未决弹窗，先按取消处理。
    if (resolver) resolver(false);
    resolver = resolve;
    pushState?.({open: true, message, ...options});
  });
}

/** ConfirmHost 挂载在应用根部，负责渲染确认框。只需挂一次。 */
export function ConfirmHost() {
  const [state, setState] = useState<ConfirmState>(initialState);

  useEffect(() => {
    pushState = setState;
    return () => {
      pushState = null;
    };
  }, []);

  const finish = (value: boolean) => {
    setState((s) => ({...s, open: false}));
    const r = resolver;
    resolver = null;
    r?.(value);
  };

  if (!state.open) return null;

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/40 p-6"
      onClick={() => finish(false)}
    >
      <div
        className="w-full max-w-sm border border-slate-200 bg-white shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b border-slate-100 px-5 py-3">
          <h3 className="text-sm font-medium text-slate-800">{state.title || "确认操作"}</h3>
        </div>
        <div className="px-5 py-4 text-sm leading-relaxed text-slate-600">{state.message}</div>
        <div className="flex justify-end gap-2 border-t border-slate-100 px-5 py-3">
          <button className="btn-secondary" type="button" onClick={() => finish(false)}>
            {state.cancelText || "取消"}
          </button>
          <button
            className={state.danger ? "btn-primary bg-rose-600 hover:bg-rose-700" : "btn-primary"}
            type="button"
            onClick={() => finish(true)}
            autoFocus
          >
            {state.confirmText || "确定"}
          </button>
        </div>
      </div>
    </div>
  );
}
