import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, CheckCircle2, ChevronDown, Loader2, Send, Wrench } from 'lucide-react';
import Editor from '@monaco-editor/react';
import { SandpackPreview, SandpackProvider } from '@codesandbox/sandpack-react';
import { appApi, chatApi } from '@/api';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { toast } from '@/hooks/use-toast';
import type { ChatHistoryVo } from '@/types/api';

type GeneratedFiles = Record<string, string>;

interface WorkflowStep {
  stepNumber: number;
  currentStep: string;
  nextStep?: string;
}

interface ToolEvent {
  type: 'tool_request' | 'tool_executed';
  name: string;
  arguments?: string;
  result?: string;
}

function getSandpackFiles(files: GeneratedFiles): Record<string, string> {
  const sandpackFiles: Record<string, string> = {};
  for (const [fileName, content] of Object.entries(files)) {
    if (fileName.endsWith('/description.md') || fileName === 'description.md') continue;
    const normalizedName = fileName.startsWith('/') ? fileName : `/${fileName}`;
    sandpackFiles[normalizedName] = fileName === 'package.json'
      ? normalizePackageJSON(content)
      : content;
  }
  return sandpackFiles;
}

function normalizePackageJSON(content: string): string {
  const trimmed = content.trim();
  let packageJSON: Record<string, unknown>;
  try {
    packageJSON = JSON.parse(trimmed) as Record<string, unknown>;
  } catch {
    const start = trimmed.indexOf('{');
    const end = trimmed.lastIndexOf('}');
    if (start >= 0 && end > start) {
      try {
        packageJSON = JSON.parse(trimmed.slice(start, end + 1)) as Record<string, unknown>;
      } catch {
        packageJSON = {};
      }
    } else {
      packageJSON = {};
    }
  }

  const devDependencies = {
    ...((packageJSON.devDependencies as Record<string, string> | undefined) ?? {}),
    vite: '4.1.4',
    '@vitejs/plugin-vue': '3.2.0',
    'esbuild-wasm': '0.17.12',
  };
  packageJSON.devDependencies = devDependencies;
  return JSON.stringify(packageJSON, null, 2);
}

