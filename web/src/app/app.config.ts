import { HttpErrorResponse, HttpInterceptorFn, provideHttpClient, withFetch, withInterceptors } from '@angular/common/http';
import { ApplicationConfig, inject, provideBrowserGlobalErrorListeners } from '@angular/core';
import { Router, provideRouter, withComponentInputBinding } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { routes } from './app.routes';

/** Send the user to the login page when the session expired. */
const authInterceptor: HttpInterceptorFn = (req, next) => {
  const router = inject(Router);
  return next(req).pipe(
    catchError((e: unknown) => {
      if (e instanceof HttpErrorResponse && e.status === 401 && !req.url.endsWith('/api/login')) {
        router.navigate(['/login']);
      }
      return throwError(() => e);
    }),
  );
};

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideHttpClient(withFetch(), withInterceptors([authInterceptor])),
    provideRouter(routes, withComponentInputBinding()),
  ],
};
