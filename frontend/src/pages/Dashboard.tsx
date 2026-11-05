/**
 * Dashboard — KPI asli dari backend + grafik 14 hari + status sistem.
 */
import React, { useEffect, useState } from 'react';
import { Loader2, FolderOpen, FileText, Users, RefreshCw, ListChecks } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { DashboardStatCard } from '../components/DashboardStatCard';
import { Badge, Button, EmptyState } from '../components/ui';
import { getOverview, getStatus, type Overview, type Status } from '../lib/bindings';

const fmt = new Intl.NumberFormat('id-ID');

export const Dashboard: React.FC<{ status: Status | null }> = ({ status }) => {
  const [ov, setOv] = useState<Overview | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = async () => {
    setBusy(true);
    try {
      setOv(await getOverview());
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

      {err && !ov ? (
        <div className="card p-6">
          <EmptyState icon={ListChecks} title="Data belum tersedia" message={err} />
        </div>
      ) : !ov ? (
        <div className="card p-10 flex items-center justify-center gap-2 text-ink-2">
          <Loader2 size={16} className="animate-spin" />
          <span className="text-body">Memuat ringkasan…</span>
        </div>
      ) : (
        <>
          <section className="grid gap-3 mb-5" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            <DashboardStatCard label="Total Arsip" value={fmt.format(ov.total_arsip)} icon={FolderOpen}
              hint="Catatan arsip aktif di database lokal" />
            <DashboardStatCard label="Pemberkasan" value={fmt.format(ov.total_pemberkasan)} icon={FileText}
              hint="Dokumen pemberkasan tercatat" />
            <DashboardStatCard label="Pengguna Aktif" value={fmt.format(ov.total_users)} icon={Users}
              hint="Akun dengan status aktif" />
            <DashboardStatCard label="Antrean Sinkron" value={fmt.format(ov.sync_queue_pending)} icon={RefreshCw}
              hint={ov.last_sync ? `Sinkron terakhir ${ov.last_sync}` : 'Belum pernah sinkron'} />
          </section>

          <section className="grid gap-4 mb-5" style={{ gridTemplateColumns: 'minmax(0, 2fr) minmax(0, 1fr)' }}>
            {/* Grafik aktivitas 14 hari (data asli dari backend) */}
            <div className="card p-4">
              <h2 className="text-card-title font-semibold text-ink-1 mb-1">Aktivitas 14 Hari</h2>
              <p className="text-caption text-ink-3 mb-3">Jumlah arsip dan pemberkasan yang dibuat per hari.</p>
              <div className="h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={ov.series} margin={{ top: 4, right: 8, bottom: 0, left: -18 }}>
                    <defs>
                      <linearGradient id="ga" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="var(--c-primary)" stopOpacity={0.25} />
                        <stop offset="100%" stopColor="var(--c-primary)" stopOpacity={0.02} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid stroke="var(--c-border-soft)" vertical={false} />
                    <XAxis dataKey="day" tickFormatter={(d: string) => d.slice(5)}
                      tick={{ fontSize: 11, fill: 'var(--c-text-3)' }} tickLine={false} axisLine={false} />
                    <YAxis tick={{ fontSize: 11, fill: 'var(--c-text-3)' }} tickLine={false} axisLine={false}
                      allowDecimals={false} />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--c-surface)', border: '1px solid var(--c-border)',
                        borderRadius: 8, fontSize: 12, color: 'var(--c-text-1)',
                      }}
                      labelFormatter={(d) => `Tanggal ${d}`}
                    />
                    <Legend wrapperStyle={{ fontSize: 11, color: 'var(--c-text-2)' }} />
                    <Area type="monotone" dataKey="arsip" name="Arsip" stroke="var(--c-primary)"
                      fill="url(#ga)" strokeWidth={1.8} />
                    <Area type="monotone" dataKey="berkas" name="Pemberkasan"
                      stroke="var(--c-info)" fill="none" strokeWidth={1.5} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>

            {/* Status sistem */}
            <div className="card p-4 flex flex-col">
              <h2 className="text-card-title font-semibold text-ink-1 mb-3">Status Sistem</h2>
              <dl className="space-y-2.5 text-body">
                <Row label="Database lokal">
                  <Badge tone={status?.db_ok ? 'success' : 'danger'}>
                    {status?.db_ok ? 'Terhubung' : 'Gagal / Offline'}
                  </Badge>
                </Row>
                <Row label="Mesin Database">
                  <span className="text-ink-2">{status?.db_engine ?? '—'}</span>
                </Row>
                <Row label="Alamat Database">
                  <span className="text-ink-2 tabular-nums">{status?.db_address ?? '—'}</span>
                </Row>
                <Row label="Nama Database">
                  <span className="text-ink-2">{status?.db_name ?? '—'}</span>
                </Row>
                <Row label="Mode Jendela">
                  <Badge tone="neutral">{status?.window ?? '—'}</Badge>
                </Row>
                <Row label="Sinkronisasi">
                  <Badge tone={status?.sync_on ? (status?.sync_ok ? 'success' : 'warning') : 'neutral'}>
                    {status?.sync_on ? (status?.sync_ok ? 'Aktif • sukses' : 'Aktif • tertunda') : 'Nonaktif'}
                  </Badge>
                </Row>
                <Row label="Identitas Node">
                  <span className="text-ink-2">{status?.node_id ?? '—'}</span>
                </Row>
                <Row label="Waktu Jalan">
                  <span className="text-ink-2 tabular-nums">{status?.uptime ?? '—'}</span>
                </Row>
              </dl>
              {status?.db_err && (
                <p className="text-caption text-bad mt-3 break-all">{status.db_err}</p>
              )}
            </div>
          </section>
        </>
      )}
    </PageContainer>
  );
};

const Row: React.FC<{ label: string; children: React.ReactNode }> = ({ label, children }) => (
  <div className="flex items-center justify-between gap-2">
    <dt className="text-ink-2">{label}</dt>
    <dd className="flex items-center gap-2 text-right">{children}</dd>
  </div>
);
