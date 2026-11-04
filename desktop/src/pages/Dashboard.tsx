import React from 'react';
export default function Dashboard() {
  return React.createElement('div', { className: 'dashboard' },
    React.createElement('h1', null, 'Dashboard SIMARC'),
    React.createElement('p', null, 'Status: Offline-First Siap — Sinkronisasi otomatis saat jaringan tersedia'),
    React.createElement('div', { className: 'cards' },
      React.createElement('div', { className: 'card' }, 'Database Lokal: AKTIF'),
      React.createElement('div', { className: 'card' }, 'Server Pusat: 192.168.1.86:8080'),
      React.createElement('div', { className: 'card' }, 'Mode: KLIEN')
    )
  );
}
