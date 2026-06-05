import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios';
import toast from 'react-hot-toast';
import { useAuthStore } from '@/stores/authStore';

const getApiUrl = (): string => {
  const apiUrl = import.meta.env.VITE_API_URL || '/api/v1';

  if (apiUrl && apiUrl !== '/api/v1') {
    try {
      new URL(apiUrl);
    } catch (err) {
      console.warn(`[API Client] Invalid VITE_API_URL format: ${apiUrl}. Falling back to /api/v1`);
      return '/api/v1';
    }
  }

  console.info('[API Client] Using API URL:', apiUrl);
  return apiUrl;
};

export const API_BASE_URL = getApiUrl();

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor: Inject Bearer token from localStorage
apiClient.interceptors.request.use((config) => {
  const authStorage = localStorage.getItem('auth-storage');

  if (authStorage) {
    try {
      const { state } = JSON.parse(authStorage);
      if (state?.token) {
        config.headers.Authorization = `Bearer ${state.token}`;
      }
    } catch (err) {
      console.error('Failed to parse auth storage:', err);
    }
  }

  return config;
});

// Response interceptor: on a 401 from anything except /auth/refresh itself,
// try to refresh the access token once and replay the original request. If
// the refresh fails (no refresh token held, revoked, expired, replay-detected,
// etc.) fall back to the legacy behavior: clear auth state, toast, redirect.
//
// Concurrency dedupe: a page mount typically fans out several GETs in
// parallel. When the access token has just expired, all of them 401 at the
// same time. We dedupe to a single in-flight refresh via `inFlightRefresh`;
// concurrent failures await the same promise instead of each minting a new
// refresh and racing to clobber the store. Without dedupe, single-use
// rotation on the server would treat all-but-one of those concurrent
// refreshes as a replay attack and revoke the whole chain.

// _retry is a per-request marker — once a request has been replayed after a
// successful refresh, a subsequent 401 must NOT trigger another refresh.
// Otherwise a backend that returns 401 for a legitimately-forbidden access
// would loop forever.
interface RetriableConfig extends InternalAxiosRequestConfig {
  _retry?: boolean;
}

let inFlightRefresh: Promise<boolean> | null = null;

const performRefresh = (): Promise<boolean> => {
  if (inFlightRefresh) return inFlightRefresh;
  inFlightRefresh = useAuthStore
    .getState()
    .refresh()
    .finally(() => {
      inFlightRefresh = null;
    });
  return inFlightRefresh;
};

const isAuthMaintenanceRequest = (url: string | undefined): boolean => {
  if (!url) return false;
  return url.endsWith('/auth/refresh') || url.endsWith('/auth/logout');
};

const forceLogoutAndRedirect = () => {
  // logout() is async (it calls /auth/logout server-side) but we don't await
  // — the user seeing the toast/redirect immediately matters more than the
  // server-side revoke completing here. logout() handles its own errors.
  void useAuthStore.getState().logout();
  toast.error('Session expired. Please log in again.');
  window.location.hash = '#login';
};

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const status = error.response?.status;
    const original = error.config as RetriableConfig | undefined;

    if (status !== 401 || !original) {
      return Promise.reject(error);
    }

    // A 401 from /auth/refresh itself means the refresh token is bad —
    // chain compromised, revoked, or expired. Hard-logout, no retry.
    // Same for /auth/logout: don't loop logout calls.
    if (isAuthMaintenanceRequest(original.url)) {
      if (original.url?.endsWith('/auth/refresh')) {
        forceLogoutAndRedirect();
      }
      return Promise.reject(error);
    }

    // Avoid infinite retry loops if the replayed request 401s again.
    if (original._retry) {
      forceLogoutAndRedirect();
      return Promise.reject(error);
    }

    // No refresh token held (e.g., very first login flow with a stale
    // access token in localStorage from before this feature) — original
    // forced-logout behavior is correct.
    if (!useAuthStore.getState().refreshToken) {
      forceLogoutAndRedirect();
      return Promise.reject(error);
    }

    const refreshed = await performRefresh();
    if (!refreshed) {
      forceLogoutAndRedirect();
      return Promise.reject(error);
    }

    // Replay the original request. The request interceptor will read the
    // new access token from the store on its way out.
    original._retry = true;
    return apiClient(original);
  }
);
