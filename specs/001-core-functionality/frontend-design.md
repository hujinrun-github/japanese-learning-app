---
feature: "001-core-functionality"
title: "前端设计文档"
status: "草稿"
version: "1.0"
created: "2026-04-03"
relates_to: "spec.md, plan.md, tasks.md"
---

# 前端设计文档：日语学习应用

> **范围**：本文档覆盖 Web 端（V1~V4）的视觉设计系统、页面布局、组件规范、交互细节和技术实现方案。iOS（V5）不在本文档范围内。

---

## 一、设计理念

### 1.1 核心原则

| 原则 | 说明 |
|---|---|
| **克制** | 不用颜色和动效堆砌，留白和字距是主要设计语言 |
| **专注** | 每个页面只有一个核心操作，减少决策疲劳 |
| **及时反馈** | 用户的每次操作（翻牌、评分、提交）必须在 300ms 内有视觉响应 |
| **可中断** | 每个学习单元自成闭环，随时关闭不丢失进度 |

### 1.2 目标用户使用场景对设计的约束

- **通勤场景（竖屏、单手、嘈杂）**：触控目标 ≥ 44px；关键操作集中在屏幕下半区（拇指可及）；不依赖声音提示
- **午休/晚间场景（横屏可能、双手、安静）**：可展示更多信息密度；支持键盘快捷操作

---

## 二、视觉设计系统

### 2.1 色彩系统

```css
:root {
  /* 主色调：靛蓝（沉静、专注） */
  --color-primary:        #3B5BDB;  /* 主要按钮、强调元素 */
  --color-primary-light:  #748FFC;  /* hover 状态、次级强调 */
  --color-primary-subtle: #EDF2FF;  /* 轻底色、选中背景 */

  /* 中性色阶 */
  --color-text-primary:   #1A1A2E;  /* 正文、标题 */
  --color-text-secondary: #6B7280;  /* 辅助说明、标签 */
  --color-text-disabled:  #9CA3AF;  /* 禁用状态 */
  --color-border:         #E5E7EB;  /* 分割线、卡片边框 */
  --color-bg-surface:     #FFFFFF;  /* 卡片、弹窗背景 */
  --color-bg-page:        #F9FAFB;  /* 页面背景 */

  /* 功能色 */
  --color-success:        #16A34A;  /* 正确、容易 */
  --color-success-bg:     #F0FDF4;
  --color-warning:        #D97706;  /* 一般、注意 */
  --color-warning-bg:     #FFFBEB;
  --color-error:          #DC2626;  /* 错误、困难 */
  --color-error-bg:       #FEF2F2;

  /* JLPT 等级徽章色 */
  --color-n5: #6EE7B7;  /* 绿：入门 */
  --color-n4: #93C5FD;  /* 蓝：基础 */
  --color-n3: #FCD34D;  /* 黄：中级 */
  --color-n2: #F9A8D4;  /* 粉：中高级 */
  --color-n1: #FCA5A5;  /* 红：高级 */
}
```

**深色模式**（后续 V2 迭代添加，预留 CSS 变量切换接口）：
```css
@media (prefers-color-scheme: dark) {
  :root {
    --color-text-primary:  #F3F4F6;
    --color-bg-surface:    #1F2937;
    --color-bg-page:       #111827;
    --color-border:        #374151;
  }
}
```

---

### 2.2 字体系统

```css
:root {
  /* 字体栈：优先系统日文字体，确保振り仮名正确渲染 */
  --font-japanese: "Noto Sans JP", "Hiragino Sans", "Hiragino Kaku Gothic ProN",
                   "Yu Gothic", "Meiryo", sans-serif;
  --font-chinese:  "PingFang SC", "Noto Sans SC", "Microsoft YaHei", sans-serif;
  --font-mono:     "JetBrains Mono", "Fira Code", monospace;

  /* 字号阶梯（基于 1rem = 16px） */
  --text-xs:   0.75rem;   /* 12px — 徽章、辅助标注 */
  --text-sm:   0.875rem;  /* 14px — 辅助说明、时间戳 */
  --text-base: 1rem;      /* 16px — 正文 */
  --text-lg:   1.125rem;  /* 18px — 卡片内容 */
  --text-xl:   1.25rem;   /* 20px — 小标题 */
  --text-2xl:  1.5rem;    /* 24px — 日语单词展示 */
  --text-3xl:  1.875rem;  /* 30px — 大号单词卡片 */
  --text-4xl:  2.25rem;   /* 36px — 首屏主词 */

  /* 行高 */
  --leading-tight:  1.25;  /* 标题 */
  --leading-normal: 1.6;   /* 正文 */
  --leading-loose:  2.0;   /* 日语正文（为振り仮名留空间） */
}
```

