/**
 * App — bootstrapping data backend dan routing halaman.
 */
import React, { useCallback, useEffect, useState } from 'react';
import { AppLayout } from './components/AppLayout';
import { Dashboard } from './pages/Dashboard';
import { ArsipPage } from './pages/ArsipPage';
import type { PageKey } from './components/AppSidebar';
import { getStatus, pingBackend, type Status } from './lib/bindings';

const TITLES: Record<PageKey, string> = {
  dashboard: 'Dashboard',
  arsip: 'Daftar Arsip',
  'arsip-create': 'Tambah Arsip',
  pemberkasan: 'Pemberkasan',
  pemusnahan: 'Pemusnahan Arsip',
  'peminjaman-create': 'Permohonan Pinjam',
  peminjaman: 'Daftar Peminjaman',
  'unit-kerja': 'Unit Kerja',
  'lokasi-arsip': 'Lokasi Arsip',
  users: 'Petugas',
  'kode-klasifikasi': 'Kode Klasifikasi',
  'jenis-arsip': 'Jenis Arsip',
  search: 'Pencarian Arsip',
  laporan: 'Dashboard Laporan',
  'laporan-arsip': 'Laporan Arsip',
  'laporan-digital': 'Arsip Digital',
  'laporan-pemberkasan': 'Laporan Pemberkasan',
  'laporan-lokasi': 'Laporan per Lokasi',
  'laporan-klasifikasi': 'Laporan Klasifikasi',
  'laporan-retensi': 'Laporan Retensi',
  'laporan-pemusnahan': 'Laporan Pemusnahan',
  'log-aktivitas': 'Log Aktivitas',
  statistik: 'Statistik',
  'monitoring-retensi': 'Monitoring Retensi',
  'jadwal-retensi': 'Jadwal Retensi',
  'dashboard-penyusutan': 'Dashboard Penyusutan',
  roles: 'Manajemen Role',
  pengaturan: 'Pengaturan',
  backup: 'Backup & Restore',
  blockchain: 'Blockchain Audit',
  sinkronisasi: 'Sinkronisasi',
};

const App: React.FC = () => {
  const [page, setPage] = useState<PageKey>('dashboard');
  const [status, setStatus] = useState<Status | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [s, ping] = await Promise.all([getStatus(), pingBackend()]);
      setStatus(s);
      void ping;
    } catch {
      /* biarkan state lama */
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), 30000);
    return () => clearInterval(t);
  }, [refresh]);

  const renderPage = () => {
    switch (page) {
      case 'dashboard':
        return <Dashboard status={status} />;
      case 'arsip':
      case 'arsip-create':
        return <ArsipPage canWrite />;
      default:
        return <ComingSoonPage title={TITLES[page]} />;
    }
  };

  return (
    <AppLayout
      page={page}
      onNavigate={setPage}
      status={status}
      onRefresh={() => void refresh()}
    >
      {renderPage()}
      <span className="sr-only" aria-live="polite">{TITLES[page]}</span>
    </AppLayout>
  );
};

export default App;
