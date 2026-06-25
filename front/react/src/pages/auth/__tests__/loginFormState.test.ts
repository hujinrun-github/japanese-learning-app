import { describe, expect, it } from 'vitest'

import { applyLoginFailureFormState } from '../loginFormState'

describe('applyLoginFailureFormState', () => {
  it('keeps the email and clears only the password after a failed login', () => {
    expect(applyLoginFailureFormState({
      email: 'learner@example.com',
      password: 'wrong-password',
    })).toEqual({
      email: 'learner@example.com',
      password: '',
    })
  })
})
