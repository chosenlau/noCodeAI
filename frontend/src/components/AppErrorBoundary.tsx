import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Button } from '@/components/ui/button';

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export default class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Uncaught render error:', error, info.componentStack);
  }

  render() {
    if (!this.state.error) return this.props.children;

    return (
      <main className="min-h-screen grid place-items-center bg-background p-6">
        <section className="max-w-md space-y-4 text-center" role="alert">
          <h1 className="text-xl font-semibold">页面暂时无法显示</h1>
          <p className="text-sm text-muted-foreground">
            页面遇到了意外错误，请刷新后重试。
          </p>
          <Button onClick={() => window.location.reload()}>刷新页面</Button>
        </section>
      </main>
    );
  }
}
