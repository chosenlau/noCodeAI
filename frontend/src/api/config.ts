import axios, { AxiosError, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { ApiError } from '@/lib/errors';
import type { BaseResponse } from '@/types/api';

const api = axios.create({
  baseURL: import.meta.env.PROD ? import.meta.env.VITE_API_BASE_URL : '',
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器
api.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器 - 自动解包 BaseResponse
api.interceptors.response.use(
  (response: AxiosResponse<BaseResponse>) => {
    const { code, message, data } = response.data;

    if (code === 0) {
      // 直接返回 data，axios 会将其包装为 Promise
      return Promise.resolve(data) as any;
    }

    throw new ApiError(code, message);
  },
  (error: AxiosError<BaseResponse>) => {
    if (error.response) {
      const { code, message } = error.response.data;
      throw new ApiError(code || 50000, message || '网络请求失败');
    }
    throw new ApiError(50000, '网络连接失败');
  }
);

export default api;
