import type {
  ChatHistoryVo,
  ChatHistoryQueryRequest,
  CursorResponse,
} from '@/types/api';
import api from './config';

function getApiBaseUrl() {
  return import.meta.env.PROD
    ? import.meta.env.VITE_API_BASE_URL
    : window.location.origin;
}

export const chatApi = {
  getChatHistory: (
    appId: string,
    params: ChatHistoryQueryRequest
  ) =>
    api.get(`/chatHistory/app/${appId}`, { params }) as Promise<CursorResponse<ChatHistoryVo>>,

  generateCode: (appId: string, message: string, signal?: AbortSignal) =>
    fetch(new URL('/app/graph', getApiBaseUrl()), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ appId, message }),
      signal,
    }),

  chatWithAgent: (appId: string, message: string, signal?: AbortSignal) =>
    fetch(new URL('/app/chat', getApiBaseUrl()), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ appId, message }),
      signal,
    }),
};
