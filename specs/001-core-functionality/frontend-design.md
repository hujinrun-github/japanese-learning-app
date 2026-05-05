# 前端设计方案：日语学习 App（React 版）

> 版本：2.0 | 日期：2026-04-04 | 框架：React 18 + TypeScript + Vite

---

## 1. 技术栈

| 层次 | 选型 | 说明 |
|------|------|------|
| 框架 | React 18 | 并发特性、Suspense |
| 语言 | TypeScript 5 | 严格类型检查 |
| 构建 | Vite 5 | 快速 HMR，生产 `dist/` |
| 路由 | React Router v6 | SPA 客户端路由 |
| 样式 | CSS Modules + CSS Variables | 零运行时，主题系统 |
| 状态 | Context + useReducer | 仅全局 Auth，不引入第三方 |
| 测试 | Vitest + React Testing Library | 组件单元测试 |
| 代码质量 | ESLint + Prettier | 统一格式 |

**刻意不引入**：Redux、MobX、styled-components、Tailwind、Ant Design、Material UI。遵循"简单性原则"，标准库/原生 CSS 优先。

---

## 2. 项目目录结构

```
front/react/
├── index.html
├── vite.config.ts
├── tsconfig.json
├── package.json
└── src/
    ├── main.tsx                  # 应用入口
    ├── App.tsx                   # 路由根组件
    ├── api/                      # API 调用层
    │   ├── client.ts             # fetch 封装（自动带 token）
    │   ├── word.ts
    │   ├── grammar.ts
    │   ├── speaking.ts
    │   ├── writing.ts
    │   ├── summary.ts
    │   └── user.ts
    ├── types/                    # 共享类型定义
    │   ├── word.ts
    │   ├── grammar.ts
    │   ├── speaking.ts
    │   ├── writing.ts
    │   ├── summary.ts
    │   └── user.ts
    ├── context/
    │   └── AuthContext.tsx       # 全局登录状态
    ├── hooks/
    │   ├── useApi.ts             # 通用数据加载 hook
    │   ├── useAudioRecorder.ts   # MediaRecorder 封装
    │   └── useLocalStorage.ts   # 持久化 hook
    ├── components/
    │   ├── ui/                   # 原子组件
    │   │   ├── Badge/
    │   │   ├── Button/
    │   │   ├── Card/
    │   │   ├── Spinner/
    │   │   ├── EmptyState/
    │   │   ├── ProgressBar/
    │   │   └── Toast/
    │   └── layout/               # 布局组件
    │       ├── TopNavBar/
    │       ├── BottomTabBar/
    │       └── PageShell/        # 统一页面容器
    └── pages/
        ├── auth/
        │   ├── LoginPage.tsx
        │   └── RegisterPage.tsx
        ├── home/
        │   └── HomePage.tsx
        ├── word/
        │   └── WordReviewPage.tsx
        ├── grammar/
        │   ├── GrammarListPage.tsx
        │   ├── GrammarDetailPage.tsx
        │   └── GrammarQuizPage.tsx
        ├── lesson/
        │   ├── LessonListPage.tsx
        │   └── LessonDetailPage.tsx
        ├── speaking/
        │   └── SpeakingPage.tsx
        ├── writing/
        │   ├── WritingQueuePage.tsx
        │   └── WritingRecordsPage.tsx
        └── summary/
            └── SummaryPage.tsx
```

---

## 3. 路由结构

```tsx
// App.tsx
<Routes>
  {/* 公开路由 */}
  <Route path="/login"    element={<LoginPage />} />
  <Route path="/register" element={<RegisterPage />} />

  {/* 需要登录 */}
  <Route element={<ProtectedLayout />}>
    <Route path="/"                       element={<HomePage />} />
    <Route path="/words/review"           element={<WordReviewPage />} />
    <Route path="/grammar"                element={<GrammarListPage />} />
    <Route path="/grammar/:id"            element={<GrammarDetailPage />} />
    <Route path="/grammar/:id/quiz"       element={<GrammarQuizPage />} />
    <Route path="/lessons"                element={<LessonListPage />} />
    <Route path="/lessons/:id"            element={<LessonDetailPage />} />
    <Route path="/speaking"               element={<SpeakingPage />} />
    <Route path="/writing"                element={<WritingQueuePage />} />
    <Route path="/writing/records"        element={<WritingRecordsPage />} />
    <Route path="/summary"                element={<SummaryPage />} />
  </Route>
</Routes>
```

