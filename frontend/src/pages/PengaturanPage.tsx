/**
 * Pengaturan — tampilkan/edit konfigurasi .env melalui backend
 * (GetSettings / SaveSettings). Tanpa rahasia (DB_PASSWORD dikosongkan).
 */
import React, { useEffect, useState } from 'react';
import { Save, RotateCcw } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { Badge, Button, FormSection, LoadingState } from '../components/ui';
import { getSettings, listMigrations, saveSettings } from '../lib/bindings';
import { useToast } from '../lib/toast';

const WINDOW_MODES = ['normal', 'maximized', 'fullscreen', 'kiosk'] as const;

type Settings = Record<string, string>;

export const PengaturanPage: React.FC = () => {
  const [s, setS] = useState<Settings | null>(null);
  const [busy, setBusy] = useState(false);
  const [migrations, setMigrations] = useState<string[]>([]);
  const { push } = useToast();

  const load = async () => {
    try {
      setS(await getSettings());
      setMigrations(await listMigrations());
    } catch (e) {
      push('danger', `Memuat pengaturan gagal: ${e}`);
    }
  };

  useEffect(() => { void load(); }, []);

  if (!s) return <PageContainer><LoadingState label="Memuat pengaturan…" /></PageContainer>;

  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setS((prev) => ({ ...(prev ?? {}), [k]: e.target.value }));

  const submit = async () => {
    if (!s) return;
    setBusy(true);
    try {
      const r = await saveSettings(s);
      push('success', r.msg);
      await load();
    } catch (e) {
      push('danger', `Simpan gagal: ${e}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageContainer>
      <PageHeader
        title="Pengaturan"
        description="Konfigurasi aplikasi desktop. Perubahan ditulis oleh backend ke file .env di samping aplikasi."
        actions={
          <>
            <Button variant="secondary" icon={RotateCcw} onClick={() => void load()}>Segarkan</Button>
            <Button icon={Save} loading={busy} onClick={() => void submit()}>Simpan</Button>
          </>
        }
      />

      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); void submit(); }}>
        <div className="card p-5">
          <FormSection title="Umum">
            <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
              <div>
                <label className="f-label" htmlFor="s-appname">Nama aplikasi</label>
                <input id="s-appname" className="f-input" value={s['APP_NAME'] ?? ''} onChange={set('APP_NAME')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-tz">Zona waktu</label>
                <input id="s-tz" className="f-input" value={s['APP_TIMEZONE'] ?? ''} onChange={set('APP_TIMEZONE')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-window">Mode jendela <span className="f-help inline">(berlaku setelah dibuka lagi)</span></label>
                <select id="s-window" className="f-input" value={s['SIMARC_WINDOW'] ?? 'fullscreen'} onChange={set('SIMARC_WINDOW')}>
                  {WINDOW_MODES.map((m) => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
            </div>
          </FormSection>
        </div>

        <div className="card p-5">
          <FormSection title="Database Lokal (MySQL/MariaDB)">
            <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
              <div>
                <label className="f-label" htmlFor="s-dbhost">Host</label>
                <input id="s-dbhost" className="f-input" value={s['DB_HOST'] ?? ''} onChange={set('DB_HOST')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-dbport">Port</label>
                <input id="s-dbport" className="f-input" value={s['DB_PORT'] ?? ''} onChange={set('DB_PORT')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-dbname">Nama database</label>
                <input id="s-dbname" className="f-input" value={s['DB_DATABASE'] ?? ''} onChange={set('DB_DATABASE')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-dbuser">Username</label>
                <input id="s-dbuser" className="f-input" value={s['DB_USERNAME'] ?? ''} onChange={set('DB_USERNAME')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-dbtls">TLS</label>
                <select id="s-dbtls" className="f-input" value={s['DB_TLS'] ?? 'preferred'} onChange={set('DB_TLS')}>
                  <option value="preferred">preferred</option>
                  <option value="required">required</option>
                  <option value="verify-full">verify-full</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
            </div>
          </FormSection>
        </div>

        <div className="card p-5">
          <FormSection title="Sinkronisasi (Opsional)">
            <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
              <div>
                <label className="f-label" htmlFor="s-sync">Status</label>
                <div className="flex items-center h-10 gap-2">
                  <Badge tone={s['SYNC_ENABLED'] === 'true' ? 'success' : 'neutral'}>
                    {s['SYNC_ENABLED'] === 'true' ? 'Aktif' : 'Nonaktif'}
                  </Badge>
                  <select
                    id="s-sync" className="f-input !h-10 !w-auto" value={s['SYNC_ENABLED'] ?? 'false'}
                    onChange={set('SYNC_ENABLED')}
                  >
                    <option value="false">Nonaktif</option>
                    <option value="true">Aktif</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="f-label" htmlFor="s-node">Node ID</label>
                <input id="s-node" className="f-input" value={s['SYNC_NODE_ID'] ?? ''} onChange={set('SYNC_NODE_ID')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-interval">Interval (menit)</label>
                <input id="s-interval" className="f-input" value={s['SYNC_INTERVAL'] ?? '15'} onChange={set('SYNC_INTERVAL_MINUTES')} />
              </div>
              <div>
                <label className="f-label" htmlFor="s-huburl">URL hub</label>
                <input id="s-huburl" className="f-input" placeholder="mis. http://192.168.1.10:8080" value={s['SYNC_HUB_URL'] ?? ''} onChange={set('SYNC_HUB_URL')} />
              </div>
            </div>
          </FormSection>
        </div>

        <div className="card p-5">
          <FormSection title="Migrasi Skema Terterapkan">
            {migrations.length === 0 ? (
              <p className="text-body text-ink-3">Belum ada — database belum terhubung.</p>
            ) : (
              <ul className="space-y-1">
                {migrations.map((m) => (
                  <li key={m} className="text-body text-ink-2 flex items-center gap-2">
                    <span className="w-1.5 h-1.5 rounded-full bg-ok shrink-0" />
                    {m}
                  </li>
                ))}
              </ul>
            )}
          </FormSection>
        </div>
      </form>
    </PageContainer>
  );
};
