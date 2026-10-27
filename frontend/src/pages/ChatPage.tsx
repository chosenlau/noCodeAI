import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderOpen,
  Loader2,
  Info,
  Send,
  Wrench,
} from 'lucide-react';
import Editor from '@monaco-editor/react';
import { SandpackPreview, SandpackProvider } from '@codesandbox/sandpack-react';
import { appApi, chatApi } from '@/api';
import type { GraphRun } from '@/api/chat.api';
import logoUrl from '@/assets/NoCodeAI.svg';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { toast } from '@/hooks/use-toast';
import type { ChatHistoryVo } from '@/types/api';

type GeneratedFiles = Record<string, string>;

interface FileTreeNode {
  name: string;
  path: string;
  type: 'file' | 'folder';
  children: FileTreeNode[];
}

interface MutableFileTreeNode extends FileTreeNode {
  childMap?: Map<string, MutableFileTreeNode>;
  children: MutableFileTreeNode[];
}

interface WorkflowStep {
  stepNumber: number;
  currentStep: string;
  nextStep?: string;
}

interface ToolEvent {
	 id?: string;
	 index?: number;
  type: 'tool_request' | 'tool_executed';
  name: string;
  arguments?: string;
  result?: string;
}

interface WorkflowTask {
  runningStep: WorkflowStep | null;
  completedStep: WorkflowStep | null;
  toolEvents: ToolEvent[];
  description: string;
  descriptionStreaming: boolean;
  status: string;
  isGenerating: boolean;
  elapsedSeconds: number;
}

function normalizeGeneratedFiles(value: unknown): GeneratedFiles {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }

  const files: GeneratedFiles = {};
  for (const [fileName, content] of Object.entries(value)) {
    if (typeof content === 'string') {
      files[fileName] = content;
    }
  }
  return files;
}

function buildFileTree(files: GeneratedFiles): FileTreeNode[] {
  const root: MutableFileTreeNode = {
    name: '',
    path: '',
    type: 'folder',
    children: [],
    childMap: new Map(),
  };

  for (const filePath of Object.keys(files).sort((a, b) => a.localeCompare(b))) {
    const parts = filePath.replace(/\\/g, '/').split('/').filter(Boolean);
    let current = root;
    const pathParts: string[] = [];

    parts.forEach((part, index) => {
      const isFile = index === parts.length - 1;
      pathParts.push(part);
      const path = pathParts.join('/');

      if (isFile) {
        current.children.push({
          name: part,
          path: filePath,
          type: 'file',
          children: [],
        });
        return;
      }

      let folder = current.childMap?.get(part);
      if (!folder) {
        folder = {
          name: part,
          path,
          type: 'folder',
          children: [],
          childMap: new Map(),
        };
        current.childMap?.set(part, folder);
        current.children.push(folder);
      }
      current = folder;
    });
  }

  const sortTree = (nodes: MutableFileTreeNode[]): FileTreeNode[] =>
    nodes
      .sort((left, right) => {
        if (left.type !== right.type) {
          return left.type === 'folder' ? -1 : 1;
        }
        return left.name.localeCompare(right.name, undefined, {
          numeric: true,
          sensitivity: 'base',
        });
      })
      .map(({ childMap: _childMap, ...node }) => ({
        ...node,
        children: sortTree(node.children),
      }));

  return sortTree(root.children);
}

function collectFolderPaths(nodes: FileTreeNode[]): string[] {
  return nodes.flatMap((node) => {
    if (node.type !== 'folder') {
      return [];
    }
    return [node.path, ...collectFolderPaths(node.children)];
  });
}