**ProtectedLayout**：检查 `AuthContext.isAuthenticated`，未登录自动重定向 `/login`，同时渲染 `TopNavBar` + `BottomTabBar`。

---

## 4. 视觉设计系统

### 4.1 色彩

```css
:root {
  /* 品牌主色 */
  --color-primary:        #3B5BDB;  /* 靛蓝 */
  --color-primary-light:  #748FFC;
  --color-primary-dark:   #2F4AC7;

  /* 功能色 */
  --color-success:        #2F9E44;
  --color-warning:        #F08C00;
  --color-danger:         #E03131;

  /* JLPT 等级色 */
  --color-n5:             #74C0FC;  /* 浅蓝 */
  --color-n4:             #63E6BE;  /* 浅绿 */
  --color-n3:             #FFD43B;  /* 黄 */
  --color-n2:             #FF922B;  /* 橙 */
  --color-n1:             #F03E3E;  /* 红 */

  /* 中性色 */
  --color-bg:             #F8F9FA;
  --color-surface:        #FFFFFF;
  --color-border:         #DEE2E6;
  --color-text-primary:   #212529;
  --color-text-secondary: #6C757D;
  --color-text-disabled:  #ADB5BD;
}

@media (prefers-color-scheme: dark) {
  :root {
    --color-bg:             #1A1B1E;
    --color-surface:        #25262B;
    --color-border:         #373A40;
    --color-text-primary:   #F8F9FA;
    --color-text-secondary: #ADB5BD;
  }
}
```

### 4.2 字体

```css
:root {
  --font-ja: "Noto Sans JP", "Hiragino Sans", "Yu Gothic", sans-serif;
  --font-ui: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;

  --text-xs:   0.75rem;   /* 12px */
  --text-sm:   0.875rem;  /* 14px */
  --text-base: 1rem;      /* 16px */
  --text-lg:   1.125rem;  /* 18px */
  --text-xl:   1.25rem;   /* 20px */
  --text-2xl:  1.5rem;    /* 24px */
  --text-3xl:  2rem;      /* 32px */
  --text-4xl:  2.5rem;    /* 40px */

  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-bold:   700;
}
```

### 4.3 间距（8px 基准网格）

```css
:root {
  --space-1:  4px;
  --space-2:  8px;
  --space-3:  12px;
  --space-4:  16px;
  --space-5:  20px;
  --space-6:  24px;
  --space-8:  32px;
  --space-10: 40px;
  --space-12: 48px;
  --space-16: 64px;
}
```

### 4.4 阴影 & 圆角

```css
:root {
  --radius-sm:  4px;
  --radius-md:  8px;
  --radius-lg:  12px;
  --radius-xl:  16px;
  --radius-full: 9999px;

  --shadow-sm: 0 1px 3px rgba(0,0,0,0.08);
  --shadow-md: 0 4px 12px rgba(0,0,0,0.10);
  --shadow-lg: 0 8px 24px rgba(0,0,0,0.12);
}
```

---

## 5. 布局系统

### 5.1 桌面端（≥ 768px）—— 顶部导航栏

```
┌─────────────────────────────────────────────────┐
│  TopNavBar (56px)                               │
│  [Logo]  单词 语法 课文 口语 写作    [用户头像] │
├─────────────────────────────────────────────────┤
│                                                 │
│              页面内容区域                        │
│         max-width: 960px, margin: auto          │
│                                                 │
└─────────────────────────────────────────────────┘
```

### 5.2 移动端（< 768px）—— 底部标签栏

```
┌───────────────────────────┐
│  页面标题 (TopBar 44px)   │
├───────────────────────────┤
│                           │
│       页面内容区域         │
│   padding-bottom: 80px    │
│                           │
├───────────────────────────┤
│  BottomTabBar (60px)      │
│  🏠  📚  🎤  ✏️  📊     │
└───────────────────────────┘
```

### 5.3 PageShell 组件

```tsx
// 统一页面容器
interface PageShellProps {
  title?: string;         // 移动端顶部标题
  backTo?: string;        // 显示返回按钮
  actions?: ReactNode;    // 右上角操作区
  noPadding?: boolean;    // 全屏内容（如单词卡片）
  children: ReactNode;
}
```

---

## 6. 核心 Hook 设计

### 6.1 `useApi<T>`

