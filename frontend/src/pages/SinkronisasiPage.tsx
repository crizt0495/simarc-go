/**
 * Sinkronisasi — status koneksi database lokal.
 */
import React from 'react';
import { Wifi, WifiOff } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { Badge, Button } from '../components/ui';
import { getStatus, type Status } from '../lib/bindings';

export const SinkronisasiPage: React.FC<{ status: Status | null }> = ({ status }) => {
  const online = status?.db_ok ?? false;
  return (
    <PageContainer>
      <PageHeader title="Sinkronisasi" description="Status koneksi database lokal (offline-first)." />
      <div className="card p-4">
        <div className="flex items-center gap-2 mb-2">
          {online ? <Wifi size={16} className="text-ok" /> : <WifiOff size={16} className="text-bad" />}
          <h2 className="text-card-title font-semibold text-ink-1">Database Lokal</h2>
        </div>
        <Badge tone={online ? 'success' : 'danger'}>
          {online ? 'Terhubung' : 'Gagal / Offline'}
        </Badge>
      </div>
    </PageContainer>
  );
};
