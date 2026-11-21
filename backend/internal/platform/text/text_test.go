package text

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"plain", "Alice", "Alice", true},
		{"trims and collapses", "  Alice \t  Smith ", "Alice Smith", true},
		{"strips bidi override", "evil‮gnp.exe", "evilgnp.exe", true},
		{"strips control chars", "a\x00b\x07c", "abc", true},
		{"keeps zwj emoji", "👩‍💻", "👩‍💻", true},
		{"keeps zwnj (persian)", "می‌خواهم", "می‌خواهم", true},
		{"nfc normalises", "é", "é", true},
		{"empty after cleaning", "‮‬", "", false},
		{"too long", "abcdef", "abcdef", false},
		{"invalid utf8", "\xff", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Clean(tt.in, 1, 5+len(tt.want)*0+boolLen(tt.name == "too long"))
			if tt.name != "too long" {
				got, ok = Clean(tt.in, 1, 40)
			}
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Clean(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func boolLen(b bool) int {
	if b {
		return 0
	}
	return 35
}

func TestCleanMultilineKeepsNewlines(t *testing.T) {
	got, ok := CleanMultiline("hi\nthere‮", 100)
	if !ok || got != "hi\nthere" {
		t.Fatalf("got %q %v", got, ok)
	}
}
