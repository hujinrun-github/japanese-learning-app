import { useTranslation } from 'react-i18next'
import styles from './LanguageSwitcher.module.css'

const LANGS = [
  { code: 'zh', label: '中' },
  { code: 'ja', label: '日' },
  { code: 'en', label: 'EN' },
]

export function LanguageSwitcher() {
  const { i18n } = useTranslation()

  return (
    <div className={styles.switcher} aria-label="Language switcher">
      <span className={styles.globe}>🌐</span>
      {LANGS.map((lang, idx) => (
        <span key={lang.code} className={styles.item}>
          {idx > 0 && <span className={styles.divider}>|</span>}
          <button
            className={`${styles.btn} ${i18n.language === lang.code ? styles.active : ''}`}
            onClick={() => i18n.changeLanguage(lang.code)}
            aria-pressed={i18n.language === lang.code}
          >
            {lang.label}
          </button>
        </span>
      ))}
    </div>
  )
}