**日语文本特殊规则**：
- 课文阅读区行高固定 `2.5rem`，为 `<ruby>` 标签上方的假名预留空间
- 单词卡片主词使用 `var(--font-japanese)` + `font-weight: 700`
- 中文释义使用 `var(--font-chinese)` + `font-weight: 400`

---

### 2.3 间距与圆角

```css
:root {
  /* 间距系统（4px 基准） */
  --space-1:  0.25rem;  /* 4px */
  --space-2:  0.5rem;   /* 8px */
  --space-3:  0.75rem;  /* 12px */
  --space-4:  1rem;     /* 16px */
  --space-5:  1.25rem;  /* 20px */
  --space-6:  1.5rem;   /* 24px */
  --space-8:  2rem;     /* 32px */
  --space-10: 2.5rem;   /* 40px */
  --space-12: 3rem;     /* 48px */

  /* 圆角 */
  --radius-sm:   4px;
  --radius-md:   8px;
  --radius-lg:   12px;
  --radius-xl:   16px;
  --radius-full: 9999px;  /* 胶囊形按钮、徽章 */
}
```

---

### 2.4 阴影

```css
:root {
  --shadow-sm:  0 1px 2px rgba(0,0,0,0.05);
  --shadow-md:  0 4px 6px -1px rgba(0,0,0,0.07), 0 2px 4px -1px rgba(0,0,0,0.04);
  --shadow-lg:  0 10px 15px -3px rgba(0,0,0,0.08), 0 4px 6px -2px rgba(0,0,0,0.04);
  --shadow-card: 0 2px 8px rgba(59,91,219,0.08);  /* 蓝调卡片阴影 */
}
```

---

## 三、布局系统

### 3.1 整体布局结构

```
┌──────────────────────────────────────────┐
│              顶部导航栏 (56px)            │  fixed, z-index: 100
├──────────────────────────────────────────┤
│                                          │
│              内容区域                    │  padding-top: 56px
│         max-width: 768px                 │  max-width: 768px, centered
│         padding: 0 16px                  │
│                                          │
├──────────────────────────────────────────┤
│           底部标签栏 (桌面端隐藏)         │  fixed, z-index: 100
│           (移动端: 60px)                 │  仅移动端显示
└──────────────────────────────────────────┘
```

### 3.2 响应式断点

```css
/* 移动优先 */
/* xs: 0~479px   — 手机竖屏（通勤场景）*/
/* sm: 480~767px — 手机横屏、小平板 */
/* md: 768~1023px — 平板 */
/* lg: 1024px+   — 桌面 */

@media (min-width: 768px) {
  /* 内容区加宽，显示两列布局（列表页） */
  .grid-auto { grid-template-columns: repeat(2, 1fr); }
  /* 底部标签栏改为左侧边栏 */
  .nav-sidebar { display: flex; }
  .nav-bottom  { display: none; }
}
```

### 3.3 顶部导航栏

```
┌────────────────────────────────────────────────────┐
│  ☰  日本語学習  ·········  [连续7天🔥]  [用户头像] │
└────────────────────────────────────────────────────┘
```

- 左侧：汉堡菜单（移动端）/ Logo 文字
- 中间：模块面包屑（进入模块后显示）
- 右侧：连续打卡天数徽章 + 用户头像（点击展开菜单）

### 3.4 底部标签栏（移动端）

```
┌──────────────────────────────────────────────────┐
│  [单词]   [语法]   [课文]   [口语]   [写作]      │
│   📚       📖      📄       🎤       ✏️           │
└──────────────────────────────────────────────────┘
```

触控目标高度 60px，每个标签宽度均等，图标 + 文字两行。

---

## 四、通用组件规范

### 4.1 JLPT 等级徽章

```
[N5]  [N4]  [N3]  [N2]  [N1]
```

```css
.badge-jlpt {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  font-weight: 600;
  font-family: var(--font-mono);
}
.badge-n5 { background: var(--color-n5); color: #065F46; }
.badge-n4 { background: var(--color-n4); color: #1E3A5F; }
.badge-n3 { background: var(--color-n3); color: #78350F; }
.badge-n2 { background: var(--color-n2); color: #831843; }
.badge-n1 { background: var(--color-n1); color: #7F1D1D; }
```

---

### 4.2 按钮规范

```
[主要操作]      [次要操作]      [危险操作]      [文字按钮]
  实心蓝色        边框灰色        实心红色         纯文字
  高度 44px       高度 44px       高度 44px        无背景
```

```css
.btn-primary {
  background: var(--color-primary);
  color: white;
  height: 44px;
  padding: 0 var(--space-6);
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--text-base);
  transition: background 150ms ease;
}
.btn-primary:hover   { background: #364FC7; }
.btn-primary:active  { background: #2F44AD; transform: scale(0.98); }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
```

**评分按钮（单词卡片专用）**：

