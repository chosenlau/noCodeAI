export interface RunStreamDependencies {
  subscribe: (after: number, signal: AbortSignal) => Promise<Response>;
  getState: () => Promise<{ status: string; lastSequence: number }>;
  retryDelay?: number;
}

export function parseFrame(frame: string) {
  const lines = frame.split(/\r?\n/);
  return {
    sequence: Number(lines.find((line) => line.startsWith('id:'))?.slice(3).trim() ?? 0),
    event: lines.find((line) => line.startsWith('event:'))?.slice(6).trim() ?? 'message',
  };
}

class SubscriptionRejected extends Error {}

// Each page mount starts from zero to rebuild its UI. Only reconnects within
// this stream continue from the delivered sequence. The database owns history.
export function reconnectableRunResponse(deps: RunStreamDependencies, signal?: AbortSignal): Response {
  const abort = new AbortController();
  const onAbort = () => abort.abort();
  signal?.addEventListener('abort', onAbort, { once: true });
  if (signal?.aborted) abort.abort();
  const iterator = frames();
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    async pull(controller) {
      try {
        const result = await iterator.next();
        if (result.done) controller.close();
        else controller.enqueue(encoder.encode(`${result.value}\n\n`));
      } catch (error) {
        controller.error(error);
      }
    },
    async cancel() {
      abort.abort();
      await iterator.return(undefined);
    },
  });
  return new Response(stream, { headers: { 'Content-Type': 'text/event-stream; charset=utf-8' } });

  async function* frames(): AsyncGenerator<string, void> {
    let after = 0;
    let retries = 0;
    try {
      while (!abort.signal.aborted) {
        try {
          const response = await deps.subscribe(after, abort.signal);
          if (!response.ok || !response.body || !response.headers.get('content-type')?.includes('text/event-stream')) {
            let message = `任务订阅被拒绝 (${response.status})`;
            try { const body = await response.json(); message = body.message || message; } catch { /* use status */ }
            throw new SubscriptionRejected(message);
          }
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';
          try {
            while (!abort.signal.aborted) {
              const { value, done } = await reader.read();
              if (done) break;
              buffer += decoder.decode(value, { stream: true });
              const parts = buffer.split(/\r?\n\r?\n/);
              buffer = parts.pop() ?? '';
              for (const frame of parts) {
                const { sequence, event } = parseFrame(frame);
                if (!frame.trim() || (sequence > 0 && sequence <= after)) continue;
                yield frame;
                if (sequence > 0) { after = sequence; retries = 0; }
                if (event === 'done') return;
              }
            }
          } finally {
            await reader.cancel().catch(() => undefined);
            reader.releaseLock();
          }
          if (abort.signal.aborted) break;
          const state = await deps.getState();
          if (!['pending', 'running', 'pausing', 'cancelling'].includes(state.status) && after >= state.lastSequence) return;
        } catch (error) {
          if (abort.signal.aborted) break;
          if (error instanceof SubscriptionRejected) throw error;
        }
        if (++retries > 10) throw new Error('进度连接中断，后台任务可能仍在执行，请重新打开项目查看');
        yield 'event: connection_status\ndata: 正在重新连接任务进度';
        await new Promise<void>((resolve) => {
          const finish = () => { clearTimeout(timer); abort.signal.removeEventListener('abort', finish); resolve(); };
          const timer = setTimeout(finish, deps.retryDelay ?? 1000);
          abort.signal.addEventListener('abort', finish, { once: true });
        });
      }
      throw new DOMException('Aborted', 'AbortError');
    } finally {
      abort.abort();
      signal?.removeEventListener('abort', onAbort);
    }
  }
}
