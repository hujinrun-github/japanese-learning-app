import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import type { StudyPlan as Plan, StudyPlanResult as Result } from '@/api/studyplan'
import { TodayStudyCard } from '@/components/studyplan/TodayStudyCard'
import { useTodayStudyPlan } from '@/components/studyplan/useTodayStudyPlan'
import { changeStudyWeekdays, createN2Plan, slotEndTime, STUDY_DAY_NAMES as dayNames } from './n2Plan'
import { StudySlotsEditor } from './StudySlotsEditor'
import styles from './StudyPlanPage.module.css'

const goals = [
  { key: 'words', name: '单词', unit: '个', max: 500, icon: '📖', path: '/words/review' },
  { key: 'grammar', name: '语法', unit: '项', max: 100, icon: '📝', path: '/grammar' },
  { key: 'reading', name: '阅读理解', unit: '篇', max: 100, icon: '📚', path: '/lesson' },
  { key: 'speaking', name: '口语', unit: '次', max: 100, icon: '🎙️', path: '/speaking' },
  { key: 'writing', name: '写作', unit: '篇', max: 100, icon: '✍️', path: '/writing' },
  { key: 'translation', name: '翻译', unit: '题', max: 100, icon: '🌐', path: '/translation' },
] as const
export function StudyPlanPage() {
  const [form, setForm] = useState<Plan>(createN2Plan)
  const [saved, setSaved] = useState<Result>({ plan: null, calendar_path: '' })
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [confirmAction, setConfirmAction] = useState<'delete' | 'rotate' | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)
  const today = useTodayStudyPlan(refreshKey)
  useEffect(() => {
    const controller = new AbortController()
    apiFetch<Result>('GET', '/api/v1/study-plan', undefined, controller.signal)
      .then(result => { if (!controller.signal.aborted) { setSaved(result); if (result.plan) setForm(result.plan) } })
      .catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '读取失败') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [])
  const update = <K extends keyof Plan>(key: K, value: Plan[K]) => setForm(old => ({ ...old, [key]: value }))
  const multiSlot = !!form.time_slots?.length
  const calendarURL = saved.calendar_path ? new URL(saved.calendar_path, window.location.origin).href : ''
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(''); setNotice('')
    try {
      const result = await apiFetch<Result>('PUT', '/api/v1/study-plan', form)
      setSaved(result); if (result.plan) setForm(result.plan)
      setRefreshKey(value => value + 1)
      setNotice('学习计划已保存，今日任务已更新。已订阅的日历将在下次刷新时更新。')
    } catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    finally { setBusy(false) }
  }
  async function act(action: 'delete' | 'rotate') {
    setBusy(true); setError(''); setNotice('')
    try {
      if (action === 'delete') {
        await apiFetch('DELETE', '/api/v1/study-plan'); setSaved({ plan: null, calendar_path: '' })
        setRefreshKey(value => value + 1)
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
    <header><p className={styles.eyebrow}>📅 每一天，前进一步</p><h1>学习计划</h1><p>把学习安排在工作日，按时段完成当天任务，周末安心休息。</p></header>
    <TodayStudyCard plan={today.plan} loading={today.loading} error={today.error} pendingTask={today.pendingTask} onToggle={today.toggle} onRetry={today.reload} />
    {error && <p className={styles.error} role="alert">{error}</p>}
    {notice && <p className={styles.notice} role="status">{notice}</p>}
    <div className={styles.template}>
      <div><strong>🎯 N3 → N2 工作日备考</strong><p>午间词汇 20 分钟 · 傍晚听力 20 分钟 · 晚间学习 60 分钟；周一补漏，周五测验，周末不安排。</p></div>
      <button type="button" disabled={busy} onClick={() => { setForm(createN2Plan()); setNotice('已填入 N2 工作日模板。可调整下方安排，点击「保存计划」后生效。') }}>应用 N2 工作日模板</button>
    </div>
    <div className={styles.columns}>
      <form className={styles.card} onSubmit={save}>
        <h2>{saved.plan ? '调整我的计划' : '创建我的计划'}</h2>
        <fieldset disabled={busy} className={styles.fields}>
          <label>计划名称<input required maxLength={80} value={form.title} onChange={e => update('title', e.target.value)} /></label>
          <div className={styles.row}>
            <label>开始日期<input type="date" required value={form.start_date} onChange={e => update('start_date', e.target.value)} /></label>
            <label>结束日期<input type="date" required min={form.start_date} value={form.end_date} onChange={e => update('end_date', e.target.value)} /></label>
          </div>
          <fieldset className={styles.days}><legend>学习日（未选择的日期不安排任务）</legend>{[1, 2, 3, 4, 5, 6, 0].map(day => <label key={day}><input type="checkbox" checked={form.weekdays.includes(day)} disabled={form.weekdays.length === 1 && form.weekdays.includes(day)} onChange={e => {
            const weekdays = e.target.checked ? [...form.weekdays, day].sort() : form.weekdays.filter(d => d !== day)
            setForm(old => changeStudyWeekdays(old, weekdays))
          }} />{dayNames[day]}</label>)}</fieldset>
          {multiSlot ? <StudySlotsEditor slots={form.time_slots!} weekdays={form.weekdays} onChange={slots => update('time_slots', slots)} /> : <div className={styles.row}>
            <label>每天开始时间<input type="time" required value={form.time} onChange={e => update('time', e.target.value)} /></label>
            <label>学习时长（分钟）<input type="number" required min={5} max={480} value={form.duration_minutes} onChange={e => update('duration_minutes', e.target.valueAsNumber)} /></label>
          </div>}
          <label>时区<input required list="study-timezones" value={form.timezone} onChange={e => update('timezone', e.target.value)} /><datalist id="study-timezones"><option value="Asia/Shanghai" /><option value="Asia/Tokyo" /><option value="America/New_York" /><option value="Europe/London" /></datalist></label>
          {!multiSlot && <fieldset className={styles.fields}>
            <legend>每日学习目标（填 0 表示不安排）</legend>
            <div className={styles.row}>
              {goals.map(({ key, name, unit, max }) => (
                <label key={key}>每日{name}（{unit}）
                  <input type="number" required min={0} max={max} value={Number.isNaN(form[key]) ? '' : (form[key] ?? 0)} onChange={e => update(key, e.target.valueAsNumber)} />
                </label>
              ))}
            </div>
          </fieldset>}
          <label>提前提醒（分钟，0 为准时）<input type="number" required min={0} max={10080} value={form.reminder_minutes} onChange={e => update('reminder_minutes', e.target.valueAsNumber)} /></label>
          <button type="submit" className={styles.primary}>{busy ? '处理中…' : '💾 保存计划'}</button>
          {saved.plan && <button type="button" className={styles.secondary} onClick={() => { setForm(saved.plan!); setNotice('已恢复已保存的安排。') }}>恢复已保存的安排</button>}
        </fieldset>
      </form>
      <div className={styles.asideColumn}><aside className={styles.card}>
        <h2>🍎 Apple 日历</h2>
        {saved.plan ? <>
          <p className={styles.summary}>{saved.plan.title}</p>
          <p>{saved.plan.start_date} 至 {saved.plan.end_date}</p>
          <p>{[1, 2, 3, 4, 5, 6, 0].filter(d => saved.plan!.weekdays.includes(d)).map(d => dayNames[d]).join('、')}</p>
          <p>{saved.plan.timezone}</p>
          {saved.plan.time_slots?.length ? <ul className={styles.savedSlots}>{saved.plan.time_slots.map(slot => <li key={slot.id}><strong>{slot.title}</strong><span>{slot.time}–{slotEndTime(slot.time, slot.duration_minutes)} · {slot.duration_minutes} 分钟</span></li>)}</ul> : <><p>{saved.plan.time} · 每次 {saved.plan.duration_minutes} 分钟</p><p>每天：{goals.filter(({ key }) => saved.plan![key] > 0).map(({ key, name, unit }) => `${name} ${saved.plan![key]} ${unit}`).join('、')}</p></>}
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
        <ol className={styles.help}><li>保存计划，然后点击「订阅到 Apple 日历」，在设备上确认添加。</li><li>每个学习时段各生成一条日程，休息日不生成日程。</li><li>在日历的订阅设置中开启「事件提醒」，并允许日历发送通知。</li><li>修改后再次保存，订阅日历会按设备的刷新频率更新；下载导入仅保存当次快照。</li></ol>
        <p className={styles.hint}>链接可读取你的学习安排，请勿公开分享。提醒以日历日程形式显示；此处不会创建「提醒事项」App 中的独立任务。</p>
        <div className={styles.practiceLinks}>
          {goals.map(({ key, path, name, icon }) => <Link key={key} to={path}>{icon} {name}</Link>)}
        </div>
      </aside>
      {multiSlot && <section className={styles.card}><h2>🎯 备考节奏</h2>
        <ol className={styles.phases}><li><strong>现在至 12 月底</strong><span>检查 N3 漏洞，建立每日听读习惯。</span></li><li><strong>1–3 月</strong><span>系统学习 N2，保持复习和阅读。</span></li><li><strong>4 月至 5 月中旬</strong><span>增加限时阅读和不同听力题型。</span></li><li><strong>5 月下旬至考前</strong><span>减少新增，集中练习薄弱题型。</span></li></ol>
        <p className={styles.hint}>每四周整理一次，新词和新语法减半。周末休息，漏学不加倍补课。每日一小时的夜间训练按分项进行，不包含连续 105 分钟整卷模拟。</p>
      </section>}</div>
    </div>
  </div>
}