```
┌──────────┐  ┌──────────┐  ┌──────────┐
│  😰 困难  │  │  😐 一般  │  │  😊 容易  │
│  1天后   │  │  3天后   │  │  7天后   │
└──────────┘  └──────────┘  └──────────┘
  红色边框      灰色边框       绿色边框
  高度 64px     高度 64px      高度 64px
```

---

### 4.3 进度条

```
今日进度  ████████░░░░░░░  8 / 15
```

```css
.progress-bar {
  height: 6px;
  background: var(--color-border);
  border-radius: var(--radius-full);
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: var(--color-primary);
  border-radius: var(--radius-full);
  transition: width 400ms cubic-bezier(0.34, 1.56, 0.64, 1); /* 弹性缓动 */
}
```

---

### 4.4 释义弹窗（单词点击）

```
┌──────────────────────────────────┐
│  勉強   べんきょう              × │
│  ────────────────────────────── │
│  名詞                            │
│  学习；用功                      │
│                                  │
│  例：毎日日本語を勉強します。     │
│      每天学习日语。               │
│                                  │
│          [+ 加入单词本]           │
└──────────────────────────────────┘
```

- 从屏幕底部滑入（`transform: translateY`）
- 背景半透明遮罩（`rgba(0,0,0,0.4)`）
- 点击遮罩或 × 关闭
- 动画时长 250ms，`ease-out`

---

### 4.5 Toast 通知

```
                  ┌────────────────────────┐
                  │  ✓  已加入单词本        │
                  └────────────────────────┘
```

- 固定在屏幕顶部居中，距顶 72px（导航栏下方）
- 自动消失：2 秒
- 动画：从上方淡入 → 停留 → 淡出
- 类型：success（绿）/ error（红）/ info（蓝）

---

### 4.6 加载状态

```
骨架屏（Skeleton）— 内容加载中
┌───────────────────────────┐
│  ████████░░░░  ░░░░       │  ← 标题骨架
│                            │
│  ░░░░░░░░░░░░░░░░░░░░░░  │  ← 内容骨架
│  ░░░░░░░░░░░░░░          │
└───────────────────────────┘
```

```css
.skeleton {
  background: linear-gradient(
    90deg,
    var(--color-border) 25%,
    #f0f0f0 50%,
    var(--color-border) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: var(--radius-sm);
}
@keyframes shimmer {
  0%   { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
```

---

## 五、页面设计详述

### 5.1 首页（仪表盘）

```
┌─────────────────────────────────────┐
│  日本語学習          🔥7天  [头像]  │
├─────────────────────────────────────┤
│                                     │
│  早上好，继续今天的学习！            │
│                                     │
│  ┌──────────────────────────────┐   │
│  │  今日待完成              3   │   │  ← 任务概览卡片
│  │  ────────────────────────── │   │
│  │  📚 单词复习      12张  [开始]│  │
│  │  📖 语法复习       2条  [开始]│  │
│  │  ✏️  造句练习       3题  [开始]│  │
│  └──────────────────────────────┘   │
│                                     │
│  ──────── 继续上次 ────────         │
│  ┌────────────┐  ┌────────────┐    │
│  │ N3 课文     │  │ 影子跟读   │   │  ← 快速续学卡片
│  │ 雨の日     │  │ 第3段落    │   │
│  │ 上次 P.2   │  │ 75分       │   │
│  └────────────┘  └────────────┘    │
│                                     │
│  ──────── 本周统计 ────────         │
│  [连续天数 7]  [总时长 3.2h]        │
│  [单词掌握 127] [语法完成 14]       │
└─────────────────────────────────────┘
```

---

### 5.2 单词记忆模块

#### 5.2.1 复习队列页

```
┌─────────────────────────────────────┐
│  ← 单词复习       8 / 12  [结束]   │
│  ████████████░░░░░░                 │  ← 进度条
├─────────────────────────────────────┤
│                                     │
│                                     │
│  ┌──────────────────────────────┐   │
│  │                              │   │
│  │          勉強                │   │  ← 正面：日语单词
│  │       べんきょう              │   │     大字 + 假名
│  │                              │   │
│  │          [N4]                │   │
│  │                              │   │
│  │      ─────────────           │   │
│  │      点击卡片查看释义          │   │
│  │                              │   │
│  └──────────────────────────────┘   │
│                                     │
│                                     │
└─────────────────────────────────────┘
```

**翻转后（背面）**：

