/**
 * Seluruh komunikasi frontend → backend Golang berjalan lewat modul ini
 * (Wails bindings). Frontend TIDAK PUNYA akses database/http server langsung.
 */
import {
  GetStatus,
  PingBackend,
  ListArsip,
} from '../../wailsjs/go/main/App';

export type Status = {
  app: string; db_ok: boolean; os: string; arch: string;
};

export async function getStatus(): Promise<Status> {
  return (await GetStatus()) as unknown as Status;
}

export async function pingBackend(): Promise<string> {
  return (await PingBackend()) as unknown as string;
}

export async function listArsip(): Promise<any[]> {
  return (await ListArsip()) as unknown as any[];
}
