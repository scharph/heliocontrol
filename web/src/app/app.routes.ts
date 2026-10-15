import { inject } from '@angular/core';
import { CanActivateFn, Router, Routes } from '@angular/router';
import { Api } from './core/api';

const authGuard: CanActivateFn = async () => {
  const api = inject(Api);
  const router = inject(Router);
  try {
    const a = await api.auth();
    return !a.required || a.authenticated ? true : router.parseUrl('/login');
  } catch {
    return true; // server unreachable: show the app with its offline state
  }
};

export const routes: Routes = [
  { path: 'login', loadComponent: () => import('./pages/login').then((m) => m.LoginPage) },
  {
    path: '',
    canActivate: [authGuard],
    loadComponent: () => import('./shell').then((m) => m.Shell),
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'overview' },
      { path: 'overview', loadComponent: () => import('./pages/overview').then((m) => m.OverviewPage) },
      { path: 'overview/:panel', loadComponent: () => import('./pages/overview').then((m) => m.OverviewPage) },
      { path: 'timeprograms', loadComponent: () => import('./pages/time-programs').then((m) => m.TimeProgramsPage) },
      { path: 'recorder', loadComponent: () => import('./pages/recorder').then((m) => m.RecorderPage) },
      { path: 'recorder/:id', loadComponent: () => import('./pages/recorder').then((m) => m.RecorderPage) },
      { path: 'trend', loadComponent: () => import('./pages/trend').then((m) => m.TrendPage) },
      { path: 'points', loadComponent: () => import('./pages/points').then((m) => m.PointsPage) },
      { path: 'log', loadComponent: () => import('./pages/log').then((m) => m.LogPage) },
      { path: 'settings', loadComponent: () => import('./pages/settings').then((m) => m.SettingsPage) },
    ],
  },
  { path: '**', redirectTo: '' },
];
