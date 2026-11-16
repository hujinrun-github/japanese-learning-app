import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import styles from './StudyPlanPage.module.css'

type Plan = {
  title: string; start_date: string; end_date: string; time: string; timezone: string
  duration_minutes: number; reminder_minutes: number; weekdays: number[]; words: number; grammar: number
  reading: number; speaking: number; writing: number; translation: number
}
type Result = { plan: Plan | null; calendar_path: string }
const goals = [
  { key: 'words', name: '单词', unit: '个', max: 500, icon: '📖', path: '/words/review' },
  { key: 'grammar', name: '语法', unit: '项', max: 100, icon: '📝', path: '/grammar' },
  { key: 'reading', name: '阅读理解', unit: '篇', max: 100, icon: '📚', path: '/lesson' },
  { key: 'speaking', name: '口语', unit: '次', max: 100, icon: '🎙️', path: '/speaking' },
  { key: 'writing', name: '写作', unit: '篇', max: 100, icon: '✍️', path: '/writing' },
  { key: 'translation', name: '翻译', unit: '题', max: 100, icon: '🌐', path: '/translation' },
] as const
const dayNames = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
function dateString(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
function initialPlan(): Plan {
  const start = new Date(), end = new Date()
  end.setDate(end.getDate() + 29)
  return { title: '日语学习计划', start_date: dateString(start), end_date: dateString(end), time: '20:00',
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai', duration_minutes: 30,
    reminder_minutes: 10, weekdays: [1, 2, 3, 4, 5], words: 20, grammar: 2, reading: 0, speaking: 0, writing: 0, translation: 0 }
}

export function StudyPlanPage() {
  const [form, setForm] = useState<Plan>(initialPlan)
  const [saved, setSaved] = useState<Result>({ plan: null, calendar_path: '' })
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [confirmAction, setConfirmAction] = useState<'delete' | 'rotate' | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    apiFetch<Result>('GET', '/api/v1/study-plan', undefined, controller.signal)
      .then(result => { setSaved(result); if (result.plan) setForm(result.plan) })
      .catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '读取失败') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [])
  const update = <K extends keyof Plan>(key: K, value: Plan[K]) => setForm(old => ({ ...old, [key]: value }))
  const calendarURL = saved.calendar_path ? new URL(saved.calendar_path, window.location.origin).href : ''
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(''); setNotice('')
    try {
      const result = await apiFetch<Result>('PUT', '/api/v1/study-plan', form)
      setSaved(result); if (result.plan) setForm(result.plan)
      setNotice('学习计划已保存。已订阅的日历将在下次刷新时更新。')
    } catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    finally { setBusy(false) }
  }
  async function act(action: 'delete' | 'rotate') {
    setBusy(true); setError(''); setNotice('')
    try {
      if (action === 'delete') {
        await apiFetch('DELETE', '/api/v1/study-plan'); setSaved({ plan: null, calendar_path: '' })
        setNotice('计划已删除。请在 Apple 日历中取消旧订阅，移除设备中的日程。')
      } else {
        setSaved(await apiFetch<Result>('POST', '/api/v1/study-plan/calendar-token'))
        setNotice('链接已重置。请取消旧订阅，并使用新链接重新订阅。')
      }
      setConfirmAction(null)
    } catch (err) { setError(err instanceof Error ? err.message : '操作失败') }
    finally { setBusy(false) }
  }
  if (loading) return <p role="status">正在读取学习计划…</p>
  return <div className={styles.page}>
    <header><p className={styles.eyebrow}>📅 每一天，前进一步</p><h1>学习计划</h1><p>安排固定的日语学习时间，让 Apple 日历在开始前提醒你。</p></header>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {notice && <p className={styles.notice} role="status">{notice}</p>}
    <div className={styles.columns}>
      <form className={styles.card} onSubmit={save}>
        <h2>{saved.plan ? '调整我的计划' : '创建我的计划'}</h2>
        <fieldset disabled={busy} className={styles.fields}>
          <label>计划名称<input required maxLength={80} value={form.title} onChange={e => update('title', e.target.value)} /></label>
          <div className={styles.row}>
            <label>开始日期<input type="date" required value={form.start_date} onChange={e => update('start_date', e.target.value)} /></label>
            <label>结束日期<input type="date" required min={form.start_date} value={form.end_date} onChange={e => update('end_date', e.target.value)} /></label>
          </div>
          <fieldset className={styles.days}><legend>学习日</legend>{[1, 2, 3, 4, 5, 6, 0].map(day => <label key={day}><input type="checkbox" checked={form.weekdays.includes(day)} onChange={e => update('weekdays', e.target.checked ? [...form.weekdays, day].sort() : form.weekdays.filter(d => d !== day))} />{dayNames[day]}</label>)}</fieldset>
          <div className={styles.row}>
            <label>每天开始时间<input type="time" required value={form.time} onChange={e => update('time', e.target.value)} /></label>
            <label>学习时长（分钟）<input type="number" required min={5} max={480} value={form.duration_minutes} onChange={e => update('duration_minutes', e.target.valueAsNumber)} /></label>
          </div>
          <label>时区<input required list="study-timezones" value={form.timezone} onChange={e => update('timezone', e.target.value)} /><datalist id="study-timezones"><option value="Asia/Shanghai" /><option value="Asia/Tokyo" /><option value="America/New_York" /><option value="Europe/London" /></datalist></label>
          <fieldset className={styles.fields}>
            <legend>每日学习目标（填 0 表示不安排）</legend>
            <div className={styles.row}>
              {goals.map(({ key, name, unit, max }) => (
                <label key={key}>每日{name}（{unit}）
                  <input type="number" required min={0} max={max} value={Number.isNaN(form[key]) ? '' : (form[key] ?? 0)} onChange={e => update(key, e.target.valueAsNumber)} />
                </label>
              ))}
            </div>
          </fieldset>
          <label>提前提醒（分钟，0 为准时）<input type="number" required min={0} max={10080} value={form.reminder_minutes} onChange={e => update('reminder_minutes', e.target.valueAsNumber)} /></label>
          <button type="submit" className={styles.primary}>{busy ? '处理中…' : '💾 保存计划'}</button>
        </fieldset>
      </form>
      <aside className={styles.card}>
        <h2>🍎 Apple 日历</h2>
        {saved.plan ? <>
          <p className={styles.summary}>{saved.plan.title}</p>
          <p>{saved.plan.start_date} 至 {saved.plan.end_date}</p>
          <p>{[1, 2, 3, 4, 5, 6, 0].filter(d => saved.plan!.weekdays.includes(d)).map(d => dayNames[d]).join('、')} · {saved.plan.time}</p>
          <p>{saved.plan.timezone} · 每次 {saved.plan.duration_minutes} 分钟</p>
          <p>每天：{goals.filter(({ key }) => saved.plan![key] > 0).map(({ key, name, unit }) => `${name} ${saved.plan![key]} ${unit}`).join('、')}</p>
          <a className={styles.primary} href={calendarURL.replace(/^https?:/, 'webcal:')} rel="noreferrer">📅 订阅到 Apple 日历</a>
          <a className={styles.secondary} href={calendarURL} download="study-plan.ics" rel="noreferrer">📥 下载 .ics 文件</a>
          <label>专属订阅地址<input readOnly value={calendarURL} onFocus={e => e.target.select()} /></label>
          <button type="button" disabled={busy} className={styles.secondary} onClick={async () => {
            try { await navigator.clipboard.writeText(calendarURL); setNotice('订阅链接已复制。') }
            catch { setError('复制失败，请选中上面的地址手动复制。') }
          }}>📋 复制订阅链接</button>
          <div className={styles.actions}><button disabled={busy} onClick={() => setConfirmAction('rotate')}>🔄 重置链接</button><button disabled={busy} onClick={() => setConfirmAction('delete')}>🗑️ 删除计划</button></div>
          {confirmAction && <div className={styles.confirm} role="alert"><p>{confirmAction === 'delete' ? '删除后旧链接将失效，还需在 Apple 日历中取消订阅。确认删除？' : '重置后旧链接立即失效，需要重新订阅。确认重置？'}</p><button disabled={busy} onClick={() => act(confirmAction)}>确认{confirmAction === 'delete' ? '删除' : '重置'}</button><button disabled={busy} onClick={() => setConfirmAction(null)}>取消</button></div>}
        </> : <p className={styles.empty}>保存计划后，这里会生成你的专属日历订阅链接。</p>}
        <ol className={styles.help}><li>保存计划，然后点击「订阅到 Apple 日历」，在设备上确认添加。</li><li>在日历的订阅设置中开启「事件提醒」，并允许日历发送通知。</li><li>修改后再次保存，订阅日历会按设备的刷新频率更新；下载导入仅保存当次快照。</li></ol>
        <p className={styles.hint}>链接可读取你的学习安排，请勿公开分享。提醒以日历日程形式显示；此处不会创建「提醒事项」App 中的独立任务。</p>
        <div className={styles.practiceLinks}>
          {goals.map(({ key, path, name, icon }) => <Link key={key} to={path}>{icon} {name}</Link>)}
        </div>
      </aside>
    </div>
  </div>
}
