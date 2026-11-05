/**
 * Backup — cadangan database lokal ke berkas .sql (dibuat backend Golang).
 * Daftar berkas diambil dari folder storage/backups pada aplikasi.
 */
import React, { useCallback, useEffect, useState } from 'react';
import { DatabaseBackup, Download, HardDrive } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { Button, LoadingState } from '../components/ui';
import {
  createBackupNow,
  listBackups,
  type BackupInfo,
} from '../lib/bindings';
import { useToast } from '../lib/toast';

export const BackupPage: React.FC = () => {
  const [rows, setRows] = useState<BackupInfo[] | null>(null);
  const [busy, setBusy] = useState(false);
  const { push } = useToast();

  const load = useCallback(async () => {
    try {
      setRows(await listBackups());
    } catch (e) {
      push('danger', `Memuat daftar backup gagal: ${e}`);
      setRows([]);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const run = async () => {
    setBusy(true);
    try {
      const r = await createBackupNow();
      push('success', `Backup dibuat: ${r.name} (${(r.size / 1024).toFixed(1)} KB)`);
    } catch (e) {
      push('danger', `Backup gagal: ${e}`);
    } finally {
      setBusy(false);
      await load();
    }
  };

  return (
    <PageContainer>
      <PageHeader
        title="Backup"
        description="Cadangkan seluruh isi database lokal ke berkas SQL juga di komputer ini — datanya tetap aman saat offline."
        actions={<Button icon={DatabaseBackup} loading={busy} onClick={() => void run()}>Backup Sekarang</Button>}
      />

      <div className="t-wrap">
        <div className="flex items-center gap-2 px-3.5 py-3 border-b border-line">
          <HardDrive size={15} className="text-ink-3" />
          <span className="text-card-title font-semibold text-ink-1">Riwayat Backup</span>
        </div>
        {!rows ? (
          <LoadingState />
        ) : rows.length === 0 ? (
          <div className="px-6 py-14 text-center">
            <div className="w-11 h-11 rounded-xl bg-surface-2 border border-line inline-flex items-center justify-center mb-3">
              <Download size={18} className="text-ink-3" />
            </div>
            <p className="text-card-title text-ink-1">Belum ada backup</p>
            <p className="text-body text-ink-2 mt-1">Klik "Backup Sekarang" untuk membuat cadangan pertama.</p>
          </div>
        ) : (
          <table className="w-full border-collapse">
            <thead>
              <tr>
                <th className="t-head px-4 py-2.5">Nama berkas</th>
                <th className="t-head px-4 py-2.5 text-right">Ukuran</th>
                <th className="t-head px-4 py-2.5">Dibuat</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((b) => (
                <tr key={b.path} className="t-row">
                  <td className="t-cell truncate">{b.name}</td>
                  <td className="t-cell text-right tabular-nums text-ink-2">{(b.size / 1024).toFixed(1)} KB</td>
                  <td className="t-cell text-ink-2">{b.created}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </PageContainer>
  );
};