```typescript
interface ApiState<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
}

function useApi<T>(
  fetcher: () => Promise<T>,
  deps: DependencyList = []
): ApiState<T> & { refetch: () => void }
```

- `loading` 初始为 `true`，请求完成后为 `false`
- 组件卸载时 abort fetch（AbortController）
- `refetch()` 手动重新发起请求

### 6.2 `useAudioRecorder`

```typescript
interface AudioRecorderState {
  isRecording: boolean;
  audioBlob: Blob | null;
  duration: number;      // 毫秒
  error: string | null;
}

function useAudioRecorder(): AudioRecorderState & {
  start: () => Promise<void>;
  stop: () => void;
  reset: () => void;
}
```

- 封装 `navigator.mediaDevices.getUserMedia` + `MediaRecorder`
- 权限拒绝时 `error` 返回友好提示

### 6.3 `AuthContext`

```typescript
interface AuthUser {
  id: string;
  username: string;
  email: string;
  token: string;
}

interface AuthContextValue {
  user: AuthUser | null;
  isAuthenticated: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}
```

- token 持久化到 `localStorage`
- 所有 API 请求由 `api/client.ts` 自动注入 `Authorization: Bearer <token>`

---

## 7. 各页面设计

### 7.1 首页（HomePage）

```
┌──────────────────────────────────┐
│  今日学习目标                     │
│  ████████░░  80%  距完成还差 4题  │
├──────────────────────────────────┤
│  快速入口（2×2 网格）             │
│  ┌──────┐  ┌──────┐             │
│  │ 📖   │  │ 📝   │             │
│  │ 单词  │  │ 语法  │             │
│  │ 复习  │  │ 学习  │             │
│  └──────┘  └──────┘             │
│  ┌──────┐  ┌──────┐             │
│  │ 🎤   │  │ ✏️   │             │
│  │ 口语  │  │ 写作  │             │
│  │ 练习  │  │ 练习  │             │
│  └──────┘  └──────┘             │
├──────────────────────────────────┤
│  最近学习记录（Timeline 3条）      │
│  • 2小时前  单词复习 ×20          │
│  • 昨天     语法 N5 て形 ✓       │
│  • 昨天     影子跟读 第3课        │
└──────────────────────────────────┘
```

**数据来源**：`GET /api/v1/summary`（最近3条）

---

### 7.2 单词复习页（WordReviewPage）

**布局**：全屏卡片翻转模式

```
┌────────────────────────────────────────┐
│  [N5] 进度: 12/30       [×] 退出      │
│                                        │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  │          食べる                   │  │
│  │                                  │  │
│  │   [点击翻转] / [Space 键翻转]    │  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│                                        │
│  ← 向左滑 = 忘记    向右滑 = 记住 →   │
│                                        │
│  ┌──────────┐  ┌──────────┐           │
│  │ 😕 忘记  │  │ 😊 记住  │           │
│  └──────────┘  └──────────┘           │
└────────────────────────────────────────┘

翻转后（背面）：
┌──────────────────────────────────┐
│  食べる    たべる                 │
│  ──────────────────               │
│  动词 [吃]                        │
│                                   │
│  例句：ご飯を食べる。             │
│       我吃饭。                    │
│                                   │
│  [🔖 加入书签]                    │
└──────────────────────────────────┘
```

**状态管理（useReducer）**：

```typescript
type WordReviewAction =
  | { type: 'FLIP' }
  | { type: 'RATE'; rating: ReviewRating }
  | { type: 'NEXT' }
  | { type: 'COMPLETE' };

interface WordReviewState {
  cards: WordCard[];
  currentIndex: number;
  isFlipped: boolean;
  completed: boolean;
}
```

**手势**：`onTouchStart` / `onTouchEnd` 判断水平滑动距离 > 60px，触发评分。

---

### 7.3 语法列表页（GrammarListPage）

```
┌────────────────────────────────────────────┐
│  语法学习                                   │
│  [N5] [N4] [N3] [N2] [N1]  ← 等级 Tab     │
├────────────────────────────────────────────┤
│  搜索框: 🔍  输入语法点名称...             │
├────────────────────────────────────────────┤
│  ┌────────────────────────────────────┐    │
│  │  [N5]  〜は〜です                  │    │
│  │  用于陈述某事物的性质或状态         │    │
│  │                           [未学习]  │    │
│  └────────────────────────────────────┘    │
│  ┌────────────────────────────────────┐    │
│  │  [N5]  〜て形                      │    │
│  │  动词 て 形，连接两个动作           │    │
│  │                           [✓ 已掌握]│    │
│  └────────────────────────────────────┘    │
│  ...                                       │
└────────────────────────────────────────────┘
```

