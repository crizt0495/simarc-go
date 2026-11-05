/**
 * Arsip — CRUD asli lewat binding backend: daftar (DataTable), tambah (modal,
 * RHF+Zod), hapus (confirm). Tanpa data dummy.
 */
import React, { useCallback, useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { FolderOpen, Plus, Trash2 } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { DataTable, type Column } from '../components/DataTable';
import { Badge, Button, ConfirmDialog, EmptyState, Modal, FormSection } from '../components/ui';
import { createArsip, deleteArsip, listArsip, type Arsip } from '../lib/bindings';
import { useToast } from '../lib/toast';

const CATEGORIES = ['umum', 'keuangan', 'kepegawaian', 'perjanjian', 'laporan'] as const;

const schema = z.object({
  nama: z.string().min(1, 'Nama arsip wajib diisi').max(256, 'Maksimal 256 karakter'),
  kategori: z.enum(CATEGORIES),
});
type FormValues = z.infer<typeof schema>;

export const ArsipPage: React.FC<{ canWrite: boolean }> = ({ canWrite }) => {
  const [rows, setRows] = useState<Arsip[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [deleting, setDeleting] = useState<Arsip | null>(null);
  const [busyDel, setBusyDel] = useState(false);
  const { push } = useToast();

  const {
    register, handleSubmit, reset, formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { nama: '', kategori: 'umum' },
  });

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

  const onSubmit = async (v: FormValues) => {
    try {
      const a = await createArsip(v.nama, v.kategori);
      push('success', `Arsip "${a.nama}" disimpan (ID ${a.id.slice(0, 8)}…)`);
      setAddOpen(false);
      reset();
      await load();
    } catch (e) {
      push('danger', `Gagal menyimpan: ${e}`);
    }
  };

  const onConfirmDelete = async () => {
    if (!deleting) return;
    setBusyDel(true);
    try {
      await deleteArsip(deleting.id);
      push('success', `Arsip "${deleting.nama}" dihapus (soft delete)`);
      setDeleting(null);
      await load();
    } catch (e) {
      push('danger', `Gagal menghapus: ${e}`);
    } finally {
      setBusyDel(false);
    }
  };

  const statusTone = (s: string) =>
    s === 'archived' ? 'success' as const :
    s === 'sent' ? 'info' as const :
    s === 'deleted' ? 'danger' as const : 'warning' as const;

  const columns: Column<Arsip>[] = [
    {
      key: 'nama', header: 'Nama Arsip', width: '34%',
      render: (a) => (
        <div className="flex items-center gap-2.5 min-w-0">
          <FolderOpen size={15} className="text-ink-3 shrink-0" />
          <span className="font-medium text-ink-1 truncate">{a.nama}</span>
        </div>
      ),
      stringValue: (a) => `${a.nama} ${a.kategori} ${a.status}`,
    },
    { key: 'kategori', header: 'Kategori', render: (a) => <span className="capitalize text-ink-2">{a.kategori}</span> },
    {
      key: 'status', header: 'Status', align: 'center',
      render: (a) => <Badge tone={statusTone(a.status)}>{a.status}</Badge>,
    },
    {
      key: 'file_size', header: 'Ukuran Berkas', align: 'right', sortable: false,
      render: (a) => (a.file_size ? `${a.file_size.toLocaleString('id-ID')} B` : '—'),
    },
    { key: 'created', header: 'Dibuat', align: 'right', stringValue: (a) => a.created },
    { key: 'updated', header: 'Diperbarui', align: 'right', stringValue: (a) => a.updated },
    {
      key: 'aksi', header: '', width: '72px', sortable: false, align: 'center',
      render: (a) => (
        <button
          onClick={() => setDeleting(a)}
          className="p-1.5 rounded-md text-ink-3 hover:text-bad hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
          title={`Hapus arsip ${a.nama}`}
          aria-label={`Hapus arsip ${a.nama}`}
        >
          <Trash2 size={15} />
        </button>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title="Arsip"
        description="Kelola catatan arsip langsung di database lokal — tetap bekerja tanpa internet."
        actions={canWrite && (
          <Button icon={Plus} onClick={() => setAddOpen(true)}>Tambah Arsip</Button>
        )}
      />
      <DataTable
        data={rows}
        columns={columns}
        loading={loading}
        error={error}
        rowKey={(a) => a.id}
        emptyTitle="Belum ada arsip"
        emptyMessage="Tabel akan terisi setelah arsip pertama dibuat."
        emptyAction={canWrite && <Button icon={Plus} onClick={() => setAddOpen(true)}>Tambah Arsip</Button>}
        maxHeight="calc(100vh - 330px)"
      />

      {/* Modal tambah */}
      <Modal
        open={addOpen}
        title="Tambah Arsip"
        onClose={() => { setAddOpen(false); reset(); }}
        footer={
          <>
            <Button variant="secondary" onClick={() => { setAddOpen(false); reset(); }}>Batal</Button>
            <Button loading={isSubmitting} onClick={() => void handleSubmit(onSubmit)()}>Simpan</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <FormSection title="Informasi Arsip">
            <div>
              <label className="f-label" htmlFor="arsip-nama">Nama arsip <span className="f-req">*</span></label>
              <input
                id="arsip-nama"
                className="f-input"
                placeholder="mis. Surat Keputusan Q4/2026"
                {...register('nama')}
              />
              {errors.nama && <p className="f-error">{errors.nama.message}</p>}
            </div>
            <div>
              <label className="f-label" htmlFor="arsip-kategori">Kategori</label>
              <select id="arsip-kategori" className="f-input" {...register('kategori')}>
                {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </div>
            <p className="f-help">
              Arsip ditulis langsung ke MySQL lokal oleh backend Golang.
            </p>
          </FormSection>
        </form>
      </Modal>

      {/* Konfirmasi hapus */}
      <ConfirmDialog
        open={!!deleting}
        title="Hapus arsip?"
        message={`"${deleting?.nama ?? ''}" akan menandai status 'deleted' (soft delete) di database lokal. Data masih dapat dipulihkan lewat DB.`}
        confirmLabel="Hapus"
        onConfirm={() => void onConfirmDelete()}
        onCancel={() => setDeleting(null)}
      />
    </PageContainer>
  );
};
