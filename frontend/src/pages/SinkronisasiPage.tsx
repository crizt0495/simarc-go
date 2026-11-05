/**
 * Sinkronisasi — status antarmuka offline-first; jalankan push/pull manual,
 * lihat identitas node, URL hub, dan antrean sync_queue lokal.
 */
import React, { useCallback, useEffect, useState } from 'react';
import { RefreshCw, WifiOff, Wifi } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { Badge, Button, EmptyState } from '../components/ui';
import {
  getStatus, syncQueueCount, triggerSync,
  type Status,
} from '../lib/bindings';
import { useToast } from '../lib/toast';

export const SinkronisasiPage: React.FC<{ status: Status | null }> = ({ status: initial }) => {
  const [st, setSt] = useState<Status | null>(initial);
  const [queue, setQueue] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const { push } = useToast();

  const load = useCallback(async () => {
    try {
      const [s, q] = await Promise.all([getStatus(), syncQueueCount()]);
      setSt(s);
      setQueue(q);
    } catch { /* biarkan state */ }
  }, []);

  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 10000);
    return () => clearInterval(t);
  }, [load]);

  const run = async () => {
    setBusy(true);
    try {
      const r = await triggerSync();
      push(r.ran ? 'success' : 'info', r.msg);
    } catch (e) {
      push('danger', `Sinkronisasi gagal: ${e}`);
    } finally {
      setBusy(false);
      await load();
    }
  };

  const on = st?.sync_on ?? false;
  const online = st?.db_ok ?? false;

  return (
    <PageContainer>
      <PageHeader
        title="Sinkronisasi"
        description="Pertukaran data dua arah dengan hub saat jaringan tersedia. Aplikasi tetap berjalan penuh dari database lokal selama offline."
        actions={<Button icon={RefreshCw} loading={busy} onClick={() => void run()}>Sinkronkan Sekarang</Button>}
      />

      <div className="grid gap-4 mb-5" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))' }}>
        <div className="card p-4">
          <div className="flex items-center gap-2 mb-2">
            {online ? <Wifi size={16} className="text-ok" /> : <WifiOff size={16} className="text-bad" />}
            <h2 className="text-card-title font-semibold text-ink-1">Mode Offline-First</h2>
          </div>
          <p className="text-body text-ink-2 leading-relaxed">
            {online
              ? 'Database lokal tersambung. Semua perubahan otomatis masuk antrean sync_queue dan dikirim ke hub di jadwal berikutnya.'
              : 'Database lokal belum tersambung. Aplikasi mencoba ulang tiap 10 detik; periksa konfigurasi di halaman Pengaturan.'}
          </p>
        </div>

        <div className="card p-4">
          <h2 className="text-card-title font-semibold text-ink-1 mb-3">Antrean Sinkronisasi</h2>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-semibold text-ink-1 tabular-nums">{queue ?? '—'}</span>
            <span className="text-caption text-ink-3">operasi pending</span>
          </div>
          <p className="text-caption text-ink-3 mt-2">Tabel sync_queue di database lokal.</p>
        </div>
      </div>

      <div className="card">
        <h2 className="text-card-title font-semibold text-ink-1 px-4 pt-4 pb-2">Konfigurasi Hub</h2>
        <p className="text-caption text-ink-3 px-4 pb-3">Konfigurasi diambil backend dari file .env (tanpa rahasia).</p>
        {st ? (
          <dl className="text-body divide-y divide-line-soft">
            <CfgRow label="Status" value={
              <Badge tone={on ? (st.sync_ok ? 'success' : st.sync_err ? 'danger' : 'warning') : 'neutral'}>
                {!on ? 'Nonaktif (SYNC_ENABLED=false)' : st.sync_err ? 'Gagal terakhir' : st.sync_ok ? 'Sehat' : 'Menunggu jadwal'}
              </Badge>
            } />
            <CfgRow label="Identitas node" value={<span className="text-ink-2">{st.node_id}</span>} />
            <CfgRow label="URL hub" value={
              <span className="text-ink-2">{st.sync_on ? (st.sync_err ? '' : 'didapat dari SYNC_HUB_URL') : '—'}</span>
            } />
            <CfgRow label="Sinkron terakhir" value={<span className="text-ink-2 tabular-nums">{st.last_sync || '—'}</span>} />
            {st.sync_err && (
              <CfgRow label="Pesan galat terakhir" value={<span className="text-bad break-all">{st.sync_err}</span>} />
            )}
          </dl>
        ) : (
          <EmptyState icon={RefreshCw} title="Memuat…" message="Status sedang diambil dari backend." />
        )}
      </div>
    </PageContainer>
  );
};

const CfgRow: React.FC<{ label: string; value: React.ReactNode }> = ({ label, value }) => (
  <div className="flex items-center justify-between gap-4 px-4 py-3">
    <dt className="text-ink-2 shrink-0">{label}</dt>
    <dd className="text-right min-w-0 break-all">{value}</dd>
  </div>
);