```
┌─────────────────────────────────────┐
│  ← 单词复习       8 / 12  [结束]   │
│  ████████████░░░░░░                 │
├─────────────────────────────────────┤
│                                     │
│  ┌──────────────────────────────┐   │
│  │  勉強  べんきょう  名詞 [N4] │   │
│  │  ──────────────────────────  │   │
│  │  学习；用功                   │   │
│  │                              │   │
│  │  例：毎日日本語を勉強します。  │   │
│  │      每天学习日语。           │   │
│  └──────────────────────────────┘   │
│                                     │
│  ┌──────┐   ┌──────┐   ┌──────┐   │
│  │😰困难 │   │😐一般 │   │😊容易 │  │  ← 评分按钮区
│  │1天后  │   │3天后  │   │7天后  │  │
│  └──────┘   └──────┘   └──────┘   │
└─────────────────────────────────────┘
```

**卡片翻转动画**：
```css
.card-wrapper {
  perspective: 1000px;
}
.card {
  transform-style: preserve-3d;
  transition: transform 400ms ease;
}
.card.flipped {
  transform: rotateY(180deg);
}
.card-front, .card-back {
  backface-visibility: hidden;
  position: absolute; inset: 0;
}
.card-back {
  transform: rotateY(180deg);
}
```

#### 5.2.2 今日完成页（队列清空）

```
┌─────────────────────────────────────┐
│           今日复习完成！🎉           │
│                                     │
│   ┌─────────────────────────────┐   │
│   │  本次复习  12 张             │   │
│   │  容易比例  ████░  58%       │   │
│   │  困难单词  3 个              │   │
│   └─────────────────────────────┘   │
│                                     │
│  【困难单词，明日优先复习】           │
│  • 覚える — 记住（困难）             │
│  • 忘れる — 忘记（困难）             │
│  • 悲しい — 悲伤（困难）             │
│                                     │
│     [提前学习新词]   [返回首页]      │
└─────────────────────────────────────┘
```

---

### 5.3 语法学习模块

#### 5.3.1 语法点列表页

```
┌─────────────────────────────────────┐
│  ← 语法学习                         │
│  [N5] [N4] [N3] [N2] [N1]  ← 筛选  │
├─────────────────────────────────────┤
│  N4 语法点 (32条)                    │
│                                     │
│  ┌────────────────────────────────┐ │
│  │ [N4]  〜てもいい               │ │
│  │       可以〜（表达许可）        │ │
│  │       ●●●○○  学习中           │ │  ← 掌握状态
│  └────────────────────────────────┘ │
│  ┌────────────────────────────────┐ │
│  │ [N4]  〜なければならない        │ │
│  │       必须〜（表达义务）        │ │
│  │       ○○○○○  未学             │ │
│  └────────────────────────────────┘ │
│  ┌────────────────────────────────┐ │
│  │ [N4]  〜たことがある            │ │
│  │       曾经〜（表达经历）        │ │
│  │       ●●●●●  已掌握 ✓        │ │
│  └────────────────────────────────┘ │
└─────────────────────────────────────┘
```

掌握状态显示：
- 未学：5个空心圆 `○○○○○`，灰色
- 学习中：部分实心圆 `●●●○○`，蓝色
- 已掌握：5个实心圆 `●●●●●` + ✓，绿色

#### 5.3.2 语法点详情页（分步骤）

**步骤 1：讲解**
```
┌─────────────────────────────────────┐
│  ← 〜てもいい              [N4]    │
│  ─────────────────────────────────  │
│  步骤 1/3：讲解  ●○○               │
├─────────────────────────────────────┤
│  意思                               │
│  可以〜，表达允许或许可              │
│                                     │
│  接续方式                           │
│  ┌─────────────────────────────┐    │
│  │  動詞て形 + もいい           │    │  ← 接续规则框
│  │  食べて + もいい = 食べてもいい│   │
│  └─────────────────────────────┘    │
│                                     │
│  使用场景                           │
│  用于请求对方许可，或告知对方某事    │
│  被允许，语气较随和。               │
│                                     │
│  例句                               │
│  ここに座ってもいいですか。          │
│  （可以坐这里吗？）                  │
│  [単語を見る]  [加入单词本 +]        │
│                                     │
│  写真を撮ってもいいですか。          │
│  （可以拍照吗？）                    │
│                                     │
│           [开始检验 →]              │
└─────────────────────────────────────┘
```

**步骤 2：检验**
```
┌─────────────────────────────────────┐
│  ← 〜てもいい     步骤 2/3：检验  ●●○│
├─────────────────────────────────────┤
│  题目 1 / 2                         │
│                                     │
│  请填入正确的语法形式：              │
│                                     │
│  「ここに＿＿＿＿＿ですか。」         │
│   （可以在这里停车吗？）             │
│                                     │
│  ┌─────────────────────────────┐    │
│  │  駐車して                   │    │  ← 输入框（日语输入法）
│  └─────────────────────────────┘    │
│                                     │
│  提示：て形 + _______              │
│                                     │
│              [提交答案]             │
└─────────────────────────────────────┘
```

