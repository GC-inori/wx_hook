package i18n

import (
	"path/filepath"
	"testing"
)

func TestBothLocaleFiles(t *testing.T) {
	for _, test := range []struct{ locale, message string }{{"zh-CN", "操作成功"}, {"en-US", "Operation succeeded"}} {
		catalog, err := Load(filepath.Join("..", "..", ".."), test.locale)
		if err != nil {
			t.Fatal(err)
		}
		if got := catalog.Text("errors.success"); got != test.message {
			t.Fatalf("wrong translation: %s", got)
		}
		if got := catalog.Text("errors.unknown_action", "testAction"); got == "" || got == "errors.unknown_action" {
			t.Fatal("format substitution failed")
		}
	}
}
func TestLocaleErrorsAreExplicit(t *testing.T) {
	if _, err := Load(t.TempDir(), "zh-CN"); err == nil {
		t.Fatal("missing locale files accepted")
	}
	if _, err := Load(".", "xx-YY"); err == nil {
		t.Fatal("unknown locale accepted")
	}
}
