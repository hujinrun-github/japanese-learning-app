import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zh from './locales/zh'
import ja from './locales/ja'
import en from './locales/en'

const STORAGE_KEY = 'preferred_language'
const SUPPORTED = ['zh', 'ja', 'en']

function detectLanguage(): string {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored && SUPPORTED.includes(stored)) return stored
  const browser = navigator.language.split('-')[0]
  return SUPPORTED.includes(browser) ? browser : 'zh'
}

i18n.use(initReactI18next).init({
  resources: {
    zh: { translation: zh },
    ja: { translation: ja },
    en: { translation: en },
  },
  lng: detectLanguage(),
  fallbackLng: 'zh',
  interpolation: { escapeValue: false },
})

i18n.on('languageChanged', (lng) => {
  localStorage.setItem(STORAGE_KEY, lng)
})

export default i18n
