package platformreg

import (
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/bluesky"
	"github.com/digkill/probot/internal/platforms/mastodon"
	"github.com/digkill/probot/internal/platforms/reddit"
	"github.com/digkill/probot/internal/platforms/telegram"
	"github.com/digkill/probot/internal/platforms/vk"
	"github.com/digkill/probot/internal/platforms/x"
)

func Default() *platforms.Registry {
	return platforms.NewRegistry(
		telegram.New(),
		vk.New(),
		bluesky.New(),
		x.New(),
		reddit.New(),
		mastodon.New(),
	)
}
