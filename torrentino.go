package main

import (
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

type TelegramConfig struct {
	ApiToken string
	AdminID  int64
}

type TransmissionConfig struct {
	URL string
}

type Config struct {
	Telegram     *TelegramConfig
	Transmission *TransmissionConfig
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
	hashLinkRx    = regexp.MustCompile(`^\w{40}$`)
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
						log.Printf("Leaving the status message %s in the chat %d unchanged",
							conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID)
					} else {
						log.Printf("An error occurred while updating the status message %s in the chat %d: %v",
							conversation.StatusMessage.MessageID, conversation.StatusMessage.ChatID, err)
					}
				}

				if torrent.LeftUntilDone == 0 {
					if err := bot.React(messageRef, messageRef, tele.Reactions{
						Reactions: []tele.Reaction{
							{
								Type:  tele.ReactionTypeEmoji,
								Emoji: EmojiHundredPoints,
							},
						},
						Big: true,
					}); err != nil {
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

func config() (*Config, error) {
	readFile := func(envKey string) (string, error) {
		path := os.Getenv(envKey)
		if path == "" {
			return "", fmt.Errorf("%s environment variable is not set", envKey)
		}

		value, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", path, err)
		}

		content := strings.TrimSpace(string(value))
		if content == "" {
			return "", fmt.Errorf("file %s is empty", path)
		}

		return content, nil
	}

	adminIdStr, err := readFile("TELEGRAM_ADMIN_ID_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to load Telegram Admin ID: %w", err)
	}

	adminId, err := strconv.ParseInt(adminIdStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Telegram Admin ID: %w", err)
	}

	apiToken, err := readFile("TELEGRAM_API_TOKEN_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to load Telegram API token: %w", err)
	}

	url := strings.TrimSpace(os.Getenv("TRANSMISSION_URL"))
	if url == "" {
		return nil, fmt.Errorf("TRANSMISSION_URL environment variable is not set")
	}

	return &Config{
		Telegram: &TelegramConfig{
			AdminID:  adminId,
			ApiToken: apiToken,
		},
		Transmission: &TransmissionConfig{
			URL: url,
		},
	}, nil
}

func main() {
	log.Printf("It's-a-me, Torrentino!")

	cfg, err := config()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	tr := tr.CreateTransmissionClient(cfg.Transmission.URL)
	shuffle := shuffle.CreateShuffle(Emojis)

	//start
	settings := tele.Settings{
		Token:  cfg.Telegram.ApiToken,
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

		if err := bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
			Reactions: []tele.Reaction{
				{
					Type:  tele.ReactionTypeEmoji,
					Emoji: emoji,
				},
			},
			Big: true,
		}); err != nil {
			//fall back to the message
			messageId, chatId := ctx.Message().MessageSig()

			log.Printf("Could not react to the message %s in the chat %d: %v",
				messageId, chatId, err)

			return ctx.Reply("Hello!")
		}

		return nil
	}

	addMagnetLinkHandler := func(ctx tele.Context, magnetLink string) error {
		match := magnetLinkRx.MatchString(magnetLink) ||
			torrentLinkRx.MatchString(magnetLink) ||
			hashLinkRx.MatchString(magnetLink)

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

		if err := bot.React(ctx.Recipient(), ctx.Message(), tele.Reactions{
			Reactions: []tele.Reaction{
				{
					Type:  tele.ReactionTypeEmoji,
					Emoji: EmojiAlienMonster,
				},
			},
			Big: true,
		}); err != nil {
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
	admin.Use(middleware.Whitelist(cfg.Telegram.AdminID))
	admin.Handle("/add", func(ctx tele.Context) error {
		args := ctx.Args()

		magnetLink := ""
		if len(args) > 0 {
			magnetLink = args[0]
		}

		return addMagnetLinkHandler(ctx, magnetLink)
	})
	admin.Handle(tele.OnText, func(ctx tele.Context) error {
		return addMagnetLinkHandler(ctx, ctx.Text())
	})

	go updateTorrentStatus(bot, tr)

	bot.Start()
}
