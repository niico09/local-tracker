package ui

import (
	"bytes"
	"testing"
)

func TestGeneratedAppCSSIsNonTrivial(t *testing.T) {
	data, err := files.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("read embedded static/app.css: %v", err)
	}
	if len(data) < 1000 {
		t.Errorf("app.css is %d bytes; run `make css`", len(data))
	}
	if !bytes.Contains(data, []byte("tailwindcss")) {
		t.Error("app.css missing tailwindcss marker; run `make css`")
	}
}
