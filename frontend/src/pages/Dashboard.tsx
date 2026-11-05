/**
 * Dashboard — KPI asli dari backend + status sistem.
 */
import React, { useEffect, useState } from 'react';
import { Loader2, FolderOpen, FileText, Users, RefreshCw, ListChecks, Trash2 } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { DashboardStatCard } from '../components/DashboardStatCard';
import { Badge, Button, EmptyState } from '../components/ui';
import { getStatus, getDashboardStats, listArsip, type Status } from '../lib/bindings';

const fmt = new Intl.NumberFormat('id-ID');

export const Dashboard: React.FC<{ status: Status | null }> = ({ status }) => {
  const [stats, setStats] = useState<Record<string, any> | null>(null);
  const [arsip, setArsip] = useState<any[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = async () => {
    setBusy(true);
    try {
      const [s, a] = await Promise.all([getDashboardStats(), listArsip()]);
      setStats(s);
      setArsip(a);
      setErr(null);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => { void load(); }, [status?.db_ok]);

  if (err && !stats) {
    return (
      <PageContainer>
        <PageHeader title="Dashboard" description="Ringkasan aktivitas dan kondisi sistem arsip." />
        <div className="card p-6">
          <EmptyState icon={ListChecks} title="Data belum tersedia" message={err} />
        </div>
      </PageContainer>
    );
  }

  if (!stats) {
    return (
      <PageContainer>
        <PageHeader title="Dashboard" description="Ringkasan aktivitas dan kondisi sistem arsip." />
        <div className="card p-10 flex items-center justify-center gap-2 text-ink-2">
          <Loader2 size={16} className="animate-spin" />
          <span className="text-body">Memuat ringkasan…</span>
        </div>
      </PageContainer>
    );
  }

  return (
    <PageContainer>
      <PageHeader
        title="Dashboard"
        description="Ringkasan aktivitas dan kondisi sistem arsip."
        actions={<Button variant="secondary" icon={RefreshCw} loading={busy} onClick={load}>Muat Ulang</Button>}
      />

      <section className="grid gap-3 mb-5" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <DashboardStatCard label="Total Arsip" value={fmt.format(stats.total ?? 0)} icon={FolderOpen}
          hint="Seluruh arsip" />
        <DashboardStatCard label="Total Item Fisik" value={fmt.format(stats.total_items ?? 0)} icon={FileText}
          hint="Jumlah item" />
        <DashboardStatCard label="Arsip Aktif" value={fmt.format(stats.aktif ?? 0)} icon={FolderOpen}
          hint={`${stats.total ? Math.round((stats.aktif / stats.total) * 100) : 0}% dari total`} />
        <DashboardStatCard label="Arsip Inaktif" value={fmt.format(stats.inaktif ?? 0)} icon={FolderOpen}
          hint={`${stats.total ? Math.round((stats.inaktif / stats.total) * 100) : 0}% dari total`} />
        <DashboardStatCard label="Siap Musnah" value={fmt.format(stats.siap_penyusutan ?? 0)} icon={RefreshCw}
          hint="Menunggu penyusutan" />
        <DashboardStatCard label="Musnah" value={fmt.format(stats.musnah ?? 0)} icon={Trash2}
          hint="Sudah dimusnahkan" />
        <DashboardStatCard label="Arsip SPJ" value={fmt.format(stats.spj ?? 0)} icon={FileText}
          hint="Pertanggungjawaban" />
        <DashboardStatCard label="Arsip Non-SPJ" value={fmt.format(stats.non_spj ?? 0)} icon={FileText}
          hint="Non pertanggungjawaban" />
        <DashboardStatCard label="Pemberkasan" value={fmt.format(stats.pemberkasan ?? 0)} icon={FolderOpen}
          hint="Dokumen pemberkasan" />
        <DashboardStatCard label="Dengan Berkas" value={fmt.format(stats.with_file ?? 0)} icon={FileText}
          hint="Arsip digital" />
        <DashboardStatCard label="Tanpa Berkas" value={fmt.format(stats.without_file ?? 0)} icon={FileText}
          hint="Belum diunggah" />
        <DashboardStatCard label="Unclassified" value={fmt.format(stats.unclassified ?? 0)} icon={ListChecks}
          hint="Belum diklasifikasi" />
      </section>

      <section className="card p-4">
        <h2 className="text-card-title font-semibold text-ink-1 mb-3">Status Sistem</h2>
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
    </PageContainer>
  );
};
