import { Link } from 'react-router-dom'
import type { TodayStudyPlan } from '@/api/studyplan'
import { resolveStudyTaskPath, slotEndTime } from '@/pages/studyplan/n2Plan'
import { isAutomaticPracticeCheckIn, practiceTaskLink, practiceTaskProgress } from '@/pages/studyplan/practiceTask'
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
              <ul>{slot.tasks.map(task => {
                const automatic = isAutomaticPracticeCheckIn(task)
                const progress = practiceTaskProgress(task)
                const path = practiceTaskLink({ ...task, path: resolveStudyTaskPath(task) }, slot.id)
                const description = progress ? task.description.replace(/(?:[，。,]\s*)?完成后在今日任务中打卡。?$/u, '').trim() : task.description
                const remaining = progress ? Math.max(0, progress.target - progress.completed) : 0
                const unit = progress?.kind === 'grammar' ? '项' : '个'
                const action = progress?.kind === 'word_review' ? '复习' : '练习'
                return <li key={task.id} className={task.completed ? styles.done : ''}>
                  <label className={`${styles.check}${automatic ? ` ${styles.automatic}` : ''}`}><input type="checkbox" aria-label={`完成${slot.title}：${task.title}`} checked={task.completed} disabled={pendingTask !== null || automatic} onChange={event => onToggle(slot.id, task.id, event.target.checked)} /><span className={styles.srOnly}>{task.completed ? '已完成' : '标记完成'}</span></label>
                  <div className={styles.task}>
                    <div className={styles.taskHeading}><strong>{task.title}</strong><span>{task.minutes} 分钟</span></div>
                    {description && (!progress || description !== task.title) && <p>{description}</p>}
                    {progress && <p className={styles.wordProgress}>
                      {progress.level === 'all' ? '全部等级' : progress.level} · 今日 {progress.completed}/{progress.target} {unit} · {automatic ? '已自动打卡' : task.completed ? '已手动确认' : `还差 ${remaining} ${unit}，达标自动打卡`}
                    </p>}
                    {path.startsWith('https://') ? <a href={path} target="_blank" rel="noreferrer">打开官方练习材料 ↗</a> : <Link to={path}>{progress && task.completed ? `打开${action}` : progress && progress.completed > 0 ? `继续${action}` : '开始练习'} →</Link>}
                  </div>
                </li>
              })}</ul>
            </section>)}</div>
            <p className={styles.note} role="status">{pendingTask ? '正在保存打卡…' : tasks.some(practiceTaskProgress) ? '单词按已评分的不同词条计数，语法按已提交测验的不同语法点计数。达标自动打卡，也可手动确认。' : '完成后勾选打卡，状态会保存到当天；不计作题目成绩。'}</p>
          </>}
      </> : null}
    </>}
  </section>
}
