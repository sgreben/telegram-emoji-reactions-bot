package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"

	emoji "github.com/sgreben/telegram-emoji-reactions-bot/internal/emoji"
	emojirx "github.com/sgreben/telegram-emoji-reactions-bot/pkg/reactions"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var spaceString = string([]byte{0xc2, 0xad, 0xc2, 0xad})

type emojiReactionBotCaches struct {
	ReactionMessageIDFor map[string]int
	MessageFor           map[string]*telegram.Message
	ForwardedFor         map[string]*telegram.Message
	Mu                   sync.RWMutex
}

type emojiReactionBot struct {
	*telegram.Bot
	*emojiReactionBotCaches
}

func (bot *emojiReactionBot) init() {
	bot.Handle(telegram.OnText, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnPhoto, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnAudio, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnDocument, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnSticker, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVideo, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVoice, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVideoNote, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnContact, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnLocation, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVenue, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnPinned, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnChannelPost, printAndHandleMessage(bot.addReactionsMessageOrAddReactionOrIgnore))
	bot.Handle(telegram.OnCallback, bot.handleCallback)
	bot.Handle(telegram.OnAddedToGroup, printAndHandleMessage(nil))
}

func (bot *emojiReactionBot) MessageForRead(chatID int64, messageID int) (*telegram.Message, bool) {
	k := globalMessageID(chatID, messageID)
	bot.Mu.RLock()
	defer bot.Mu.RUnlock()
	m, ok := bot.MessageFor[k]
	return m, ok
}

func (bot *emojiReactionBot) MessageForWrite(m *telegram.Message) {
	k := globalMessageID(m.Chat.ID, m.ID)
	bot.Mu.Lock()
	defer bot.Mu.Unlock()
	bot.MessageFor[k] = m
}

func (bot *emojiReactionBot) NotificationForwardCacheRead(chatID int64, messageID int) (*telegram.Message, bool) {
	k := globalMessageID(chatID, messageID)
	bot.Mu.RLock()
	defer bot.Mu.RUnlock()
	m, ok := bot.ForwardedFor[k]
	return m, ok
}

func (bot *emojiReactionBot) NotificationForwardCacheWrite(chatID int64, messageID int, forwardedMessage *telegram.Message) {
	k := globalMessageID(chatID, messageID)
	bot.Mu.Lock()
	defer bot.Mu.Unlock()
	bot.ForwardedFor[k] = forwardedMessage
}

func (bot *emojiReactionBot) ReactionMessageIDForRead(chatID int64, messageID int) (int, bool) {
	k := globalMessageID(chatID, messageID)
	bot.Mu.RLock()
	defer bot.Mu.RUnlock()
	m, ok := bot.ReactionMessageIDFor[k]
	return m, ok
}

func (bot *emojiReactionBot) ReactionMessageIDForWrite(chatID int64, messageID int, reactionMessageID int) {
	k := globalMessageID(chatID, messageID)
	bot.Mu.Lock()
	defer bot.Mu.Unlock()
	bot.ReactionMessageIDFor[k] = reactionMessageID
}

func globalMessageID(chatID int64, messageID int) string {
	return fmt.Sprintf("%x:%x", chatID, messageID)
}

func printAndHandleMessage(f func(*telegram.Message)) func(*telegram.Message) {
	return func(m *telegram.Message) {
		jsonOut.Encode(m)
		if f != nil {
			f(m)
		}
	}
}

func (bot *emojiReactionBot) handleCallback(m *telegram.Callback) {
	jsonOut.Encode(m)
	reactions := newReactionSet()
	if err := reactions.ParseMessage(m.Message); err != nil {
		log.Printf("callback %v: %v", m.ID, err)
	}
	reaction := &emojirx.Single{}
	if err := reaction.ParseButtonData(m.Data); err != nil {
		log.Printf("callback %v: %v", m.ID, err)
	}
	added, _ := reactions.AddOrRemove(m.Sender.ID, []string{reaction.Emoji})
	jsonOut.Encode(reactions)
	edited, err := bot.Edit(m.Message, reactions.MessageText(), reactions.ReplyMarkup(fmt.Sprint(m.Message.ID), bot.handleCallback), telegram.ModeHTML)
	if err != nil {
		log.Printf("callback %v: edit: %v", m.ID, err)
	} else {
		jsonOut.Encode(edited)
		bot.MessageForWrite(edited)
	}
	if added > 0 {
		bot.notifyOfReaction(reaction.Emoji, m.Sender, reactions.To.ID, reactions.To.ChatID, &telegram.User{ID: reactions.To.UserID})
	}
}

func (bot *emojiReactionBot) addReactionsMessageTo(m *telegram.Message, reactions *emojirx.Set) {
	if reactionsMessageID, ok := bot.ReactionMessageIDForRead(m.ReplyTo.Chat.ID, m.ReplyTo.ID); ok {
		if reactionsMessage, ok := bot.MessageForRead(m.Chat.ID, reactionsMessageID); ok {
			m.ReplyTo = reactionsMessage
			bot.addReactionOrIgnore(m)
			return
		}
	}
	m = m.ReplyTo
	reactions.To = &emojirx.To{
		UserID: m.Sender.ID,
		ChatID: m.Chat.ID,
		ID:     m.ID,
	}
	jsonOut.Encode(reactions)
	reactionsMessage, err := bot.Reply(m, reactions.MessageText(), reactions.ReplyMarkup(fmt.Sprint(m.ID), bot.handleCallback), telegram.Silent, telegram.ModeHTML)
	if err != nil {
		log.Printf("add reaction post: %v", err)
	} else {
		jsonOut.Encode(reactionsMessage)
		bot.ReactionMessageIDForWrite(m.Chat.ID, m.ID, reactionsMessage.ID)
		bot.MessageForWrite(reactionsMessage)
	}
}

