export interface LoginFormState {
  email: string
  password: string
}

export function applyLoginFailureFormState(form: LoginFormState): LoginFormState {
  return {
    ...form,
    password: '',
  }
}
