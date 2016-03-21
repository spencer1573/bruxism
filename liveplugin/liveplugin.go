package liveplugin

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/iopred/bruxism"
)

const livePluginGuildID = "126798577153474560"
const livePluginChannelID = "126889593005015040"

type liveChannel struct {
	UserID           string
	UserName         string
	ChannelID        string
	DiscordChannelID string
	Live             []string
	Last             time.Time
}

type livePlugin struct {
	discord *bruxism.Discord
	youTube *bruxism.YouTube
	// Map from UserID -> liveChannel
	Live map[string]*liveChannel
}

// Name returns the name of the plugin.
func (p *livePlugin) Name() string {
	return "Live"
}

// Load will load plugin state from a byte array.
func (p *livePlugin) Load(bot *bruxism.Bot, service bruxism.Service, data []byte) error {
	if data != nil {
		if err := json.Unmarshal(data, p); err != nil {
			log.Println("Error loading data", err)
		}
	}

	go p.Run(bot, service)
	return nil
}

func (p *livePlugin) pollChannel(bot *bruxism.Bot, service bruxism.Service, lc *liveChannel) {
	liveVideos, err := p.youTube.GetLiveVideos(lc.ChannelID)
	if err != nil {
		return
	}

	live := []string{}
	for _, v := range liveVideos {
		live = append(live, v.Id)
	}

	// If this is the first time getting results, just exit.
	if lc.Live == nil {
		lc.Live = live
		return
	}

	for _, v := range live {
		found := false
		for _, v2 := range lc.Live {
			if v == v2 {
				found = true
				break
			}
		}
		if !found {
			if lc.Last.Add(30 * time.Minute).Before(time.Now()) {
				lc.Last = time.Now()
				if service.Name() == bruxism.DiscordServiceName {
					service.SendMessage(livePluginChannelID, fmt.Sprintf("<@%s> just went live: https://gaming.youtube.com/watch?v=%s", lc.UserID, v))
					if lc.DiscordChannelID != "" {
						service.SendMessage(lc.DiscordChannelID, fmt.Sprintf("<@%s> just went live: https://gaming.youtube.com/watch?v=%s", lc.UserID, v))
					}
				} else {
					service.SendMessage(livePluginChannelID, fmt.Sprintf("%s just went live: https://gaming.youtube.com/watch?v=%s", lc.UserName, v))
					if lc.DiscordChannelID != "" {
						service.SendMessage(lc.DiscordChannelID, fmt.Sprintf("%s just went live: https://gaming.youtube.com/watch?v=%s", lc.UserName, v))
					}
				}
			}
		}
	}

	lc.Live = live
}

func (p *livePlugin) poll(bot *bruxism.Bot, service bruxism.Service) {
	for _, lc := range p.Live {
		go p.pollChannel(bot, service, lc)
	}
}

// Run will poll YouTube for channels going live and send messages.
func (p *livePlugin) Run(bot *bruxism.Bot, service bruxism.Service) {
	for {
		p.poll(bot, service)
		<-time.After(1 * time.Minute)
	}

}

// Save will save plugin state to a byte array.
func (p *livePlugin) Save() ([]byte, error) {
	return json.Marshal(p)
}

// Help returns a list of help strings that are printed when the user requests them.
func (p *livePlugin) Help(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message, detailed bool) []string {
	c, err := p.discord.Session.State.Channel(message.Channel())
	if err != nil || c.GuildID != livePluginGuildID {
		return nil
	}

	if detailed {
		return []string{
			fmt.Sprintf("Announces when you go live in <#%s> as well as an optional channel.", livePluginChannelID),
			bruxism.CommandHelp(service, "setyoutubechannel", "<youtube channel id>", "Sets your youtube channel id.")[0],
			bruxism.CommandHelp(service, "setdiscordchannel", "", "Will additionally announce you going live in this channel.")[0],
			bruxism.CommandHelp(service, "unsetdiscordchannel", "", "Disables additional live announcement of channel.")[0],
			"Example:",
			fmt.Sprintf("`%ssetyoutubechannel UC392dac34_32fafe2deadbeef`", service.CommandPrefix()),
		}
	}

	return bruxism.CommandHelp(service, "setyoutubechannel", "<youtube channel id>", "Sets your youtube channel id.")
}

// Message handler.
func (p *livePlugin) Message(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	defer bruxism.MessageRecover()
	if !service.IsMe(message) {
		messageChannel := message.Channel()

		if bruxism.MatchesCommand(service, "setyoutubechannel", message) || bruxism.MatchesCommand(service, "setchannel", message) {
			query, _ := bruxism.ParseCommand(service, message)
			if len(query) == 24 && strings.Index(query, ",") == -1 {
				uid := message.UserID()

				lc, ok := p.Live[uid]
				if ok {
					lc.ChannelID = query
				} else {
					lc = &liveChannel{
						UserID:    uid,
						UserName:  message.UserName(),
						ChannelID: query,
						Live:      nil,
					}
					p.Live[uid] = lc
				}

				p.pollChannel(bot, service, lc)

				service.SendMessage(messageChannel, fmt.Sprintf("YouTube Channel ID set. A message will be posted to %s when you go live.", "https://discord.gg/0huaakl2TuIAkv97"))
			} else {
				service.SendMessage(messageChannel, "Sorry, please provide a YouTube Channel ID. eg: setyoutubechannel UC392dac34_32fafe2deadbeef")
			}
		} else if bruxism.MatchesCommand(service, "setdiscordchannel", message) {
			for _, lc := range p.Live {
				if lc.UserID == message.UserID() {
					c, err := p.discord.Session.State.Channel(messageChannel)
					if err != nil || c.GuildID == livePluginGuildID {
						service.SendMessage(messageChannel, fmt.Sprintf("Live messages are sent in <#%s>. Use this on your own server.", livePluginChannelID))
						return
					}

					lc.DiscordChannelID = messageChannel
					service.SendMessage(messageChannel, fmt.Sprintf("Discord Channel ID set. A message will be sent here when you go live."))
					return
				}
			}
			service.SendMessage(message.Channel(), "You haven't registered a YouTube Channel ID yet. eg: setyoutubechannel UC392dac34_32fafe2deadbeef")
		} else if bruxism.MatchesCommand(service, "unsetdiscordchannel", message) {
			for _, lc := range p.Live {
				if lc.UserID == message.UserID() {
					lc.DiscordChannelID = ""
					service.SendMessage(messageChannel, fmt.Sprintf("Discord Channel ID unset. messages will not be sent hire when you go live."))
					return
				}
			}
		}
	}
}

// New will create a new slow mode plugin.
func New(d *bruxism.Discord, yt *bruxism.YouTube) bruxism.Plugin {
	return &livePlugin{
		discord: d,
		youTube: yt,
		Live:    map[string]*liveChannel{},
	}
}
