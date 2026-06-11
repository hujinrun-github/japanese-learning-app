package main

import "testing"

func TestPasswordResetMailerSettings(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want mailerSettings
	}{
		{
			name: "uses Resend when API key is set",
			env: map[string]string{
				"RESEND_API_KEY": "re_test_key",
				"RESEND_FROM":    "noreply@example.com",
				"SMTP_HOST":      "smtp.other.test",
				"SMTP_PORT":      "2525",
				"SMTP_USER":      "other-user",
				"SMTP_PASS":      "other-pass",
			},
			want: mailerSettings{
				provider: "resend",
				host:     "smtp.resend.com",
				port:     "587",
				user:     "resend",
				pass:     "re_test_key",
				from:     "noreply@example.com",
			},
		},
		{
			name: "falls back to SMTP_FROM for Resend sender",
			env: map[string]string{
				"RESEND_API_KEY": "re_test_key",
				"SMTP_FROM":      "reset@example.com",
			},
			want: mailerSettings{
				provider: "resend",
				host:     "smtp.resend.com",
				port:     "587",
				user:     "resend",
				pass:     "re_test_key",
				from:     "reset@example.com",
			},
		},
		{
			name: "uses generic SMTP when Resend is not configured",
			env: map[string]string{
				"SMTP_HOST": "smtp.example.com",
				"SMTP_PORT": "2525",
				"SMTP_USER": "smtp-user",
				"SMTP_PASS": "smtp-pass",
				"SMTP_FROM": "smtp-from@example.com",
			},
			want: mailerSettings{
				provider: "smtp",
				host:     "smtp.example.com",
				port:     "2525",
				user:     "smtp-user",
				pass:     "smtp-pass",
				from:     "smtp-from@example.com",
			},
		},
		{
			name: "uses stub when no mail provider is configured",
			env:  map[string]string{},
			want: mailerSettings{provider: "stub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}

			got := passwordResetMailerSettings(getenv)
			if got != tt.want {
				t.Fatalf("passwordResetMailerSettings() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
