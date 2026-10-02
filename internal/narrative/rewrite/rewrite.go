// Package rewrite 处理玩家文字里的禁用词（v0.2.0-rc1）：优先请 AI 改写整句（保留意图、去掉违规设定），
// 没有 AI 或改写不合格时用模板改写——有改写词的替换为改写词，没有的删掉含禁用词的整个分句，
// 并清理标点，避免出现“会一点，”这样的残句。
package rewrite

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

// AI 是 AI 改写函数：输入原文与禁用词，返回改写后的文本。
type AI func(ctx context.Context, text string, forbidden []string) (string, error)

// Contains 报告文字是否含有任一禁用词。
func Contains(text string, forbidden []string) bool {
	for _, f := range forbidden {
		if f != "" && strings.Contains(text, f) {
			return true
		}
	}
	return false
}

var sentenceEnd = "。！？!?\n"
var clauseSep = "，,；;、"

// Template 用模板改写：有 rewrites[词] 的替换，否则删掉含禁用词的分句。
func Template(text string, forbidden []string, rewrites map[string]string) string {
	if !Contains(text, forbidden) {
		return text
	}
	for _, f := range forbidden {
		if r, ok := rewrites[f]; ok && f != "" && r != "" && !Contains(r, forbidden) {
			text = strings.ReplaceAll(text, f, r)
		}
	}
	if !Contains(text, forbidden) {
		return tidy(text)
	}
	var out strings.Builder
	for _, sent := range splitKeep(text, sentenceEnd) {
		body, end := cutEnd(sent, sentenceEnd)
		if !Contains(body, forbidden) {
			out.WriteString(sent)
			continue
		}
		var kept []string
		var seps []string
		for _, cl := range splitKeep(body, clauseSep) {
			c, sep := cutEnd(cl, clauseSep)
			if Contains(c, forbidden) || strings.TrimSpace(c) == "" {
				continue
			}
			kept = append(kept, c)
			seps = append(seps, sep)
		}
		if len(kept) == 0 {
			continue // 整句都是违规设定：删去
		}
		for i, c := range kept {
			out.WriteString(c)
			if i < len(kept)-1 {
				sep := seps[i]
				if sep == "" {
					sep = "，"
				}
				out.WriteString(sep)
			}
		}
		out.WriteString(end)
	}
	return tidy(out.String())
}

// Rewrite 先请 AI 改写，结果仍含禁用词、为空或过长时退回模板改写。返回文本与来源（ai / template / none）。
func Rewrite(ctx context.Context, ai AI, text string, forbidden []string, rewrites map[string]string, maxRunes int) (string, string) {
	if !Contains(text, forbidden) {
		return text, "none"
	}
	if ai != nil {
		if r, err := ai(ctx, text, forbidden); err == nil {
			r = tidy(strings.TrimSpace(r))
			if r != "" && !Contains(r, forbidden) && (maxRunes <= 0 || utf8.RuneCountInString(r) <= maxRunes) {
				return r, "ai"
			}
		}
	}
	t := Template(text, forbidden, rewrites)
	if maxRunes > 0 {
		if rs := []rune(t); len(rs) > maxRunes {
			t = string(rs[:maxRunes])
		}
	}
	return t, "template"
}

// splitKeep 按分隔符切分并保留分隔符在片段末尾。
func splitKeep(s, seps string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if strings.ContainsRune(seps, r) {
			end := i + utf8.RuneLen(r)
			out = append(out, s[start:end])
			start = end
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func cutEnd(s, seps string) (string, string) {
	r, n := utf8.DecodeLastRuneInString(s)
	if n > 0 && strings.ContainsRune(seps, r) {
		return s[:len(s)-n], s[len(s)-n:]
	}
	return s, ""
}

var (
	dupComma   = regexp.MustCompile(`[，,；;、]{2,}`)
	commaEnd   = regexp.MustCompile(`[，,；;、]+([。！？!?])`)
	leadPunct  = regexp.MustCompile(`^[，,；;、。！？!?\s]+`)
	trailComma = regexp.MustCompile(`[，,；;、\s]+$`)
)

// tidy 清理改写后的标点：合并连续逗号、去掉句末逗号与开头的标点。
func tidy(s string) string {
	s = dupComma.ReplaceAllStringFunc(s, func(m string) string { r, _ := utf8.DecodeRuneInString(m); return string(r) })
	s = commaEnd.ReplaceAllString(s, "$1")
	s = leadPunct.ReplaceAllString(s, "")
	s = trailComma.ReplaceAllString(s, "")
	if strings.TrimSpace(s) != "" && !strings.ContainsRune(sentenceEnd, lastRune(s)) && strings.ContainsAny(s, "。！？") {
		s += "。"
	}
	return strings.TrimSpace(s)
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}
