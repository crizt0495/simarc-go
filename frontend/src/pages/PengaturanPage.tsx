/**
 * Pengaturan — placeholder.
 */
import React from 'react';
import { Settings } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { EmptyState } from '../components/ui';

export const PengaturanPage: React.FC = () => (
  <PageContainer>
    <PageHeader title="Pengaturan" description="Konfigurasi aplikasi." />
    <div className="card p-6">
      <EmptyState icon={Settings} title="Segera Hadir" message="Fitur pengaturan sedang dalam pengembangan." />
    </div>
  </PageContainer>
);
