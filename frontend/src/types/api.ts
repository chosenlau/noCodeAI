// ============== 基础类型 ==============

export interface BaseResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

export interface PageQuery {
  pageNum: number;
  pageSize: number;
  sortField?: string;
  sortOrder?: 'asc' | 'desc';
}

export interface PageResult<T> {
  records: T[];
  pageNum: number;
  pageSize: number;
  totalPage: number;
  totalRow: number;
  optimizeCountQuery?: boolean;
}

// ============== 用户相关 ==============

export type UserRole = 'admin' | 'user';

export interface User {
  id: string;
  userAccount: string;
  userName: string;
  userAvatar: string;
  userProfile: string;
  userRole: UserRole;
  createTime: string;
  updateTime: string;
}

export interface RegisterRequest {
  userAccount: string;
  userPassword: string;
  checkPassword: string;
}

export interface LoginRequest {
  userAccount: string;
  userPassword: string;
}

// ============== 应用相关 ==============

export type CodeGenType = 'html' | 'multi_file' | 'vue_project';

export interface AppVo {
  id: string;
  appName: string;
  cover: string;
  initPrompt: string;
  codeGenType: CodeGenType;
  deployKey: string;
  deployedTime: string;
  priority: number;
  userId: string;
  user: User;
  createTime: string;
  updateTime: string;
  tokenUsage: number;
  promptTokens: number;
  completionTokens: number;
  memory: MemoryVo;
}

export interface MemoryVo {
  summary: string;
  round: number;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  summarizing: boolean;
  summaryError: string;
  updatedAt: string;
}

export interface CreateAppRequest {
  initPrompt: string;
}

export interface UpdateAppRequest {
  id: number;
  appName?: string;
}

export interface AppQueryRequest extends PageQuery {
  appName?: string;
  codeGenType?: CodeGenType;
  initPrompt?: string;
  priority?: number;
}

export interface MyAppQueryRequest extends PageQuery {
  appName?: string;
}

// ============== 聊天历史相关 ==============

export type MessageType = 'user' | 'ai' | 'summary';

export interface ChatHistoryVo {
  id: string;
  message: string;
  messageType: MessageType;
  appId: string;
  userId: string;
  turnNumber: number;
  createTime: string;
  updateTime: string;
  isDelete: number;
}

export interface ChatHistoryQueryRequest {
  pageSize?: number;
  lastCreateTime?: string;
  lastId?: string;
}

export interface CursorResponse<T> {
  records: T[];
  nextCreateTime?: string;
  nextId?: string;
  hasMore: boolean;
}

// ============== SSE 流式响应 ==============

export interface StreamMessage {
  d: string;
}

export type StreamEventType = 'message' | 'error' | 'done';
