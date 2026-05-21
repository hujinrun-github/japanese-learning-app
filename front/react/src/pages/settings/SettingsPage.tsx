import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../../contexts/AuthContext'
import { getVolume, setVolume } from '../../util/audioVolume'
import { updateProfile, changePassword } from '../../api/user'
import type { JLPTLevel } from '../../types/api'
import styles from './SettingsPage.module.css'

const ALL_LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user, updateUser } = useAuth()

  // Account form
  const [name, setName] = useState(user?.name ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [selectedLevels, setSelectedLevels] = useState<JLPTLevel[]>(user?.jlpt_levels ?? ['N5'])
  const [profileMsg, setProfileMsg] = useState<{ text: string; ok: boolean } | null>(null)

  // Password form
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwMsg, setPwMsg] = useState<{ text: string; ok: boolean } | null>(null)

  // Volume
  const [volume, setVol] = useState(getVolume() * 100)

  const toggleLevel = (level: JLPTLevel) => {
    setSelectedLevels(prev =>
      prev.includes(level) ? prev.filter(l => l !== level) : [...prev, level]
    )
  }

  const handleSaveProfile = async () => {
    try {
      const updated = await updateProfile(name.trim(), email.trim(), selectedLevels)
      updateUser(updated)
      setProfileMsg({ text: t('settings.saveProfileSuccess'), ok: true })
    } catch {
      setProfileMsg({ text: 'Error', ok: false })
    }
  }

  const handleSavePassword = async () => {
    if (newPw !== confirmPw) {
      setPwMsg({ text: t('settings.passwordMismatch'), ok: false })
      return
    }
    try {
      await changePassword(currentPw, newPw)
      setPwMsg({ text: t('settings.savePasswordSuccess'), ok: true })
      setCurrentPw(''); setNewPw(''); setConfirmPw('')
    } catch (e: any) {
      const msg = e?.code === 'ERR_WRONG_PASSWORD' ? t('settings.wrongPassword') : 'Error'
      setPwMsg({ text: msg, ok: false })
    }
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('settings.title')}</h1>

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.account')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.name')}</label>
          <input className={styles.input} value={name} onChange={e => setName(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.email')}</label>
          <input className={styles.input} value={email} onChange={e => setEmail(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.jlptLevels')}</label>
          <div className={styles.tags}>
            {ALL_LEVELS.map(l => (
              <button
                key={l}
                type="button"
                className={`${styles.tag} ${selectedLevels.includes(l) ? styles.tagSelected : ''}`}
                onClick={() => toggleLevel(l)}
              >
                {l}
              </button>
            ))}
          </div>
        </div>
        <button className={styles.saveBtn} onClick={handleSaveProfile}>
          {t('settings.saveProfile')}
        </button>
        {profileMsg && (
          <div className={`${styles.msg} ${profileMsg.ok ? styles.msgSuccess : styles.msgError}`}>
            {profileMsg.text}
          </div>
        )}
      </div>

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.password')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.currentPassword')}</label>
          <input className={styles.input} type="password" value={currentPw} onChange={e => setCurrentPw(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.newPassword')}</label>
          <input className={styles.input} type="password" value={newPw} onChange={e => setNewPw(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.confirmPassword')}</label>
          <input className={styles.input} type="password" value={confirmPw} onChange={e => setConfirmPw(e.target.value)} />
        </div>
        <button className={styles.saveBtn} onClick={handleSavePassword}>
          {t('settings.savePassword')}
        </button>
        {pwMsg && (
          <div className={`${styles.msg} ${pwMsg.ok ? styles.msgSuccess : styles.msgError}`}>
            {pwMsg.text}
          </div>
        )}
      </div>

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.audio')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.volume')}: {Math.round(volume)}%</label>
          <input
            type="range"
            className={styles.slider}
            min={0}
            max={100}
            value={volume}
            onChange={e => {
              const v = Number(e.target.value)
              setVol(v)
              setVolume(v / 100)
            }}
          />
        </div>
      </div>
    </div>
  )
}
