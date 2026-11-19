import { Link } from 'react-router-dom'
import type { TodayStudyPlan } from '@/api/studyplan'
import { resolveStudyTaskPath, slotEndTime } from '@/pages/studyplan/n2Plan'
import styles from './TodayStudyCard.module.css'

interface Props {
  plan: TodayStudyPlan | null
  loading: boolean
  error: string
  pendingTask: string | null
  onToggle: (slotID: string, taskID: string, completed: boolean) => void
  onRetry?: () => void
}

export function TodayStudyCard({ plan, loading, error, pendingTask, onToggle, onRetry }: Props) {
  const tasks = plan?.slots.flatMap(slot => slot.tasks) ?? []
  const completed = tasks.filter(task => task.completed).length
  const rest = plan?.rest_day || plan?.reason === 'rest_day'
  return <section className={styles.card} aria-label="今日学习任务">
    <div className={styles.header}>
      <div><h2>📅 今日学习任务</h2>{plan?.has_plan && <p>{plan.date} · {plan.timezone}</p>}</div>
      <Link to="/study-plan" className={styles.manage}>调整计划</Link>
    </div>
    {loading ? <p className={styles.message} role="status">正在读取今日任务…</p> : <>
      {error && <div className={styles.error} role="alert"><span>{error}</span>{onRetry && <button type="button" onClick={onRetry}>重新读取</button>}</div>}
      {!error || plan ? <>
        {!plan?.has_plan ? <p className={styles.message}>还没有学习计划。<Link to="/study-plan">创建工作日 N2 计划</Link></p>
          : plan.reason === 'before_start' ? <p className={styles.message}>计划尚未开始，今天没有安排。</p>
          : plan.reason === 'ended' ? <p className={styles.message}>本期计划已结束，可以调整日期继续学习。</p>
          : rest ? <div className={styles.rest}><strong>🌿 今天休息</strong><p>今天不安排学习任务，也不需要补打卡。下个学习日继续。</p></div>
          : plan.reason === 'legacy' ? <p className={styles.message}>当前为单时段计划。可在下方编辑，或使用 N2 模板安排每天三个时段。</p>
          : <>
            <div className={styles.summary}><strong>{plan.title}</strong><span>{plan.total_minutes} 分钟 · 已完成 {completed} / {tasks.length} 项</span></div>
            <div className={styles.progress} role="progressbar" aria-label="今日任务完成进度" aria-valuemin={0} aria-valuemax={tasks.length || 1} aria-valuenow={completed}><span style={{ width: `${tasks.length ? completed * 100 / tasks.length : 0}%` }} /></div>
            <div className={styles.slots}>{plan.slots.map(slot => <section key={slot.id} className={styles.slot}>
              <div className={styles.slotHeading}><h3>{slot.title}</h3><span>{slot.time}–{slotEndTime(slot.time, slot.duration_minutes)} · {slot.duration_minutes} 分钟</span></div>
              <ul>{slot.tasks.map(task => <li key={task.id} className={task.completed ? styles.done : ''}>
                <label className={styles.check}><input type="checkbox" aria-label={`完成${slot.title}：${task.title}`} checked={task.completed} disabled={pendingTask !== null} onChange={event => onToggle(slot.id, task.id, event.target.checked)} /><span className={styles.srOnly}>{task.completed ? '已完成' : '标记完成'}</span></label>
                <div className={styles.task}><div className={styles.taskHeading}><strong>{task.title}</strong><span>{task.minutes} 分钟</span></div><p>{task.description}</p>
                  {resolveStudyTaskPath(task).startsWith('https://') ? <a href={resolveStudyTaskPath(task)} target="_blank" rel="noreferrer">打开官方练习材料 ↗</a> : <Link to={resolveStudyTaskPath(task)}>开始练习 →</Link>}
                </div>
              </li>)}</ul>
            </section>)}</div>
            <p className={styles.note} role="status">{pendingTask ? '正在保存打卡…' : '完成后勾选打卡，状态会保存到当天；不计作题目成绩。'}</p>
          </>}
      </> : null}
    </>}
  </section>
}
