import api from './config';
import type {
  AppVo,
  UpdateAppRequest,
  AppQueryRequest,
  PageResult
} from '@/types/api';

export const appApi = {
  listFeatured: (query: AppQueryRequest) =>
    api.post('/app/good/list/page/vo', query) as Promise<PageResult<AppVo>>,

  listMyApps: () =>
    api.get('/app/my/list/page/vo') as Promise<PageResult<AppVo>>,

  getDetail: (id: string) =>
    api.get('/app/get/vo', { params: { id } }) as Promise<AppVo>,

  getSourceCode: (appId: string, generationType: string) =>
    api.get(`/app/${appId}/source`, { params: { generationType } }) as Promise<Record<string, string>>,

  create: (prompt: string) =>
    api.post('/app/add', { initPrompt: prompt }) as Promise<string>,

  update: (data: UpdateAppRequest) =>
    api.post('/app/update', data) as Promise<boolean>,

  delete: (id: number) =>
    api.post('/app/delete', { id }) as Promise<boolean>,
};
