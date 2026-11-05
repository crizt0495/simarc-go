/**
 * ComingSoonPage — placeholder untuk fitur yang belum diimplementasikan
 * di desktop app, namun sudah tersedia di referensi web app.
 */
import React from 'react';
import { Construction } from 'lucide-react';
import { PageHeader, PageContainer } from '../components/PageHeader';
import { EmptyState } from '../components/ui';

export const ComingSoonPage: React.FC<{ title: string }> = ({ title }) => (
  <PageContainer>
    <PageHeader title={title} description="Modul ini sedang dalam pengembangan untuk versi desktop." />
    <div className="card p-6">
      <EmptyState
        icon={Construction}
        title="Segera Hadir"
        message={`Fitur "${title}" akan tersedia pada rilis berikutnya. Saat ini fokus pada modul Dashboard, Arsip, Sinkronisasi, Backup, dan Pengaturan.`}
      />
    </div>
  </PageContainer>
);
