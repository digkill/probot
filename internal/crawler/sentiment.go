package crawler

import (
	"strings"
)

type weightedTerm struct {
	term   string
	weight int
}

var highNeg = []weightedTerm{
	{"scam", 3}, {"fraud", 3}, {"ripoff", 3}, {"rip-off", 3},
	{"мошен", 3}, {"развод", 3}, {"кинул", 3}, {"кидают", 3},
	{"верните деньги", 3}, {"steal", 3}, {"lawsuit", 3},
	{"не рекоменду", 3}, {"never again", 3}, {"worst", 3},
}

var midNeg = []weightedTerm{
	{"terrible", 2}, {"hate", 2}, {"broken", 2}, {"complaint", 2},
	{"refund", 2}, {"doesn't work", 2}, {"doesnt work", 2}, {"not working", 2},
	{"не работает", 2}, {"ужас", 2}, {"кошмар", 2}, {"обман", 2},
	{"support sucks", 2}, {"unusable", 2}, {"отвратитель", 2},
}

var lowNeg = []weightedTerm{
	{"bug", 1}, {"slow", 1}, {"customer service", 1}, {"проблема", 1},
	{"баг", 1}, {"дорого", 1}, {"disappoint", 1}, {"overpriced", 1},
	{"waited", 1}, {"ignored me", 1}, {"no response", 1},
}

var positiveTerms = []string{
	"love", "great", "awesome", "recommend", "best",
	"отлично", "супер", "рекомендую", "удобно", "спасибо", "люблю",
}

func Classify(title, snippet string) (sentiment, severity string) {
	hay := strings.ToLower(StripHTML(title + " " + snippet))
	neg, pos := 0, 0
	highHits := 0
	for _, t := range highNeg {
		if strings.Contains(hay, t.term) {
			neg += t.weight
			highHits++
		}
	}
	for _, t := range midNeg {
		if strings.Contains(hay, t.term) {
			neg += t.weight
		}
	}
	for _, t := range lowNeg {
		if strings.Contains(hay, t.term) {
			neg += t.weight
		}
	}
	for _, w := range positiveTerms {
		if strings.Contains(hay, w) {
			pos++
		}
	}
	switch {
	case neg == 0 && pos == 0:
		return "unknown", "none"
	case neg > pos && (highHits >= 1 && neg >= 3 || neg >= 5):
		return "negative", "high"
	case neg > pos && neg >= 3:
		return "negative", "medium"
	case neg > pos:
		return "negative", "low"
	case pos > neg:
		return "positive", "none"
	default:
		return "mixed", "low"
	}
}

func KeepForWatch(purpose, sentiment string) bool {
	if purpose != "reviews" {
		return true
	}
	return sentiment == "negative" || sentiment == "mixed"
}