---

### 7.4 语法详情页（GrammarDetailPage）

```
┌────────────────────────────────────────────┐
│  ← 返回   〜て形   [N5]                    │
├────────────────────────────────────────────┤
│  📖 说明                                   │
│  动词 て 形是动词的连用形之一，用于连接     │
│  多个动作，表示先后顺序或并列。             │
│                                             │
│  📌 接续方式                               │
│  ┌──────────────────────────────────────┐  │
│  │ I 类动词：語尾 u → って / ite        │  │
│  │ II 类动词：去掉 る + て              │  │
│  │ III 类：する → して / くる → きて   │  │
│  └──────────────────────────────────────┘  │
│                                             │
│  💬 例句                                   │
│  1. 朝起きて、歯を磨く。                   │
│     早上起床，刷牙。                        │
│  2. 本を読んで、寝る。                     │
│     看书，睡觉。                            │
│  3. 友達と会って、話す。                   │
│     与朋友见面，交谈。                      │
│                                             │
├────────────────────────────────────────────┤
│        [开始测验 →]                        │
└────────────────────────────────────────────┘
```

---

### 7.5 语法测验页（GrammarQuizPage）

```
┌────────────────────────────────────────────┐
│  ← 返回   测验 1/3                         │
├────────────────────────────────────────────┤
│                                             │
│  请选择正确的 て 形：                        │
│                                             │
│  「書く」                                   │
│                                             │
│  ○  書いて     ← 点击选择                  │
│  ○  書って                                 │
│  ○  書けて                                 │
│  ○  書して                                 │
│                                             │
│         [确认答案]                          │
└────────────────────────────────────────────┘

答对后：
┌────────────────────────────────────────────┐
│  ✅ 正确！                                 │
│                                             │
│  書く → 書いて                             │
│  く 结尾 I 类动词：く → いて              │
│                                             │
│         [下一题 →]                         │
└────────────────────────────────────────────┘
```

---

### 7.6 课文详情页（LessonDetailPage）

```
┌────────────────────────────────────────────┐
│  ← 返回   第3课：駅で                  🔊  │
├────────────────────────────────────────────┤
│  段落（带振り仮名）：                       │
│                                             │
│        えき
│  駅のホームで電車を待っていました。         │
│                                             │
│  [▶ 播放]  ████████░░  0:15 / 0:32        │
│                                             │
│  ─────────────────────────────             │
│  📌 本课语法点                              │
│  [N5] 〜ていました（过去进行时）  →         │
│                                             │
│  📖 单词表                                 │
│  駅 (えき) - 车站  [+ 加入单词本]           │
│  ホーム      - 站台                         │
└────────────────────────────────────────────┘
```

- 振り仮名使用 `<ruby>` + `<rt>` HTML 标签
- 音频进度条使用 `<audio>` 事件同步高亮当前句

---

### 7.7 口语练习页（SpeakingPage）

**Tab 切换：影子跟读 / 自由朗读**

```
┌────────────────────────────────────────────┐
│  口语练习                                   │
│  [影子跟读]  [自由朗读]                     │
├────────────────────────────────────────────┤
│  影子跟读模式：                             │
│                                             │
│  駅のホームで電車を待っていました。          │
│                                             │
│  1. 播放原音：[▶ 播放原文]                 │
│  2. 跟读录音：                             │
│     ┌────────────────────────────────┐     │
│     │  🔴 录音中... 00:05           │     │
│     │  ████████████░░░░             │     │
│     └────────────────────────────────┘     │
│     [■ 停止录音]                           │
│                                             │
│  3. 评分结果：                             │
│     相似度：87%  ⭐⭐⭐⭐☆            │
│     [重录]    [下一句 →]                   │
└────────────────────────────────────────────┘
```

**录音状态机**（useReducer）：
`idle` → `playing_ref` → `recording` → `processing` → `result` → `idle`

---

### 7.8 写作练习页（WritingQueuePage）

**Tab 切换：输入练习 / 造句练习**

