// Promise-based confirmation dialogs. Two tiers:
//  - confirm: standard danger dialog (service stop, mc stop/restart)
//  - typed:   the user must type the exact target name (kill, restore,
//             delete, snapraid sync, power actions)
// The resolved value feeds the X-Confirm header, so the server re-verifies.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Icon } from "./Icon";

export interface ConfirmRequest {
  title: string;
  body: ReactNode;
  /** Exact string the server expects in X-Confirm (and the user must type). */
  target: string;
  confirmLabel?: string;
  typed?: boolean;
  danger?: boolean;
}

type ConfirmFn = (req: ConfirmRequest) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext);
  if (!fn) throw new Error("useConfirm outside ConfirmProvider");
  return fn;
}

interface Pending extends ConfirmRequest {
  resolve: (ok: boolean) => void;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<Pending | null>(null);
  const [typedValue, setTypedValue] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const confirm = useCallback<ConfirmFn>((req) => {
    return new Promise((resolve) => {
      setTypedValue("");
      setPending({ ...req, resolve });
    });
  }, []);

  const close = useCallback(
    (ok: boolean) => {
      pending?.resolve(ok);
      setPending(null);
    },
    [pending],
  );

  useEffect(() => {
    if (!pending) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close(false);
    };
    window.addEventListener("keydown", onKey);
    inputRef.current?.focus();
    return () => window.removeEventListener("keydown", onKey);
  }, [pending, close]);

  const ready = !pending?.typed || typedValue === pending.target;

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {pending && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && close(false)}>
          <div className="modal" role="alertdialog" aria-modal="true">
            <div className="modal-h">
              {pending.danger !== false && (
                <Icon name="warning" size={18} className="crit-text" />
              )}
              {pending.title}
            </div>
            <div className="modal-b">
              <div>{pending.body}</div>
              {pending.typed && (
                <>
                  <div className="danger-strip">
                    This action cannot be undone from the panel. Type{" "}
                    <strong className="mono">{pending.target}</strong> to confirm.
                  </div>
                  <input
                    ref={inputRef}
                    className="input mono"
                    value={typedValue}
                    placeholder={pending.target}
                    onChange={(e) => setTypedValue(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && ready) close(true);
                    }}
                    autoComplete="off"
                    spellCheck={false}
                  />
                </>
              )}
            </div>
            <div className="modal-f">
              <button className="btn" onClick={() => close(false)}>
                Cancel
              </button>
              <button
                className={pending.danger === false ? "btn btn-primary" : "btn btn-danger-solid"}
                disabled={!ready}
                onClick={() => close(true)}
                autoFocus={!pending.typed}
              >
                {pending.confirmLabel ?? "Confirm"}
              </button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  );
}
