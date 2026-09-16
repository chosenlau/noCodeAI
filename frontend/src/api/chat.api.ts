import type {
  ChatHistoryVo,
  ChatHistoryQueryRequest,
  PageResult,
} from '@/types/api';
import api from './config';

export const chatApi = {
  getChatHistory: (
    appId: string,
    params: ChatHistoryQueryRequest
  ) =>
    api.get(`/chatHistory/app/${appId}`, { params }) as Promise<PageResult<ChatHistoryVo>>,
};
