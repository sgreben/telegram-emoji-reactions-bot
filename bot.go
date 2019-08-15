package main

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"

	emoji "github.com/sgreben/telegram-emoji-reactions-bot/internal/emoji"
	emojirx "github.com/sgreben/telegram-emoji-reactions-bot/pkg/reactions"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

type emojiReactionBot struct {
	*telegram.Bot
	ReactionPostCache map[int]int
	MessageCache      map[int]*telegram.Message
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

func printAndHandleMessage(f func(*telegram.Message)) func(*telegram.Message) {
	return func(m *telegram.Message) {
		jsonOut.Encode(m)
		if f == nil {
			return
		}
		f(m)
	}
}

var spaceString = "­­"

func (bot *emojiReactionBot) handleCallback(m *telegram.Callback) {
	jsonOut.Encode(m)
	reactions := &emojirx.Set{}
	if err := reactions.ParseMessage(m.Message); err != nil {
		log.Printf("callback %v: %v", m.ID, err)
	}
	reaction := &emojirx.Single{}
	if err := reaction.ParseButtonData(m.Data); err != nil {
		log.Printf("callback %v: %v", m.ID, err)
	}
	reactions.Add([]string{reaction.Emoji})
	jsonOut.Encode(reactions)
	edited, err := bot.Edit(m.Message, reactions.MessageText(), reactions.ReplyMarkup(fmt.Sprint(m.Message.ID), bot.handleCallback), telegram.ModeHTML)
	if err != nil {
		log.Printf("callback %v: edit: %v", m.ID, err)
	} else {
		jsonOut.Encode(edited)
		bot.MessageCache[edited.ID] = edited
	}
	bot.notifyOfReaction(
		reaction.Emoji,
		m.Sender,
		reactions.To.ID,
		reactions.To.ChatID,
		&telegram.User{ID: reactions.To.UserID},
	)
}

func (bot *emojiReactionBot) addReactionsMessageTo(m *telegram.Message, reactions *emojirx.Set) {
	if reactionsMessageID, ok := bot.ReactionPostCache[m.ReplyTo.ID]; ok {
		if reactionsMessage, ok := bot.MessageCache[reactionsMessageID]; ok {
			m.ReplyTo = reactionsMessage
			bot.addReactionOrIgnore(m)
			return
		}
	}
	m = m.ReplyTo
	reactions.To = emojirx.To{
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
		bot.ReactionPostCache[m.ID] = reactionsMessage.ID
		bot.MessageCache[reactionsMessage.ID] = reactionsMessage
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
	forwardedMessage, err := bot.Forward(recipient, &telegram.Message{ID: reactionToMessageID, Chat: &telegram.Chat{ID: reactionToChatID}}, telegram.Silent)
	if err != nil {
		log.Printf("notify: %v", err)
	} else {
		jsonOut.Encode(forwardedMessage)
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

func (bot *emojiReactionBot) addReactionOrIgnore(m *telegram.Message) {
	textEmoji, textWithoutEmoji := partitionEmoji(m.Text)
	if len(textEmoji) == 0 {
		log.Printf("ignoring, no emoji in %v", m.ID)
		return // no emoji, ignore
	}
	if textWithoutEmoji != "" {
		log.Printf("ignoring, not only emoji in %v", m.ID)
		return // not only emoji, ignore
	}
	defer bot.Delete(m)
	reactions := &emojirx.Set{}
	reactionsMessage := m.ReplyTo
	if err := reactions.ParseMessage(reactionsMessage); err != nil {
		log.Printf("%v: %v", m.ID, err)
	}
	reactions.Add(textEmoji)
	jsonOut.Encode(reactions)
	edited, err := bot.Edit(reactionsMessage, reactions.MessageText(), reactions.ReplyMarkup(fmt.Sprint(m.ID), bot.handleCallback), telegram.ModeHTML)
	if err != nil {
		log.Printf("edit: %v", err)
	} else {
		jsonOut.Encode(edited)
		bot.MessageCache[edited.ID] = edited
		if edited.ReplyTo != nil {
			bot.ReactionPostCache[edited.ReplyTo.ID] = edited.ID
		}
	}
	bot.notifyOfReaction(strings.Join(textEmoji, ""), m.Sender, reactions.To.ID, reactions.To.ChatID, &reactions.To)
}

func (bot *emojiReactionBot) addReactionsMessageOrAddReactionOrIgnore(m *telegram.Message) {
	switch {
	case m.Sender != nil && m.Sender.ID == bot.Me.ID:
		// ignore
	case m.IsReply() && m.ReplyTo.Sender.ID == bot.Me.ID:
		bot.addReactionOrIgnore(m)
	case m.IsReply() && isEmojiOnly(m):
		defer bot.Delete(m)
		textEmoji, _ := partitionEmoji(m.Text)
		reactions := &emojirx.Set{}
		reactions.Add(textEmoji)
		bot.addReactionsMessageTo(m, reactions)
		bot.notifyOfReaction(
			m.Text,
			m.Sender,
			m.ReplyTo.ID,
			m.ReplyTo.Chat.ID,
			m.ReplyTo.Sender,
		)
	}
}