**答错后展开解析**：
```
┌─────────────────────────────────────┐
│  ✗  你的答案：駐車して              │  ← 红色背景
│  ✓  正确答案：駐車してもいい        │  ← 绿色背景
│                                     │
│  解析                               │
│  「てもいい」表示许可，完整形式      │
│  为「動詞て形 + もいいですか」。    │
│  「もいい」不可省略。               │
│                                     │
│              [我知道了，继续]        │
└─────────────────────────────────────┘
```

---

### 5.4 课文学习模块

#### 5.4.1 课文阅读页

```
┌─────────────────────────────────────┐
│  ← 雨の日        [N3]  [显示翻译]  │
├─────────────────────────────────────┤
│  ▶  播放全文   ◀▶  逐句播放  ×1.0  │  ← 音频控制栏
├─────────────────────────────────────┤
│                                     │
│  今日は  雨  が  降って  います。   │  ← 振り仮名渲染
│         あめ       ふって            │    (ruby标签)
│                                     │  ← 当前播放句：淡蓝色背景高亮
│  ────────────────────────────────  │
│                                     │
│        昨日  から  ずっと           │
│        きのう                       │
│        雨  が  続いて  います。     │
│        あめ     つづいて            │
│                                     │
│       ▶  [播放本句]                 │  ← 每句旁有播放按钮
│  ────────────────────────────────  │
│                                     │
│  ...（更多段落）                    │
│                                     │
├─────────────────────────────────────┤
│  [← 上一句]           [下一句 →]   │
│          [加入单词本 (3)]           │  ← 已标记3个生词
└─────────────────────────────────────┘
```

**振り仮名 HTML 实现**：
```html
<!-- 有读音的汉字词 -->
<ruby>勉強<rt>べんきょう</rt></ruby>

<!-- 无需标注的假名直接输出 -->
します

<!-- 整句结构 -->
<span class="sentence" data-index="0" data-start-ms="1200" data-end-ms="3800">
  毎日
  <ruby>日本語<rt>にほんご</rt></ruby>
  を
  <ruby class="word-clickable" data-word-id="42">勉強<rt>べんきょう</rt></ruby>
  します。
</span>
```

```css
/* 振り仮名样式 */
ruby { ruby-align: center; }
rt {
  font-size: 0.55em;
  color: var(--color-text-secondary);
  font-family: var(--font-japanese);
}

/* 课文行高必须足够容纳 rt */
.lesson-text {
  font-family: var(--font-japanese);
  font-size: var(--text-lg);
  line-height: 2.5rem;  /* 固定行高，防止 ruby 行距不一致 */
}

/* 音频同步高亮 */
.sentence.active {
  background: var(--color-primary-subtle);
  border-radius: var(--radius-sm);
  padding: 2px 4px;
  transition: background 200ms ease;
}

/* 可点击单词 */
.word-clickable {
  cursor: pointer;
  border-bottom: 1px dashed var(--color-primary-light);
}
.word-clickable:hover {
  background: var(--color-primary-subtle);
  border-radius: 2px;
}
```

**音频同步高亮 TypeScript 核心逻辑**：
```typescript
// lesson.ts 核心逻辑（伪代码展示思路）

interface Sentence {
  index: number;
  startMs: number;
  endMs: number;
}

function syncHighlight(audio: HTMLAudioElement, sentences: Sentence[]) {
  audio.addEventListener('timeupdate', () => {
    const currentMs = audio.currentTime * 1000;
    const active = sentences.find(
      s => currentMs >= s.startMs && currentMs < s.endMs
    );
    // 移除所有高亮，添加当前句高亮
    document.querySelectorAll('.sentence').forEach(el => el.classList.remove('active'));
    if (active) {
      document.querySelector(`[data-index="${active.index}"]`)?.classList.add('active');
    }
  });
}
```

---

### 5.5 口语练习模块

#### 5.5.1 影子跟读页

```
┌─────────────────────────────────────┐
│  ← 影子跟读       [N3]  材料 3/10  │
├─────────────────────────────────────┤
│  速度: [0.5x] [0.75x] [▶1x] [1.25x] [1.5x]│
├─────────────────────────────────────┤
│                                     │
│  今日は雨が降っています。            │  ← 当前句（高亮）
│  きょうはあめがふっています。         │  ← 假名注音（淡色）
│                                     │
│  ────────────────────────────────  │
│  昨日からずっと雨が続いています。    │  ← 其余句（常规色）
│                                     │
│  ────────────────────────────────  │
│                                     │
│  ┌────────────────────────────────┐ │
│  │  ───────────────────────────   │ │  ← 音频进度条
│  │  ▶  00:12 / 01:35             │ │
│  └────────────────────────────────┘ │
│                                     │
│  ┌────────────────────────────────┐ │
│  │         ● 开始跟读             │ │  ← 录音按钮（大）
│  └────────────────────────────────┘ │
│                                     │
│  上次得分：78分   历史最高：85分     │
└─────────────────────────────────────┘
```