func partitionEmoji(s string) ([]string, string) {
	noSpaceOrPunct := transform.Chain(
		runes.Remove(runes.In(unicode.Space)),
		runes.Remove(runes.In(unicode.Punct)),
	)
	textEmoji := emoji.FindAll(s)
	textWithoutEmoji, _, _ := transform.String(noSpaceOrPunct, emoji.RemoveAll(s))
	var add []string
	for _, r := range textEmoji {
		if e, ok := r.Match.(emoji.Emoji); ok {
			add = append(add, e.Value)
		}
	}
	return add, textWithoutEmoji
}

func (bot *emojiReactionBot) notifyOfReaction(reaction string, reactingUser *telegram.User, reactionToMessageID int, reactionToChatID int64, recipient telegram.Recipient) {
	who := fmt.Sprintf("%s %s", reactingUser.FirstName, reactingUser.LastName)
	if reactingUser.Username != "" {
		who = fmt.Sprintf("@%s", reactingUser.Username)
	}
	var forwardedMessage *telegram.Message
	if m, ok := bot.NotificationForwardCacheRead(reactionToChatID, reactionToMessageID); ok {
		forwardedMessage = m
		jsonOut.Encode(forwardedMessage)
	} else {
		var err error
		forwardedMessage, err = bot.Forward(recipient, &telegram.Message{ID: reactionToMessageID, Chat: &telegram.Chat{ID: reactionToChatID}}, telegram.Silent)
		if err != nil {
			log.Printf("notify: %v", err)
		} else {
			jsonOut.Encode(forwardedMessage)
			bot.NotificationForwardCacheWrite(reactionToChatID, reactionToMessageID, forwardedMessage)
		}
	}
	notification := fmt.Sprintf("%s %s reacted", reaction, who)
	notificationMessage, err := bot.Reply(forwardedMessage, notification, telegram.Silent)
	if err != nil {
		log.Printf("notify: %v", err)
	} else {
		jsonOut.Encode(notificationMessage)
	}
}

func isEmojiOnly(m *telegram.Message) bool {
	textEmoji, textWithoutEmoji := partitionEmoji(m.Text)
	if len(textEmoji) == 0 {
		return false
	}
	if textWithoutEmoji != "" {
		return false
	}
	return true
}

func newReactionSet() *emojirx.Set {
	out := &emojirx.Set{Previous: &emojirx.Previous{}}
	out.Config.ButtonRowLength = config.ButtonRowLength
	out.Config.ButtonRowMinLength = config.ButtonRowMinLength
	return out
}

func (bot *emojiReactionBot) addReactionOrIgnore(m *telegram.Message) {
	textEmoji, textWithoutEmoji := partitionEmoji(m.Text)
	switch {
	case len(textWithoutEmoji) == 0 && len(textEmoji) > 0:
	// ok
	case len(textWithoutEmoji) == 1 && len(textEmoji) == 0:
		textEmoji = []string{m.Text} // ok
	default:
		return // ignore
	}
	defer bot.Delete(m)
	reactions := newReactionSet()
	reactionsMessage := m.ReplyTo
	if err := reactions.ParseMessage(reactionsMessage); err != nil {
		log.Printf("%v: %v", m.ID, err)
	}
	added, _ := reactions.AddOrRemove(m.Sender.ID, textEmoji)
	jsonOut.Encode(reactions)
	edited, err := bot.Edit(reactionsMessage, reactions.MessageText(), reactions.ReplyMarkup(fmt.Sprint(m.ID), bot.handleCallback), telegram.ModeHTML)
	if err != nil {
		log.Printf("edit: %v", err)
	} else {
		jsonOut.Encode(edited)
		bot.MessageForWrite(edited)
		if edited.ReplyTo != nil {
			bot.ReactionMessageIDForWrite(edited.Chat.ID, edited.ReplyTo.ID, edited.ID)
		}
	}
	if added > 0 {
		bot.notifyOfReaction(strings.Join(textEmoji, ""), m.Sender, reactions.To.ID, reactions.To.ChatID, reactions.To)
	}
}

func (bot *emojiReactionBot) addReactionsMessageOrAddReactionOrIgnore(m *telegram.Message) {
	switch {
	case m.Sender != nil && m.Sender.ID == bot.Me.ID:
		// ignore
	case m.IsReply() && m.ReplyTo.Sender.ID == bot.Me.ID:
		bot.addReactionOrIgnore(m)
	case m.IsReply() && len(m.Text) == 1:
		defer bot.Delete(m)
		reactions := newReactionSet()
		added, _ := reactions.AddOrRemove(m.Sender.ID, []string{m.Text})
		bot.addReactionsMessageTo(m, reactions)
		if added > 0 {
			bot.notifyOfReaction(
				m.Text,
				m.Sender,
				m.ReplyTo.ID,
				m.ReplyTo.Chat.ID,
				m.ReplyTo.Sender,
			)
		}
	case m.IsReply() && isEmojiOnly(m):
		defer bot.Delete(m)
		textEmoji, _ := partitionEmoji(m.Text)
		reactions := newReactionSet()
		added, _ := reactions.AddOrRemove(m.Sender.ID, textEmoji)
		bot.addReactionsMessageTo(m, reactions)
		if added > 0 {
			bot.notifyOfReaction(
				m.Text,
				m.Sender,
				m.ReplyTo.ID,
				m.ReplyTo.Chat.ID,
				m.ReplyTo.Sender,
			)
		}
	}
}