```
┌────────────────────────────────────────────┐
│  写作练习                                   │
│  [输入练习]  [造句练习]                     │
├────────────────────────────────────────────┤
│  输入练习（N4 汉字书写）：                  │
│                                             │
│  请输入以下假名对应的汉字：                  │
│                                             │
│  　　　たべる                               │
│  ┌────────────────────────────────────┐    │
│  │  食べる                            │    │
│  └────────────────────────────────────┘    │
│                                             │
│  ✅ 正确！              12/20              │
│                                             │
│  ─────────────────────────────────         │
│  造句练习：                                 │
│                                             │
│  语法点：〜て形   提示：[吃饭之后看书]       │
│                                             │
│  ご飯を食べて、本を読みます。               │
│                                             │
│  [提交批改]                                │
│                                             │
│  AI 批改结果：                             │
│  ✅ 语法正确  ✅ 用词自然                  │
│  建议：可以加上时间词让句子更完整。          │
└────────────────────────────────────────────┘
```

---

### 7.9 学习总结页（SummaryPage）

```
┌────────────────────────────────────────────┐
│  学习记录                                   │
├────────────────────────────────────────────┤
│  本周概览                                   │
│  ┌────────┬────────┬────────┬────────┐     │
│  │ 单词   │ 语法   │ 口语   │ 写作   │     │
│  │  120   │   8    │   5次  │  12句  │     │
│  │  个    │ 个点   │        │        │     │
│  └────────┴────────┴────────┴────────┘     │
│                                             │
│  连续打卡：🔥 7 天                          │
│                                             │
│  最近记录                                   │
│  ┌────────────────────────────────────┐    │
│  │ 今天 14:30  语法学习 て形           │    │
│  │             得分：3/3  🌟🌟🌟      │    │
│  └────────────────────────────────────┘    │
│  ┌────────────────────────────────────┐    │
│  │ 今天 10:15  单词复习 N5 ×20        │    │
│  │             记住：18  忘记：2       │    │
│  └────────────────────────────────────┘    │
└────────────────────────────────────────────┘
```

---

## 8. UI 组件规范

### Badge（等级标签）

```tsx
<Badge level="N5" />  // 浅蓝底
<Badge level="N1" />  // 红底
<Badge status="mastered" />   // 绿色 ✓ 已掌握
<Badge status="learning" />   // 黄色 学习中
<Badge status="new" />        // 灰色 未学习
```

### Button

```tsx
// 变体
<Button variant="primary">开始学习</Button>
<Button variant="secondary">查看记录</Button>
<Button variant="ghost">跳过</Button>
<Button variant="danger">重置进度</Button>

// 尺寸
<Button size="sm" />   // 32px
<Button size="md" />   // 40px（默认）
<Button size="lg" />   // 48px

// 状态
<Button loading />
<Button disabled />
```

### Card

```tsx
<Card
  onClick={handleClick}
  hoverable        // 悬停阴影效果
  padding="md"     // sm | md | lg
>
  内容
</Card>
```

### Spinner

```tsx
<Spinner size="sm" />   // 16px 内联
<Spinner size="md" />   // 32px 居中
<Spinner size="lg" />   // 48px 全页加载
```

### EmptyState

```tsx
<EmptyState
  icon="📭"
  title="暂无数据"
  description="今日单词已全部复习完成！"
  action={<Button>返回首页</Button>}
/>
```

---

## 9. API 客户端设计

```typescript
// api/client.ts
async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const token = localStorage.getItem('auth_token');
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(10000),
  });

  if (res.status === 401) {
    // 清除 token，重定向到登录页
    localStorage.removeItem('auth_token');
    window.location.href = '/login';
  }

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error?.message || 'Unknown error');
  }

  const json = await res.json();
  return json.data as T;
}

export const api = {
  get:    <T>(path: string)                => request<T>('GET', path),
  post:   <T>(path: string, body: unknown) => request<T>('POST', path, body),
  delete: <T>(path: string)               => request<T>('DELETE', path),
};
```

---

## 10. 移动端优化

| 场景 | 方案 |
|------|------|
| 单词卡片翻转 | CSS `perspective` + `rotateY` 3D 翻转动画 |
| 左右滑动评分 | `touchstart`/`touchend` 判断 deltaX |
| 底部安全区域 | `env(safe-area-inset-bottom)` |
| 软键盘弹出 | `visualViewport` resize 事件推上内容 |
| 音频录制 | `useAudioRecorder` hook，权限请求前显示说明 |
| 长列表优化 | 100 条以内不做虚拟滚动，超出使用分页 |
| 触摸目标大小 | 所有可点击元素最小 44×44px |

