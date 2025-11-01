import axios from 'axios';
import toast from 'react-hot-toast';
import { useAuthStore } from '@/stores/authStore';

const getApiUrl = (): string => {
  const apiUrl = import.meta.env.VITE_API_URL;

  if (!apiUrl) {
    const errorMsg = 'VITE_API_URL environment variable is not set. Please check your .env file.';
    console.error('[API Client]', errorMsg);
    throw new Error(errorMsg);
  }

  try {
    new URL(apiUrl);
  } catch (err) {
    const errorMsg = `Invalid VITE_API_URL format: ${apiUrl}. Must be a valid URL.`;
    console.error('[API Client]', errorMsg);
    throw new Error(errorMsg);
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
