import axios, {
  AxiosError,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from 'axios';
import { ApiError } from '@/lib/errors';
import { useAuthStore } from '@/store/auth';
import type { BaseResponse } from '@/types/api';

const api = axios.create({
  baseURL: import.meta.env.PROD ? import.meta.env.VITE_API_BASE_URL : '',
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
});

function handleAuthExpired() {
  useAuthStore.getState().logout();
  if (window.location.pathname !== '/') {
    window.location.replace('/');
  }
}

api.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => config,
  (error) => Promise.reject(error),
);

api.interceptors.response.use(
  (response: AxiosResponse<BaseResponse>) => {
    const { code, message, data } = response.data;

    if (code === 0) {
      return Promise.resolve(data) as any;
    }

    const apiError = new ApiError(code, message);
    if (apiError.isNotLogin()) {
      handleAuthExpired();
    }
    throw apiError;
  },
  (error: AxiosError<BaseResponse>) => {
    if (error.response) {
      const { code, message } = error.response.data;
      const apiError = new ApiError(
        code || (error.response.status === 401 ? 40100 : 50000),
        message || '网络请求失败',
      );
      if (apiError.isNotLogin()) {
        handleAuthExpired();
      }
      throw apiError;
    }
    throw new ApiError(50000, '网络连接失败');
  },
);

export default api;
