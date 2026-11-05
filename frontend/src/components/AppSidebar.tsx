/**
 * AppSidebar — navigasi utama enterprise: collapsible, tooltip saat collapsed,
 * grup menu, profil pengguna, tema & keluar di bawah.
 */
import React, { useState } from 'react';
import {
  LayoutDashboard, FolderOpen, RefreshCw, DatabaseBackup, Settings,
  PanelLeftClose, PanelLeft, Sun, Moon, Monitor, Archive,
} from 'lucide-react';
import { type ThemeMode, useTheme } from '../lib/theme';

export type PageKey = 'dashboard' | 'arsip' | 'sinkronisasi' | 'backup' | 'pengaturan';

interface NavItem {
  key: PageKey;
  label: string;
  icon: React.ElementType;
  group: string;
}

export const NAV_ITEMS: NavItem[] = [
  { key: 'dashboard',    label: 'Dashboard',     icon: LayoutDashboard, group: 'Utama' },
  { key: 'arsip',        label: 'Arsip',         icon: FolderOpen,      group: 'Utama' },
  { key: 'sinkronisasi', label: 'Sinkronisasi',  icon: RefreshCw,       group: 'Sistem' },
  { key: 'backup',       label: 'Backup',        icon: DatabaseBackup,  group: 'Sistem' },
  { key: 'pengaturan',   label: 'Pengaturan',    icon: Settings,        group: 'Sistem' },
];

const GROUPS: Array<'Utama' | 'Sistem'> = ['Utama', 'Sistem'];

export const AppSidebar: React.FC<{
  page: PageKey;
  onNavigate: (p: PageKey) => void;
}> = ({ page, onNavigate }) => {
  const [collapsed, setCollapsed] = useState(false);
  const { mode, setMode, isDark } = useTheme();
  const cycleTheme = () => {
    const order: ThemeMode[] = ['light', 'dark', 'system'];
    setMode(order[(order.indexOf(mode) + 1) % order.length]);
  };

  const ThemeIcon = mode === 'system' ? Monitor : isDark ? Moon : Sun;
  const nextLabel = mode === 'light' ? 'gelap' : mode === 'dark' ? 'sistem' : 'terang';

  return (
    <aside
      className={`h-full bg-sidebar border-r border-line flex flex-col transition-[width] duration-200 shrink-0 ${collapsed ? 'w-[72px]' : 'w-[264px]'}`}
    >
      {/* Logo + nama */}
      <div className={`flex items-center h-16 border-b border-line ${collapsed ? 'justify-center px-2' : 'px-5'}`}>
        <div className="w-9 h-9 rounded-lg bg-primary flex items-center justify-center shrink-0">
          <Archive size={18} className="text-white" />
        </div>
        {!collapsed && (
          <div className="ml-3 min-w-0">
            <p className="text-card-title font-semibold text-ink-1 leading-tight truncate">SIMARC</p>
            <p className="text-caption text-ink-3 leading-tight truncate">Arsip Record Center</p>
          </div>
        )}
      </div>

      {/* Grup menu */}
      <nav className="flex-1 overflow-y-auto clamp py-3 px-2.5">
        {GROUPS.map((g) => (
          <div key={g} className="mb-3">
            {!collapsed && (
              <p className="text-caption font-medium text-ink-3 px-2.5 mb-1.5 tracking-wide uppercase">{g}</p>
            )}
            <ul className="space-y-0.5">
              {NAV_ITEMS.filter((n) => n.group === g).map((n) => {
                const active = page === n.key;
                const li = (
                  <button
                    key={n.key}
                    onClick={() => onNavigate(n.key)}
                    className={`w-full flex items-center h-10 rounded-lg text-body font-medium transition-colors duration-150
                      ${active ? 'bg-primary-soft text-primary' : 'text-ink-2 hover:bg-surface-2 hover:text-ink-1'}
                      ${collapsed ? 'justify-center px-0' : 'px-2.5 gap-3'}`}
                    title={collapsed ? n.label : undefined}
                  >
                    <n.icon size={16} strokeWidth={1.8} className={active ? 'text-primary' : ''} />
                    {!collapsed && <span className="truncate">{n.label}</span>}
                  </button>
                );
                return collapsed ? (
                  <li key={n.key} title={n.label} className="relative group">
                    {li}
                    <span className="pointer-events-none absolute left-full ml-2 top-1/2 -translate-y-1/2 whitespace-nowrap bg-ink-1 text-app px-2 py-1 rounded-md text-caption opacity-0 group-hover:opacity-100 transition-opacity duration-150 z-30">
                      {n.label}
                    </span>
                  </li>
                ) : (
                  <li key={n.key}>{li}</li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      {/* Aksi bawah: tema + collapse */}
      <div className="border-t border-line p-2.5 space-y-0.5">
        <button
          onClick={cycleTheme}
          className={`w-full flex items-center h-10 rounded-lg text-body text-ink-2 hover:bg-surface-2 hover:text-ink-1 transition-colors
            ${collapsed ? 'justify-center px-0' : 'px-2.5 gap-3'}`}
          title={collapsed ? `Tema: ${mode}` : undefined}
        >
          <ThemeIcon size={16} strokeWidth={1.8} />
          {!collapsed && <span>Tema: {mode === 'system' ? 'Sistem' : isDark ? 'Gelap' : 'Terang'}</span>}
          {!collapsed && <span className="ml-auto text-caption text-ink-3">→ {nextLabel}</span>}
        </button>
        <button
          onClick={() => setCollapsed((c) => !c)}
          className={`w-full flex items-center h-10 rounded-lg text-body text-ink-2 hover:bg-surface-2 hover:text-ink-1 transition-colors
            ${collapsed ? 'justify-center px-0' : 'px-2.5 gap-3'}`}
          title={collapsed ? 'Perluas sidebar' : undefined}
        >
          {collapsed ? <PanelLeft size={16} strokeWidth={1.8} /> : <PanelLeftClose size={16} strokeWidth={1.8} />}
          {!collapsed && <span>Ciutkan sidebar</span>}
        </button>
      </div>
    </aside>
  );
};
