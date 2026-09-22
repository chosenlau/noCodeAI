import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
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
import { LogOut, Plus } from 'lucide-react';
import type { AppVo } from '@/types/api';

export default function AppsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, logout: authLogout } = useAuthStore();
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [initPrompt, setInitPrompt] = useState('');

  const { data: myApps, isLoading, error, refetch } = useQuery({
    queryKey: ['myApps'],
    queryFn: () =>
      appApi.listMyApps(),
    retry: false,
  });

  useEffect(() => {
    if (error && error instanceof ApiError && error.isNotLogin()) {
      authLogout();
      navigate('/login', { replace: true });
      toast({
        title: '登录已过期',
        description: '请重新登录',
        variant: 'destructive',
      });
    }
  }, [error, authLogout, navigate]);

  const logoutMutation = useMutation({
    mutationFn: userApi.logout,
    onSuccess: () => {
      authLogout();
      navigate('/login');
      toast({
        title: '已退出登录',
      });
    },
    onError: (error: ApiError) => {
      toast({
        variant: 'destructive',
        title: '退出失败',
        description: error.message,
      });
    },
  });

  const createAppMutation = useMutation({
    mutationFn: appApi.create,
    onSuccess: (appId: string) => {
      setIsCreateDialogOpen(false);
      setInitPrompt('');
      queryClient.invalidateQueries({ queryKey: ['myApps'] });
      toast({
        title: '创建成功',
        description: '应用已成功创建',
      });
      navigate(`/chat/${appId}`);
    },
    onError: (error: ApiError) => {
      toast({
        variant: 'destructive',
        title: '创建失败',
        description: error.message,
      });
    },
  });

  const handleCreateApp = (e: React.FormEvent) => {
    e.preventDefault();
    if (!initPrompt.trim()) {
      toast({
        variant: 'destructive',
        title: '提示不能为空',
      });
      return;
    }
    createAppMutation.mutate(initPrompt);
  };

  const handleCardClick = (app: AppVo) => {
    navigate(`/chat/${app.id}`);
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <h1 className="text-2xl font-semibold">我的应用</h1>
          <div className="flex items-center gap-4">
            <span className="text-sm text-muted-foreground">
              {user?.userName}
            </span>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => logoutMutation.mutate()}
            >
              <LogOut className="h-5 w-5" />
            </Button>
          </div>
        </div>
      </header>

      <main className="container mx-auto px-4 py-8">
        <div className="mb-6">
          <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button>
                <Plus className="h-4 w-4 mr-2" />
                创建应用
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>创建新应用</DialogTitle>
                <DialogDescription>
                  输入应用描述，AI 将为您生成代码
                </DialogDescription>
              </DialogHeader>
              <form onSubmit={handleCreateApp} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="initPrompt">应用描述</Label>
                  <Input
                    id="initPrompt"
                    placeholder="例如：创建一个待办事项应用"
                    value={initPrompt}
                    onChange={(e) => setInitPrompt(e.target.value)}
                  />
                </div>
                <Button
                  type="submit"
                  className="w-full"
                  disabled={createAppMutation.isPending}
                >
                  {createAppMutation.isPending ? '创建中...' : '创建'}
                </Button>
              </form>
            </DialogContent>
          </Dialog>
        </div>

        {isLoading ? (
          <div className="text-center py-12 text-muted-foreground">
            加载中...
          </div>
        ) : error && !(error instanceof ApiError && error.isNotLogin()) ? (
          <div className="text-center py-12 space-y-4" role="alert">
            <p className="text-destructive">应用列表加载失败：{error.message}</p>
            <Button variant="outline" onClick={() => void refetch()}>
              重新加载
            </Button>
          </div>
        ) : myApps && myApps.records && myApps.records.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {myApps.records.map((app: AppVo) => (
              <Card
                key={app.id}
                className="cursor-pointer hover:shadow-lg transition-shadow"
                onClick={() => handleCardClick(app)}
              >
                <CardHeader>
                  <CardTitle>{app.appName || '未命名应用'}</CardTitle>
                  <CardDescription className="line-clamp-2">
                    {app.initPrompt}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="text-xs text-muted-foreground">
                    创建于 {new Date(app.createTime).toLocaleDateString()}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <div className="text-center py-12 text-muted-foreground">
            暂无应用，点击上方按钮创建第一个应用
          </div>
        )}
      </main>
    </div>
  );
}
