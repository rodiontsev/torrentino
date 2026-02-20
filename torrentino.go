package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	shuffle "torrentino/shuffle"
	tr "torrentino/transmission"

	tele "gopkg.in/telebot.v4"
	middleware "gopkg.in/telebot.v4/middleware"
)

//config

//go:embed torrentino.json
var configData []byte // or string

type TelegramConfig struct {
	Token string `json:"token"`
}

type TransmissionConfig struct {
	URL string `json:"rpc"`
}

type TorrentinoConfig struct {
	Admins []int64 `json:"admins"`
}

type Config struct {
	Telegram     *TelegramConfig     `json:"telegram"`
	Transmission *TransmissionConfig `json:"transmission"`
	Torrentino   *TorrentinoConfig   `json:"torrentino"`
}

type MessageRef struct {
	MessageID string
	ChatID    int64
}

func (m *MessageRef) Recipient() string {
	return strconv.FormatInt(m.ChatID, 10)
}

func (m *MessageRef) MessageSig() (string, int64) {
	return m.MessageID, m.ChatID
}

type Conversation struct {
	TorrentInfo   *tr.TorrentInfo
	StatusMessage *MessageRef
}

var (
	conversations sync.Map
	magnetLinkRx  = regexp.MustCompile(`^magnet:\?(.+)?`)
	torrentLinkRx = regexp.MustCompile(`^https?://(.+)?`)
)

const (
	EmojiAlienMonster             string = "👾"
	EmojiHundredPoints            string = "💯"
	EmojiEyes                     string = "👀"
	EmojiThumbsUp                 string = "👍"
	EmojiFire                     string = "🔥"
	EmojiStarStruck               string = "🤩"
	EmojiPileOfPoo                string = "💩"
	EmojiSmilingFaceWithHeartEyes string = "😍"
	EmojiSpoutingWhale            string = "🐳"
	EmojiSleepingFace             string = "😴"
	EmojiGhost                    string = "👻"
	EmojiJackOLantern             string = "🎃"
	EmojiSpeakNoEvilMonkey        string = "🙊"
	EmojiSeeNoEvilMonkey          string = "🙈"
	EmojiHearNoEvilMonkey         string = "🙉"
	EmojiChristmasTree            string = "🎄"
	EmojiSnowman                  string = "☃️"
	EmojiZanyFace                 string = "🤪"
	EmojiMoai                     string = "🗿"
	EmojiUnicorn                  string = "🦄"
	EmojiFaceWithRaisedEyebrow    string = "🤨"
)

var Emojis = []string{
	EmojiAlienMonster,
	EmojiHundredPoints,
	EmojiEyes,
	EmojiThumbsUp,
	EmojiFire,
	EmojiStarStruck,
	EmojiPileOfPoo,
	EmojiSmilingFaceWithHeartEyes,
	EmojiSpoutingWhale,
	EmojiSleepingFace,
	EmojiGhost,
	EmojiJackOLantern,
	EmojiSpeakNoEvilMonkey,
	EmojiSeeNoEvilMonkey,
	EmojiHearNoEvilMonkey,
	EmojiChristmasTree,
	EmojiSnowman,
	EmojiZanyFace,
	EmojiMoai,
	EmojiUnicorn,
}

func updateTorrentStatus(bot *tele.Bot, tr *tr.TransmissionClient) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		conversations.Range(func(key, value any) bool {
			messageRef := key.(*MessageRef)
			conversation := value.(*Conversation)

			torrentInfo := conversation.TorrentInfo

			getTorrentResponse, err := tr.GetTorrents(torrentInfo.HashString)
			if err != nil {
				log.Printf("An error occurred while retireving details for torrent %v: %v", torrentInfo, err)
				return true //continue
			}

			torrents := getTorrentResponse.Arguments.Torrents
			if len(torrents) != 1 {
				log.Printf("Exactly one torrent is expected in the response for %v", torrentInfo)
				return true //continue
			}

			torrent := torrents[0]

			text := fmt.Sprintf("<b>%s</b>\u00A0— %s, %s of %s (%.1f%%) ↓\u202F%s %s",
				html.EscapeString(torrent.Name),
				torrent.TorrentStatus(), torrent.Downloaded(), torrent.Size(), torrent.PercentDone*100,
				torrent.DownloadRate(), torrent.ETA())

			if conversation.StatusMessage != nil {
				log.Printf("Updating the status message %s in the chat %d",
					conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID)

				_, err := bot.Edit(conversation.StatusMessage, text,
					&tele.SendOptions{
						ParseMode: tele.ModeHTML,
					})
				if err != nil {
					if errors.Is(err, tele.ErrSameMessageContent) {
						log.Printf("Leaving the status message %s in the chat %d unchanged ",
							conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID)
					} else {
						log.Printf("An error occurred while updating the status message %s in the chat %d: %v",
							conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID, err)
					}
				}

				if torrent.LeftUntilDone == 0 {
					if bot.React(messageRef, messageRef, tele.Reactions{
						Reactions: []tele.Reaction{
							{
								Type:  tele.ReactionTypeEmoji,
								Emoji: EmojiHundredPoints,
							},
						},
						Big: true,
					}) != nil {
						log.Printf("An error occurred while updating the status message %s in the chat %d: %v",
							conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID, err)
					}

					//torrent is downloaded - stop updating the status message
					conversations.Delete(messageRef)
				}

				return true //continue
			}

			log.Printf("Status message does not exist. Replying to the message %s in the chat %d",
				messageRef.MessageID, messageRef.ChatID)

			//FIXME: recreating the message from MessageRef
			messageId, err := strconv.Atoi(messageRef.MessageID)
			if err != nil {
				log.Printf("An error occurred while converting the message id %s in the chat %d to int: %v",
					messageRef.MessageID, messageRef.ChatID, err)

				//delete the dodgy message from the queue
				conversations.Delete(messageRef)
				return true //continue
			}

			message := &tele.Message{
				ID: messageId,
				Chat: &tele.Chat{
					ID: messageRef.ChatID,
				},
			}
			message.ReplyTo = message

			statusMessage, err := bot.Reply(message, text,
				&tele.SendOptions{
					ParseMode: tele.ModeHTML,
				})

			if err != nil {
				log.Printf("An error occurred while replying to the message %s in the chat %d: %v",
					messageRef.MessageID, messageRef.ChatID, err)
				return true //continue
			}

			statusMessageId, statusMessageChatId := statusMessage.MessageSig()
			statusMessageRef := &MessageRef{
				MessageID: statusMessageId,
				ChatID:    statusMessageChatId,
			}

			conversation.StatusMessage = statusMessageRef

			return true
		})
	}
}