---

## 11. 后端集成（Go + Vite）

### 构建流程

```bash
# 前端构建输出到 dist/
cd front/react && npm run build
# 输出：front/react/dist/index.html + assets/

# Go 嵌入静态文件
//go:embed dist
var staticFiles embed.FS
```

### Go SPA 路由回退

```go
// 所有非 /api/ 请求返回 index.html（支持前端 SPA 路由）
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    if strings.HasPrefix(r.URL.Path, "/api/") {
        http.NotFound(w, r)
        return
    }
    // 先尝试静态文件
    f, err := staticFiles.Open("dist" + r.URL.Path)
    if err != nil {
        // fallback to index.html
        http.ServeFileFS(w, r, staticFiles, "dist/index.html")
        return
    }
    f.Close()
    http.FileServerFS(staticFiles).ServeHTTP(w, r)
})
```

### 开发代理（Vite 反向代理）

```typescript
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
```

开发时：`npm run dev`（`:5173`）自动代理 API 到后端 `:8080`。
生产时：Go 直接服务 `dist/`，无需 Node.js。

---

## 12. 登录页密码可见性功能

### 功能描述

登录页密码输入框右侧提供一个眼睛图标按钮，允许用户切换密码的显示/隐藏状态，便于确认输入是否正确。

### 交互设计

```
┌────────────────────────────────────────┐
│  パスワード                             │
│  ┌──────────────────────────────────┐  │
│  │  ••••••••                    👁  │  │  ← 点击切换为明文
│  └──────────────────────────────────┘  │
│                                        │
│  ┌──────────────────────────────────┐  │
│  │  mypassword123               🙈  │  │  ← 再次点击恢复隐藏
│  └──────────────────────────────────┘  │
└────────────────────────────────────────┘
```

### 实现规范

| 属性 | 说明 |
|------|------|
| 默认状态 | `type="password"`，密码隐藏，显示眼睛图标 |
| 点击后 | `type="text"`，密码明文显示，显示划线眼睛图标 |
| 按钮类型 | `type="button"`，防止触发表单提交 |
| 无障碍 | `aria-label` 随状态切换（"パスワードを表示する" / "パスワードを隠す"） |
| 键盘访问 | 支持 Tab 聚焦，`:focus-visible` 显示轮廓 |
| 图标 | 内联 SVG，无额外依赖；显示时为普通眼睛，隐藏时为带斜线眼睛 |
| 触摸目标 | 按钮尺寸 40×40px，满足移动端最小触摸目标要求 |

### 组件结构

```tsx
// LoginPage.tsx — 密码字段
<div className={styles.passwordWrapper}>
  <input
    type={showPassword ? 'text' : 'password'}
    ...
  />
  <button
    type="button"
    className={styles.eyeButton}
    onClick={() => setShowPassword(v => !v)}
    aria-label={showPassword ? 'パスワードを隐す' : 'パスワードを表示する'}
  >
    <EyeIcon visible={showPassword} />
  </button>
</div>
```

### 状态

```typescript
const [showPassword, setShowPassword] = useState(false)
```

---

## 14. 忘记密码 / 密码重置功能

> 新增日期：2026-04-06

### 功能流程

```
用户点击「パスワードをお忘れですか？」
         ↓
/forgot-password 页面
  输入注册邮箱 → POST /api/v1/auth/forgot-password
         ↓
后端：生成 32 字节随机 token（有效期 30 分钟）
      存入 password_reset_tokens 表
      发送重置邮件（含 /reset-password?token=xxx 链接）
         ↓
用户点击邮件中的链接 → /reset-password?token=xxx
  输入新密码 + 确认 → POST /api/v1/auth/reset-password
         ↓
后端：验证 token（未过期、未使用）
      更新 users.password_hash
      将 token 标记为 used
         ↓
页面显示「パスワードが正常にリセットされました。」
  → 跳转至登录页
```

---

### 14.1 页面：ForgotPasswordPage（`/forgot-password`）

