# 前端项目总结

## 项目概览

本项目是一个基于 Go 后端的 NoCode AI 代码生成平台的完整前端实现，使用现代化的 React 技术栈构建。

## 技术栈

### 核心框架
- **React 18.3.1** - UI 框架
- **TypeScript 5.6.2** - 类型系统
- **Vite 5.4.7** - 构建工具

### 状态管理
- **TanStack Query 5.56.2** - 服务器状态管理（API 缓存、查询、变更）
- **Zustand 4.5.7** - 客户端状态管理（用户认证状态）

### 路由与网络
- **React Router 6.26.2** - 前端路由
- **Axios 1.7.7** - HTTP 客户端

### UI 组件
- **shadcn/ui** - 基于 Radix UI 的组件库
- **Tailwind CSS 3.4.12** - 实用优先的 CSS 框架
- **Lucide React** - 图标库
- **Monaco Editor** - 代码编辑器（VS Code 内核）

## 项目结构

```
frontend/
├── src/
│   ├── api/              # API 客户端
│   │   ├── config.ts     # Axios 配置与拦截器
│   │   ├── user.api.ts   # 用户 API
│   │   ├── app.api.ts    # 应用 API
│   │   ├── chat.api.ts   # 聊天 API
│   │   └── index.ts      # API 导出
│   ├── components/       # 组件
│   │   ├── ui/          # shadcn/ui 组件
│   │   └── ProtectedRoute.tsx  # 路由守卫
│   ├── hooks/           # 自定义 Hooks
│   │   └── use-toast.ts # Toast 通知
│   ├── lib/             # 工具库
│   │   ├── errors.ts    # 错误类
│   │   └── utils.ts     # 工具函数
│   ├── pages/           # 页面组件
│   │   ├── LoginPage.tsx
│   │   ├── RegisterPage.tsx
│   │   ├── AppsPage.tsx
│   │   └── ChatPage.tsx
│   ├── store/           # Zustand Store
│   │   └── auth.ts      # 认证状态
│   ├── types/           # TypeScript 类型
│   │   └── api.ts       # API 类型定义
│   ├── App.tsx          # 根组件
│   ├── main.tsx         # 入口文件
│   └── index.css        # 全局样式
├── public/              # 静态资源
├── docs/                # 文档
│   ├── API_ANALYSIS.md         # 后端 API 分析
│   ├── API_DOCUMENTATION.md    # API 使用文档
│   ├── FRONTEND_LEARNING.md    # 前端学习指南
│   └── README.md               # 文档索引
├── package.json
├── vite.config.ts
├── tailwind.config.js
└── tsconfig.json
```

## 核心功能

### 1. 用户认证系统
- **登录/注册页面**：表单验证、错误处理
- **路由守卫**：`ProtectedRoute` 组件保护需要登录的路由
- **状态持久化**：使用 Zustand + localStorage 保持登录状态

### 2. 应用管理
- **应用列表**：展示用户创建的所有应用
- **创建应用**：通过 Dialog 弹窗创建新应用
- **删除应用**：带确认的删除操作
- **点击进入**：导航到聊天界面

### 3. 代码生成界面（ChatPage）
- **左侧聊天区**：
  - 消息列表（用户消息/AI 消息）
  - 输入框（支持 Enter 发送）
  - 自动滚动到最新消息
  
- **右侧预览区**：
  - 上半部分：Monaco Editor 显示生成的代码
  - 下半部分：iframe 实时预览 HTML 效果
  
- **SSE 流式传输**：
  - 使用 Server-Sent Events 接收流式代码
  - 实时更新代码编辑器和聊天记录
  - 累积显示生成内容

## 关键技术实现

### 1. API 拦截器

```typescript
// src/api/config.ts
api.interceptors.response.use(
  (response) => {
    const { code, message, data } = response.data;
    if (code === 0) {
      return Promise.resolve(data); // 自动解包 BaseResponse
    }
    throw new ApiError(code, message);
  },
  (error) => {
    // 统一错误处理
    throw new ApiError(code || 50000, message || '网络请求失败');
  }
);
```

### 2. React Query 集成

