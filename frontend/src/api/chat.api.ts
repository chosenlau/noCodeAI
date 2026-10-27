import type {
  ChatHistoryVo,
  ChatHistoryQueryRequest,
  CursorResponse,
} from '@/types/api';
import api from './config';
import { reconnectableRunResponse } from './run-stream';

export interface GraphRun {
  id: string;
  appId: string;
  userId: string;
  requestId: string;
  kind: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'interrupted' | 'pausing' | 'paused' | 'awaiting_approval' | 'cancelling' | 'canceled' | 'recoverable';
  checkpoint?: {
    nextNode: string;
    planHash: string;
    plan: { goal: string; generationType: string; constraints: string[]; acceptance: string[]; todos: { id: string; title: string; status: string }[] };
    budget: { modelCalls: number; toolCalls: number; tokens: number };
  };
  sourceVersionId?: string;
  lastSequence: number;
  errorMessage?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

interface GraphRunCreateResult {
  run: GraphRun;
  created: boolean;
}

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

  createGraphRun: (appId: string, message: string, requestId: string) =>
    api.post('/app/graph/runs', { appId, message, requestId }) as Promise<GraphRunCreateResult>,

  getGraphRun: (runId: string) =>
    api.get(`/app/graph/runs/${runId}`) as Promise<GraphRun>,

  controlGraphRun: (runId: string, action: 'approve' | 'reject' | 'pause' | 'resume' | 'cancel', planHash?: string) =>
    api.post(`/app/graph/runs/${runId}/control`, { action, planHash }) as Promise<GraphRun>,

  getActiveGraphRun: (appId: string) =>
    api.get('/app/graph/runs/active', { params: { appId } }) as Promise<GraphRun>,

  subscribeGraphRun: (runId: string, afterSequence: number, signal?: AbortSignal) =>
    fetch(new URL(`/app/graph/runs/${runId}/events?afterSeq=${afterSequence}`, getApiBaseUrl()), {
      method: 'GET',
      headers: afterSequence > 0 ? { 'Last-Event-ID': String(afterSequence) } : undefined,
      credentials: 'include',
      signal,
    }),

  observeGraphRun: (appId: string, runId: string, signal?: AbortSignal) =>
    createReconnectableRunResponse(appId, runId, signal),

  generateCode: async (appId: string, message: string, signal?: AbortSignal) => {
    const pendingKey = `graph-request:${appId}`;
    const stored = sessionStorage.getItem(pendingKey);
    let pending: { message: string; requestId: string } | null = null;
    try { pending = stored ? JSON.parse(stored) : null; } catch { /* new request */ }
    const requestId = (pending?.message === message ? pending.requestId : undefined)
      ?? globalThis.crypto?.randomUUID?.()
      ?? `graph-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    sessionStorage.setItem(pendingKey, JSON.stringify({ message, requestId }));
    try {
      const { run } = await chatApi.createGraphRun(appId, message, requestId);
      sessionStorage.setItem(`graph-run:${appId}`, JSON.stringify({ runId: run.id, lastSequence: 0 }));
      sessionStorage.removeItem(pendingKey);
      return await createReconnectableRunResponse(appId, run.id, signal);
    } catch (error) {
      if (!(error instanceof Error) || !error.message.includes('disabled')) {
        throw error;
      }
      sessionStorage.removeItem(pendingKey);
      return fetch(new URL('/app/graph', getApiBaseUrl()), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ appId, message }),
        signal,
      });
    }
  },

  chatWithAgent: (appId: string, message: string, signal?: AbortSignal) =>
    fetch(new URL('/app/chat', getApiBaseUrl()), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ appId, message }),
      signal,
    }),
};

async function createReconnectableRunResponse(
  appId: string,
  runId: string,
  signal?: AbortSignal,
): Promise<Response> {
  sessionStorage.setItem(`graph-run:${appId}`, JSON.stringify({ runId }));
  return reconnectableRunResponse({
    subscribe: (after, subscriptionSignal) => chatApi.subscribeGraphRun(runId, after, subscriptionSignal),
    getState: () => chatApi.getGraphRun(runId),
  }, signal);
}
