/**
 * Arsip — daftar arsip dari database lokal (GORM, MySQL/MariaDB).
 */
import React, { useCallback, useEffect, useState } from 'react';
import { FolderOpen, Plus } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { DataTable, type Column } from '../components/DataTable';
import { Badge, Button, EmptyState, LoadingState } from '../components/ui';
import { listArsip } from '../lib/bindings';
import { useToast } from '../lib/toast';

export const ArsipPage: React.FC<{ canWrite: boolean }> = ({ canWrite }) => {
  const [rows, setRows] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { push } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setRows(await listArsip());
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, []);

  const statusTone = (s: string) =>
    s === 'aktif' ? 'success' as const :
    s === 'inaktif' ? 'warning' as const :
    s === 'deleted' ? 'danger' as const : 'neutral' as const;

  const columns: Column<any>[] = [
    {
      key: 'nama', header: 'Nama Arsip', width: '40%',
      render: (a) => (
        <div className="flex items-center gap-2.5 min-w-0">
          <FolderOpen size={15} className="text-ink-3 shrink-0" />
          <span className="font-medium text-ink-1 truncate">{a.nama || a.nomor || '—'}</span>
        </div>
      ),
      stringValue: (a) => `${a.nama ?? ''} ${a.nomor ?? ''} ${a.status ?? ''}`,
    },
    { key: 'nomor', header: 'Nomor Arsip', render: (a) => <span className="text-ink-2 font-mono text-caption">{a.nomor || '—'}</span> },
    {
      key: 'status', header: 'Status', align: 'center',
      render: (a) => <Badge tone={statusTone(a.status)}>{a.status || '—'}</Badge>,
    },
    { key: 'created', header: 'Dibuat', align: 'right', stringValue: (a) => a.created },
    { key: 'updated', header: 'Diperbarui', align: 'right', stringValue: (a) => a.updated },
  ];

  return (
    <PageContainer>
      <PageHeader
        title="Daftar Arsip"
        description="Kelola catatan arsip langsung di database lokal — tetap bekerja tanpa internet."
      />
      <DataTable
        data={rows}
        columns={columns}
        loading={loading}
        error={error}
        rowKey={(a) => a.id}
        emptyTitle="Belum ada arsip"
        emptyMessage="Tabel akan terisi setelah arsip pertama dibuat."
        maxHeight="calc(100vh - 330px)"
      />
    </PageContainer>
  );
};
