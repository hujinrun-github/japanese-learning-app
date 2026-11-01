import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Spinner } from '@/components/ui/Spinner'
import type { DailyPlan, DailyPlanItem, DailyPlanItemStatus, DailyPlanModuleKey } from '@/types/api'
import { isPlanComplete, pickNextPlanItem } from './dailyPlan'
import styles from './DailyPlanCard.module.css'

const MODULE_META: Record<DailyPlanModuleKey, { icon: string; labelKey: string; tone: string }> = {
  word: { icon: '📖', labelKey: 'home.modules.word', tone: 'blue' },
  grammar: { icon: '📗', labelKey: 'home.modules.grammar', tone: 'green' },
  speaking: { icon: '🎙', labelKey: 'home.modules.speaking', tone: 'violet' },
  writing: { icon: '✍️', labelKey: 'home.modules.writing', tone: 'amber' },
}

const STATUS_CLASS: Record<DailyPlanItemStatus, string> = {
  done: styles.statusDone,
  in_progress: styles.statusInProgress,
  todo: styles.statusTodo,
}

interface DailyPlanCardProps {
  plan: DailyPlan | null
  loading: boolean
}

export function DailyPlanCard({ plan, loading }: DailyPlanCardProps) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <section className={styles.card}>
        <div className={styles.header}>
          <h2>{t('home.dailyPlan.title')}</h2>
        </div>
        <div className={styles.loading}>
          <Spinner size="md" />
        </div>
      </section>
    )
  }

  if (!plan || plan.items.length === 0) {
    return (
      <section className={styles.card}>
        <div className={styles.header}>
          <h2>{t('home.dailyPlan.title')}</h2>
        </div>
        <p className={styles.empty}>{t('home.dailyPlan.empty')}</p>
        <Link to="/settings" className={styles.emptyAction}>{t('home.dailyPlan.setGoals')}</Link>
      </section>
    )
  }

  const done = isPlanComplete(plan)
  const nextItem = pickNextPlanItem(plan)

  return (
    <section className={styles.card}>
      <div className={styles.header}>
        <h2>{t('home.dailyPlan.title')}</h2>
        <span className={done ? styles.summaryDone : styles.summary}>
          {t('home.todayProgress', { completed: plan.total_completed, goal: plan.total_goal })}
        </span>
      </div>

      <div className={styles.overallBar} aria-label={t('home.todayProgress', { completed: plan.total_completed, goal: plan.total_goal })}>
        <span className={done ? styles.overallFillDone : styles.overallFill} style={{ width: `${plan.percent}%` }} />
      </div>

      {done ? (
        <p className={styles.allDone}>{t('home.dailyPlan.allDone')}</p>
      ) : null}

      <ul className={styles.list}>
        {plan.items.map(item => (
          <DailyPlanRow key={item.key} item={item} />
        ))}
      </ul>

      {nextItem ? (
        <Link to={nextItem.path} className={styles.primaryAction}>
          {t('home.dailyPlan.continueNext', { module: t(MODULE_META[nextItem.key].labelKey) })}
          <span aria-hidden="true">→</span>
        </Link>
      ) : null}
    </section>
  )
}

function DailyPlanRow({ item }: { item: DailyPlanItem }) {
  const { t } = useTranslation()
  const meta = MODULE_META[item.key]

  return (
    <li>
      <Link to={item.path} className={`${styles.row} ${STATUS_CLASS[item.status]}`}>
        <span className={styles.icon} aria-hidden="true">{meta.icon}</span>

        <span className={styles.body}>
          <strong>{t(meta.labelKey)}</strong>
          <small>
            {t('home.todayProgress', { completed: item.completed, goal: item.goal })}
            {item.status === 'done' ? '' : ` · ${t('home.dailyPlan.remaining', { count: item.remaining })}`}
            {item.due_count > 0 ? ` · ${t('home.dailyPlan.due', { count: item.due_count })}` : ''}
          </small>
        </span>

        <span className={styles.trailing}>
          <em className={styles.statusChip}>{t(`home.dailyPlan.status.${item.status}`)}</em>
          <span className={styles.bar} aria-hidden="true">
            <span style={{ width: `${item.percent}%` }} />
          </span>
        </span>
      </Link>
    </li>
  )
}
