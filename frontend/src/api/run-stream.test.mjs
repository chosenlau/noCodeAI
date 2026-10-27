import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reconnectableRunResponse } from './run-stream.ts';

function response(...chunks) {
  return new Response(new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(new TextEncoder().encode(chunk));
      controller.close();
    },
  }), { headers: { 'Content-Type': 'text/event-stream' } });
}

test('reconnect resumes sequence, removes duplicate events and handles split frames', async () => {
  const cursors = [];
  const stream = reconnectableRunResponse({
    retryDelay: 0,
    subscribe: async (after) => {
      cursors.push(after);
      return after === 0
        ? response('id: 1\nevent: step_started\nda', 'ta: first\n\n')
        : response('id: 1\nevent: step_started\ndata: duplicate\n\n',
          'id: 2\nevent: done\ndata: 1\n\n');
    },
    getState: async () => ({ status: 'running', lastSequence: 2 }),
  });
  const text = await stream.text();
  assert.deepEqual(cursors, [0, 1]);
  assert.match(text, /data: first/);
  assert.doesNotMatch(text, /duplicate/);
  assert.match(text, /event: done/);
});

test('each page starts replay from zero, including completed results', async () => {
  for (let page = 0; page < 2; page++) {
    const stream = reconnectableRunResponse({
      subscribe: async (after) => {
        assert.equal(after, 0);
        return response('id: 1\nevent: code_completed\ndata: {"index.html":"ok"}\n\n',
          'id: 2\nevent: done\ndata: 1\n\n');
      },
      getState: async () => ({ status: 'completed', lastSequence: 2 }),
    });
    assert.match(await stream.text(), /index.html/);
  }
});

test('rejects JSON/auth responses without retry', async () => {
  let calls = 0;
  const stream = reconnectableRunResponse({
    subscribe: async () => { calls++; return Response.json({ message: 'login required' }); },
    getState: async () => { throw new Error('must not query'); },
  });
  await assert.rejects(stream.text(), /login required/);
  assert.equal(calls, 1);
});

test('canceling a page subscription aborts its connection', async () => {
  let signal;
  let canceled = false;
  const stream = reconnectableRunResponse({
    subscribe: async (_after, subscriptionSignal) => {
      signal = subscriptionSignal;
      return new Response(new ReadableStream({
        start(controller) { controller.enqueue(new TextEncoder().encode('id: 1\nevent: step_started\ndata: ok\n\n')); },
        cancel() { canceled = true; },
      }), { headers: { 'Content-Type': 'text/event-stream' } });
    },
    getState: async () => ({ status: 'running', lastSequence: 1 }),
  });
  const reader = stream.body.getReader();
  await reader.read();
  await reader.cancel();
  assert.equal(signal.aborted, true);
  assert.equal(canceled, true);
});

test('paused and approval streams finish without synthesizing completion', async () => {
  for (const status of ['paused', 'awaiting_approval', 'recoverable']) {
    let calls = 0;
    const stream = reconnectableRunResponse({
      subscribe: async () => { calls++; return response('id: 1\nevent: run_suspended\ndata: {}\n\n'); },
      getState: async () => ({ status, lastSequence: 1 }),
    });
    const text = await stream.text();
    assert.equal(calls, 1);
    assert.match(text, /run_suspended/);
    assert.doesNotMatch(text, /event: done/);
  }
});
