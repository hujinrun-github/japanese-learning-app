import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../../contexts/AuthContext'
import { getVolume, setVolume } from '../../util/audioVolume'
import { updateProfile, changePassword, updateDailyGoals, getStats } from '../../api/user'
import { confirmNoteSyncDeletion, createNoteSyncTarget, deleteNoteSyncTarget, listNoteSyncDeletions, listNoteSyncTargets, pullNoteSyncTarget, pushNoteSyncTarget, restoreNoteSyncDeletion, testNoteSyncTarget, updateNoteSyncTarget } from '../../api/noteSync'
import type { JLPTLevel, NoteSyncResult, NoteSyncState, NoteSyncTarget, NoteSyncTargetInput } from '../../types/api'
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

  // Daily goals
  const [dailyWord, setDailyWord] = useState(20)
  const [dailyGrammar, setDailyGrammar] = useState(5)
  const [dailySpeaking, setDailySpeaking] = useState(3)
  const [dailyWriting, setDailyWriting] = useState(3)
  const [dailyTranslation, setDailyTranslation] = useState(5)
  const [dailyGoalMsg, setDailyGoalMsg] = useState<{ text: string; ok: boolean } | null>(null)

  // Notion sync target
  const [syncTargets, setSyncTargets] = useState<NoteSyncTarget[]>([])
  const [selectedSyncTargetID, setSelectedSyncTargetID] = useState<number | null>(null)
  const [syncName, setSyncName] = useState('')
  const [syncDataSourceID, setSyncDataSourceID] = useState('')
  const [syncTokenEnv, setSyncTokenEnv] = useState('JLA_NOTION_TOKEN')
  const [syncTitleProperty, setSyncTitleProperty] = useState('Name')
  const [syncTagsProperty, setSyncTagsProperty] = useState('Tags')
  const [syncFlowIDProperty, setSyncFlowIDProperty] = useState('')
  const [syncTags, setSyncTags] = useState('')
  const [syncMsg, setSyncMsg] = useState<{ text: string; ok: boolean } | null>(null)
  const [syncBusy, setSyncBusy] = useState(false)
  const [pendingDeletes, setPendingDeletes] = useState<NoteSyncState[]>([])
  const [syncResult, setSyncResult] = useState<NoteSyncResult | null>(null)

  useEffect(() => {
    getStats().then(stats => {
      setDailyWord(stats.modules.word?.daily_goal ?? 20)
      setDailyGrammar(stats.modules.grammar?.daily_goal ?? 5)
      setDailySpeaking(stats.modules.speaking?.daily_goal ?? 3)
      setDailyWriting(stats.modules.writing?.daily_goal ?? 5)
      setDailyTranslation(stats.modules.translation?.daily_goal ?? 5)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    listNoteSyncTargets().then(setSyncTargets).catch(() => setSyncTargets([]))
  }, [])

  const toggleLevel = (level: JLPTLevel) => {
    setSelectedLevels(prev =>
      prev.includes(level) ? prev.filter(l => l !== level) : [...prev, level]
    )
  }

  const handleSaveDailyGoals = async () => {
    try {
      await updateDailyGoals({
        word: dailyWord,
        grammar: dailyGrammar,
        speaking: dailySpeaking,
        writing: dailyWriting,
        translation: dailyTranslation,
      })
      setDailyGoalMsg({ text: t('settings.saveDailyGoalsSuccess'), ok: true })
    } catch {
      setDailyGoalMsg({ text: 'Error', ok: false })
    }
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

  const resetSyncForm = () => {
    setSelectedSyncTargetID(null)
    setSyncName('')
    setSyncDataSourceID('')
    setSyncTokenEnv('JLA_NOTION_TOKEN')
    setSyncTitleProperty('Name')
    setSyncTagsProperty('Tags')
    setSyncFlowIDProperty('')
    setSyncTags('')
    setSyncMsg(null)
    setPendingDeletes([])
    setSyncResult(null)
  }

  const refreshPendingDeletes = async (targetID: number) => {
    try {
      setPendingDeletes((await listNoteSyncDeletions(targetID)) ?? [])
    } catch {
      setPendingDeletes([])
    }
  }

  const applySyncTarget = (target: NoteSyncTarget) => {
    setSelectedSyncTargetID(target.id)
    setSyncName(target.name)
    setSyncDataSourceID(target.data_source_id)
    setSyncTokenEnv(target.token_env)
    setSyncTitleProperty(target.title_property)
    setSyncTagsProperty(target.tags_property)
    setSyncFlowIDProperty(target.flow_id_property)
    setSyncTags(target.sync_tags.join(', '))
    void refreshPendingDeletes(target.id)
  }

  const selectSyncTarget = (value: string) => {
    if (!value) {
      resetSyncForm()
      return
    }
    const target = syncTargets.find(item => item.id === Number(value))
    if (!target) return
    applySyncTarget(target)
    setSyncMsg(null)
  }

  const syncTargetInput = (): NoteSyncTargetInput => ({
    name: syncName.trim(),
    data_source_id: syncDataSourceID.trim(),
    token_env: syncTokenEnv.trim(),
    title_property: syncTitleProperty.trim(),
    tags_property: syncTagsProperty.trim(),
    flow_id_property: syncFlowIDProperty.trim(),
    sync_tags: syncTags.split(',').map(tag => tag.trim()).filter(Boolean),
  })

  const handleSaveSyncTarget = async () => {
    setSyncBusy(true)
    setSyncMsg(null)
    try {
      const input = syncTargetInput()
      const saved = selectedSyncTargetID
        ? await updateNoteSyncTarget(selectedSyncTargetID, input)
        : await createNoteSyncTarget(input)
      setSyncTargets(prev => selectedSyncTargetID
        ? prev.map(target => target.id === saved.id ? saved : target)
        : [saved, ...prev])
      applySyncTarget(saved)
      setSyncMsg({ text: t('settings.notionSyncSaved'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handlePushSyncTarget = async () => {
    if (!selectedSyncTargetID) return
    setSyncBusy(true)
    setSyncMsg(null)
    try {
      const result = await pushNoteSyncTarget(selectedSyncTargetID)
      setSyncResult(result)
      setSyncMsg({ text: t('settings.notionSyncCompleted'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handlePullSyncTarget = async () => {
    if (!selectedSyncTargetID) return
    setSyncBusy(true)
    setSyncMsg(null)
    try {
      const result = await pullNoteSyncTarget(selectedSyncTargetID)
      setSyncResult(result)
      await refreshPendingDeletes(selectedSyncTargetID)
      setSyncMsg({ text: t('settings.notionSyncCompleted'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handleConfirmPendingDelete = async (item: NoteSyncState) => {
    if (!selectedSyncTargetID || !window.confirm(t('settings.notionSyncConfirmDelete'))) return
    setSyncBusy(true)
    try {
      await confirmNoteSyncDeletion(selectedSyncTargetID, item.note_id)
      await refreshPendingDeletes(selectedSyncTargetID)
      setSyncMsg({ text: t('settings.notionSyncDeleted'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handleRestorePendingDelete = async (item: NoteSyncState) => {
    if (!selectedSyncTargetID) return
    setSyncBusy(true)
    try {
      await restoreNoteSyncDeletion(selectedSyncTargetID, item.note_id)
      await refreshPendingDeletes(selectedSyncTargetID)
      setSyncMsg({ text: t('settings.notionSyncRestored'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handleTestSyncTarget = async () => {
    setSyncBusy(true)
    setSyncMsg(null)
    try {
      await testNoteSyncTarget(syncTargetInput())
      setSyncMsg({ text: t('settings.notionSyncTestSuccess'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
    }
  }

  const handleDeleteSyncTarget = async () => {
    if (!selectedSyncTargetID || !window.confirm(t('settings.notionSyncDeleteConfirm'))) return
    setSyncBusy(true)
    try {
      await deleteNoteSyncTarget(selectedSyncTargetID)
      setSyncTargets(prev => prev.filter(target => target.id !== selectedSyncTargetID))
      resetSyncForm()
      setSyncMsg({ text: t('settings.notionSyncDeleted'), ok: true })
    } catch (err) {
      setSyncMsg({ text: err instanceof Error ? err.message : t('settings.notionSyncError'), ok: false })
    } finally {
      setSyncBusy(false)
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

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.dailyGoals')}</h2>
        <div className={styles.goalGrid}>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.dailyGoalWord')}</label>
            <input className={styles.input} type="number" min={0} max={200} value={dailyWord} onChange={e => setDailyWord(Number(e.target.value))} />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.dailyGoalGrammar')}</label>
            <input className={styles.input} type="number" min={0} max={50} value={dailyGrammar} onChange={e => setDailyGrammar(Number(e.target.value))} />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.dailyGoalSpeaking')}</label>
            <input className={styles.input} type="number" min={0} max={20} value={dailySpeaking} onChange={e => setDailySpeaking(Number(e.target.value))} />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.dailyGoalWriting')}</label>
            <input className={styles.input} type="number" min={0} max={20} value={dailyWriting} onChange={e => setDailyWriting(Number(e.target.value))} />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.dailyGoalTranslation')}</label>
            <input className={styles.input} type="number" min={0} max={50} value={dailyTranslation} onChange={e => setDailyTranslation(Number(e.target.value))} />
          </div>
        </div>
        <button className={styles.saveBtn} onClick={handleSaveDailyGoals}>
          {t('settings.saveDailyGoals')}
        </button>
        {dailyGoalMsg && (
          <div className={`${styles.msg} ${dailyGoalMsg.ok ? styles.msgSuccess : styles.msgError}`}>
            {dailyGoalMsg.text}
          </div>
        )}
      </div>

      <div className={styles.card}>
        <h2 className={styles.cardTitle}>🔗 {t('settings.notionSync')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.notionSyncTarget')}</label>
          <select className={styles.input} value={selectedSyncTargetID ?? ''} onChange={e => selectSyncTarget(e.target.value)}>
            <option value="">{t('settings.notionSyncNew')}</option>
            {syncTargets.map(target => <option key={target.id} value={target.id}>{target.name}</option>)}
          </select>
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.notionSyncName')}</label>
          <input className={styles.input} value={syncName} onChange={e => setSyncName(e.target.value)} placeholder={t('settings.notionSyncNamePlaceholder')} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.notionSyncDataSource')}</label>
          <input className={styles.input} value={syncDataSourceID} onChange={e => setSyncDataSourceID(e.target.value)} placeholder="https://www.notion.so/..." />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.notionSyncTokenEnv')}</label>
          <input className={styles.input} list="notion-token-envs" value={syncTokenEnv} onChange={e => setSyncTokenEnv(e.target.value)} />
          <datalist id="notion-token-envs"><option value="JLA_NOTION_TOKEN" /></datalist>
          <small>{t('settings.notionSyncTokenHint')}</small>
        </div>
        <div className={styles.goalGrid}>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.notionSyncTitleProperty')}</label>
            <input className={styles.input} value={syncTitleProperty} onChange={e => setSyncTitleProperty(e.target.value)} />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.notionSyncTagsProperty')}</label>
            <input className={styles.input} value={syncTagsProperty} onChange={e => setSyncTagsProperty(e.target.value)} />
          </div>
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.notionSyncFilter')}</label>
          <input className={styles.input} value={syncTags} onChange={e => setSyncTags(e.target.value)} placeholder={t('settings.notionSyncFilterPlaceholder')} />
        </div>
        <div className={styles.buttonRow}>
          <button className={styles.saveBtn} onClick={handleSaveSyncTarget} disabled={syncBusy || !syncName.trim() || !syncDataSourceID.trim()}>{t('settings.notionSyncSave')}</button>
          <button className={styles.secondaryBtn} onClick={handleTestSyncTarget} disabled={syncBusy || !syncDataSourceID.trim()}>{t('settings.notionSyncTest')}</button>
          {selectedSyncTargetID && <>
            <button className={styles.secondaryBtn} onClick={handlePushSyncTarget} disabled={syncBusy}>{t('settings.notionSyncPush')}</button>
            <button className={styles.secondaryBtn} onClick={handlePullSyncTarget} disabled={syncBusy}>{t('settings.notionSyncPull')}</button>
          </>}
          {selectedSyncTargetID && <button className={styles.dangerBtn} onClick={handleDeleteSyncTarget} disabled={syncBusy}>{t('settings.notionSyncDelete')}</button>}
        </div>
        {selectedSyncTargetID && <p className={styles.syncHint}>{syncTargets.find(target => target.id === selectedSyncTargetID)?.has_token ? t('settings.notionSyncTokenConfigured') : t('settings.notionSyncTokenMissing')}</p>}
        {syncMsg && <div className={`${styles.msg} ${syncMsg.ok ? styles.msgSuccess : styles.msgError}`}>{syncMsg.text}</div>}
        {syncResult && <p className={styles.syncHint}>{t('settings.notionSyncStats')}: {t('settings.notionSyncCreated')} {syncResult.created}, {t('settings.notionSyncUpdated')} {syncResult.updated}, {t('settings.notionSyncImported')} {syncResult.imported}, {t('settings.notionSyncPending')} {syncResult.pending_delete}</p>}
        {pendingDeletes.length > 0 && <div className={styles.pendingList}>
          <h3 className={styles.pendingTitle}>{t('settings.notionSyncPendingTitle')}</h3>
          {pendingDeletes.map(item => (
            <div className={styles.pendingItem} key={`${item.target_id}-${item.note_id}`}>
              <span>{t('settings.notionSyncNote')} #{item.note_id}</span>
              <div className={styles.buttonRow}>
                <button className={styles.secondaryBtn} onClick={() => handleRestorePendingDelete(item)} disabled={syncBusy}>{t('settings.notionSyncRestore')}</button>
                <button className={styles.dangerBtn} onClick={() => handleConfirmPendingDelete(item)} disabled={syncBusy}>{t('settings.notionSyncConfirmDeleteAction')}</button>
              </div>
            </div>
          ))}
        </div>}
      </div>
    </div>
  )
}
