/**
 * AppTopbar — breadcrumb, status koneksi DB + sinkronisasi, tombol sync manual.
 */
import React from 'react';
import { FolderTree, ChevronRight, Database, RefreshCw, CircleCheck, CircleAlert, CircleX } from 'lucide-react';
import type { Status } from '../lib/bindings';
import { useToast } from '../lib/toast';

type ConnTone = 'ok' | 'warn' | 'bad';

function connTone(s: Status | null): ConnTone {
  if (!s) return 'warn';
  if (s.db_ok) return 'ok';
  return 'bad';
}

export const connectionState = (s: Status | null): string => {
  if (!s) return 'connecting' as const;
  if (s.db_ok && s.sync_on && !s.sync_ok && s.sync_err) return 'sync failed';
  if (s.db_ok && s.sync_on && s.sync_ok) return 'synchronizing' as const;
  if (s.db_ok) return 'online' as const;
  if (s.sync_on) return 'offline' as const;
  return 'connecting';
};

export const AppTopbar: React.FC<{
  baseUrl: string;
  pageLabel: string;
  status: Status | null;
  onRefresh: () => void;
}> = ({ baseUrl, pageLabel, status, onRefresh }) => {
  const tone = connTone(status);
  const {
    push,
  } = useToast();

  const syncBusy = connectionState(status) === 'synchronizing';

  return (
    <header className="h-topbar shrink-0 bg-sidebar border-b border-line flex items-center px-5 gap-3">
      {/* Breadcrumb — konsisten di semua halaman */}
      <nav className="flex items-center gap-1.5 min-w-0" aria-label="Breadcrumb">
        <FolderTree size={13} className="text-ink-3 shrink-0" />
        <span className="text-caption text-ink-3 uppercase tracking-wide">{baseUrl}</span>
        <ChevronRight size={13} className="text-ink-3" />
        <span className="text-label font-medium text-ink-1 truncate">{pageLabel}</span>
      </nav>

      <div className="flex-1" />

      {/* Status koneksi DB lokal */}
      <div className="flex items-center gap-2 px-2.5 h-8 rounded-lg border border-line" title={
        status?.db_ok ? 'Database lokal siap' : `Database lokal: ${status?.db_err ?? 'menyambung…'}`
      }>
        {tone === 'ok' ? (
          <CircleCheck size={14} className="text-ok" />
        ) : tone === 'bad' ? (
          <CircleX size={14} className="text-bad animate-pulse" />
        ) : (
          <CircleAlert size={14} className="text-warn" />
        )}
        <span className="text-caption font-medium text-ink-2 hidden sm:inline">
          {tone === 'ok' ? `${status?.db_engine} • ${status?.db_name}` : 'DB Lokal'}
        </span>
      </div>

      {/* Status sinkronisasi + tombol manual */}
      <button
        onClick={async () => {
          try {
            const r = await import('../lib/bindings').then((m) => m.triggerSync());
            push(r.ran ? 'success' : 'info', r.msg);
          } catch (e) {
            push('danger', String(e));
          }
          onRefresh();
        }}
        className="flex items-center gap-2 px-2.5 h-8 rounded-lg border border-line hover:bg-surface-2 transition-colors"
        title="Jalankan sinkronisasi manual"
      >
        <RefreshCw size={14} strokeWidth={1.9} className={`text-primary ${syncBusy ? 'animate-spin' : ''}`} />
        {status?.last_sync ? (
          <span className="text-caption text-ink-2 hidden md:inline">{status.last_sync.split(' ')[1]} • sync {syncBusy ? 'berjalan' : 'siap'}</span>
        ) : (
          <span className="text-caption text-ink-2 hidden md:inline">Sinkronkan</span>
        )}
      </button>
    </header>
  );
};
