import { FormEvent, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  ArrowUp,
  Code2,
  LayoutTemplate,
  LogOut,
  MessageCircle,
  Plus,
  Settings,
  Sparkles,
  UserCircle,
  WandSparkles,
} from 'lucide-react';
import { appApi, userApi } from '@/api';
import { useAuthStore } from '@/store/auth';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { toast } from '@/hooks/use-toast';
import { ApiError } from '@/lib/errors';
import type { AppVo } from '@/types/api';
import logo from '@/assets/NoCodeAI.svg';

const assistantMessages = [
  '你好，我可以把你的想法变成一个可以运行的网页。',
  '试着描述一个页面、一个功能，或者一段用户流程。',
  '例如：做一个有作品集、联系方式和暗色切换的个人主页。',
];

const projectTypeLabels: Record<string, string> = {
  html: 'HTML 页面',
  multi_file: '多文件项目',
  vue_project: 'Vue 3 项目',
};

export default function AppsPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const { user, isAuthenticated, logout: authLogout } = useAuthStore();
  const [prompt, setPrompt] = useState('');
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [initPrompt, setInitPrompt] = useState('');
  const [assistantMessage] = useState(
    () =>
      assistantMessages[
        Math.floor(Math.random() * assistantMessages.length)
      ]
  );
  const consumedPromptRef = useRef('');
  const pendingPrompt = (
    location.state as { pendingPrompt?: string } | null
  )?.pendingPrompt;

  const {
    data: myApps,
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: ['myApps'],
    queryFn: appApi.listMyApps,
    enabled: isAuthenticated,
    retry: false,
  });

  const logoutMutation = useMutation({
    mutationFn: userApi.logout,
    onSuccess: () => {
      authLogout();
      toast({ title: '已退出登录' });
    },
    onError: (mutationError: ApiError) => {
      toast({
        variant: 'destructive',
        title: '退出失败',
        description: mutationError.message,
      });
    },
  });

  const createAppMutation = useMutation({
    mutationFn: appApi.create,
    onSuccess: (appId: string, createdPrompt: string) => {
      setIsCreateDialogOpen(false);
      setInitPrompt('');
      void queryClient.invalidateQueries({ queryKey: ['myApps'] });
      navigate(`/chat/${appId}`, {
        state: { initialPrompt: createdPrompt.trim() },
      });
    },
    onError: (mutationError: ApiError) => {
      toast({
        variant: 'destructive',
        title: '创建失败',
        description: mutationError.message,
      });
    },
  });

  useEffect(() => {
    if (
      !isAuthenticated ||
      !pendingPrompt?.trim() ||
      consumedPromptRef.current === pendingPrompt
    ) {
      return;
    }

    consumedPromptRef.current = pendingPrompt;
    navigate('/', { replace: true, state: null });
    createAppMutation.mutate(pendingPrompt.trim());
  }, [createAppMutation, isAuthenticated, navigate, pendingPrompt]);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!prompt.trim()) {
      return;
    }
    if (!isAuthenticated) {
      navigate('/login', { state: { pendingPrompt: prompt.trim() } });
      return;
    }
    createAppMutation.mutate(prompt.trim());
    setPrompt('');
  };

  const handleCreateApp = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!initPrompt.trim()) {
      return;
    }
    createAppMutation.mutate(initPrompt.trim());
  };

  const handleCardClick = (app: AppVo) => {
    navigate(`/chat/${app.id}`);
  };

  const apps = myApps?.records ?? [];

  return (
    <main className={`home-shell ${isAuthenticated ? 'home-shell-authenticated' : ''}`}>
      <header className="home-header">
        <button
          type="button"
          className="brand-lockup"
          onClick={() => navigate('/')}
          aria-label="返回首页"
        >
          <img src={logo} alt="NoCodeAI" className="brand-logo" />
          <span>NoCodeAI</span>
        </button>

        <div className="header-actions">
          <button type="button" className="icon-button" title="设置" aria-label="设置">
            <Settings size={18} />
          </button>
          {isAuthenticated ? (
            <button
              type="button"
              className="profile-chip"
              title="个人中心"
              aria-label="个人中心"
            >
              <UserCircle size={19} />
              <span>{user?.userName || '我的账户'}</span>
            </button>
          ) : (
            <button
              type="button"
              className="icon-button"
              title="登录"
              aria-label="登录"
              onClick={() => navigate('/login')}
            >
              <UserCircle size={19} />
            </button>
          )}
          {isAuthenticated && (
            <button
              type="button"
              className="icon-button"
              title="退出登录"
              aria-label="退出登录"
              onClick={() => logoutMutation.mutate()}
            >
              <LogOut size={17} />
            </button>
          )}
        </div>
      </header>

      <section className="home-content">
        <div className="hero-copy">
          <div className="eyebrow">
            <Sparkles size={14} />
            <span>从想法到可运行产品</span>
          </div>
          <h1>{isAuthenticated ? '继续构建你的下一个项目' : '需要我为你做些什么？'}</h1>
          <p>
            {isAuthenticated
              ? '告诉我你想修改或创建什么，NoCodeAI 会陪你一步步完成。'
              : '描述一个网页或产品想法，马上开始你的第一次生成。'}
          </p>
        </div>

        {!isAuthenticated && (
          <div className="conversation-preview" aria-label="助手示例对话">
            <div className="assistant-avatar">
              <WandSparkles size={17} />
            </div>
            <div>
              <span className="message-label">NoCodeAI 助手</span>
              <p>{assistantMessage}</p>
            </div>
          </div>
        )}

        <form className="prompt-composer" onSubmit={handleSubmit}>
          <div className="composer-icon">
            <MessageCircle size={18} />
          </div>
          <Input
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="描述你想创建的网页..."
            aria-label="输入你的需求"
            className="composer-input"
          />
          <Button
            type="submit"
            size="icon"
            className="composer-submit"
            disabled={!prompt.trim() || createAppMutation.isPending}
            aria-label="发送需求"
          >
            <ArrowUp size={18} />
          </Button>
        </form>

        {!isAuthenticated && (
          <div className="suggestion-row">
            {['个人作品集', '产品落地页', '数据看板'].map((suggestion) => (
              <button
                type="button"
                key={suggestion}
                className="suggestion-pill"
                onClick={() => setPrompt(`创建一个${suggestion}`)}
              >
                <Plus size={14} />
                {suggestion}
              </button>
            ))}
          </div>
        )}

        {isAuthenticated && (
          <section className="projects-section">
            <div className="section-heading">
              <div>
                <span className="section-kicker">WORKSPACE</span>
                <h2>我的应用</h2>
              </div>
              <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
                <DialogTrigger asChild>
                  <Button variant="outline" className="new-project-button">
                    <Plus size={16} />
                    新建应用
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>创建新应用</DialogTitle>
                    <DialogDescription>
                      描述你的应用，AI 会为你生成初始项目。
                    </DialogDescription>
                  </DialogHeader>
                  <form onSubmit={handleCreateApp} className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="initPrompt">应用描述</Label>
                      <Input
                        id="initPrompt"
                        placeholder="例如：创建一个极简的产品介绍页"
                        value={initPrompt}
                        onChange={(event) => setInitPrompt(event.target.value)}
                      />
                    </div>
                    <Button
                      type="submit"
                      className="w-full"
                      disabled={createAppMutation.isPending}
                    >
                      {createAppMutation.isPending ? '创建中...' : '开始创建'}
                    </Button>
                  </form>
                </DialogContent>
              </Dialog>
            </div>

            {isLoading ? (
              <div className="projects-placeholder">正在加载你的应用...</div>
            ) : error ? (
              <div className="projects-placeholder projects-error">
                <span>应用列表加载失败</span>
                <Button variant="outline" size="sm" onClick={() => void refetch()}>
                  重试
                </Button>
              </div>
            ) : apps.length > 0 ? (
              <div className="projects-scroller">
                {apps.map((app) => (
                  <Card
                    key={app.id}
                    className="project-card"
                    onClick={() => handleCardClick(app)}
                  >
                    <CardHeader>
                      <div className="project-card-icon">
                        {app.codeGenType === 'vue_project' ? (
                          <Code2 size={19} />
                        ) : (
                          <LayoutTemplate size={19} />
                        )}
                      </div>
                      <CardTitle>{app.appName || '未命名应用'}</CardTitle>
                      <CardDescription>{app.initPrompt}</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <span className="project-type">
                        {projectTypeLabels[app.codeGenType] || '网页项目'}
                      </span>
                      <span className="project-date">
                        {new Date(app.createTime).toLocaleDateString()}
                      </span>
                    </CardContent>
                  </Card>
                ))}
              </div>
            ) : (
              <div className="empty-projects">
                <div className="empty-projects-icon">
                  <LayoutTemplate size={21} />
                </div>
                <div>
                  <strong>还没有应用</strong>
                  <p>从上方输入框开始创建你的第一个项目。</p>
                </div>
              </div>
            )}
          </section>
        )}
      </section>
    </main>
  );
}
