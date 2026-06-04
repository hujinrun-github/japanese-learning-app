# 单词复习计划综合仪表盘设计

## 背景

当前 `/words/review` 页面负责执行单词刷卡复习。首页的“查看复习计划”入口实际也跳到 `/words/review`，用户无法在开始复习前看到今天、未来几天和不同掌握度的复习安排。

现有数据已经具备计划视图所需基础：

- `word_records.next_review_at` 表示下次复习时间。
- `word_records.mastery_level` 表示 SM-2 掌握阶段。
- `word_records.interval` 和 `ease_factor` 表示复习间隔和难度因子。
- `word_records.review_history_json` 记录历史复习事件。
- `users.daily_goals_json` 提供每日单词目标。

第一版复习计划只覆盖单词模块，不把语法、笔记、翻译混入同一页面。

## 目标

新增一个可查看单词复习安排的综合仪表盘：

- 用户可以看到今天待复习、今日目标、今日已完成、未来 7 天预计复习量。
- 用户可以看到未来 7 天每天的到期复习量、新词补充量和计划总量。
- 用户可以按状态查看单词清单：全部、今天、逾期、未来 7 天、已掌握、新词候选。
- 用户可以从计划页直接开始复习，进入现有 `/words/review` 流程。

## 非目标

- 不重新设计 SM-2 算法。
- 不新增复杂任务表或计划落库机制。
- 不在第一版支持跨模块复习计划。
- 不在第一版提供手动拖拽调整某个单词复习日期。

## 用户体验

新增路由：

```text
/words/review-plan
```

入口调整：

- 首页“查看复习计划”跳转到 `/words/review-plan`。
- 单词复习页顶部增加“查看计划”按钮。
- 计划页提供“开始复习”按钮，跳转到 `/words/review`。
- 桌面顶部导航和移动底栏继续使用 `/words/review` 作为单词主入口，避免导航变复杂。

页面结构：

1. 顶部摘要区
   - 今日待复习
   - 今日目标
   - 今日已完成
   - 未来 7 天

2. 7 天计划区
   - 每天一行或一列，显示日期、到期复习、逾期复习、新词补充、计划总量。
   - 当今天到期复习量低于每日目标时，用新词候选补齐 `new_slots`。

3. 掌握度分布区
   - 展示 0 到 5 级掌握度数量。
   - `mastery_level >= 5` 视为已掌握。

4. 单词清单区
   - 筛选：全部、今天、逾期、未来 7 天、已掌握、新词候选。
   - 每行显示单词、读音、JLPT 等级、掌握度、间隔天数、下次复习时间。

空状态：

- 没有学习记录且没有新词时，提示当前等级暂无可规划单词。
- 没有到期复习但有新词时，展示新词候选并提示可以开始学习新词。

## API 设计

新增接口：

```http
GET /api/v1/words/review-plan?level=N5&days=7
```

参数：

- `level`: JLPT 等级，默认 `N5`。
- `days`: 计划天数，默认 `7`，限制范围 `1..30`。

响应：

```json
{
  "level": "N5",
  "daily_goal": 20,
  "today_completed": 3,
  "due_count": 12,
  "new_candidates_count": 40,
  "future_due_count": 28,
  "days": [
    {
      "date": "2026-06-05",
      "due_count": 10,
      "overdue_count": 2,
      "new_slots": 8,
      "total_planned": 20
    }
  ],
  "mastery_distribution": {
    "0": 8,
    "1": 12,
    "2": 5,
    "3": 4,
    "4": 2,
    "5": 10
  },
  "items": [
    {
      "word": {
        "id": 1,
        "kanji_form": "勉強",
        "reading": "べんきょう",
        "meaning": "学习",
        "jlpt_level": "N5"
      },
      "record": {
        "mastery_level": 2,
        "next_review_at": "2026-06-05T08:00:00Z",
        "interval": 6,
        "ease_factor": 2.5
      },
      "status": "due",
      "days_until_review": 0
    }
  ]
}
```

`status` 可取值：

- `overdue`: `next_review_at` 早于今天。
- `due`: 今天到期。
- `future`: 未来 `days` 天内到期。
- `mastered`: `mastery_level >= 5`。
- `new`: 没有 `word_records` 的新词候选。

## 后端设计

新增模型：

- `ReviewPlan`
- `ReviewPlanDay`
- `ReviewPlanItem`
- `ReviewPlanStatus`

新增服务方法：

```go
func (s *WordService) GetReviewPlan(userID int64, level JLPTLevel, days int, dailyGoal int) (*ReviewPlan, error)
```

新增 store 能力：

- 查询指定等级、指定日期范围内的 `word_records` 与 `words`。
- 统计指定等级的新词候选数量。
- 拉取有限数量的新词候选用于列表展示。
- 统计掌握度分布。
- 统计今天已完成数量，口径与首页 `today_completed` 保持一致。

计划计算规则：

- `today` 使用数据库时间或服务端时间归一到本地日期边界，第一版保持与现有 `date('now')` 查询口径一致。
- 今天的 `overdue_count` 只归入今天，不分散到未来日期。
- 今天 `new_slots = max(0, daily_goal - due_count)`。
- 未来日期只显示已有记录的到期复习量，不预估今天新学词未来再次出现的量。
- `total_planned = due_count + overdue_count + new_slots`。

错误处理：

- 参数不合法返回 `400`。
- 未登录返回 `401`。
- 存储层错误用 `fmt.Errorf("...: %w", err)` 包装，并在 handler 打日志。

## 前端设计

新增文件：

```text
front/react/src/pages/word/WordReviewPlanPage.tsx
front/react/src/pages/word/WordReviewPlanPage.module.css
```

新增类型：

```text
WordReviewPlan
WordReviewPlanDay
WordReviewPlanItem
WordReviewPlanStatus
```

交互：

- 顶部 JLPT 分段按钮与现有复习页一致。
- 切换等级后重新请求计划接口。
- 筛选按钮只在前端过滤 `items`。
- “开始复习”跳转到 `/words/review`，并保留当前等级可作为后续增强。

视觉风格：

- 延续首页仪表盘风格，信息密度适中。
- 卡片圆角不超过现有设计系统。
- 每个可操作元素有文字标签。
- 移动端 7 天计划纵向排列，摘要指标改为两列或单列。

## 测试设计

后端测试优先表格驱动：

- 有逾期、今天、未来记录时，能正确分组。
- 到期词少于每日目标时，能正确计算新词补充数。
- `mastery_level >= 5` 进入已掌握分布，不误算为今天任务。
- 不同 JLPT 等级之间不串数据。
- `days` 小于 1 或大于 30 时按接口规则处理。

前端测试：

- 有数据时显示摘要、7 天计划和单词清单。
- 空数据时显示空状态。
- 筛选按钮能切换不同状态列表。
- “开始复习”按钮指向 `/words/review`。

验证命令：

```text
make test
cd front/react && npm test
cd front/react && npm run build
```

浏览器验证：

- 打开 `http://localhost:5173/words/review-plan`。
- 检查页面非空、无框架错误覆盖层。
- 检查控制台无新增应用错误。
- 切换 JLPT 等级，计划数据刷新。
- 点击筛选按钮，列表内容切换。
- 点击“开始复习”，进入 `/words/review`。

## 交付顺序

1. 后端先加失败测试，定义计划计算行为。
2. 实现 `WordService.GetReviewPlan` 和 store 查询。
3. 增加 `GET /api/v1/words/review-plan`。
4. 前端新增计划页和类型。
5. 改首页“查看复习计划”入口。
6. 在 `/words/review` 增加“查看计划”按钮。
7. 运行后端、前端和浏览器验证。
