package cli

import (
	"reflect"
	"testing"
)

func TestNewTTSClientSelectsProvider(t *testing.T) {
	tests := []struct {
		name     string
		cfg      TTSConfig
		wantType string
	}{
		{
			name:     "vllm default",
			cfg:      TTSConfig{Provider: "vllm"},
			wantType: "*speaking.VLLMTTSClient",
		},
		{
			name:     "style bert vits",
			cfg:      TTSConfig{Provider: "sbv"},
			wantType: "*speaking.StyleBertVITSClient",
		},
		{
			name:     "gradio",
			cfg:      TTSConfig{Provider: "gradio"},
			wantType: "*speaking.GradioTTSClient",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewTTSClient(tt.cfg)
			if typeName := reflect.TypeOf(got).String(); typeName != tt.wantType {
				t.Fatalf("NewTTSClient() type = %s, want %s", typeName, tt.wantType)
			}
		})
	}
}
