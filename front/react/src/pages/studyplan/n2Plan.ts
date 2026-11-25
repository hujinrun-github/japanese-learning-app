import type { StudyPlan, StudyTask } from '@/api/studyplan'

export const STUDY_DAY_NAMES = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
export const OFFICIAL_N2_MATERIALS = 'https://www.jlpt.jp/e/samples/sampleindex.html'

// Resolve the old built-in listening link without changing the saved schedule.
export function resolveStudyTaskPath(task: Pick<StudyTask, 'id' | 'path'>): string {
  return task.id === 'listening' && task.path === OFFICIAL_N2_MATERIALS
    ? '/listening?level=N2' : task.path
}

export function tasksForWeekday(tasks: StudyTask[], day: number, planWeekdays = [1, 2, 3, 4, 5]) {
  if (!planWeekdays.includes(day)) return []
  return tasks.filter(task => (task.weekdays?.length ? task.weekdays : planWeekdays).includes(day))
}

export function changeStudyWeekdays(plan: StudyPlan, weekdays: number[]): StudyPlan {
  return { ...plan, weekdays }
}

export function slotEndTime(time: string, minutes: number) {
  const [hour, minute] = time.split(':').map(Number)
  if (!Number.isFinite(hour) || !Number.isFinite(minute) || !Number.isFinite(minutes)) return '—'
  const end = hour * 60 + minute + minutes
  return `${String(Math.floor(end / 60) % 24).padStart(2, '0')}:${String(end % 60).padStart(2, '0')}`
}

export function createN2Plan(now = new Date()): StudyPlan {
  const start = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai' }).format(now)
  const year = Number(start.slice(0, 4))
  const endYear = start.slice(5) > '06-30' ? year + 1 : year
  return {
    title: 'N2 工作日备考计划', start_date: start, end_date: `${endYear}-06-30`,
    timezone: 'Asia/Shanghai', time: '22:00', duration_minutes: 60,
    reminder_minutes: 10, weekdays: [1, 2, 3, 4, 5],
    words: 10, grammar: 2, reading: 1, speaking: 0, writing: 0, translation: 0,
    time_slots: [
      {
        id: 'lunch', title: '午间词汇', time: '12:50', duration_minutes: 20,
        tasks: [
          { id: 'words-review', title: '词汇复习', description: '优先复习到期词；周一消化积压，周五巩固本周旧词，不要求新增。', path: '/words/review?level=N2', minutes: 20, weekdays: [1, 5] },
          { id: 'words-learn', title: '词汇复习与新词', description: '先复习到期词，有余力再学 10–15 个新词，记读音、搭配和例句。复习占满 20 分钟时暂停新词。', path: '/words/review?level=N2', minutes: 20, weekdays: [2, 3, 4] },
        ],
      },
      {
        id: 'evening', title: '傍晚听力', time: '18:40', duration_minutes: 20,
        tasks: [{ id: 'listening', title: 'N2 听力训练', description: '先盲听答题，再定位漏听句、对照文本精听，最后关闭原文复测；优先复习到期材料。', path: '/listening?level=N2', minutes: 20, weekdays: [1, 2, 3, 4, 5] }],
      },
      {
        id: 'night', title: '晚间学习', time: '22:00', duration_minutes: 60,
        tasks: [
          { id: 'catch-up', title: '补上周漏洞', description: '重做上周仍未理解的词汇、语法错题，不急于增加新内容。', path: '/mistakes', minutes: 20, weekdays: [1] },
          { id: 'grammar', title: '学习新语法', description: '学习 N2 语法的接续、含义、例句和易混表达，然后完成测验。', path: '/grammar?level=N2', minutes: 20, weekdays: [2, 3, 4], practice_target: 2 },
          { id: 'reading', title: '阅读精读', description: '先关闭假名和中文独立阅读，再核对长句、文章主旨和转折。一篇可以分两天完成。', path: '/lesson?level=N2', minutes: 30, weekdays: [1, 2, 3, 4] },
          { id: 'mistakes', title: '错题复盘', description: '重做旧错题，记录当天最需要记住的 1–3 个问题及错误原因。', path: '/mistakes', minutes: 10, weekdays: [1, 2, 3, 4] },
          { id: 'weekly-test', title: '限时分项测验', description: '使用未做过的 N2 题，按周轮换词汇语法、阅读和听力，记录正确率与用时。', path: OFFICIAL_N2_MATERIALS, minutes: 45, weekdays: [5] },
          { id: 'weekly-review', title: '测验分析与下周调整', description: '分析错题，优先安排最弱项。每四周新词、新语法减半，用于整理复习；漏学不加倍补课。', path: '/notes', minutes: 15, weekdays: [5] },
        ],
      },
    ],
  }
}
