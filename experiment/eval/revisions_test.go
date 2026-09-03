package eval

import (
	"strings"
	"testing"
)

func TestParseGitCommitHeadUsesStdoutOnly(t *testing.T) {
	const validHead = "0123456789abcdef0123456789abcdef01234567"
	// SSH may emit a host-key warning on stderr; it is not part of stdout.
	got, err := parseGitCommitHead(validHead + "\n")
	if err != nil || got != validHead {
		t.Fatalf("head = %q, %v", got, err)
	}
}

func TestParseGitCommitHeadRejectsInvalidStdout(t *testing.T) {
	const validHead = "0123456789abcdef0123456789abcdef01234567"
	tests := []string{
		"",
		"Warning: Permanently added 'worker' to the list of known hosts.\n" + validHead,
		validHead + "\n" + validHead,
		strings.Repeat("g", 40),
		strings.Repeat("0", 39),
		strings.Repeat("0", 41),
	}
	for _, input := range tests {
		if got, err := parseGitCommitHead(input); err == nil {
			t.Fatalf("accepted invalid head %q as %q", input, got)
		}
	}
}
