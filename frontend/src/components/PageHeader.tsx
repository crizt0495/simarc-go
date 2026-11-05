/**
 * PageHeader — pola header konsisten: breadcrumb handled topbar, judul,
 * deskripsi, aksi utama & sekunder, filter/tab di kanan bila relevan.
 */
import React from 'react';

export const PageHeader: React.FC<{
  title: string;
  description?: string;
  actions?: React.ReactNode;
}> = ({ title, description, actions }) => (
  <div className="flex items-start justify-between gap-4 mb-5">
    <div className="min-w-0">
      <h1 className="text-page-title font-semibold text-ink-1 leading-tight">{title}</h1>
      {description && <p className="text-body text-ink-2 mt-1">{description}</p>}
    </div>
    {actions && <div className="flex items-center gap-2 shrink-0 pt-1">{actions}</div>}
  </div>
);

export const PageContainer: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div className="p-6 max-w-[1440px] mx-auto w-full">{children}</div>
);
