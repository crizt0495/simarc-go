/**
 * KPI card — label kecil, nilai utama, ikon kecil, delta (bila tersedia).
 * Untuk grafik mini (Sparkline) terpisah agar pemanggil opsional.
 */
import React from 'react';
import { LucideIcon } from 'lucide-react';

export const DashboardStatCard: React.FC<{
  label: string;
  value: string | number;
  icon: LucideIcon;
  delta?: string;
  deltaTone?: 'up' | 'down' | 'neutral';
  hint?: string;
}> = ({ label, value, icon: Icon, delta, deltaTone = 'neutral', hint }) => (
  <div className="card p-4 flex items-start gap-3">
    <div className="w-8 h-8 rounded-lg bg-primary-soft flex items-center justify-center shrink-0 mt-0.5">
      <Icon size={15} strokeWidth={1.9} className="text-primary" />
    </div>
    <div className="min-w-0 flex-1">
      <p className="text-label font-medium text-ink-2">{label}</p>
      <div className="flex items-baseline gap-2 mt-0.5">
        <p className="text-2xl font-semibold text-ink-1 leading-tight tabular-nums">{value}</p>
        {delta && (
          <span className={`text-caption font-medium ${deltaTone === 'up' ? 'text-ok' : deltaTone === 'down' ? 'text-bad' : 'text-ink-3'}`}>
            {delta}
          </span>
        )}
      </div>
      {hint && <p className="text-caption text-ink-3 mt-1 truncate">{hint}</p>}
    </div>
  </div>
);
