package api

import (
	"strings"
	"testing"
)

func TestMarkdownText(t *testing.T) {
	src := strings.Join([]string{
		"# Лабораторная работа №3",
		"",
		"## Тема: Исследование почерка",
		"",
		"**Цель:** научиться генерировать рукописные работы из *markdown*.",
		"",
		"- пункт первый",
		"* пункт второй",
		"1. нумерованный",
		"",
		"> цитата из методички",
		"",
		"```go",
		"fmt.Println(\"код остаётся как текст\")",
		"```",
		"",
		"[ссылка](https://example.com) и ![картинка](a.png)",
		"",
		"| a | b |",
		"|---|---|",
		"| 1 | 2 |",
		"",
		"---",
		"",
		"не трогать snake_case и 2*2",
	}, "\n")

	got := normalizeImportedText(markdownText(src))
	want := strings.Join([]string{
		"Лабораторная работа №3",
		"",
		"Тема: Исследование почерка",
		"",
		"Цель: научиться генерировать рукописные работы из markdown.",
		"",
		"пункт первый",
		"пункт второй",
		"нумерованный",
		"",
		"цитата из методички",
		"",
		"fmt.Println(\"код остаётся как текст\")",
		"",
		"ссылка и",
		"",
		"a b",
		"",
		"1 2",
		"",
		"не трогать snake_case и 2*2",
	}, "\n")
	if got != want {
		t.Fatalf("markdownText mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestMarkdownTextPlainSurvives(t *testing.T) {
	src := "Обычный текст без разметки.\nВторая строка."
	if got := markdownText(src); got != src {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestMarkdownTextUnclosedFence(t *testing.T) {
	if got := markdownText("```\nabc"); !strings.Contains(got, "abc") {
		t.Fatalf("content after fence lost: %q", got)
	}
}
