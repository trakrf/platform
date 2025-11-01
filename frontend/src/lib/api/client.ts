import axios from 'axios';
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

const API_BASE_URL = getApiUrl();

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

// Response interceptor: Handle 401 (expired/invalid token)
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear Zustand auth state (important!)
      useAuthStore.getState().logout();

      // Note: logout() will clear persisted localStorage via Zustand middleware
      // No need to manually call localStorage.removeItem('auth-storage')

      // Show user notification
      toast.error('Session expired. Please log in again.');

      // Redirect to login
      window.location.hash = '#login';
    }
    return Promise.reject(error);
  }
);