```
┌─────────────────────────────────────┐
│               🔑                    │
│          日本語学習                  │
│      パスワードをお忘れですか？       │
├─────────────────────────────────────┤
│  ご登録のメールアドレスを入力してくだ │
│  さい。リセット用リンクをお送りします  │
│                                     │
│  メールアドレス                      │
│  ┌───────────────────────────────┐  │
│  │  example@mail.com             │  │
│  └───────────────────────────────┘  │
│                                     │
│  ┌───────────────────────────────┐  │
│  │   リセットリンクを送信         │  │
│  └───────────────────────────────┘  │
│                                     │
│         ← ログインに戻る             │
└─────────────────────────────────────┘

送信成功後（anti-enumeration：不透露邮箱是否注册）：
┌─────────────────────────────────────┐
│  ✅ 登録済みのメールアドレスの場合、 │
│     パスワードリセットリンクを       │
│     お送りしました。                 │
│                                     │
│         ← ログインに戻る             │
└─────────────────────────────────────┘
```

**路由**：`/forgot-password`（公开路由，无需登录）

**API**：`POST /api/v1/auth/forgot-password`
- 请求体：`{ "email": "..." }`
- 响应：始终返回 200（防止邮箱枚举攻击）

---

### 14.2 页面：ResetPasswordPage（`/reset-password`）

```
┌─────────────────────────────────────┐
│               🔒                    │
│          日本語学習                  │
│       新しいパスワードを設定          │
├─────────────────────────────────────┤
│  新しいパスワード                    │
│  ┌────────────────────────────  👁 ┐ │
│  │  ••••••••                       │ │
│  └─────────────────────────────────┘ │
│                                     │
│  パスワード（確認）                  │
│  ┌────────────────────────────  👁 ┐ │
│  │  ••••••••                       │ │
│  └─────────────────────────────────┘ │
│                                     │
│  ┌───────────────────────────────┐  │
│  │   パスワードをリセット         │  │
│  └───────────────────────────────┘  │
│                                     │
│         ← ログインに戻る             │
└─────────────────────────────────────┘

成功後：
┌─────────────────────────────────────┐
│  ✅ パスワードが正常にリセットされ  │
│     ました。                        │
│                                     │
│            ログインページへ          │
└─────────────────────────────────────┘
```

**路由**：`/reset-password?token=<TOKEN>`（公开路由）
- URL 中无 token 时自动跳转 `/forgot-password`

**API**：`POST /api/v1/auth/reset-password`
- 请求体：`{ "token": "...", "new_password": "..." }`
- 错误码：
  - `ERR_TOKEN_INVALID` → 400（token 不存在、已使用、或已过期）
  - `ERR_INTERNAL` → 500

---

### 14.3 LoginPage 改动

在提交按钮下方新增"忘记密码"链接：

```tsx
<Button type="submit" ...>ログイン</Button>

<p className={styles.forgotLink}>
  <Link to="/forgot-password">パスワードをお忘れですか？</Link>
</p>
```

样式：`.forgotLink` — 居中、小字、`color-text-secondary`，带下划线。

---

### 14.4 后端实现

#### 数据库（migration 005）

```sql
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token      TEXT    NOT NULL PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    used       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

#### API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/forgot-password` | 生成 token 并发送邮件 |
| POST | `/api/v1/auth/reset-password` | 验证 token 并更新密码 |

两个端点均为**公开路由**（无需 JWT）。

#### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SMTP_HOST` | `""` | SMTP 服务器主机名；为空时使用 StubMailer（仅打印日志） |
| `SMTP_PORT` | `"587"` | SMTP 端口 |
| `SMTP_USER` | `""` | SMTP 认证用户名 |
| `SMTP_PASS` | `""` | SMTP 认证密码 |
| `SMTP_FROM` | `"noreply@japanese-learning.app"` | 发件人地址 |
| `APP_BASE_URL` | `"http://localhost:5173"` | 重置链接前缀 |

#### Token 生命周期

- 有效期：**30 分钟**
- 生成方式：`crypto/rand` 32 字节，hex 编码（64 字符）
- 一次性：使用后立即标记 `used=1`，重复使用返回 `ERR_TOKEN_INVALID`

#### Anti-enumeration

`POST /api/v1/auth/forgot-password` 无论邮箱是否注册，始终返回 HTTP 200，不向前端泄露"此邮箱未注册"信息。

---

