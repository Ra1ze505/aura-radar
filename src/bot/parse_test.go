package bot

import "testing"

func TestClipPreview(t *testing.T) {
	if got := clipPreview("  hello  ", 10); got != "hello" {
		t.Fatalf("short %q", got)
	}
	got := clipPreview("абвгдеёжзий", 3)
	if got != "абв" {
		t.Fatalf("clip %q", got)
	}
}
