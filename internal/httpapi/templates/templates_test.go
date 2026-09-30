package templates

import (
	"strings"
	"testing"
)

func TestDefaultTemplates(t *testing.T) {
	html404 := Default404HTML()
	if html404 == "" {
		t.Fatal("Default404HTML() is empty")
	}
	if !strings.Contains(html404, "404") {
		t.Errorf("Default404HTML() missing '404'")
	}
	if !strings.Contains(html404, "<html") || !strings.Contains(html404, "</html>") {
		t.Errorf("Default404HTML() missing HTML tags")
	}

	html429 := Default429HTML()
	if html429 == "" {
		t.Fatal("Default429HTML() is empty")
	}
	if !strings.Contains(html429, "429") {
		t.Errorf("Default429HTML() missing '429'")
	}
	if !strings.Contains(html429, "<html") || !strings.Contains(html429, "</html>") {
		t.Errorf("Default429HTML() missing HTML tags")
	}
}
