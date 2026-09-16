import { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { appApi, chatApi } from '@/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card } from '@/components/ui/card';
import { toast } from '@/hooks/use-toast';
import { extractHtmlCode } from '@/lib/codeResponse';
import { ArrowLeft, Send } from 'lucide-react';
import Editor from '@monaco-editor/react';
import type { ChatHistoryVo } from '@/types/api';

export default function ChatPage() {
  const { appId } = useParams<{ appId: string }>();
  const navigate = useNavigate();
  const [message, setMessage] = useState('');
  const [chatHistory, setChatHistory] = useState<ChatHistoryVo[]>([]);
  const [generatedCode, setGeneratedCode] = useState('');
  const [isGenerating, setIsGenerating] = useState(false);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const accumulatedCodeRef = useRef('');
  const updateTimerRef = useRef<number | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const {
    data: app,
    isLoading: isLoadingApp,
    error: appError,
    refetch: refetchApp,
  } = useQuery({
    queryKey: ['app', appId],
    queryFn: () => appApi.getDetail(appId!),
    enabled: !!appId,
  });

  const {
    data: historyData,
    isLoading: isLoadingHistory,
    error: historyError,
    refetch: refetchHistory,
  } = useQuery({
    queryKey: ['chatHistory', appId],
    queryFn: () =>
      chatApi.getChatHistory(appId!, {
        pageSize: 50,
      }),
    enabled: !!appId,
  });

  // Hydrate the local conversation once when persisted history becomes available.
  useEffect(() => {
    if (!isLoadingHistory && historyData?.records && chatHistory.length === 0) {
      const chronologicalHistory = [...historyData.records].reverse();
      setChatHistory(chronologicalHistory);

      const latestAiMessage = historyData.records.find(
        (item) => item.messageType === 'ai'
      );
      if (latestAiMessage) {
        setGeneratedCode(extractHtmlCode(latestAiMessage.message));
      }
    }
  }, [isLoadingHistory, historyData, chatHistory.length]);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chatHistory]);

  useEffect(() => {
    return () => {
      eventSourceRef.current?.close();
      eventSourceRef.current = null;
      if (updateTimerRef.current) {
        clearTimeout(updateTimerRef.current);
        updateTimerRef.current = null;
      }
    };
  }, []);

  const handleSendMessage = async () => {
    if (!message.trim() || !appId || isGenerating) return;

    const userMessage: ChatHistoryVo = {
      id: Date.now().toString(),
      message: message.trim(),
      messageType: 'user',
      appId: appId,
      userId: '',
      turnNumber: chatHistory.length + 1,
      createTime: new Date().toISOString(),
      updateTime: new Date().toISOString(),
      isDelete: 0,
    };

    setChatHistory((prev) => [...prev, userMessage]);
    setMessage('');
    setIsGenerating(true);
    accumulatedCodeRef.current = '';

    try {
      const apiBaseUrl = import.meta.env.PROD
        ? import.meta.env.VITE_API_BASE_URL
        : window.location.origin;
      const streamUrl = new URL('/app/chat', apiBaseUrl || window.location.origin);
      streamUrl.searchParams.set('appId', appId);
      streamUrl.searchParams.set('message', message.trim());
      const eventSource = new EventSource(streamUrl, { withCredentials: true });
      eventSourceRef.current = eventSource;

      const aiMessageId = (Date.now() + 1).toString();
      let messageCount = 0;
      let finalized = false;

      const commitUpdate = (isFinal = false) => {
        const currentCode = accumulatedCodeRef.current;
        if (isFinal) {
          setGeneratedCode(extractHtmlCode(currentCode));
        }
        setChatHistory((prev) => {
          const existingAiMessage = prev.find((msg) => msg.id === aiMessageId);
          if (existingAiMessage) {
            return prev.map((msg) =>
              msg.id === aiMessageId ? { ...msg, message: currentCode } : msg
            );
          }

          const aiMessage: ChatHistoryVo = {
            id: aiMessageId,
            message: currentCode,
            messageType: 'ai',
            appId,
            userId: '',
            turnNumber: chatHistory.length + 2,
            createTime: new Date().toISOString(),
            updateTime: new Date().toISOString(),
            isDelete: 0,
          };
          return [...prev, aiMessage];
        });
      };

      const finalizeStream = (isSuccessful: boolean) => {
        if (finalized) return;
        finalized = true;

        if (updateTimerRef.current) {
          clearTimeout(updateTimerRef.current);
          updateTimerRef.current = null;
        }
        if (accumulatedCodeRef.current) {
          commitUpdate(isSuccessful);
        }
        eventSource.close();
        if (eventSourceRef.current === eventSource) {
          eventSourceRef.current = null;
        }
        setIsGenerating(false);
      };

      // Batch UI updates - only update every 10 messages or 200ms
      const scheduleUpdate = () => {
        if (updateTimerRef.current) return;

        updateTimerRef.current = setTimeout(() => {
          updateTimerRef.current = null;
          commitUpdate();
        }, 200);
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.d) {
            accumulatedCodeRef.current += data.d;
            messageCount++;

            // Update UI every 10 messages or schedule a debounced update
            if (messageCount % 10 === 0) {
              if (updateTimerRef.current) {
                clearTimeout(updateTimerRef.current);
                updateTimerRef.current = null;
              }
              commitUpdate();
            } else {
              scheduleUpdate();
            }
          }
        } catch (err) {
          console.error('Failed to parse SSE message:', err);
        }
      };

      eventSource.onerror = (event) => {
        const message = event instanceof MessageEvent ? event.data : '';
        toast({
          variant: 'destructive',
          title: '生成失败',
          description: message || '流式连接意外中断，请重试',
        });
        finalizeStream(false);
      };

      eventSource.addEventListener('done', () => finalizeStream(true));
    } catch (error) {
      console.error('Failed to start SSE:', error);
      toast({
        variant: 'destructive',
        title: '发送失败',
        description: '请稍后重试',
      });
      setIsGenerating(false);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  const queryError = appError || historyError;

  if (queryError) {
    return (
      <main className="h-screen grid place-items-center bg-background p-6">
        <section className="max-w-md space-y-4 text-center" role="alert">
          <h1 className="text-xl font-semibold">聊天页面加载失败</h1>
          <p className="text-sm text-muted-foreground">
            {queryError instanceof Error ? queryError.message : '请稍后重试'}
          </p>
          <Button onClick={() => void Promise.all([refetchApp(), refetchHistory()])}>
            重新加载
          </Button>
        </section>
      </main>
    );
  }

  if (isLoadingApp || isLoadingHistory) {
    return (
      <main className="h-screen grid place-items-center bg-background" aria-busy="true">
        <p className="text-muted-foreground">正在加载聊天内容...</p>
      </main>
    );
  }

  return (
    <div className="h-screen flex flex-col bg-background">
      <header className="border-b px-4 py-3 flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/apps')}>
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <h1 className="text-lg font-semibold">
          {app?.appName || '未命名应用'}
        </h1>
      </header>

      <div className="flex-1 flex overflow-hidden">
        <div className="w-1/2 flex flex-col border-r">
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {chatHistory.map((msg) => (
              <div
                key={msg.id}
                className={`flex ${
                  msg.messageType === 'user' ? 'justify-end' : 'justify-start'
                }`}
              >
                <Card
                  className={`max-w-[80%] p-3 ${
                    msg.messageType === 'user'
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-muted'
                  }`}
                >
                  <pre className="whitespace-pre-wrap text-sm font-sans">
                    {msg.message}
                  </pre>
                </Card>
              </div>
            ))}
            <div ref={chatEndRef} />
          </div>

          <div className="border-t p-4">
            <div className="flex gap-2">
              <Input
                placeholder="输入消息..."
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyPress={handleKeyPress}
                disabled={isGenerating}
              />
              <Button
                onClick={handleSendMessage}
                disabled={isGenerating || !message.trim()}
              >
                <Send className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        <div className="w-1/2 flex flex-col">
          <div className="flex-1 overflow-hidden">
            <div className="h-1/2 border-b">
              <Editor
                height="100%"
                defaultLanguage="html"
                value={generatedCode}
                theme="vs-light"
                options={{
                  readOnly: true,
                  minimap: { enabled: false },
                  fontSize: 14,
                  lineNumbers: 'on',
                  scrollBeyondLastLine: false,
                }}
              />
            </div>
            <div className="h-1/2 bg-white">
              <iframe
                className="w-full h-full border-0"
                title="Preview"
                sandbox="allow-scripts"
                srcDoc={generatedCode}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
