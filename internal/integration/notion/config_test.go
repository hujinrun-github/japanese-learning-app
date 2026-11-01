package notion

import "testing"

func TestNormalizeDataSourceID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "raw uuid", input: "12345678-1234-1234-1234-1234567890ab", want: "12345678-1234-1234-1234-1234567890ab"},
		{name: "raw compact id", input: "123456781234123412341234567890ab", want: "12345678-1234-1234-1234-1234567890ab"},
		{name: "url", input: "https://www.notion.so/workspace/My-Notes-123456781234123412341234567890ab", want: "12345678-1234-1234-1234-1234567890ab"},
		{name: "empty", input: "  ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeDataSourceID(tt.input); got != tt.want {
				t.Fatalf("NormalizeDataSourceID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateTokenEnv(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "default", input: "JLA_NOTION_TOKEN"},
		{name: "scoped", input: "JLA_WORK_NOTION_TOKEN"},
		{name: "empty", input: "", wantErr: false},
		{name: "arbitrary env", input: "PATH", wantErr: true},
		{name: "wrong prefix", input: "FLOWSPACE_NOTION_TOKEN", wantErr: true},
		{name: "wrong suffix", input: "JLA_TOKEN", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenEnv(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTokenEnv(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
