package playbook

import "time"

type Step struct {
	PlatformSlug string `json:"platform_slug"`
	DelayMinutes int    `json:"delay_minutes"`
	Required     bool   `json:"required"`
}

type Definition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       []Step `json:"steps"`
}

func All() []Definition {
	return []Definition{
		{
			ID:          "launch_app",
			Name:        "Launch app",
			Description: "Landing → TG → VK → Bluesky → Reddit → X",
			Steps: []Step{
				{PlatformSlug: "telegram", DelayMinutes: 0, Required: true},
				{PlatformSlug: "vk", DelayMinutes: 10, Required: false},
				{PlatformSlug: "bluesky", DelayMinutes: 20, Required: false},
				{PlatformSlug: "reddit", DelayMinutes: 35, Required: false},
				{PlatformSlug: "x", DelayMinutes: 45, Required: false},
			},
		},
		{
			ID:          "video_wave",
			Name:        "Video wave",
			Description: "Short posts pointing to video URL across fast channels",
			Steps: []Step{
				{PlatformSlug: "telegram", DelayMinutes: 0, Required: true},
				{PlatformSlug: "vk", DelayMinutes: 5, Required: false},
				{PlatformSlug: "x", DelayMinutes: 15, Required: false},
				{PlatformSlug: "bluesky", DelayMinutes: 25, Required: false},
				{PlatformSlug: "mastodon", DelayMinutes: 35, Required: false},
			},
		},
		{
			ID:          "ru_tech_pr",
			Name:        "RU tech PR",
			Description: "Manual VC/Habr first, then TG/VK/X amplification",
			Steps: []Step{
				{PlatformSlug: "vc", DelayMinutes: 0, Required: false},
				{PlatformSlug: "habr", DelayMinutes: 0, Required: false},
				{PlatformSlug: "telegram", DelayMinutes: 30, Required: true},
				{PlatformSlug: "vk", DelayMinutes: 45, Required: false},
				{PlatformSlug: "x", DelayMinutes: 60, Required: false},
			},
		},
	}
}

func Get(id string) (Definition, bool) {
	for _, p := range All() {
		if p.ID == id {
			return p, true
		}
	}
	return Definition{}, false
}

func ScheduleAt(base time.Time, delayMinutes int) time.Time {
	return base.Add(time.Duration(delayMinutes) * time.Minute)
}
