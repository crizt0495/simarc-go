/**
 * AppLayout — kerangka 3 bagian: sidebar / topbar / konten utama.
 */
import React from 'react';
import { AppSidebar, NAV_ITEMS } from './AppSidebar';
import { AppTopbar } from './AppTopbar';
import type { Status } from '../lib/bindings';
import type { PageKey } from './AppSidebar';

export const AppLayout: React.FC<{
  page: PageKey;
  onNavigate: (p: PageKey) => void;
  status: Status | null;
  onRefresh: () => void;
  children: React.ReactNode;
}> = ({ page, onNavigate, status, onRefresh, children }) => {
  const pageLabel = NAV_ITEMS.find((n) => n.key === page)?.label ?? '';
  return (
    <div className="h-full flex overflow-hidden">
      <AppSidebar page={page} onNavigate={onNavigate} />
      <div className="flex-1 flex flex-col min-w-0">
        <AppTopbar
          baseUrl="Simarc"
          pageLabel={pageLabel}
          status={status}
          onRefresh={onRefresh}
        />
        <main className="flex-1 overflow-y-auto clamp">{children}</main>
      </div>
    </div>
  );
};
