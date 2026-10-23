export interface NavigationItem {
  to: string
  labelKey: string
  icon: string
  end?: boolean
}

export const DESKTOP_NAV_ITEMS: NavigationItem[] = [
  { to: '/', labelKey: 'nav.home', icon: '🏠', end: true },
  { to: '/words/review', labelKey: 'nav.words', icon: '📖' },
  { to: '/grammar', labelKey: 'nav.grammar', icon: '📝' },
  { to: '/lesson', labelKey: 'nav.lesson', icon: '📚' },
  { to: '/speaking', labelKey: 'nav.speaking', icon: '🎙️' },
  { to: '/writing', labelKey: 'nav.writing', icon: '✏️' },
  { to: '/notes', labelKey: 'nav.notes', icon: '🗒️' },
  { to: '/translation', labelKey: 'nav.translation', icon: '🌐' },
  { to: '/settings', labelKey: 'nav.settings', icon: '⚙️' },
]

export const MOBILE_PRIMARY_ITEMS: NavigationItem[] = [
  { to: '/', labelKey: 'nav.home', icon: '🏠', end: true },
  { to: '/words/review', labelKey: 'nav.words', icon: '📖' },
  { to: '/grammar', labelKey: 'nav.grammar', icon: '📝' },
  { to: '/lesson', labelKey: 'nav.lesson', icon: '📚' },
]

export const MOBILE_MORE_ITEMS: NavigationItem[] = [
  { to: '/speaking', labelKey: 'nav.speaking', icon: '🎙️' },
  { to: '/writing', labelKey: 'nav.writing', icon: '✏️' },
  { to: '/notes', labelKey: 'nav.notes', icon: '🗒️' },
  { to: '/translation', labelKey: 'nav.translation', icon: '🌐' },
  { to: '/settings', labelKey: 'nav.settings', icon: '⚙️' },
]