**录音中状态**：
```
┌────────────────────────────────────┐
│  ● 录音中   00:08                  │  ← 红色脉冲动画
│                                    │
│  ████████░░░░░░░░░░░░░░░░░░░░░░   │  ← 音频波形可视化
│                                    │
│         [■ 结束录音]               │
└────────────────────────────────────┘
```

**评分结果**：
```
┌────────────────────────────────────┐
│  本次跟读得分                       │
│                                    │
│           78                       │  ← 大号分数
│          ─────                     │
│         较上次 +5 ↑                │
│                                    │
│  句子得分详情：                     │
│  今日は雨が...    ████████░  85分  │  ← 绿色
│  昨日からずっと...  ████░░░░  62分  │  ← 橙色，需注意
│                                    │
│  [▶ 回放我的录音]  [▶ 播放原音]    │
│                                    │
│          [再来一次]  [继续]         │
└────────────────────────────────────┘
```

**音频波形可视化（Canvas + WebAudio API）**：
```typescript
// speaking.ts 波形绘制核心逻辑

function drawWaveform(analyser: AnalyserNode, canvas: HTMLCanvasElement) {
  const bufferLength = analyser.frequencyBinCount;
  const dataArray = new Uint8Array(bufferLength);
  const ctx = canvas.getContext('2d')!;
  const W = canvas.width, H = canvas.height;

  function draw() {
    requestAnimationFrame(draw);
    analyser.getByteTimeDomainData(dataArray);

    ctx.fillStyle = 'var(--color-bg-surface)';
    ctx.fillRect(0, 0, W, H);
    ctx.lineWidth = 2;
    ctx.strokeStyle = 'var(--color-primary)';
    ctx.beginPath();

    const sliceWidth = W / bufferLength;
    let x = 0;
    for (let i = 0; i < bufferLength; i++) {
      const v = dataArray[i] / 128.0;
      const y = (v * H) / 2;
      i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
      x += sliceWidth;
    }
    ctx.stroke();
  }
  draw();
}
```

---

### 5.6 写作练习模块

#### 5.6.1 造句练习页

```
┌─────────────────────────────────────┐
│  ← 造句练习        题目 2 / 3       │
│  ●●○                               │  ← 进度点
├─────────────────────────────────────┤
│                                     │
│  语法点：〜てもいい          [N4]   │
│                                     │
│  请用上述语法点，将下列中文译为日语：│
│                                     │
│  ┌─────────────────────────────┐    │
│  │  "你可以在这里停车。"        │    │  ← 题目框（中文）
│  └─────────────────────────────┘    │
│                                     │
│  你的回答：                         │
│  ┌─────────────────────────────┐    │
│  │  ここに駐車してもいいです。  │    │  ← 日语输入区
│  │                             │    │    支持日语IME
│  └─────────────────────────────┘    │
│                                     │
│  字符数：14    参考长度：10~20字    │
│                                     │
│              [提交批改]             │
│                                     │
│  ─── AI 批改中，请稍候… ───         │  ← 提交后显示
│  ┌──────────────────────────────┐   │
│  │  ⠋ 正在批改...               │   │  ← 加载动画
│  └──────────────────────────────┘   │
└─────────────────────────────────────┘
```

**AI 批改结果**：
```
┌─────────────────────────────────────┐
│  ✓ 语法正确   ✓ 用词准确    92分    │  ← 绿色通过徽章
│                                     │
│  你的答案：                          │
│  ここに駐車してもいいです。          │  ← 绿色底色
│                                     │
│  参考答案：                          │
│  ここに駐車してもいいです。（✓同）   │
│                                     │
│  其他地道说法：                      │
│  • ここで駐車できます。              │
│  • 駐車場はこちらです。              │
│                                     │
│              [下一题 →]             │
└─────────────────────────────────────┘
```

**答案有误时**：
```
┌─────────────────────────────────────┐
│  ✗ 语法有误    ✓ 用词准确    65分  │
│                                     │
│  你的答案：                          │
│  ここに駐車してもです。              │  ← 红色标注错误部分
│             ~~~~~~~~~~              │
│                                     │
│  问题说明：                          │
│  「してもです」不是正确形式，        │
│  「してもいい」中的「いい」不可省略。│
│                                     │
│  正确答案：                          │
│  ここに駐車して**もいい**です。      │  ← 加粗修正部分
│                                     │
│     [再试一次]        [下一题 →]    │
└─────────────────────────────────────┘
```

---

### 5.7 练习总结模块

