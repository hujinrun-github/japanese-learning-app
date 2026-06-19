import { Link, useParams } from 'react-router-dom'
import styles from './ShadowingPage.module.css'

export function ShadowingPage() {
  const { id } = useParams()

  return (
    <div className={styles.page}>
      <Link className={styles.backLink} to={id ? `/lesson/${id}` : '/lesson'}>
        返回课文
      </Link>
      <h1 className={styles.title}>影子跟读</h1>
      <p className={styles.intro}>音频跟读页面正在准备中。</p>
    </div>
  )
}