function buildStaticPreview(files: GeneratedFiles): string {
  let html = files['index.html'] ?? '';
  const css = files['style.css'] ?? '';
  const js = files['script.js'] ?? '';

  if (css) {
    const styleTag = `<style>${css}</style>`;
    html = /<\/head>/i.test(html)
      ? html.replace(/<\/head>/i, `${styleTag}</head>`)
      : `${styleTag}${html}`;
  }

  if (js) {
    const scriptTag = `<script>${js.replace(/<\/script/gi, '<\\/script')}</script>`;
    html = /<\/body>/i.test(html)
      ? html.replace(/<\/body>/i, `${scriptTag}</body>`)
      : `${html}${scriptTag}`;
  }

  return html;
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

interface WorkflowTaskCardProps {
  task: WorkflowTask;
  elapsedSeconds: number;
  expanded: boolean;
  onToggleTools: () => void;
}

interface FileTreeProps {
  nodes: FileTreeNode[];
  selectedFile: string;
  expandedFolders: Set<string>;
  onToggleFolder: (path: string) => void;
  onSelectFile: (path: string) => void;
  depth?: number;
}

function FileTree({
  nodes,
  selectedFile,
  expandedFolders,
  onToggleFolder,
  onSelectFile,
  depth = 0,
}: FileTreeProps) {
  return (
    <div className={depth === 0 ? 'space-y-0.5' : undefined}>
      {nodes.map((node) => {
        const paddingLeft = 6 + depth * 14;

        if (node.type === 'folder') {
          const expanded = expandedFolders.has(node.path);
          return (
            <div key={node.path}>
              <button
                type="button"
                className="flex h-7 w-full min-w-0 items-center gap-1 rounded px-1.5 text-left text-sm hover:bg-muted"
                style={{ paddingLeft }}
                onClick={() => onToggleFolder(node.path)}
                aria-expanded={expanded}
                title={node.path}
              >
                <ChevronRight
                  className={`h-3.5 w-3.5 shrink-0 transition-transform ${
                    expanded ? 'rotate-90' : ''
                  }`}
                />
                {expanded ? (
                  <FolderOpen className="h-3.5 w-3.5 shrink-0 text-primary" />
                ) : (
                  <Folder className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                )}
                <span className="min-w-0 truncate">{node.name}</span>
              </button>
              {expanded && (
                <FileTree
                  nodes={node.children}
                  selectedFile={selectedFile}
                  expandedFolders={expandedFolders}
                  onToggleFolder={onToggleFolder}
                  onSelectFile={onSelectFile}
                  depth={depth + 1}
                />
              )}
            </div>
          );
        }

        return (
          <button
            key={node.path}
            type="button"
            className={`flex h-7 w-full min-w-0 items-center gap-1.5 rounded px-1.5 text-left text-sm ${
              selectedFile === node.path
                ? 'bg-primary text-primary-foreground'
                : 'hover:bg-muted'
            }`}
            style={{ paddingLeft: paddingLeft + 18 }}
            onClick={() => onSelectFile(node.path)}
            title={node.path}
          >
            <FileText className="h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 truncate">{node.name}</span>
          </button>
        );
      })}
    </div>
  );
}

function WorkflowTaskCard({
  task,
  elapsedSeconds,
  expanded,
  onToggleTools,
}: WorkflowTaskCardProps) {
  const currentStep = task.runningStep?.currentStep;
  const completedStep = task.completedStep;
  const displayStatus = task.isGenerating
    ? currentStep
      ? `正在${currentStep}`
      : '正在准备生成'
    : task.status;
  const displayElapsed = task.isGenerating ? elapsedSeconds : task.elapsedSeconds;

  return (
    <Card className="relative overflow-hidden border-primary/30 bg-primary/5 p-4">
      <div className="absolute inset-x-0 top-0 h-1 overflow-hidden bg-primary/10">
        <div
          className={`h-full bg-primary transition-all duration-700 ${
            task.isGenerating ? 'animate-pulse' : ''
          }`}
          style={{
            width: `${task.isGenerating
              ? Math.min(92, 18 + (completedStep?.stepNumber ?? 0) * 18)
              : 100}%`,
          }}
        />
      </div>
      <div className="flex items-center justify-between gap-2 text-sm font-medium">
        <div className="flex min-w-0 items-center gap-2">
          {task.isGenerating ? (
            <span className="h-2.5 w-2.5 shrink-0 animate-ping rounded-full bg-primary" />
          ) : (
            <CheckCircle2 className="h-4 w-4 shrink-0 text-green-600" />
          )}
          <span className="truncate">{displayStatus}</span>
          {task.isGenerating && (
            <span className="inline-flex w-7 text-left">
              <span className="animate-pulse">...</span>
            </span>
          )}
        </div>
        <span className="font-mono text-xs text-muted-foreground">
          {displayElapsed}s
        </span>
      </div>
      {(task.runningStep || completedStep) && (
        <div className="mt-2 text-xs text-muted-foreground">
          工作流步骤 {task.runningStep?.stepNumber ?? completedStep?.stepNumber}
          {completedStep && task.isGenerating && ` · 已完成：${completedStep.currentStep}`}
        </div>
      )}
      {task.toolEvents.length > 0 && (
        <div className="mt-3 border-t border-primary/10 pt-3">
          <button
            type="button"
            className="flex w-full items-center justify-between gap-2 text-left text-xs font-medium"
            onClick={onToggleTools}
            aria-expanded={expanded}
          >
            <span className="flex min-w-0 items-center gap-2">
              <Wrench className="h-3.5 w-3.5 shrink-0 text-primary" />
              <span className="truncate">工具调用过程（{task.toolEvents.length}）</span>
              {task.isGenerating && (
                <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-primary" />
              )}
            </span>
            <ChevronDown
              className={`h-4 w-4 shrink-0 transition-transform ${
                expanded ? 'rotate-180' : ''
              }`}
            />
          </button>
          {expanded && (
            <div className="mt-3 max-h-52 space-y-2 overflow-y-auto">
              {task.toolEvents.map((toolEvent, index) => (
                <div
                  key={`${toolEvent.name}-${index}`}
                  className="flex items-start gap-2 text-xs"
                >
                  {toolEvent.type === 'tool_executed' ? (
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-green-600" />
                  ) : (
                    <Loader2 className="mt-0.5 h-3.5 w-3.5 animate-spin text-primary" />
                  )}
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
        </div>
      )}
      {task.description && (
        <div className="mt-3 border-t border-primary/10 pt-3">
          <div className="mb-2 text-xs font-medium text-muted-foreground">
            生成说明
          </div>
          <pre className="whitespace-pre-wrap text-sm font-sans">{task.description}</pre>
          {task.descriptionStreaming && <span className="ml-1 animate-pulse">▍</span>}
        </div>
      )}
    </Card>
  );
}

export default function ChatPage() {
  const { appId } = useParams<{ appId: string }>();
  return <ChatSession key={appId} />;
}

function ChatSession() {
  const { appId } = useParams<{ appId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const [message, setMessage] = useState('');
  const [chatHistory, setChatHistory] = useState<ChatHistoryVo[]>([]);
  const [generatedCode, setGeneratedCode] = useState('');
  const [generatedFiles, setGeneratedFiles] = useState<GeneratedFiles>({});
  const [selectedFile, setSelectedFile] = useState('');
  const [workflowTasks, setWorkflowTasks] = useState<Record<string, WorkflowTask>>({});
  const [thinkingMessageIds, setThinkingMessageIds] = useState<Set<string>>(new Set());
  const [expandedTaskIds, setExpandedTaskIds] = useState<Set<string>>(new Set());
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
  const [activeTaskId, setActiveTaskId] = useState('');
  const [activePanel, setActivePanel] = useState<'source' | 'preview'>('preview');
  const [isGenerating, setIsGenerating] = useState(false);
  const [useGraph, setUseGraph] = useState(false);
  const [controlledRun, setControlledRun] = useState<GraphRun | null>(null);
  const [controlBusy, setControlBusy] = useState(false);
  const [showVersions, setShowVersions] = useState(false);
  const [restoreBusy, setRestoreBusy] = useState(false);
  const [runDiscoveryDone, setRunDiscoveryDone] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [chatPaneWidth, setChatPaneWidth] = useState(50);
  const [fileListWidth, setFileListWidth] = useState(220);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const generationStartedRef = useRef(false);
  const mountedRef = useRef(false);
  const initialPrompt = (
    location.state as { initialPrompt?: string } | null
  )?.initialPrompt;
  const sandpackFiles = useMemo(() => getSandpackFiles(generatedFiles), [generatedFiles]);
  const fileTree = useMemo(() => buildFileTree(generatedFiles), [generatedFiles]);
  const activeTask = activeTaskId ? workflowTasks[activeTaskId] : undefined;

  // Poll only the persisted task currently observed by this page. This also
  // discovers the safe-point status after the SSE subscription has closed.
  useEffect(() => {
    if (!appId || !useGraph) return;
    let canceled = false;
    const refresh = async () => {
      try {
        const stored = sessionStorage.getItem(`graph-run:${appId}`);
        const id = stored ? (JSON.parse(stored) as { runId?: string }).runId : undefined;
        if (id) { const run = await chatApi.getGraphRun(id); if (!canceled) setControlledRun(run); }
      } catch { /* SSE reports connection errors; retain the last known state. */ }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 1500);
    return () => { canceled = true; window.clearInterval(timer); };
  }, [appId, useGraph]);

  const updateWorkflowTask = (
    taskId: string,
    patch: Partial<WorkflowTask>,
  ) => {
    setWorkflowTasks((previous) => ({
      ...previous,
      [taskId]: {
        ...previous[taskId],
        ...patch,
      },
    }));
  };

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

  const handleToggleFolder = (path: string) => {
    setExpandedFolders((previous) => {
      const next = new Set(previous);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  const handleSelectFile = (fileName: string) => {
    setSelectedFile(fileName);
    setGeneratedCode(generatedFiles[fileName]);
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
    queryFn: () => appApi.getSourceCode(appId!, appQuery.data?.codeGenType ?? ''),
    enabled: activePanel === 'source' && !!appId && !!appQuery.data,
  });

  const versionsQuery = useQuery({
    queryKey: ['sourceVersions', appId],
    queryFn: () => appApi.listVersions(appId!),
    enabled: showVersions && !!appId,
    refetchInterval: showVersions && isGenerating ? 3000 : false,
  });
  const hasActiveRun = !!controlledRun && !['completed', 'failed', 'interrupted', 'canceled'].includes(controlledRun.status);
  const restoreVersion = async (versionId: string) => {
    if (!appId || !versionsQuery.data || restoreBusy || isGenerating || hasActiveRun || !runDiscoveryDone) return;
    setRestoreBusy(true);
    try {
      await appApi.restoreVersion(appId, versionId, versionsQuery.data.currentVersionId);
      const files = normalizeGeneratedFiles(await appApi.getSourceCode(appId, ''));
      const firstFile = Object.keys(files)[0] ?? '';
      setGeneratedFiles(files);
      setSelectedFile(firstFile);
      setGeneratedCode(firstFile ? files[firstFile] : '');
      setActivePanel('preview');
      await Promise.all([appQuery.refetch(), sourceCodeQuery.refetch(), versionsQuery.refetch()]);
      toast({ title: '已恢复网站', description: '恢复前的版本仍保留在历史记录中。' });
    } catch (error) {
      toast({ variant: 'destructive', title: '未能完成恢复', description: error instanceof Error ? error.message : '请稍后重试' });
      void versionsQuery.refetch();
    } finally { setRestoreBusy(false); }
  };

  useEffect(() => {
    if (showVersions && !isGenerating) void versionsQuery.refetch();
  }, [showVersions, isGenerating, controlledRun?.status]);

  useEffect(() => {
    if (activePanel === 'source' && sourceCodeQuery.data && !activeTaskId) {
      const files = normalizeGeneratedFiles(sourceCodeQuery.data);
      const firstFile = Object.keys(files)[0] ?? '';
      setGeneratedFiles(files);
      setSelectedFile((current) => current && files[current] ? current : firstFile);
      setGeneratedCode((current) => current || (firstFile ? files[firstFile] : ''));
    }
  }, [activePanel, sourceCodeQuery.data, activeTaskId]);

  useEffect(() => {
    setExpandedFolders(new Set(collectFolderPaths(fileTree)));
  }, [fileTree]);

  useEffect(() => {
    if (historyQuery.data?.records && chatHistory.length === 0) {
      setChatHistory([...historyQuery.data.records].reverse());
    }
  }, [historyQuery.data, chatHistory.length]);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chatHistory]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      const controller = abortControllerRef.current;
      window.setTimeout(() => {
        if (
          !mountedRef.current &&
          abortControllerRef.current === controller
        ) {
          controller?.abort();
        }
      }, 0);
    };
  }, []);

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

  const handleSendMessage = async (requestedMessage?: string, resumeRunId?: string) => {
    const trimmedMessage = (requestedMessage ?? message).trim();
    const isResume = Boolean(resumeRunId);
    if ((!trimmedMessage && !isResume) || !appId || isGenerating || restoreBusy) return;

    const taskId = resumeRunId
      ? `workflow-run-${resumeRunId}`
      : `workflow-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const isGraphRequest = isResume || useGraph;
    if (!isResume) setChatHistory((previous) => [...previous, {
      id: Date.now().toString(),
      message: trimmedMessage,
      messageType: 'user',
      appId,
      userId: '',
      turnNumber: previous.length + 1,
      createTime: new Date().toISOString(),
      updateTime: new Date().toISOString(),
      isDelete: 0,
    }, {
      id: taskId,
      message: '',
      messageType: 'ai',
      appId,
      userId: '',
      turnNumber: previous.length + 2,
      createTime: new Date().toISOString(),
      updateTime: new Date().toISOString(),
      isDelete: 0,
    }]);
    if (isResume) setChatHistory((previous) => previous.some((item) => item.id === taskId) ? previous : [...previous, {
      id: taskId, message: '', messageType: 'ai', appId, userId: '',
      turnNumber: previous.length + 1, createTime: new Date().toISOString(),
      updateTime: new Date().toISOString(), isDelete: 0,
    }]);
    if (isGraphRequest) {
      setWorkflowTasks((previous) => ({
        ...previous,
        [taskId]: {
          runningStep: null,
          completedStep: null,
          toolEvents: [],
          description: '',
          descriptionStreaming: false,
          status: '正在准备生成',
          isGenerating: true,
          elapsedSeconds: 0,
        },
      }));
    } else {
      setThinkingMessageIds((previous) => new Set(previous).add(taskId));
    }
    setActiveTaskId(taskId);
    setMessage('');
    setIsGenerating(true);
    setGeneratedFiles({});
    setSelectedFile('');

    const controller = new AbortController();
    abortControllerRef.current = controller;
    let streamReader: ReadableStreamDefaultReader<Uint8Array> | undefined;

    try {
      const response = isGraphRequest
        ? resumeRunId
          ? await chatApi.observeGraphRun(appId, resumeRunId, controller.signal)
          : await chatApi.generateCode(appId, trimmedMessage, controller.signal)
        : await chatApi.chatWithAgent(appId, trimmedMessage, controller.signal);
      if (!response.ok || !response.body) {
        throw new Error(`请求失败 (${response.status})`);
      }
      const contentType = response.headers.get('content-type') ?? '';
      if (!contentType.includes('text/event-stream')) {
        const body = await response.text();
        try {
          const payload = JSON.parse(body) as { message?: string };
          throw new Error(payload.message || '服务端返回了非 SSE 响应');
        } catch (error) {
          if (error instanceof Error && error.message !== '服务端返回了非 SSE 响应') {
            throw error;
          }
          throw new Error(body || '服务端返回了非 SSE 响应');
        }
      }

      const reader = response.body.getReader();
      streamReader = reader;
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
          if (!isGraphRequest && eventName === 'message') {
            const assistantText = data;
            setThinkingMessageIds((previous) => {
              const next = new Set(previous);
              next.delete(taskId);
              return next;
            });
            setChatHistory((previous) => {
              const last = previous[previous.length - 1];
              if (!last || last.id !== taskId) return previous;
              return [...previous.slice(0, -1), { ...last, message: `${last.message}${assistantText}` }];
            });
          } else if (eventName === 'run_canceled' || eventName === 'run_control') {
            const state = JSON.parse(data) as { status?: string };
            if (state.status === 'canceled') {
              updateWorkflowTask(taskId, { status: '任务已终止，草稿未发布', isGenerating: false });
              return;
            }
          } else if (eventName === 'connection_status') {
            updateWorkflowTask(taskId, { status: data });
          } else if (eventName === 'context_compacting' || eventName === 'context_ready') {
            updateWorkflowTask(taskId, { status: eventName === 'context_compacting'
              ? '正在整理任务信息，随后继续' : '任务信息整理结束' });
          } else if (eventName === 'step_started') {
            const step = JSON.parse(data) as WorkflowStep;
            if (step.currentStep) {
              updateWorkflowTask(taskId, {
                runningStep: step,
                status: `正在${step.currentStep}`,
              });
            }
          } else if (eventName === 'step_completed') {
            const step = JSON.parse(data) as WorkflowStep;
            if (step.currentStep) {
              const patch: Partial<WorkflowTask> = { completedStep: step };
              if (step.nextStep) {
                patch.runningStep = { ...step, currentStep: step.nextStep };
                patch.status = `正在${step.nextStep}`;
              }
              updateWorkflowTask(taskId, patch);
            }
          } else if (eventName === 'tool_request' || eventName === 'tool_executed') {
            const toolEvent = JSON.parse(data) as ToolEvent;
            setWorkflowTasks((previous) => {
              const task = previous[taskId];
              if (!task) return previous;
              const nextToolEvents = [...task.toolEvents];
              const index = nextToolEvents.findIndex((item) =>
                toolEvent.id ? item.id === toolEvent.id : item.type === 'tool_request' && item.name === toolEvent.name,
              );
              if (eventName === 'tool_executed' && index >= 0) {
                nextToolEvents[index] = toolEvent;
              } else {
                nextToolEvents.push(toolEvent);
              }
              return {
                ...previous,
                [taskId]: {
                  ...task,
                  toolEvents: nextToolEvents,
                  status: eventName === 'tool_executed'
                    ? `工具 ${toolEvent.name} 执行完成`
                    : `正在调用工具 ${toolEvent.name}`,
                },
              };
            });
          } else if (eventName === 'code_completed') {
            const files = normalizeGeneratedFiles(JSON.parse(data) as unknown);
            const fileNames = Object.keys(files);
            const firstFile = fileNames[0] ?? '';
            setGeneratedFiles(files);
            setSelectedFile(firstFile);
            setGeneratedCode(firstFile ? files[firstFile] : '');
            updateWorkflowTask(taskId, {
              status: `代码文件已生成，共 ${Object.keys(files).length} 个`,
            });
          } else if (eventName === 'description_completed') {
            updateWorkflowTask(taskId, {
              descriptionStreaming: true,
              description: '',
            });
            updateWorkflowTask(taskId, { description: data });
            window.setTimeout(() => updateWorkflowTask(taskId, { descriptionStreaming: false }), 0);
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
      // Some browsers report stream completion immediately after the final
      // SSE frame. ChatAgent has no payload after the assistant message, so
      // the closed response is a valid completion for this mode.
      if (!finished && !isGraphRequest) {
        finished = true;
      }
      if (!finished && isGraphRequest) {
        const stored = sessionStorage.getItem(`graph-run:${appId}`);
        const id = stored ? (JSON.parse(stored) as { runId?: string }).runId : undefined;
        const run = id ? await chatApi.getGraphRun(id) : undefined;
        if (run) setControlledRun(run);
        if (run && ['paused', 'awaiting_approval', 'recoverable'].includes(run.status)) {
          updateWorkflowTask(taskId, { status: run.status === 'awaiting_approval' ? '等待确认计划' : '已保存进度，等待继续', isGenerating: false });
          return;
        }
        throw new Error('生成连接提前结束，未收到完成信号');
      }
      if (isGraphRequest) {
        updateWorkflowTask(taskId, {
          status: '代码生成流程完成',
          isGenerating: false,
          elapsedSeconds,
        });
      }
      setThinkingMessageIds((previous) => {
        const next = new Set(previous);
        next.delete(taskId);
        return next;
      });
    } catch (error) {
      if (isGraphRequest) updateWorkflowTask(taskId, {
        status: error instanceof Error ? error.message : '无法获取任务进度',
      });
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
      await streamReader?.cancel().catch(() => undefined);
      controller.abort();
      abortControllerRef.current = null;
      setIsGenerating(false);
      void appQuery.refetch();
      if (isGraphRequest) {
        updateWorkflowTask(taskId, {
          isGenerating: false,
          elapsedSeconds,
        });
      }
      setThinkingMessageIds((previous) => {
        const next = new Set(previous);
        next.delete(taskId);
        return next;
      });
    }
  };

  const controlRun = async (action: 'approve' | 'reject' | 'pause' | 'resume' | 'cancel') => {
    if (!controlledRun || controlBusy) return;
    setControlBusy(true);
    try {
      const run = await chatApi.controlGraphRun(controlledRun.id, action, controlledRun.checkpoint?.planHash);
      setControlledRun(run);
      if ((action === 'approve' || action === 'resume') && !isGenerating) {
        void handleSendMessage(undefined, run.id);
      }
    } catch (error) {
      toast({ variant: 'destructive', title: '任务操作失败', description: error instanceof Error ? error.message : '请稍后重试' });
    } finally { setControlBusy(false); }
  };

  useEffect(() => {
    if (!appId || isGenerating || !historyQuery.isSuccess || generationStartedRef.current) return;
    let canceled = false;
    const restoreActiveRun = async () => {
      try {
        let run;
        try {
          run = await chatApi.getActiveGraphRun(appId);
        } catch (error) {
          if (!(error instanceof Error) || !/task run not found|disabled/.test(error.message)) throw error;
        }
        // A lost create response can be recovered using the original request ID.
        const pending = sessionStorage.getItem(`graph-request:${appId}`);
        if (!run && pending) {
          const request = JSON.parse(pending) as { message: string; requestId: string };
          const result = await chatApi.createGraphRun(appId, request.message, request.requestId);
          run = result.run;
          sessionStorage.removeItem(`graph-request:${appId}`);
        }
        const stored = sessionStorage.getItem(`graph-run:${appId}`);
        const storedRunId = stored
          ? (JSON.parse(stored) as { runId?: string }).runId
          : undefined;
        if (!run && storedRunId) run = await chatApi.getGraphRun(storedRunId);
        if (canceled || !run) return;
        generationStartedRef.current = true;
        setUseGraph(true);
        void handleSendMessage(undefined, run.id);
      } catch (error) {
        if (!canceled && error instanceof Error && !/task run not found|disabled/.test(error.message)) {
          toast({ variant: 'destructive', title: '任务恢复失败', description: error.message });
        }
      } finally {
        if (!canceled) setRunDiscoveryDone(true);
      }
    };
    void restoreActiveRun();
    return () => { canceled = true; };
  }, [appId, historyQuery.isSuccess]);

  useEffect(() => {
    if (
      !initialPrompt?.trim() ||
      !appId ||
      !appQuery.data ||
      !historyQuery.isSuccess ||
      !runDiscoveryDone ||
      generationStartedRef.current
    ) {
      return;
    }

    generationStartedRef.current = true;
    navigate(location.pathname, { replace: true, state: null });
    void handleSendMessage(initialPrompt);
  }, [
    appId,
    appQuery.data,
    handleSendMessage,
    historyQuery.isSuccess,
    runDiscoveryDone,
    initialPrompt,
    location.pathname,
    navigate,
  ]);

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
        <details className="relative ml-auto">
          <summary className="flex cursor-pointer list-none items-center gap-1 rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-muted">
            <Info className="h-4 w-4" />
            信息
          </summary>
          <div className="absolute right-0 top-9 z-20 w-72 rounded-lg border bg-background p-3 text-xs shadow-lg">
            <div className="mb-2 font-medium">Memory</div>
            <div>Summary: {appQuery.data?.memory?.summary || 'None'}</div>
            <div>Round: {appQuery.data?.memory?.round ?? 0}</div>
            <div>Prompt tokens: {appQuery.data?.memory?.promptTokens ?? 0}</div>
            <div>Completion tokens: {appQuery.data?.memory?.completionTokens ?? 0}</div>
            <div>Total tokens: {appQuery.data?.memory?.totalTokens ?? 0}</div>
            <div>Status: {appQuery.data?.memory?.summarizing ? 'Summarizing' : 'Idle'}</div>
            {appQuery.data?.memory?.summaryError && (
              <div className="mt-1 text-destructive">
                Error: {appQuery.data.memory.summaryError}
              </div>
            )}
          </div>
        </details>
        {activeTask?.completedStep && (
          <span className="text-sm text-muted-foreground">
            已完成：{activeTask.completedStep.currentStep}
          </span>
        )}
      </header>
      <div className="flex-1 flex overflow-hidden">
        <div className="min-w-0 flex flex-col border-r" style={{ width: `${chatPaneWidth}%` }}>
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {chatHistory.map((item) => {
              const task = workflowTasks[item.id];
              if (task) {
                return (
                  <div key={item.id} className="flex justify-start">
                    <div className="max-w-[90%] flex-1">
                      <WorkflowTaskCard
                        task={task}
                        elapsedSeconds={elapsedSeconds}
                        expanded={expandedTaskIds.has(item.id)}
                        onToggleTools={() => {
                          setExpandedTaskIds((previous) => {
                            const next = new Set(previous);
                            if (next.has(item.id)) {
                              next.delete(item.id);
                            } else {
                              next.add(item.id);
                            }
                            return next;
                          });
                        }}
                      />
                    </div>
                  </div>
                );
              }
              return (
                <div
                  key={item.id}
                  className={`flex ${
                    item.messageType === 'user' ? 'justify-end' : 'justify-start'
                  }`}
                >
                  <div className="flex max-w-[80%] items-start gap-2">
                    {item.messageType === 'ai' && (
                      <div className="mt-1 flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-full border bg-background">
                        <img
                          src={logoUrl}
                          alt="NoCodeAI"
                          className="h-full w-full object-cover"
                        />
                      </div>
                    )}
                    <div className="flex min-w-0 flex-col items-start gap-1">
                    {item.messageType === 'ai' && thinkingMessageIds.has(item.id) && (
                      <div className="flex items-center gap-1.5 px-1 text-xs text-muted-foreground">
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        <span>正在思考</span>
                      </div>
                    )}
                    <Card
                      className={`p-3 ${
                        item.messageType === 'user'
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-muted'
                      }`}
                    >
                      <pre className="whitespace-pre-wrap text-sm font-sans">
                        {item.message}
                      </pre>
                    </Card>
                    </div>
                  </div>
                </div>
              );
            })}
            <div ref={chatEndRef} />
          </div>
          <div className="border-t p-4">
            {useGraph && controlledRun && !['completed', 'failed', 'interrupted', 'canceled'].includes(controlledRun.status) && (
              <div className="mb-3 space-y-2 rounded-md border p-3 text-sm">
                <p>{({ pending: '任务排队中', running: '任务执行中', pausing: '正在暂停：等待当前节点完成并保存', paused: '任务已暂停', awaiting_approval: '请确认执行计划', cancelling: '正在终止任务', recoverable: '服务重启，进度可恢复' } as Record<string, string>)[controlledRun.status]}</p>
                {controlledRun.checkpoint && <>
                  <p className="whitespace-pre-wrap">目标：{controlledRun.checkpoint.plan.goal}</p>
                  <p>代码类型：{controlledRun.checkpoint.plan.generationType}</p>
                  <ul className="list-inside list-disc">{controlledRun.checkpoint.plan.todos.map(todo => <li key={todo.id}>{todo.status === 'completed' ? '已完成' : '待执行'}：{todo.title}</li>)}</ul>
                  <p>约束：{controlledRun.checkpoint.plan.constraints.join('；')}</p>
                  <p>验收：{controlledRun.checkpoint.plan.acceptance.join('；')}</p>
                </>}
                <div className="flex flex-wrap gap-2">
                  {controlledRun.status === 'awaiting_approval' && <><Button size="sm" disabled={controlBusy || isGenerating} onClick={() => void controlRun('approve')}>批准并执行</Button><Button size="sm" variant="outline" disabled={controlBusy} onClick={() => void controlRun('reject')}>拒绝计划</Button></>}
                  {controlledRun.status === 'running' && <Button size="sm" variant="outline" disabled={controlBusy} onClick={() => void controlRun('pause')}>暂停任务</Button>}
                  {['paused', 'recoverable'].includes(controlledRun.status) && <Button size="sm" disabled={controlBusy || isGenerating} onClick={() => void controlRun('resume')}>继续任务</Button>}
                  <Button size="sm" variant="outline" disabled={controlBusy || controlledRun.status === 'cancelling'} onClick={() => void controlRun('cancel')}>终止任务</Button>
                </div>
              </div>
            )}
            <div className="flex items-center gap-2">
              <label className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={useGraph}
                  disabled={isGenerating || restoreBusy || !runDiscoveryDone}
                  onChange={(event) => setUseGraph(event.target.checked)}
                />
                代码生成
              </label>
              <Input value={message} placeholder="输入消息..." disabled={isGenerating || restoreBusy || !runDiscoveryDone}
                onChange={(event) => setMessage(event.target.value)}
                onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void handleSendMessage(); } }} />
              <Button onClick={() => void handleSendMessage()} disabled={isGenerating || restoreBusy || !runDiscoveryDone || !message.trim()}>
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
            <Button variant={showVersions ? 'secondary' : 'ghost'} size="sm" onClick={() => setShowVersions(value => !value)}>历史版本</Button>
          </div>
          {showVersions && <div className="max-h-60 overflow-y-auto border-b bg-muted/20 p-3 text-sm">
            <p className="mb-2 text-muted-foreground">恢复后会保留所有历史版本。正在处理任务时，请先完成或终止任务。</p>
            {versionsQuery.isLoading && <p>正在加载历史版本…</p>}
            {versionsQuery.isError && <p>历史版本加载失败。<button className="underline" onClick={() => void versionsQuery.refetch()}>重试</button></p>}
            {versionsQuery.data?.versions.length === 0 && <p>完成第一次生成后，这里会保存历史版本。</p>}
            {versionsQuery.data?.versions.map(version => <div key={version.id} className="flex items-center justify-between gap-2 border-t py-2">
              <span>{new Date(version.createdAt).toLocaleString('zh-CN')}{version.restoredFromId !== '0' ? ' · 恢复记录' : ''}</span>
              {version.current ? <span>当前版本</span> : <Button size="sm" variant="outline" disabled={!version.restorable || restoreBusy || isGenerating || hasActiveRun || !runDiscoveryDone} onClick={() => void restoreVersion(version.id)}>{!version.restorable ? '早期版本无法恢复' : restoreBusy ? '正在恢复…' : '恢复到这个版本'}</Button>}
            </div>)}
          </div>}
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
                  <FileTree
                    nodes={fileTree}
                    selectedFile={selectedFile}
                    expandedFolders={expandedFolders}
                    onToggleFolder={handleToggleFolder}
                    onSelectFile={handleSelectFile}
                  />
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
                  srcDoc={buildStaticPreview(generatedFiles)}
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
