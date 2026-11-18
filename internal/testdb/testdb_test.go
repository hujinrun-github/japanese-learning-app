package testdb

import "testing"

func TestValidateURLs(t *testing.T) {
	for _, tc := range []struct {
		name, test, app string
		valid           bool
	}{
		{"isolated database", "postgres://localhost/app_test", "postgres://localhost/app", true},
		{"same database with different options", "postgres://localhost/app_test?sslmode=disable", "postgres://localhost/app_test?application_name=app", false},
		{"application database", "postgres://localhost/app", "", false},
		{"reserved database", "postgres://localhost/postgres", "", false},
		{"per-run test database", "postgres://localhost/app_test_123", "postgres://localhost/app", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateURLs(tc.test, tc.app); (err == nil) != tc.valid {
				t.Fatalf("ValidateURLs error=%v, want valid=%v", err, tc.valid)
			}
		})
	}
}