```
┌─────────────────────────────────────┐
│           本次练习总结              │
│         单词复习 · 刚刚             │
├─────────────────────────────────────┤
│                                     │
│  ┌───────────────────────────────┐  │
│  │  复习单词  12  ·  容易 58%    │  │  ← 数字概要卡片
│  │  ──────────────────────────   │  │
│  │  连续学习  🔥 7 天             │  │
│  └───────────────────────────────┘  │
│                                     │
│  🌟 亮点                            │
│  ┌───────────────────────────────┐  │
│  │  ✓  日本語   连续3次评为容易  │  │
│  │  ✓  先生     上次困难，今天容易│  │
│  └───────────────────────────────┘  │
│                                     │
│  💪 待改进                          │
│  ┌───────────────────────────────┐  │
│  │  •  覚える   本次困难，明日优先│  │
│  │  •  忘れる   连续2次困难       │  │
│  │  •  悲しい   本次困难          │  │
│  └───────────────────────────────┘  │
│                                     │
│  💡 建议                            │
│  「覚える」「忘れる」为同类动词，   │
│  建议对比记忆，明日一起复习。       │
│                                     │
│  [查看详情]                         │
│                                     │
│  ┌──────────────┐  ┌────────────┐  │
│  │  继续学习    │  │ 今日完成  │   │
│  └──────────────┘  └────────────┘  │
└─────────────────────────────────────┘
```

---

## 六、关键交互规范

### 6.1 单词卡片翻转

| 触发方式 | 行为 |
|---|---|
| 点击/轻触卡片任意位置 | 卡片 3D 翻转（Y 轴旋转 180°，400ms） |
| 空格键（桌面端） | 同上 |
| 左滑（已翻开） | 困难评分 |
| 右滑（已翻开） | 容易评分 |
| 上滑（已翻开） | 一般评分 |

**滑动手势实现**：
```typescript
// 记录触摸起点
let touchStartX = 0, touchStartY = 0;

card.addEventListener('touchstart', e => {
  touchStartX = e.touches[0].clientX;
  touchStartY = e.touches[0].clientY;
});

card.addEventListener('touchend', e => {
  const dx = e.changedTouches[0].clientX - touchStartX;
  const dy = e.changedTouches[0].clientY - touchStartY;
  const threshold = 60; // px

  if (!card.classList.contains('flipped')) return; // 未翻开不响应

  if (Math.abs(dx) > threshold && Math.abs(dx) > Math.abs(dy)) {
    submitRating(dx < 0 ? 'hard' : 'easy');
  } else if (dy < -threshold && Math.abs(dy) > Math.abs(dx)) {
    submitRating('normal');
  }
});
```

---

### 6.2 音频播放控制

| 状态 | 视觉表现 |
|---|---|
| 未加载 | 播放按钮灰色，进度条骨架屏 |
| 加载中 | 播放按钮转圈动画 |
| 播放中 | 按钮变为"暂停"图标，进度条实时更新，当前句高亮 |
| 暂停 | 按钮变回"播放"图标，高亮保持 |
| 加载失败 | 按钮显示"重试"图标，Toast 提示"音频加载失败" |

**速度调节**：
```typescript
// 使用 Web Audio API 的 playbackRate 确保不失真
const audioCtx = new AudioContext();
const source = audioCtx.createBufferSource();
source.playbackRate.value = 0.75; // 0.5 / 0.75 / 1.0 / 1.25 / 1.5
```

---

### 6.3 录音交互流程

```
[等待] → 点击"开始跟读" → [请求麦克风权限]
                              ↓ 授权成功          ↓ 授权失败
                         [录音中]              [提示授权说明]
                              ↓ 点击"结束录音"    [只听模式入口]
                         [上传中，显示进度]
                              ↓ ≤5s
                         [评分结果页]
```

**麦克风权限处理**：
```typescript
async function requestMicrophone(): Promise<MediaStream | null> {
  try {
    return await navigator.mediaDevices.getUserMedia({ audio: true });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'NotAllowedError') {
      showPermissionGuide(); // 显示引导弹窗
      return null;
    }
    throw err; // 其他错误继续抛出
  }
}
```

---

### 6.4 日语输入法联动（写作练习）

写作练习的输入框需要与日语 IME（输入法）正确配合：

```typescript
// 使用 compositionstart/end 事件避免 IME 候选字期间触发验证
let isComposing = false;

input.addEventListener('compositionstart', () => isComposing = true);
input.addEventListener('compositionend', () => {
  isComposing = false;
  validateInput(); // 候选字确定后再验证
});

input.addEventListener('input', () => {
  if (!isComposing) validateInput();
});
```

---

## 七、离线支持方案

### 7.1 缓存策略