### 14.5 受影响的文件

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/data/migrations/005_password_reset_tokens.sql` | **新建** | DB 表 |
| `internal/data/user_store.go` | **修改** | 新增 `CreateResetToken`、`GetResetToken`、`MarkTokenUsed`、`UpdatePassword` |
| `internal/data/adapters.go` | **修改** | `UserStoreAdapter` 实现新接口方法 |
| `internal/module/user/model.go` | **修改** | 新增 `ResetToken`、`ForgotPasswordReq`、`ResetPasswordReq` 类型 |
| `internal/module/user/mailer.go` | **新建** | `Mailer` 接口 + `SMTPMailer` + `StubMailer` |
| `internal/module/user/service.go` | **修改** | 扩展 `UserStoreInterface`；新增 `ForgotPassword`、`ResetPassword`；`NewUserService` 增加 `mailer`、`appBaseURL` 参数 |
| `internal/module/user/handler.go` | **修改** | 新增 `handleForgotPassword`、`handleResetPassword` |
| `internal/module/user/service_test.go` | **修改** | `fakeUserStore` 实现新接口；新增 6 条测试 |
| `backend/cmd/server/main.go` | **修改** | SMTP 环境变量；构建 `Mailer`；更新 `NewUserService` 调用 |
| `front/react/src/App.tsx` | **修改** | 添加 `/forgot-password`、`/reset-password` 路由 |
| `front/react/src/pages/auth/LoginPage.tsx` | **修改** | 添加"忘记密码"链接 |
| `front/react/src/pages/auth/ForgotPasswordPage.tsx` | **新建** | 忘记密码页 |
| `front/react/src/pages/auth/ResetPasswordPage.tsx` | **新建** | 重置密码页 |
| `front/react/src/pages/auth/AuthPage.module.css` | **修改** | 新增 `.successBox`、`.hint`、`.forgotLink` |



### Phase 1：基础设施（1-2 天）
- [ ] 初始化 Vite + React 18 + TypeScript 项目到 `front/react/`
- [ ] 配置 ESLint + Prettier + CSS Modules
- [ ] 实现设计系统（CSS Variables + 全局样式）
- [ ] 实现布局组件（TopNavBar、BottomTabBar、PageShell）
- [ ] 实现原子组件（Badge、Button、Card、Spinner、EmptyState）
- [ ] 实现 `AuthContext` + 登录/注册页
- [ ] 实现 `useApi` hook + `api/client.ts`
- [ ] 配置路由 + ProtectedLayout
- [ ] Go 后端配置 SPA 回退 + embed 静态文件

### Phase 2：核心模块（2-3 天）
- [ ] 首页（Dashboard）
- [ ] 单词复习页（翻转卡片 + 手势 + useReducer）
- [ ] 语法列表页 + 语法详情页
- [ ] 语法测验页

### Phase 3：进阶模块（2-3 天）
- [ ] 课文列表 + 课文详情（ruby 标注 + 音频同步）
- [ ] 口语练习页（`useAudioRecorder` + 跟读/自由朗读）
- [ ] 写作练习页（输入练习 + 造句 + AI 批改）

### Phase 4：完善（1-2 天）
- [ ] 学习总结页
- [ ] 错误边界（ErrorBoundary）
- [ ] 加载骨架屏（Skeleton）
- [ ] Toast 全局通知
- [ ] `prefers-color-scheme: dark` 暗色主题
- [ ] `prefers-reduced-motion` 无动画模式
- [ ] 基础组件测试（Vitest + RTL）

---

## 13. 与现有后端 API 的映射

| 页面 | API |
|------|-----|
| 单词复习 | `GET /api/v1/words/queue?level=N5` |
| 单词评分 | `POST /api/v1/words/{id}/rate` |
| 单词书签 | `POST /api/v1/words/{id}/bookmark` |
| 语法列表 | `GET /api/v1/grammar?level=N5` |
| 语法详情 | `GET /api/v1/grammar/{id}` |
| 语法测验 | `POST /api/v1/grammar/{id}/quiz` |
| 口语练习 | `POST /api/v1/speaking/practice` （multipart） |
| 口语记录 | `GET /api/v1/speaking/records` |
| 写作队列 | `GET /api/v1/writing/queue` |
| 写作提交 | `POST /api/v1/writing/input` |
| 造句提交 | `POST /api/v1/writing/sentence` |
| 写作记录 | `GET /api/v1/writing/records` |
| 学习总结 | `GET /api/v1/summary` |
| 登录 | `POST /api/v1/users/login` |
| 注册 | `POST /api/v1/users/register` |
