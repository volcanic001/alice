package ui

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownForNarrowViewport(t *testing.T) {
	model := Model{markdownCache: make(map[string]string)}
	source := "# Título\n\n**Negrita** y `código`.\n\n- uno\n- dos\n\n1. primero\n2. segundo\n\n> una cita\n\n```sh\npkg install git\n```\n\n[Termux](https://termux.dev)"

	rendered := model.renderMarkdown(source, 28)
	plain := xansi.Strip(rendered)

	for _, marker := range []string{"**", "```", "`código`", "# Título"} {
		if strings.Contains(plain, marker) {
			t.Fatalf("el marcador Markdown %q sigue visible en %q", marker, plain)
		}
	}
	for _, expected := range []string{"Título", "Negrita", "código", "• uno", "1. primero", "2. segundo", "│ una cita", "pkg install git", "Termux", "https://termux.dev"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("falta %q en el renderizado %q", expected, plain)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if width := xansi.StringWidth(line); width > 28 {
			t.Fatalf("línea de ancho %d excede el viewport: %q", width, xansi.Strip(line))
		}
	}
}

func TestRenderMarkdownCachesFinalMessagesByWidth(t *testing.T) {
	model := Model{markdownCache: make(map[string]string)}
	source := "**respuesta final**"

	first := model.renderMarkdown(source, 30)
	second := model.renderMarkdown(source, 30)
	if first != second {
		t.Fatal("el resultado cacheado cambió")
	}
	if len(model.markdownCache) != 1 {
		t.Fatalf("se esperó una entrada de caché, hay %d", len(model.markdownCache))
	}

	model.renderMarkdown(source, 20)
	if model.markdownWidth != 20 || len(model.markdownCache) != 1 {
		t.Fatalf("la caché no se reconstruyó para el nuevo ancho: width=%d entries=%d", model.markdownWidth, len(model.markdownCache))
	}
}
