import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { userApi } from '@/api';
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

export default function RegisterPage() {
  const navigate = useNavigate();
  const [userAccount, setUserAccount] = useState('');
  const [userPassword, setUserPassword] = useState('');
  const [checkPassword, setCheckPassword] = useState('');

  const registerMutation = useMutation({
    mutationFn: userApi.register,
    onSuccess: () => {
      toast({
        title: '注册成功',
        description: '请使用新账号登录',
      });
      navigate('/login');
    },
    onError: (error: ApiError) => {
      toast({
        variant: 'destructive',
        title: '注册失败',
        description: error.message,
      });
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    if (!userAccount || !userPassword || !checkPassword) {
      toast({
        variant: 'destructive',
        title: '表单验证失败',
        description: '请填写完整的注册信息',
      });
      return;
    }

    if (userPassword !== checkPassword) {
      toast({
        variant: 'destructive',
        title: '表单验证失败',
        description: '两次输入的密码不一致',
      });
      return;
    }

    if (userPassword.length < 6) {
      toast({
        variant: 'destructive',
        title: '表单验证失败',
        description: '密码长度至少为 6 位',
      });
      return;
    }

    registerMutation.mutate({ userAccount, userPassword, checkPassword });
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>注册</CardTitle>
          <CardDescription>创建一个新账户开始使用</CardDescription>
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
                placeholder="请输入密码（至少 6 位）"
                value={userPassword}
                onChange={(e) => setUserPassword(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="checkPassword">确认密码</Label>
              <Input
                id="checkPassword"
                type="password"
                placeholder="请再次输入密码"
                value={checkPassword}
                onChange={(e) => setCheckPassword(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              className="w-full"
              disabled={registerMutation.isPending}
            >
              {registerMutation.isPending ? '注册中...' : '注册'}
            </Button>
            <div className="text-center text-sm text-muted-foreground">
              已有账号？{' '}
              <Link to="/login" className="text-primary hover:underline">
                立即登录
              </Link>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
