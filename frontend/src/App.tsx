/**
 * App — bootstrapping data backend (GetStatus, PingBackend) dan routing
 * halaman sederhana berbasis state (tanpa router web; ini aplikasi desktop).
 */
import React, { useCallback, useEffect, useState } from 'react';
import { AppLayout } from './components/AppLayout';
import { Dashboard } from './pages/Dashboard';
import { ArsipPage } from './pages/ArsipPage';
import { SinkronisasiPage } from './pages/SinkronisasiPage';
import { BackupPage } from './pages/BackupPage';
import { PengaturanPage } from './pages/PengaturanPage';
import type { PageKey } from './components/AppSidebar';
import { getStatus, pingBackend, type Status } from './lib/bindings';

const TITLES: Record<PageKey, string> = {
  dashboard: 'Dashboard',
  arsip: 'Arsip',
  sinkronisasi: 'Sinkronisasi',
  backup: 'Backup',
  pengaturan: 'Pengaturan',
};

const App: React.FC = () => {
  const [page, setPage] = useState<PageKey>('dashboard');
  const [status, setStatus] = useState<Status | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [s, ping] = await Promise.all([getStatus(), pingBackend()]);
      setStatus((prev) => {
        void ping;
        return s;
      });
    } catch {
      setStatus((prev) => prev); // network err — biarkan nilai lama
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), 30000);
    return () => clearInterval(t);
  }, [refresh]);

  return (
    <AppLayout
      page={page}
      onNavigate={setPage}
      status={status}
      onRefresh={() => void refresh()}
    >
      {page === 'dashboard' && <Dashboard status={status} />}
      {page === 'arsip' && <ArsipPage canWrite />}
      {page === 'sinkronisasi' && <SinkronisasiPage status={status} />}
      {page === 'backup' && <BackupPage />}
      {page === 'pengaturan' && <PengaturanPage />}
      <span className="sr-only" aria-live="polite">{TITLES[page]}</span>
    </AppLayout>
  );
};

export default App;
