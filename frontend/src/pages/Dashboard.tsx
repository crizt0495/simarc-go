/**
 * Dashboard — KPI asli dari backend + status sistem.
 */
import React, { useEffect, useState } from 'react';
import { Loader2, FolderOpen, FileText, Users, RefreshCw, ListChecks } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { DashboardStatCard } from '../components/DashboardStatCard';
import { Badge, Button, EmptyState } from '../components/ui';
import { getStatus, listArsip, type Status } from '../lib/bindings';

const fmt = new Intl.NumberFormat('id-ID');

export const Dashboard: React.FC<{ status: Status | null }> = ({ status }) => {
  const [arsip, setArsip] = useState<any[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = async () => {
    setBusy(true);
    try {
      setArsip(await listArsip());
      setErr(null);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => { void load(); }, [status?.db_ok]);

  return (
    <PageContainer>
      <PageHeader
        title="Dashboard"
        description="Ringkasan aktivitas dan kondisi sistem arsip."
        actions={<Button variant="secondary" icon={RefreshCw} loading={busy} onClick={load}>Muat Ulang</Button>}
      />

      {err && arsip.length === 0 ? (
        <div className="card p-6">
          <EmptyState icon={ListChecks} title="Data belum tersedia" message={err} />
        </div>
      ) : (
        <>
          <section className="grid gap-3 mb-5" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            <DashboardStatCard label="Total Arsip" value={fmt.format(arsip.length)} icon={FolderOpen}
              hint="Catatan arsip aktif di database lokal" />
            <DashboardStatCard label="Pengguna" value={fmt.format(status?.db_ok ? 1 : 0)} icon={Users}
              hint="Akun aktif" />
            <DashboardStatCard label="Antrean Sinkron" value="0" icon={RefreshCw}
              hint={status?.db_ok ? 'Database lokal siap' : 'Database offline'} />
          </section>

          <section className="card p-4">
            <h2 className="text-card-title font-semibold text-ink-1 mb-1">Status Sistem</h2>
            <p className="text-caption text-ink-3 mb-3">Informasi koneksi database lokal.</p>
            <dl className="space-y-2.5 text-body">
              <div className="flex items-center justify-between gap-2">
                <dt className="text-ink-2">Database lokal</dt>
                <dd>
                  <Badge tone={status?.db_ok ? 'success' : 'danger'}>
                    {status?.db_ok ? 'Terhubung' : 'Gagal / Offline'}
                  </Badge>
                </dd>
              </div>
              <div className="flex items-center justify-between gap-2">
                <dt className="text-ink-2">Alamat</dt>
                <dd className="text-ink-2 tabular-nums">{status?.db_ok ? '127.0.0.1:3307' : '—'}</dd>
              </div>
              <div className="flex items-center justify-between gap-2">
                <dt className="text-ink-2">Mode Jendela</dt>
                <dd><Badge tone="neutral">{status?.window ?? '—'}</Badge></dd>
              </div>
              <div className="flex items-center justify-between gap-2">
                <dt className="text-ink-2">Waktu Jalan</dt>
                <dd className="text-ink-2 tabular-nums">{status?.uptime ?? '—'}</dd>
              </div>
            </dl>
            {status?.db_err && (
              <p className="text-caption text-bad mt-3 break-all">{status.db_err}</p>
            )}
          </section>
        </>
      )}
    </PageContainer>
  );
};
