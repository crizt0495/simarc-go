/**
 * Komponen UI reusable — Button, Badge, Modal, ConfirmDialog,
 * EmptyState/FormSection/LoadingState.
 */
import React from 'react';
import { Loader2, X, LucideIcon } from 'lucide-react';

/* ── Button ────────────────────────────────────────────────────────── */
export type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';

const BTN: Record<ButtonVariant, string> = {
  primary: 'bg-primary text-white hover:bg-primary-hover focus-visible:ring-[var(--c-primary-ring)]',
  secondary: 'bg-surface text-ink-1 border border-line hover:bg-surface-2 focus-visible:ring-[var(--c-primary-ring)]',
  outline: 'bg-transparent text-ink-1 border border-line hover:bg-surface-2 focus-visible:ring-[var(--c-primary-ring)]',
  ghost: 'bg-transparent text-ink-2 hover:text-ink-1 hover:bg-surface-2 focus-visible:ring-[var(--c-primary-ring)]',
  danger: 'bg-bad text-white hover:brightness-90 focus-visible:ring-red-400/40',
};

export const Button: React.FC<{
  variant?: ButtonVariant;
  size?: 'sm' | 'md';
  loading?: boolean;
  icon?: LucideIcon;
  onClick?: () => void;
  type?: 'button' | 'submit';
  disabled?: boolean;
  className?: string;
  children: React.ReactNode;
  title?: string;
}> = ({
  variant = 'primary', size = 'md', loading = false,
  icon: Icon, onClick, type = 'button', disabled,
  className = '', children, title,
}) => (
  <button
    type={type}
    title={title}
    onClick={onClick}
    disabled={disabled || loading}
    className={`inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-colors duration-150
      focus-visible:outline-none focus-visible:ring-2 disabled:opacity-50 disabled:cursor-not-allowed
      ${size === 'sm' ? 'h-8 px-3 text-caption' : 'h-10 px-4 text-btn'}
      ${BTN[variant]} ${className}`}
  >
    {loading ? <Loader2 size={15} className="animate-spin" /> : Icon ? <Icon size={15} /> : null}
    {children}
  </button>
);

/* ── Badge ─────────────────────────────────────────────────────────── */
export type BadgeTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral';

const TONE: Record<BadgeTone, string> = {
  success: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/25',
  warning: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-500/10 dark:text-amber-400 dark:border-amber-500/25',
  danger: 'bg-red-50 text-red-700 border-red-200 dark:bg-red-500/10 dark:text-red-400 dark:border-red-500/25',
  info: 'bg-sky-50 text-sky-700 border-sky-200 dark:bg-sky-500/10 dark:text-sky-400 dark:border-sky-500/25',
  neutral: 'bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-500/10 dark:text-slate-300 dark:border-slate-500/20',
};

export const Badge: React.FC<{ tone?: BadgeTone; children: React.ReactNode }> = ({
  tone = 'neutral', children,
}) => (
  <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full border text-caption font-medium ${TONE[tone]}`}>
    {children}
  </span>
);

/* ── Modal ─────────────────────────────────────────────────────────── */
export const Modal: React.FC<{
  open: boolean;
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: string;
}> = ({ open, title, onClose, children, footer, width = 'max-w-md' }) => {
  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div className="absolute inset-0 bg-black/30" onClick={onClose} />
      <div className={`relative bg-surface rounded-xl shadow-modal w-full ${width} animate-fade-in max-h-[85vh] flex flex-col`}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-line">
          <h2 className="text-section font-semibold text-ink-1">{title}</h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-ink-3 hover:text-ink-1 hover:bg-surface-2 transition-colors"
            aria-label="Tutup modal"
          >
            <X size={16} />
          </button>
        </div>
        <div className="px-5 py-4 overflow-y-auto clamp flex-1">{children}</div>
        {footer && <div className="px-5 py-3.5 border-t border-line flex items-center justify-end gap-2">{footer}</div>}
      </div>
    </div>
  );
};

/* ── ConfirmDialog ─────────────────────────────────────────────────── */
export const ConfirmDialog: React.FC<{
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}> = ({ open, title, message, confirmLabel = 'Hapus', onConfirm, onCancel }) => (
  <Modal
    open={open}
    title={title}
    onClose={onCancel}
    width="max-w-sm"
    footer={
      <>
        <Button variant="secondary" onClick={onCancel}>Batal</Button>
        <Button variant="danger" onClick={onConfirm}>{confirmLabel}</Button>
      </>
    }
  >
    <p className="text-body text-ink-2">{message}</p>
  </Modal>
);

/* ── Empty / Loading / Section ─────────────────────────────────────── */
export const EmptyState: React.FC<{
  icon: LucideIcon;
  title: string;
  message: string;
  action?: React.ReactNode;
}> = ({ icon: Icon, title, message, action }) => (
  <div className="flex flex-col items-center justify-center py-16 text-center">
    <div className="w-11 h-11 rounded-xl bg-surface-2 border border-line flex items-center justify-center mb-3">
      <Icon size={20} className="text-ink-3" />
    </div>
    <p className="text-card-title text-ink-1 mb-1">{title}</p>
    <p className="text-body text-ink-2 max-w-xs">{message}</p>
    {action && <div className="mt-4">{action}</div>}
  </div>
);

export const LoadingState: React.FC<{ label?: string }> = ({ label = 'Memuat…' }) => (
  <div className="flex items-center justify-center gap-2 py-14 text-ink-2">
    <Loader2 size={16} className="animate-spin" />
    <span className="text-body">{label}</span>
  </div>
);

export const FormSection: React.FC<{ title: string; children: React.ReactNode; className?: string }> = ({
  title, children, className = '',
}) => (
  <section className={className}>
    <h3 className="text-label font-semibold text-ink-1 uppercase tracking-wide mb-3">{title}</h3>
    <div className="space-y-4">{children}</div>
  </section>
);
