import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useReducer,
  type ReactNode,
} from 'react'
import type { User } from '@/types/api'

interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
}

type AuthAction =
  | { type: 'LOGIN'; payload: { token: string; user: User } }
  | { type: 'LOGOUT' }
  | { type: 'RESTORE'; payload: { token: string; user: User } }

function authReducer(_state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'LOGIN':
    case 'RESTORE':
      return {
        user: action.payload.user,
        token: action.payload.token,
        isAuthenticated: true,
      }
    case 'LOGOUT':
      return { user: null, token: null, isAuthenticated: false }
  }
}

interface AuthContextValue extends AuthState {
  login: (token: string, user: User) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

const STORAGE_TOKEN_KEY = 'token'
const STORAGE_USER_KEY = 'user'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [_state, dispatch] = useReducer(authReducer, {
    user: null,
    token: null,
    isAuthenticated: false,
  })

  // Restore session from localStorage on mount
  useEffect(() => {
    const token = localStorage.getItem(STORAGE_TOKEN_KEY)
    const userRaw = localStorage.getItem(STORAGE_USER_KEY)
    if (token && userRaw) {
      try {
        const user = JSON.parse(userRaw) as User
        dispatch({ type: 'RESTORE', payload: { token, user } })
      } catch {
        localStorage.removeItem(STORAGE_TOKEN_KEY)
        localStorage.removeItem(STORAGE_USER_KEY)
      }
    }
  }, [])

  const login = useCallback((token: string, user: User) => {
    localStorage.setItem(STORAGE_TOKEN_KEY, token)
    localStorage.setItem(STORAGE_USER_KEY, JSON.stringify(user))
    dispatch({ type: 'LOGIN', payload: { token, user } })
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(STORAGE_TOKEN_KEY)
    localStorage.removeItem(STORAGE_USER_KEY)
    dispatch({ type: 'LOGOUT' })
  }, [])

  return (
    <AuthContext.Provider value={{ ..._state, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within <AuthProvider>')
  }
  return ctx
}
