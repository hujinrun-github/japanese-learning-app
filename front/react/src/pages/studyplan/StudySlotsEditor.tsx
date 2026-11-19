import type { StudyTask, StudyTimeSlot } from '@/api/studyplan'
import { STUDY_DAY_NAMES } from './n2Plan'
import styles from './StudyPlanPage.module.css'

interface Props {
  slots: StudyTimeSlot[]
  weekdays: number[]
  onChange: (slots: StudyTimeSlot[]) => void
}

export function StudySlotsEditor({ slots, weekdays, onChange }: Props) {
  const updateSlot = (slotID: string, patch: Partial<StudyTimeSlot>) => onChange(slots.map(slot => slot.id === slotID ? { ...slot, ...patch } : slot))
  const updateTask = (slotID: string, taskID: string, patch: Partial<StudyTask>) => onChange(slots.map(slot => slot.id === slotID ? { ...slot, tasks: slot.tasks.map(task => task.id === taskID ? { ...task, ...patch } : task) } : slot))
  return <fieldset className={styles.fields}><legend>每天的学习时段</legend>
    <p className={styles.hint}>共 {slots.length} 个时段，合计 {slots.reduce((sum, slot) => sum + (Number.isFinite(slot.duration_minutes) ? slot.duration_minutes : 0), 0)} 分钟。任务按星期显示，完成后可在上方打卡。</p>
    {slots.map(slot => <section key={slot.id} className={styles.slotEditor}>
      <label>时段名称<input required maxLength={80} value={slot.title} onChange={e => updateSlot(slot.id, { title: e.target.value })} /></label>
      <div className={styles.row}>
        <label>{slot.title}开始时间<input type="time" required value={slot.time} onChange={e => updateSlot(slot.id, { time: e.target.value })} /></label>
        <label>{slot.title}时长（分钟）<input type="number" required min={5} max={480} value={Number.isNaN(slot.duration_minutes) ? '' : slot.duration_minutes} onChange={e => updateSlot(slot.id, { duration_minutes: e.target.valueAsNumber })} /></label>
      </div>
      <details className={styles.taskDetails}>
        <summary>📝 查看或调整{slot.title}任务</summary>
        {slot.tasks.map(task => <div key={task.id} className={styles.taskEditor}>
          <label>任务名称<input required maxLength={80} value={task.title} onChange={e => updateTask(slot.id, task.id, { title: e.target.value })} /></label>
          <label>任务说明<textarea required maxLength={1000} rows={3} value={task.description} onChange={e => updateTask(slot.id, task.id, { description: e.target.value })} /></label>
          <label>任务时长（分钟）<input type="number" required min={1} max={480} value={Number.isNaN(task.minutes) ? '' : task.minutes} onChange={e => updateTask(slot.id, task.id, { minutes: e.target.valueAsNumber })} /></label>
          <fieldset className={styles.days}><legend>{task.title}的适用星期（仅在计划学习日执行）</legend>{[1, 2, 3, 4, 5, 6, 0].map(day => <label key={day}><input type="checkbox" checked={(task.weekdays?.length ? task.weekdays : weekdays).includes(day)} disabled={(task.weekdays?.length ? task.weekdays : weekdays).length === 1 && (task.weekdays?.length ? task.weekdays : weekdays).includes(day)} onChange={e => {
            const current = task.weekdays?.length ? task.weekdays : weekdays
            const selected = e.target.checked ? [...current, day].sort() : current.filter(value => value !== day)
            if (selected.length) updateTask(slot.id, task.id, { weekdays: selected })
          }} />{STUDY_DAY_NAMES[day]}</label>)}</fieldset>
        </div>)}
      </details>
    </section>)}
  </fieldset>
}
