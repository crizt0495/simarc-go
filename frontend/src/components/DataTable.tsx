/**
 * DataTable — standar seluruh modul.
 * Pencarian, sorting, pagination, page-size, loading/empty/error state,
 * sticky header, hover lembut, font 13px, tanpa border vertikal berlebihan.
 */
import React, { useMemo, useState } from 'react';
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Search, Inbox } from 'lucide-react';
import { EmptyState } from './ui';

export interface Column<T> {
  key: string;
  header: string;
  align?: 'left' | 'right' | 'center';
  width?: string;
  sortable?: boolean;
  render?: (row: T) => React.ReactNode;
  stringValue?: (row: T) => string; // untuk pencarian
}

interface Props<T> {
  data: T[];
  columns: Column<T>[];
  loading?: boolean;
  error?: string | null;
  searchValue?: string;          // controlled bila tool sendiri
  onSearchValue?: (v: string) => void;
  emptyTitle?: string;
  emptyMessage?: string;
  emptyAction?: React.ReactNode;
  rowKey: (row: T) => string;
  maxHeight?: string;
  toolbar?: React.ReactNode;     // tombol/filter tambahan di kanan
  pageSizeOptions?: number[];
}

const PAGE_SIZES = [10, 25, 50, 100];

export function DataTable<T>({
  data, columns, loading = false, error = null,
  searchValue: extSearch, onSearchValue,
  emptyTitle = 'Belum ada data', emptyMessage = 'Data akan muncul di sini bila tersedia.',
  emptyAction, rowKey, maxHeight = '56vh', toolbar, pageSizeOptions = PAGE_SIZES,
}: Props<T>) {
  const [search, setSearch] = useState(extSearch ?? '');
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc');
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(pageSizeOptions.includes(25) ? 25 : pageSizeOptions[0]);

  const onSearch = (v: string) => {
    setSearch(v);
    onSearchValue?.(v);
    setPage(0);
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    let out = q
      ? data.filter((row) =>
          columns.some((c) =>
            (c.stringValue ? c.stringValue(row) : '').toLowerCase().includes(q) ||
            (c.render ? '' : String((row as Record<string, unknown>)[c.key] ?? '').toLowerCase().includes(q)),
          ),
        )
      : [...data];

    if (sortKey) {
      const col = columns.find((c) => c.key === sortKey);
      const val = (r: T): string | number => {
        if (col?.stringValue) return col.stringValue(r);
        const v = (r as Record<string, unknown>)[sortKey];
        return typeof v === 'number' ? v : String(v ?? '');
      };
      out.sort((a, b) => {
        const va = val(a);
        const vb = val(b);
        const cmp = typeof va === 'number' && typeof vb === 'number'
          ? va - vb
          : String(va).localeCompare(String(vb), 'id', { numeric: true });
        return sortDir === 'asc' ? cmp : -cmp;
      });
    }
    return out;
  }, [data, search, columns, sortKey, sortDir]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const curPage = Math.min(page, pageCount - 1);
  const rows = filtered.slice(curPage * pageSize, (curPage + 1) * pageSize);
  const rangeFrom = filtered.length === 0 ? 0 : curPage * pageSize + 1;
  const rangeTo = Math.min(filtered.length, (curPage + 1) * pageSize);

  const toggleSort = (key: string) => {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  };

  return (
    <div className="t-wrap">
      {/* Toolbar: search + slot */}
      <div className="flex items-center gap-2 px-3.5 py-3 border-b border-line">
        <div className="relative flex-1 max-w-xs">
          <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-3 pointer-events-none" />
          <input
            value={search}
            onChange={(e) => onSearch(e.target.value)}
            placeholder="Cari…"
            className="f-input !h-8 !pl-8 text-caption"
          />
        </div>
        {toolbar}
      </div>

      {error ? (
        <EmptyState icon={Inbox} title="Terjadi kesalahan" message={error} />
      ) : loading ? (
        <div className="p-4 space-y-2" aria-busy="true">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-10 rounded-md bg-surface-2 animate-pulse" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <EmptyState icon={Inbox} title={emptyTitle} message={emptyMessage} action={emptyAction} />
      ) : (
        <div className="overflow-auto clamp" style={{ maxHeight }}>
          <table className="w-full border-collapse">
            <thead>
              <tr>
                {columns.map((c) => {
                  const sortable = c.sortable !== false;
                  const isSorted = sortKey === c.key;
                  return (
                    <th
                      key={c.key}
                      style={{ width: c.width }}
                      className={`t-head px-4 py-2.5 ${sortable ? 'cursor-pointer select-none hover:text-ink-1' : ''} ${
                        c.align === 'right' ? 'text-right' : c.align === 'center' ? 'text-center' : ''
                      }`}
                      onClick={sortable ? () => toggleSort(c.key) : undefined}
                      aria-sort={isSorted ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                    >
                      <span className={`inline-flex items-center gap-1 ${c.align === 'right' ? 'flex-row-reverse' : ''}`}>
                        {c.header}
                        {sortable && (
                          isSorted
                            ? sortDir === 'asc'
                              ? <ChevronUp size={12} className="text-primary" />
                              : <ChevronDown size={12} className="text-primary" />
                            : <ChevronDown size={12} className="text-ink-3/40" />
                        )}
                      </span>
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={rowKey(row)} className="t-row">
                  {columns.map((c) => (
                    <td
                      key={c.key}
                      className={`t-cell ${c.align === 'right' ? 'text-right tabular-nums' : c.align === 'center' ? 'text-center' : ''}`}
                    >
                      {c.render ? c.render(row) : String((row as Record<string, unknown>)[c.key] ?? '—')}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      <div className="flex items-center justify-between px-3.5 py-2.5 border-t border-line">
        <p className="text-caption text-ink-3">
          {filtered.length === 0 ? 'Tidak ada data' : `Menampilkan ${rangeFrom}–${rangeTo} dari ${filtered.length} baris`}
        </p>
        <div className="flex items-center gap-2">
          <select
            value={pageSize}
            onChange={(e) => { setPageSize(Number(e.target.value)); setPage(0); }}
            className="f-input !h-7 !w-auto text-caption pr-1"
            aria-label="Baris per halaman"
          >
            {pageSizeOptions.map((n) => <option key={n} value={n}>{n} / hal.</option>)}
          </select>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setPage(Math.max(0, curPage - 1))}
              disabled={curPage === 0}
              className="p-1.5 rounded-md border border-line text-ink-2 hover:bg-surface-2 disabled:opacity-40 transition-colors"
              aria-label="Halaman sebelumnya"
            >
              <ChevronLeft size={13} />
            </button>
            <span className="text-caption text-ink-2 px-1.5 tabular-nums">{curPage + 1} / {pageCount}</span>
            <button
              onClick={() => setPage(Math.min(pageCount - 1, curPage + 1))}
              disabled={curPage >= pageCount - 1}
              className="p-1.5 rounded-md border border-line text-ink-2 hover:bg-surface-2 disabled:opacity-40 transition-colors"
              aria-label="Halaman berikutnya"
            >
              <ChevronRight size={13} />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