```typescript
// 查询示例
const { data: myApps, isLoading } = useQuery({
  queryKey: ['myApps'],
  queryFn: () => appApi.listMyApps({ pageNum: 1, pageSize: 100 }),
});

// 变更示例
const deleteMutation = useMutation({
  mutationFn: appApi.delete,
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ['myApps'] });
  },
});
```

### 3. Zustand 认证状态

```typescript
// src/store/auth.ts
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      setUser: (user) => set({ user }),
      logout: () => set({ user: null }),
    }),
    { name: 'auth-storage' }
  )
);
```

### 4. SSE 流式接收

```typescript
const eventSource = new EventSource(url, { withCredentials: true });

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  accumulatedCode += data.d;
  setGeneratedCode(accumulatedCode);
};

eventSource.addEventListener('done', () => {
  eventSource.close();
});
```

## 设计特色

### ChatGPT 风格主题
- **浅色模式**：白色背景 + 灰色侧边栏 + 绿色强调色
- **深色模式**：深灰背景 + 更深侧边栏
- **语义化颜色系统**：使用 CSS 变量实现主题切换

```css
:root {
  --background: 0 0% 100%;          /* #FFFFFF */
  --primary: 171 76% 34%;           /* #10A37F ChatGPT 绿色 */
  --border: 0 0% 93%;               /* #EDEDED */
}

.dark {
  --background: 216 13% 20%;        /* #343541 */
  --primary: 171 76% 34%;           /* 主色保持一致 */
}
```

### 响应式布局
- 使用 Tailwind 的响应式类（`md:`、`lg:`）
- 聊天界面采用左右分栏（50% / 50%）
- 应用卡片采用网格布局（1 列 → 2 列 → 3 列）

## 开发工具配置

### Vite 配置
```typescript
// vite.config.ts
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'), // 路径别名
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/user': { target: 'http://localhost:8080', changeOrigin: true },
      '/app': { target: 'http://localhost:8080', changeOrigin: true },
      '/chatHistory': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
});
```

### TypeScript 配置
- 严格模式启用
- 路径别名配置：`@/*` → `src/*`
- 包含 Vite 类型定义

## 启动与部署

### 开发环境

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 访问地址
http://localhost:5173
```

### 生产构建

```bash
# 类型检查
npm run type-check

# 代码检查
npm run lint

# 构建生产版本
npm run build

# 预览生产构建
npm run preview
```

### 环境变量

创建 `.env` 文件：

```env
VITE_API_BASE_URL=http://localhost:8080
```

## 浏览器兼容性

- Chrome/Edge 90+
- Firefox 88+
- Safari 14+

## 性能优化

1. **代码分割**：React Router 自动按路由分割
2. **懒加载**：Monaco Editor 按需加载
3. **缓存策略**：React Query 5 分钟 staleTime
4. **状态持久化**：Zustand persist 减少重复请求

## 已知问题与改进

### 当前限制
1. SSE 端点 `/app/chat` 未在后端 router 注册（需后端修复）
2. 暂不支持文件上传
3. 聊天历史分页加载未实现

### 未来改进方向
1. 添加深色模式切换按钮
2. 实现应用搜索与过滤
3. 添加"收藏应用"功能
4. 优化移动端适配
5. 添加代码语法高亮选择（HTML/CSS/JS）
6. 实现聊天记录导出
7. 添加 WebSocket 支持（替代 SSE）

## 学习资源

### 新手入门
详见 `docs/FRONTEND_LEARNING.md`，包含：
- React 基础教程
- TypeScript 快速入门
- TanStack Query 使用指南
- Zustand 状态管理
- Tailwind CSS 速查
- 综合实践案例

### API 文档
- `docs/API_ANALYSIS.md` - 后端 API 技术分析
- `docs/API_DOCUMENTATION.md` - 前端调用文档

## 贡献指南

### 代码风格
- 使用 ESLint + Prettier
- 遵循 React Hooks 规则
- TypeScript 严格模式
- 组件使用函数式声明

### 提交规范
```
feat: 添加新功能
fix: 修复 bug
docs: 更新文档
style: 代码格式调整
refactor: 代码重构
test: 添加测试
chore: 构建/工具调整
```

## 许可证

MIT

---

**项目状态**：✅ 核心功能已完成，可投入使用

**最后更新**：2024-01-10
