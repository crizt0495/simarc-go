import React from 'react';
export default function Layout({ children }: { children: React.ReactNode }) {
  return React.createElement('div', { className: 'layout desktop-modern' },
    React.createElement('aside', { className: 'sidebar-nav' },
      React.createElement('nav', null,
        React.createElement('a', { href: '/' }, 'Dashboard'),
        React.createElement('a', { href: '/arsip' }, 'Arsip'),
        React.createElement('a', { href: '/sync' }, 'Sinkronisasi'),
        React.createElement('a', { href: '/backup' }, 'Backup')
      )
    ),
    React.createElement('main', null, children)
  );
}
