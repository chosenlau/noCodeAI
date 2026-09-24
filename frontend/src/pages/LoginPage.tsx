import { useState } from 'react';
import { useLocation, useNavigate, Link } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { userApi } from '@/api';
import { useAuthStore } from '@/store/auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { toast } from '@/hooks/use-toast';
import { ApiError } from '@/lib/errors';
import type { User, LoginRequest } from '@/types/api';

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const setUser = useAuthStore((state) => state.setUser);
  const [userAccount, setUserAccount] = useState('');
  const [userPassword, setUserPassword] = useState('');

  const loginMutation = useMutation<User, ApiError, LoginRequest>({
    mutationFn: userApi.login,
    onSuccess: (user: User) => {
      setUser(user);
      toast({
        title: '登录成功',
        description: `欢迎回来，${user.userName}`,
      });
      const pendingPrompt = (
        location.state as { pendingPrompt?: string } | null
      )?.pendingPrompt;
      navigate('/', {
        state: pendingPrompt ? { pendingPrompt } : null,
        replace: true,
      });
    },
    onError: (error: ApiError) => {
      toast({
        variant: 'destructive',
        title: '登录失败',
        description: error.message,
      });
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!userAccount || !userPassword) {
      toast({
        variant: 'destructive',
        title: '表单验证失败',
        description: '请填写完整的登录信息',
      });
      return;
    }
    loginMutation.mutate({ userAccount, userPassword });
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>登录</CardTitle>
          <CardDescription>输入账号密码登录到您的账户</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="userAccount">账号</Label>
              <Input
                id="userAccount"
                type="text"
                placeholder="请输入账号"
                value={userAccount}
                onChange={(e) => setUserAccount(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="userPassword">密码</Label>
              <Input
                id="userPassword"
                type="password"
                placeholder="请输入密码"
                value={userPassword}
                onChange={(e) => setUserPassword(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              className="w-full"
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? '登录中...' : '登录'}
            </Button>
            <div className="text-center text-sm text-muted-foreground">
              还没有账号？{' '}
              <Link to="/register" className="text-primary hover:underline">
                立即注册
              </Link>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
