package service_test

import (
	"reflect"
	"testing"

	"opsflow/backend/internal/service"
)

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "no mentions",
			input:    "This is a comment without any mentions.",
			expected: nil,
		},
		{
			name:     "single mention",
			input:    "Hello @alice, please review.",
			expected: []string{"alice"},
		},
		{
			name:     "multiple mentions",
			input:    "Ping @alice and @bob_smith and @charlie-123",
			expected: []string{"alice", "bob_smith", "charlie-123"},
		},
		{
			name:     "trailing punctuation stripped",
			input:    "Hi @alice! How are you, @bob? What about @charlie; and @david: or @eve.",
			expected: []string{"alice", "bob", "charlie", "david", "eve"},
		},
		{
			name:     "deduplicates same handle",
			input:    "Hey @alice, and again @alice and @ALICE!",
			expected: []string{"alice"},
		},
		{
			name:     "ignores email addresses",
			input:    "Send email to support@example.com, not @support",
			expected: []string{"support"},
		},
		{
			name:     "mention at start of line",
			input:    "@lead please approve this item.",
			expected: []string{"lead"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ExtractMentions(tt.input)
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("ExtractMentions(%q) = %v, expected %v", tt.input, got, tt.expected)
			}
		})
	}
}
