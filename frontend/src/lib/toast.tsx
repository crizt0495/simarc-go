/**
 * ToastProvider — notifikasi halus di pojok kanan atas, auto-hide 3.5 dtk.
 */
import React, { createContext, useCallback, useContext, useState } from 'react';
import { CheckCircle2, AlertTriangle, XCircle, Info, X } from 'lucide-react';

export type ToastKind = 'success' | 'warning' | 'danger' | 'info';
export interface Toast { id: number; kind: ToastKind; msg: string }

const Ctx = createContext<{ push: (kind: ToastKind, msg: string) => void }>({ push: () => {} });

const ICONS: Record<ToastKind, React.ReactNode> = {
  success: <CheckCircle2 size={16} className="text-ok shrink-0" />,
  warning: <AlertTriangle size={16} className="text-warn shrink-0" />,
  danger: <XCircle size={16} className="text-bad shrink-0" />,
  info: <Info size={16} className="text-info shrink-0" />,
};

let nextId = 1;

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((kind: ToastKind, msg: string) => {
    const id = nextId++;
    setToasts((ts) => [...ts, { id, kind, msg }]);
    setTimeout(() => setToasts((ts) => ts.filter((t) => t.id !== id)), 3500);
  }, []);

  const dismiss = (id: number) => setToasts((ts) => ts.filter((t) => t.id !== id));

  return (
    <Ctx.Provider value={{ push }}>
      {children}
      <div className="fixed top-4 right-4 z-[80] flex flex-col gap-2 w-80 pointer-events-none">
        {toasts.map((t) => (
          <div
            key={t.id}
            className="pointer-events-auto flex items-start gap-2.5 card rounded-lg px-3.5 py-2.5 shadow-modal animate-toast-in"
          >
            {ICONS[t.kind]}
            <p className="flex-1 text-body text-ink-1 leading-snug">{t.msg}</p>
            <button
              onClick={() => dismiss(t.id)}
              className="text-ink-3 hover:text-ink-1 transition-colors"
              aria-label="Tutup"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
};

export const useToast = () => useContext(Ctx);
