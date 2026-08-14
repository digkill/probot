package crawler

import (
	"fmt"
	"strings"

	"github.com/digkill/probot/internal/domain"
)

func DraftObjection(m domain.Mention, brand *domain.Brand) string {
	name := "мы"
	site := ""
	if brand != nil {
		if strings.TrimSpace(brand.Name) != "" {
			name = brand.Name
		}
		site = strings.TrimSpace(brand.CanonicalURL)
	}
	topic := strings.TrimSpace(m.Title)
	if topic == "" {
		topic = "ваш отзыв"
	}
	if len([]rune(topic)) > 80 {
		topic = string([]rune(topic)[:80]) + "…"
	}
	if LooksRussian(m.Title + m.Snippet + name) {
		next := "Напишите нам в поддержку детали и ссылку на обращение — разберёмся и ответим."
		if site != "" {
			next = fmt.Sprintf("Напишите нам через %s детали и ссылку — разберёмся и ответим.", site)
		}
		return fmt.Sprintf("Спасибо, что написали об этом. Команда %s увидела претензию («%s») и не отмахивается. %s", name, topic, next)
	}
	next := "Please send our support the details and a link — we will look into it and follow up."
	if site != "" {
		next = fmt.Sprintf("Please reach us via %s with the details and a link — we will look into it and follow up.", site)
	}
	return fmt.Sprintf("Thanks for flagging this. The %s team has seen the complaint about “%s” and wants to make it right. %s", name, topic, next)
}