export default function ChatPage() {
  const { appId } = useParams<{ appId: string }>();
  const navigate = useNavigate();
  const [message, setMessage] = useState('');
  const [chatHistory, setChatHistory] = useState<ChatHistoryVo[]>([]);
  const [generatedCode, setGeneratedCode] = useState('');
  const [generatedFiles, setGeneratedFiles] = useState<GeneratedFiles>({});
  const [selectedFile, setSelectedFile] = useState('');
  const [runningStep, setRunningStep] = useState<WorkflowStep | null>(null);
  const [completedStep, setCompletedStep] = useState<WorkflowStep | null>(null);
  const [description, setDescription] = useState('');
  const [descriptionStreaming, setDescriptionStreaming] = useState(false);
  const [toolEvents, setToolEvents] = useState<ToolEvent[]>([]);
  const [toolEventsExpanded, setToolEventsExpanded] = useState(false);
  const [activePanel, setActivePanel] = useState<'source' | 'preview'>('preview');
  const [isGenerating, setIsGenerating] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [chatPaneWidth, setChatPaneWidth] = useState(50);
  const [fileListWidth, setFileListWidth] = useState(176);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const sandpackFiles = useMemo(() => getSandpackFiles(generatedFiles), [generatedFiles]);

  const startChatPaneResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = chatPaneWidth;
    const handleMove = (moveEvent: PointerEvent) => {
      const nextWidth = startWidth + ((moveEvent.clientX - startX) / window.innerWidth) * 100;
      setChatPaneWidth(Math.min(75, Math.max(25, nextWidth)));
    };
    const handleUp = () => {
      window.removeEventListener('pointermove', handleMove);
      window.removeEventListener('pointerup', handleUp);
    };
    window.addEventListener('pointermove', handleMove);
    window.addEventListener('pointerup', handleUp);
  };

  const startFileListResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = fileListWidth;
    const handleMove = (moveEvent: PointerEvent) => {
      setFileListWidth(Math.min(320, Math.max(120, startWidth + moveEvent.clientX - startX)));
    };
    const handleUp = () => {
      window.removeEventListener('pointermove', handleMove);
      window.removeEventListener('pointerup', handleUp);
    };
    window.addEventListener('pointermove', handleMove);
    window.addEventListener('pointerup', handleUp);
  };

  const appQuery = useQuery({
    queryKey: ['app', appId],
    queryFn: () => appApi.getDetail(appId!),
    enabled: !!appId,
  });
  const historyQuery = useQuery({
    queryKey: ['chatHistory', appId],
    queryFn: () => chatApi.getChatHistory(appId!, { pageSize: 50 }),
    enabled: !!appId,
  });
  const sourceCodeQuery = useQuery({
    queryKey: ['sourceCode', appId, appQuery.data?.codeGenType],
    queryFn: () => appApi.getSourceCode(appId!, appQuery.data!.codeGenType),
    enabled: activePanel === 'source' && !!appId && !!appQuery.data?.codeGenType,
  });

  useEffect(() => {
    if (activePanel === 'source' && sourceCodeQuery.data) {
      const files = sourceCodeQuery.data;
      const firstFile = Object.keys(files)[0] ?? '';
      setGeneratedFiles(files);
      setSelectedFile((current) => current && files[current] ? current : firstFile);
      setGeneratedCode((current) => current || (firstFile ? files[firstFile] : ''));
    }
  }, [activePanel, sourceCodeQuery.data]);

  useEffect(() => {
    if (historyQuery.data?.records && chatHistory.length === 0) {
      setChatHistory([...historyQuery.data.records].reverse());
    }
  }, [historyQuery.data, chatHistory.length]);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chatHistory]);

  useEffect(() => () => abortControllerRef.current?.abort(), []);

  useEffect(() => {
    if (!isGenerating) {
      setElapsedSeconds(0);
      return;
    }
    const startedAt = Date.now();
    const timer = window.setInterval(() => {
      setElapsedSeconds(Math.floor((Date.now() - startedAt) / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [isGenerating]);

  const addAssistantStatus = (id: string, status: string) => {
    setChatHistory((previous) => {
      const existing = previous.find((item) => item.id === id);
      if (existing) {
        return previous.map((item) => item.id === id ? { ...item, message: status } : item);
      }
      return [...previous, {
        id,
        message: status,
        messageType: 'ai',
        appId: appId ?? '',
        userId: '',
        turnNumber: previous.length + 1,
        createTime: new Date().toISOString(),
        updateTime: new Date().toISOString(),
        isDelete: 0,
      }];
    });
  };

  const handleSendMessage = async () => {
    const trimmedMessage = message.trim();
    if (!trimmedMessage || !appId || isGenerating) return;

    setChatHistory((previous) => [...previous, {
      id: Date.now().toString(),
      message: trimmedMessage,
      messageType: 'user',
      appId,
      userId: '',
      turnNumber: previous.length + 1,
      createTime: new Date().toISOString(),
      updateTime: new Date().toISOString(),
      isDelete: 0,
    }]);
    setMessage('');
    setIsGenerating(true);
    setRunningStep(null);
    setCompletedStep(null);
    setDescription('');
    setDescriptionStreaming(false);
    setToolEvents([]);
    setToolEventsExpanded(false);
    setGeneratedFiles({});
    setSelectedFile('');

    const controller = new AbortController();
    abortControllerRef.current = controller;
    const assistantId = `${Date.now()}-assistant`;

    try {
      const baseUrl = import.meta.env.PROD
        ? import.meta.env.VITE_API_BASE_URL
        : window.location.origin;
      const response = await fetch(new URL('/app/graph', baseUrl), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ appId, message: trimmedMessage }),
        signal: controller.signal,
      });
      if (!response.ok || !response.body) {
        throw new Error(`浠ｇ爜鐢熸垚璇锋眰澶辫触 (${response.status})`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let finished = false;

      while (!finished) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split(/\r?\n\r?\n/);
        buffer = events.pop() ?? '';

        for (const eventText of events) {
          const lines = eventText.split(/\r?\n/);
          const eventName = lines.find((line) => line.startsWith('event:'))?.slice(6).trim() ?? 'message';
          const data = lines
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).replace(/^ /, ''))
            .join('\n');
          if (eventName === 'step_started') {
            const step = JSON.parse(data) as WorkflowStep;
            if (step.currentStep) {
              setRunningStep(step);
              addAssistantStatus(assistantId, `正在${step.currentStep}`);
            }
          } else if (eventName === 'step_completed') {
            const step = JSON.parse(data) as WorkflowStep;
            if (step.currentStep) {
              setCompletedStep(step);
              if (step.nextStep) {
                setRunningStep({ ...step, currentStep: step.nextStep });
                addAssistantStatus(assistantId, `正在${step.nextStep}`);
              }
            }
          } else if (eventName === 'tool_request' || eventName === 'tool_executed') {            const toolEvent = JSON.parse(data) as ToolEvent;
            setToolEvents((previous) => {
              const next = [...previous];
              const index = next.findIndex(
                (item) => item.type === 'tool_request' && item.name === toolEvent.name,
              );
              if (eventName === 'tool_executed' && index >= 0) {
                next[index] = toolEvent;
                return next;
              }
              return [...next, toolEvent];
            });
            addAssistantStatus(
              assistantId,
              eventName === 'tool_executed'
                ? `宸ュ叿 ${toolEvent.name} 鎵ц瀹屾垚`
                : `姝ｅ湪璋冪敤宸ュ叿 ${toolEvent.name}`,
            );
          } else if (eventName === 'code_completed') {
            const files = JSON.parse(data) as GeneratedFiles;
            const fileNames = Object.keys(files);
            const firstFile = fileNames[0] ?? '';
            setGeneratedFiles(files);
            setSelectedFile(firstFile);
            setGeneratedCode(firstFile ? files[firstFile] : '');
            addAssistantStatus(assistantId, `代码生成完成，共 ${Object.keys(files).length} 个文件`);
          } else if (eventName === 'description_completed') {
            setDescriptionStreaming(true);
            setDescription('');
            for (let index = 0; index <= data.length; index += 1) {
              await new Promise((resolve) => window.setTimeout(resolve, 12));
              setDescription(data.slice(0, index));
            }
            setDescriptionStreaming(false);
          } else if (eventName === 'error') {
            const errorMessage = data || '代码生成失败';
            if (errorMessage.includes('lock') || errorMessage.includes('锁')) {
              throw new Error('当前应用正在生成中，请等待当前任务完成后再发送');
            }
            throw new Error(errorMessage);
          } else if (eventName === 'done') {
            finished = true;
            break;
          }
        }
      }
      addAssistantStatus(assistantId, '代码生成完成');
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        toast({
          variant: 'destructive',
          title: error instanceof Error && error.message.includes('正在生成中')
            ? '应用正在生成中'
            : '代码生成失败',
          description: error instanceof Error ? error.message : '请稍后重试',
        });
      }
    } finally {
      abortControllerRef.current = null;
      setIsGenerating(false);
    }
  };

  const queryError = appQuery.error || historyQuery.error;
  if (queryError) {
    return (
      <main className="h-screen grid place-items-center bg-background p-6">
        <section className="max-w-md space-y-4 text-center" role="alert">
          <h1 className="text-xl font-semibold">聊天页面加载失败</h1>
          <p className="text-sm text-muted-foreground">
            {queryError instanceof Error ? queryError.message : '请稍后重试'}
          </p>
          <Button onClick={() => void Promise.all([appQuery.refetch(), historyQuery.refetch()])}>
            重新加载
          </Button>
        </section>
      </main>
    );
  }

  if (appQuery.isLoading || historyQuery.isLoading) {
    return <main className="h-screen grid place-items-center bg-background">姝ｅ湪鍔犺浇鑱婂ぉ鍐呭...</main>;
  }

  return (
    <div className="h-screen flex flex-col bg-background">
      <header className="border-b px-4 py-3 flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/apps')}>
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <h1 className="text-lg font-semibold">{appQuery.data?.appName || '未命名应用'}</h1>
        {completedStep && <span className="text-sm text-muted-foreground">已完成：{completedStep.currentStep}</span>}
      </header>
      <div className="flex-1 flex overflow-hidden">
        <div className="min-w-0 flex flex-col border-r" style={{ width: `${chatPaneWidth}%` }}>
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {isGenerating && (
              <Card className="relative overflow-hidden border-primary/30 bg-primary/5 p-4">
                <div className="absolute inset-x-0 top-0 h-1 overflow-hidden bg-primary/10">
                  <div
                    className="h-full animate-pulse bg-primary transition-all duration-700"
                    style={{ width: `${Math.min(92, 18 + (completedStep?.stepNumber ?? 0) * 18)}%` }}
                  />
                </div>
                <div className="flex items-center justify-between gap-2 text-sm font-medium">
                  <div className="flex items-center gap-2">
                    <span className="h-2.5 w-2.5 animate-ping rounded-full bg-primary" />
                    <span>{runningStep ? `正在${runningStep.currentStep}` : '正在准备生成'}</span>
                    <span className="inline-flex w-7 text-left">
                      <span className="animate-pulse">...</span>
                    </span>
                  </div>
                  <span className="font-mono text-xs text-muted-foreground">{elapsedSeconds}s</span>
                </div>
                {(runningStep || completedStep) && (
                  <div className="mt-2 text-xs text-muted-foreground">
                    工作流步骤 {runningStep?.stepNumber ?? completedStep?.stepNumber}
                  </div>
                )}
              </Card>
            )}
            {toolEvents.length > 0 && (
              <Card className="border-primary/20 bg-muted/40 p-3 animate-in fade-in slide-in-from-bottom-2 duration-300">
                <button
                  type="button"
                  className="flex w-full items-center justify-between gap-2 text-left text-sm font-medium"
                  onClick={() => setToolEventsExpanded((expanded) => !expanded)}
                  aria-expanded={toolEventsExpanded}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Wrench className="h-4 w-4 shrink-0 text-primary" />
                    <span className="truncate">
                      工具调用过程（{toolEvents.length}）
                    </span>
                    {isGenerating && <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-primary" />}
                  </span>
                  <ChevronDown className={`h-4 w-4 shrink-0 transition-transform ${toolEventsExpanded ? 'rotate-180' : ''}`} />
                </button>
                {toolEventsExpanded && (
                  <div className="mt-3 max-h-52 space-y-2 overflow-y-auto border-t pt-3">
                    {toolEvents.map((toolEvent, index) => (
                      <div key={`${toolEvent.name}-${index}`} className="flex items-start gap-2 text-xs">
                        {toolEvent.type === 'tool_executed'
                          ? <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-green-600" />
                          : <Loader2 className="mt-0.5 h-3.5 w-3.5 animate-spin text-primary" />}
                        <div className="min-w-0">
                          <div className="font-medium">{toolEvent.name}</div>
                          {toolEvent.type === 'tool_executed' && toolEvent.result && (
                            <pre className="mt-1 max-h-20 overflow-auto whitespace-pre-wrap text-muted-foreground">
                              {toolEvent.result}
                            </pre>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </Card>
            )}
            {chatHistory.map((item) => (
              <div key={item.id} className={`flex ${item.messageType === 'user' ? 'justify-end' : 'justify-start'}`}>
                <Card className={`max-w-[80%] p-3 ${item.messageType === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted'}`}>
                  <pre className="whitespace-pre-wrap text-sm font-sans">{item.message}</pre>
                </Card>
              </div>
            ))}
            {(description || descriptionStreaming) && (
              <Card className="bg-muted p-4">
                <div className="mb-2 text-xs font-medium text-muted-foreground">
                  生成说明
                </div>
                <pre className="whitespace-pre-wrap text-sm font-sans">{description}</pre>
                {descriptionStreaming && <span className="ml-1 animate-pulse">▍</span>}
              </Card>
            )}
            <div ref={chatEndRef} />
          </div>
          <div className="border-t p-4">
            <div className="flex gap-2">
              <Input value={message} placeholder="输入消息..." disabled={isGenerating}
                onChange={(event) => setMessage(event.target.value)}
                onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void handleSendMessage(); } }} />
              <Button onClick={() => void handleSendMessage()} disabled={isGenerating || !message.trim()}>
                <Send className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
        <div
          className="w-1 shrink-0 cursor-col-resize bg-border transition-colors hover:bg-primary/60"
          onPointerDown={startChatPaneResize}
          role="separator"
          aria-label="调整聊天区和预览区宽度"
        />
        <div className="min-w-0 flex flex-1 flex-col">
          <div className="flex items-center gap-1 border-b px-3 py-2">
            <Button
              variant={activePanel === 'preview' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setActivePanel('preview')}
            >
              预览
            </Button>
            <Button
              variant={activePanel === 'source' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setActivePanel('source')}
            >
              源码
            </Button>
          </div>
          <div className="flex-1 overflow-hidden">
            {activePanel === 'source' ? (
              <div className="flex h-full min-h-0">
                <aside
                  className="relative shrink-0 overflow-y-auto border-r bg-muted/30 p-2"
                  style={{ width: `${fileListWidth}px` }}
                >
                  <div className="mb-2 px-2 text-xs font-medium text-muted-foreground">
                    生成文件
                  </div>
                  {Object.keys(generatedFiles).map((fileName) => (
                    <button
                      key={fileName}
                      type="button"
                      className={`mb-1 w-full truncate rounded px-2 py-1.5 text-left text-sm ${
                        selectedFile === fileName
                          ? 'bg-primary text-primary-foreground'
                          : 'hover:bg-muted'
                      }`}
                      onClick={() => {
                        setSelectedFile(fileName);
                        setGeneratedCode(generatedFiles[fileName]);
                      }}
                    >
                      {fileName}
                    </button>
                  ))}
                  {Object.keys(generatedFiles).length === 0 && (
                    <div className="px-2 text-xs text-muted-foreground">
                      暂无生成文件
                    </div>
                  )}
                </aside>
                <div
                  className="w-1 shrink-0 cursor-col-resize bg-border transition-colors hover:bg-primary/60"
                  onPointerDown={startFileListResize}
                  role="separator"
                  aria-label="调整文件列表和源码编辑器宽度"
                />
                <div className="min-w-0 flex-1">
                  <Editor height="100%" defaultLanguage="html" value={generatedCode} theme="vs-light"
                    options={{ readOnly: true, minimap: { enabled: false }, fontSize: 14, lineNumbers: 'on', scrollBeyondLastLine: false }} />
                </div>
              </div>
            ) : Object.keys(generatedFiles).length > 0 ? (
              generatedFiles['package.json'] ? (
                <SandpackProvider
                  template="vite-vue"
                  files={sandpackFiles}
                  className="h-full"
                  customSetup={{
                    entry: '/src/main.js',
                    dependencies: {
                      vue: '3.3.4',
                      'vue-router': '4.2.4',
                    },
                    devDependencies: {
                      vite: '4.1.4',
                      '@vitejs/plugin-vue': '3.2.0',
                      'esbuild-wasm': '0.17.12',
                    },
                  }}
                >
                  <SandpackPreview style={{ height: '100%' }} showOpenInCodeSandbox={false} />
                </SandpackProvider>
              ) : (
                <iframe
                  className="h-full w-full border-0 bg-white"
                  title="Preview"
                  sandbox="allow-scripts"
                  srcDoc={generatedFiles['index.html'] ?? ''}
                />
              )
            ) : (
              <div className="grid h-full place-items-center text-sm text-muted-foreground">
                生成完成后将在这里显示预览
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
