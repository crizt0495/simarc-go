/**
 * Backup — placeholder.
 */
import React from 'react';
import { DatabaseBackup } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { EmptyState } from '../components/ui';

export const BackupPage: React.FC = () => (
  <PageContainer>
    <PageHeader title="Backup & Restore" description="Cadangkan database lokal." />
    <div className="card p-6">
      <EmptyState icon={DatabaseBackup} title="Segera Hadir" message="Fitur backup sedang dalam pengembangan." />
    </div>
  </PageContainer>
);
