import { useState } from 'react';
import { useParams } from 'react-router-dom';

export default function ChatPageDebug() {
  const { appId } = useParams<{ appId: string }>();
  const [logs, setLogs] = useState<string[]>([]);
  const [isRunning, setIsRunning] = useState(false);

  const addLog = (msg: string) => {
    const timestamp = new Date().toISOString().substr(11, 12);
    setLogs(prev => [...prev, `[${timestamp}] ${msg}`]);
    console.log(msg);
  };

  const testEventSource = () => {
    setIsRunning(true);
    setLogs([]);
    addLog('[DEBUG-01] Starting EventSource test...');

    const message = 'test message 测试';
    const url = `/app/chat?appId=${appId}&message=${encodeURIComponent(message)}`;

    addLog(`[DEBUG-02] URL: ${url}`);
    addLog(`[DEBUG-03] appId: ${appId}`);

    try {
      const es = new EventSource(url, { withCredentials: true });
      addLog('[DEBUG-04] ✓ EventSource created');

      let messageCount = 0;
      let accumulatedData = '';

      es.onopen = () => {
        addLog('[DEBUG-05] ✓ Connection opened');
      };

      es.onmessage = (event) => {
        messageCount++;
        addLog(`[DEBUG-06] Message #${messageCount}: ${event.data.substring(0, 50)}`);

        try {
          const parsed = JSON.parse(event.data);
          if (parsed.d) {
            accumulatedData += parsed.d;
            addLog(`[DEBUG-07] Accumulated ${accumulatedData.length} chars`);
          }
          if (parsed.code) {
            addLog(`[DEBUG-08] ⚠ Error code: ${parsed.code}, message: ${parsed.message}`);
          }
        } catch (err) {
          addLog(`[DEBUG-09] Parse error: ${err}`);
        }
      };

      es.onerror = () => {
        addLog(`[DEBUG-10] ⚠ EventSource error`);
        addLog(`[DEBUG-11] ReadyState: ${es.readyState} (0=CONNECTING, 1=OPEN, 2=CLOSED)`);
        es.close();
        setIsRunning(false);
      };

      es.addEventListener('done', () => {
        addLog('[DEBUG-12] ✓ Received "done" event');
        addLog(`[DEBUG-13] Total messages: ${messageCount}`);
        addLog(`[DEBUG-14] Total data: ${accumulatedData.length} chars`);
        es.close();
        setIsRunning(false);
      });

      // Timeout after 15 seconds
      setTimeout(() => {
        if (es.readyState !== 2) {
          addLog('[DEBUG-15] Timeout - closing');
          es.close();
          setIsRunning(false);
        }
      }, 15000);

    } catch (err: any) {
      addLog(`[DEBUG-16] EXCEPTION: ${err.message}`);
      addLog(`[DEBUG-17] Stack: ${err.stack}`);
      setIsRunning(false);
    }
  };

  return (
    <div style={{ padding: '20px', fontFamily: 'monospace', background: '#1a1a1a', color: '#0f0', minHeight: '100vh' }}>
      <h1 style={{ color: '#0f0' }}>ChatPage Debug - appId: {appId}</h1>

      <button
        onClick={testEventSource}
        disabled={isRunning}
        style={{
          padding: '10px 20px',
          marginBottom: '20px',
          background: isRunning ? '#666' : '#0f0',
          color: '#000',
          border: 'none',
          cursor: isRunning ? 'not-allowed' : 'pointer',
          fontSize: '16px'
        }}
      >
        {isRunning ? 'Running...' : 'Test EventSource'}
      </button>

      <button
        onClick={() => setLogs([])}
        style={{
          padding: '10px 20px',
          marginBottom: '20px',
          marginLeft: '10px',
          background: '#f00',
          color: '#fff',
          border: 'none',
          cursor: 'pointer',
          fontSize: '16px'
        }}
      >
        Clear Logs
      </button>

      <div style={{ background: '#000', padding: '10px', borderLeft: '3px solid #0f0' }}>
        {logs.map((log, i) => (
          <div key={i} style={{ margin: '5px 0' }}>{log}</div>
        ))}
      </div>
    </div>
  );
}