```
缓存分层：
┌────────────────────────────────────────────┐
│  Layer 1: Service Worker Cache（静态资源）  │
│  main.css, *.js, 图标、字体               │
│  策略：Cache First + 后台更新              │
├────────────────────────────────────────────┤
│  Layer 2: localStorage（用户数据）         │
│  待同步的评分事件、当日单词队列            │
│  策略：Write-through（立即存本地+异步同步）│
├────────────────────────────────────────────┤
│  Layer 3: IndexedDB（内容缓存）            │
│  已下载的单词卡片、课文、语法点           │
│  策略：预取（进入模块时提前缓存今日内容）  │
└────────────────────────────────────────────┘
```

### 7.2 离线评分同步

```typescript
// 离线时评分事件存入队列，恢复网络后批量同步
interface PendingReview {
  wordId: number;
  rating: 'easy' | 'normal' | 'hard';
  reviewedAt: string; // ISO 8601
}

function submitRating(wordId: number, rating: string) {
  const event: PendingReview = { wordId, rating, reviewedAt: new Date().toISOString() };

  if (navigator.onLine) {
    apiFetch('POST', `/api/v1/words/review/${wordId}`, { rating });
  } else {
    // 存入离线队列
    const queue: PendingReview[] = JSON.parse(localStorage.getItem('pending_reviews') || '[]');
    queue.push(event);
    localStorage.setItem('pending_reviews', JSON.stringify(queue));
    showToast('已离线保存，网络恢复后自动同步', 'info');
  }
}

// 网络恢复后批量提交
window.addEventListener('online', async () => {
  const queue: PendingReview[] = JSON.parse(localStorage.getItem('pending_reviews') || '[]');
  if (queue.length === 0) return;

  for (const event of queue) {
    await apiFetch('POST', `/api/v1/words/review/${event.wordId}`, {
      rating: event.rating,
      reviewed_at: event.reviewedAt,
    });
  }
  localStorage.removeItem('pending_reviews');
  showToast(`已同步 ${queue.length} 条学习记录`, 'success');
});
```

---

## 八、无障碍与国际化

### 8.1 无障碍（a11y）要求

| 要求 | 实现方式 |
|---|---|
| 键盘导航完整 | 所有交互元素可 Tab 聚焦；卡片评分支持方向键 |
| 屏幕阅读器支持 | 日语单词加 `lang="ja"` 属性；按钮有 `aria-label` |
| 色彩对比度 | 正文与背景对比度 ≥ 4.5:1（WCAG AA） |
| 触控目标 | 所有可点击区域 ≥ 44×44px |
| 动效开关 | 遵守 `prefers-reduced-motion`，关闭翻转动画 |

```css
@media (prefers-reduced-motion: reduce) {
  .card { transition: none; }
  .skeleton { animation: none; }
  * { transition-duration: 0.01ms !important; }
}
```

```html
<!-- 日语内容加 lang 属性，确保屏幕阅读器正确发音 -->
<span lang="ja"><ruby>勉強<rt>べんきょう</rt></ruby></span>
<span lang="zh-CN">学习</span>
```

### 8.2 字体加载策略

```css
/* 避免 FOUT（无样式文字闪烁）*/
@font-face {
  font-family: 'Noto Sans JP';
  font-display: swap; /* 立即显示回退字体，加载完成后切换 */
  src: url('/static/fonts/NotoSansJP-Regular.woff2') format('woff2');
  unicode-range: U+3000-9FFF, U+FF00-FFEF; /* 仅加载日文字符范围 */
}
```

---

## 九、前端文件与任务对应关系

| 文件 | 关联任务 | 核心职责 |
|---|---|---|
| `main.css` | T079 | 设计系统变量、全局样式、组件基础样式 |
| `api.ts` | T072 | Fetch 封装、token 注入、错误处理、离线队列 |
| `word.ts` | T073 | 卡片翻转、滑动手势、评分提交、进度更新 |
| `grammar.ts` | T074 | 检验题交互、答案收集、结果渲染、解析展开 |
| `lesson.ts` | T075 | 音频同步高亮、释义弹窗、翻译切换、生词收藏 |
| `speaking.ts` | T076 | 录音控制、波形可视化、音频上传、评分展示 |
| `writing.ts` | T077 | IME 联动、即时判题、AI 反馈轮询与展示 |
| `summary.ts` | T078 | 总结数据渲染、亮点/待改进列表、跳转逻辑 |
| `base.html` | T062 | 公共布局、导航栏、字体引用 |
| `word/index.html` | T063 | 单词卡片容器、评分按钮组 |
| `grammar/detail.html` | T066 | 步骤进度、讲解区、检验题区 |
| `lesson/detail.html` | T068 | `<ruby>` 振り仮名结构、音频控制区 |
| `speaking/index.html` | T069 | 录音按钮、Canvas 波形区、评分结果区 |
| `writing/index.html` | T070 | 输入框、AI 反馈展示区 |
| `summary/index.html` | T071 | 总结卡片、亮点/待改进列表 |
