/**
 * Seluruh komunikasi frontend → backend Golang berjalan lewat modul ini
 * (Wails bindings). Frontend TIDAK PUNYA akses database/http server langsung.
 */
import {
  GetStatus,
  PingBackend,
  ListArsip,
  CreateArsip,
  DeleteArsip,
  TriggerSync,
  SyncQueueCount,
  GetSettings,
  SaveSettings,
  GetOverview,
  ListMigrations,
  CreateBackupNow as SaveNow,
  ListBackups,
} from '../../wailsjs/go/main/App';

// Semantic wrapper — melempar Error bila backend gagal, wrap bila ok.
export type Status = {
  app: string; version: string; os: string; arch: string;
  started: string; uptime: string; window: string;
  db_ok: boolean; db_err: string; db_engine: string; db_address: string;
  db_name: string; db_tls: string;
  sync_ok: boolean; sync_err: string; sync_on: boolean;
  last_sync: string; node_id: string;
};

export type Arsip = {
  id: string; nama: string; kategori: string; status: string;
  file_size: number; created: string; updated: string;
};

export type OverviewPoint = { day: string; arsip: number; berkas: number };
export type Overview = {
  total_arsip: number; total_pemberkasan: number; total_users: number;
  sync_queue_pending: number; last_sync: string; series: OverviewPoint[];
};

export async function getStatus(): Promise<Status> {
  return (await GetStatus()) as unknown as Status;
}

export async function pingBackend(): Promise<string> {
  return (await PingBackend()) as unknown as string;
}

export async function listArsip(): Promise<Arsip[]> {
  return (await ListArsip()) as unknown as Arsip[];
}

export async function createArsip(nama: string, kategori: string): Promise<Arsip> {
  return (await CreateArsip(nama, kategori)) as unknown as Arsip;
}

export async function deleteArsip(id: string): Promise<boolean> {
  return (await DeleteArsip(id)) as unknown as boolean;
}

export async function triggerSync(): Promise<{ ran: boolean; msg: string }> {
  return (await TriggerSync()) as unknown as { ran: boolean; msg: string };
}

export async function syncQueueCount(): Promise<number> {
  return (await SyncQueueCount()) as unknown as number;
}

export async function getSettings(): Promise<Record<string, string>> {
  return (await GetSettings()) as unknown as Record<string, string>;
}

export async function saveSettings(values: Record<string, string>): Promise<{ msg: string }> {
  return (await SaveSettings(values)) as unknown as { msg: string };
}

export async function getOverview(): Promise<Overview> {
  return (await GetOverview()) as unknown as Overview;
}

export async function listMigrations(): Promise<string[]> {
  return (await ListMigrations()) as unknown as string[];
}

// ── Backup ─────────────────────────────────────────────────────────────
export type BackupInfo = { name: string; path: string; size: number; created: string };

export async function createBackupNow(): Promise<{ name: string; path: string; size: number }> {
  return (await SaveNow()) as unknown as { name: string; path: string; size: number };
}

export async function listBackups(): Promise<BackupInfo[]> {
  return (await ListBackups()) as unknown as BackupInfo[];
}
