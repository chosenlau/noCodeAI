import api from './config';
import type { User, LoginRequest, RegisterRequest } from '@/types/api';

export const userApi = {
  register: (data: RegisterRequest) =>
    api.post('/user/register', data) as Promise<string>,

  login: (data: LoginRequest) =>
    api.post('/user/login', data) as Promise<User>,

  logout: () =>
    api.get('/user/logout') as Promise<boolean>,

  getCurrentUser: () =>
    api.get('/user/get/login') as Promise<User>,

  getUserById: (id: string) =>
    api.get('/user/get/vo', { params: { id } }) as Promise<User>,
};
