import { matchPath } from 'react-router-dom'

export interface NavigationItem {
  to: string
  labelKey: string
  icon: string
  end?: boolean
}

const items = {
  home: { to: '/', labelKey: 'nav.home', icon: '🏠', end: true },
  search: { to: '/search', labelKey: 'nav.search', icon: '🔍' },
  words: { to: '/words/review', labelKey: 'nav.words', icon: '📖' },
  grammar: { to: '/grammar', labelKey: 'nav.grammar', icon: '📝' },
  lesson: { to: '/lesson', labelKey: 'nav.lesson', icon: '📚' },
  listening: { to: '/listening', labelKey: 'nav.listening', icon: '🎧' },
  speaking: { to: '/speaking', labelKey: 'nav.speaking', icon: '🎙️' },
  writing: { to: '/writing', labelKey: 'nav.writing', icon: '✏️' },
  notes: { to: '/notes', labelKey: 'nav.notes', icon: '🗒️' },
  translation: { to: '/translation', labelKey: 'nav.translation', icon: '🌐' },
  favorites: { to: '/favorites', labelKey: 'nav.favorites', icon: '⭐' },
  mistakes: { to: '/mistakes', labelKey: 'nav.mistakes', icon: '📕' },
  studyPlan: { to: '/study-plan', labelKey: 'nav.studyPlan', icon: '📅' },
} satisfies Record<string, NavigationItem>

const learningItems = [items.words, items.grammar, items.lesson]

export type DesktopGroupID = 'learn' | 'practice' | 'organize'

interface NavigationGroup {
  id: DesktopGroupID
  labelKey: string
  items: NavigationItem[]
}

export const DESKTOP_PRIMARY_ITEMS: NavigationItem[] = [items.home, items.search]

export const DESKTOP_NAV_GROUPS: NavigationGroup[] = [
  { id: 'learn', labelKey: 'nav.groups.learn', items: learningItems },
  { id: 'practice', labelKey: 'nav.groups.practice', items: [items.listening, items.speaking, items.writing, items.translation] },
  { id: 'organize', labelKey: 'nav.groups.organize', items: [items.notes, items.favorites, items.mistakes, items.studyPlan] },
]

export function activeDesktopGroup(pathname: string): DesktopGroupID | null {
  return DESKTOP_NAV_GROUPS.find(group =>
    group.items.some(item => matchPath({ path: item.to, end: item.end ?? false }, pathname)),
  )?.id ?? null
}

export const ACCOUNT_NAV_ITEMS: NavigationItem[] = [
  items.favorites,
  { to: '/errata', labelKey: 'errata.history.title', icon: '📝' },
  items.studyPlan,
  { to: '/settings', labelKey: 'nav.settings', icon: '⚙️' },
]

export const MOBILE_PRIMARY_ITEMS: NavigationItem[] = [...DESKTOP_PRIMARY_ITEMS, ...learningItems]

export const MOBILE_MORE_ITEMS: NavigationItem[] = [
  items.listening,
  items.speaking,
  items.writing,
  items.notes,
  items.translation,
  items.mistakes,
  ...ACCOUNT_NAV_ITEMS,
]