func main() {
	log.Printf("It's-a-me, Torrentino!")

	//config
	var cfg Config
	if err := json.Unmarshal(configData, &cfg); err != nil {
		log.Panic(err)
	}

	//config override
	if token := os.Getenv("TELEGRAM_API_TOKEN"); len(token) > 0 {
		log.Printf("Overriding the default Telegram token")
		cfg.Telegram.Token = token
	}

	if url := os.Getenv("TRANSMISSION_URL"); len(url) > 0 {
		log.Printf("Overriding the default Transmission URL")
		cfg.Transmission.URL = url
	}

	admins := func(s string) []int64 {
		parts := strings.Split(s, ",")

		var result []int64

		for _, p := range parts {
			if n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
				result = append(result, n)
			}
		}

		return result

	}(os.Getenv("TORRENTINO_ADMINS"))

	if len(admins) > 0 {
		log.Printf("Overriding the default list of administrators")
		cfg.Torrentino.Admins = admins
	}
	//end override

	tr := tr.CreateTransmissionClient(cfg.Transmission.URL)
	shuffle := shuffle.CreateShuffle(Emojis)

	//start
	settings := tele.Settings{
		Token:  cfg.Telegram.Token,
		Poller: &tele.LongPoller{Timeout: 20 * time.Second},
	}

	bot, err := tele.NewBot(settings)
	if err != nil {
		log.Panic(err)
	}

	helloHandler := func(ctx tele.Context) error {
		ctx.Notify(tele.Typing)

		var (
			user = ctx.Sender()
			text = ctx.Text()
		)

		log.Printf("Message received from %s %s (%d): %s", user.FirstName, user.LastName, user.ID, text)

		emoji := shuffle.Next()

		if bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
			Reactions: []tele.Reaction{
				{
					Type:  tele.ReactionTypeEmoji,
					Emoji: emoji,
				},
			},
			Big: true,
		}) != nil {
			//fall back to the message
			messageId, chatId := ctx.Message().MessageSig()

			log.Printf("Could not react to the message %s in the chat %d: %v",
				messageId, chatId, err)

			return ctx.Reply("Hello!")
		}

		return nil
	}

	addMagnetLinkHandler := func(ctx tele.Context, magnetLink string) error {
		match := magnetLinkRx.MatchString(magnetLink) || torrentLinkRx.MatchString(magnetLink)
		if !match {
			return bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
				Reactions: []tele.Reaction{
					{
						Type:  tele.ReactionTypeEmoji,
						Emoji: EmojiFaceWithRaisedEyebrow,
					},
				},
				Big: true,
			})
		}

		addTorrentReponse, err := tr.AddTorrent(magnetLink)
		if err != nil {
			log.Panic(err)
		}

		torrentInfo := addTorrentReponse.GetTorrentInfo()

		messageId, chatId := ctx.Message().MessageSig()
		messageRef := &MessageRef{
			MessageID: messageId,
			ChatID:    chatId,
		}

		conversation := &Conversation{
			TorrentInfo: torrentInfo,
		}

		conversations.Store(messageRef, conversation)

		if bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
			Reactions: []tele.Reaction{
				{
					Type:  tele.ReactionTypeEmoji,
					Emoji: EmojiAlienMonster,
				},
			},
			Big: true,
		}) != nil {
			//fall back to the message
			log.Printf("Could not react to the status message %s in the chat %d: %v",
				messageId, chatId, err)

			return ctx.Reply(
				fmt.Sprintf("Transmission added <b>%s</b> successfully", html.EscapeString(torrentInfo.Name)),
				&tele.SendOptions{
					ParseMode: tele.ModeHTML,
				},
			)
		}
		return nil
	}

	bot.Handle("/start", helloHandler)
	bot.Handle("/hello", helloHandler)

	admin := bot.Group()
	admin.Use(middleware.Whitelist(cfg.Torrentino.Admins...))
	admin.Handle("/add", func(ctx tele.Context) error {
		args := ctx.Args()

		magnetLink := ""
		if len(args) > 0 {
			magnetLink = args[0]
		}

		return addMagnetLinkHandler(ctx, magnetLink)
	})
	admin.Handle(tele.OnText, func(ctx tele.Context) error {
		//magnet link
		match := magnetLinkRx.MatchString(ctx.Text())
		if match {
			return addMagnetLinkHandler(ctx, ctx.Text())
		}

		return bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
			Reactions: []tele.Reaction{
				{
					Type:  tele.ReactionTypeEmoji,
					Emoji: EmojiFaceWithRaisedEyebrow,
				},
			},
			Big: true,
		})
	})

	go updateTorrentStatus(bot, tr)

	bot.Start()
}
