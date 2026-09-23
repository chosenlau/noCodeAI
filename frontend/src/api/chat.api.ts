import type {
  ChatHistoryVo,
  ChatHistoryQueryRequest,
  CursorResponse,
} from '@/types/api';
import api from './config';

export const chatApi = {
  getChatHistory: (
    appId: string,
    params: ChatHistoryQueryRequest
  ) =>
    api.get(`/chatHistory/app/${appId}`, { params }) as Promise<CursorResponse<ChatHistoryVo>>,
};
