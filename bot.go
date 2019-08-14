package main

import (
	"fmt"
	"html"
	"log"
	"strings"

	"golang.org/x/text/transform"

	emoji "github.com/sgreben/telegram-emoji-reactions-bot/internal/emoji"

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
	log.Printf("callback %v: %q", m.ID, m.Data)
	jsonOut.Encode(m)
	reactions := &emojiReactions{}
	if err := reactions.ParseMessage(m.Message); err != nil {
		log.Printf("callback %v: %v", m.ID, err)
	}
	reaction := &emojiReaction{}
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
		m.Sender.Username,
		reactions.To.Text,
		&telegram.User{ID: *reactions.To.UserID},
	)
}

func (bot *emojiReactionBot) addReactionsMessageTo(m *telegram.Message, reactions *emojiReactions) {
	if reactionsMessageID, ok := bot.ReactionPostCache[m.ReplyTo.ID]; ok {
		if reactionsMessage, ok := bot.MessageCache[reactionsMessageID]; ok {
			m.ReplyTo = reactionsMessage
			bot.addReactionOrIgnore(m)
			return
		}
	}
	m = m.ReplyTo
	text := m.Text
	senderID := m.Sender.ID
	reactions.To = emojiReactionsTo{
		UserID: &senderID,
		Text:   text,
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

func (bot *emojiReactionBot) notifyOfReaction(reaction, reactingUser string, reactionToText string, recipient telegram.Recipient) {
	notification := fmt.Sprintf(
		"%s @%s reacted to <pre>%s</pre>",
		reaction,
		reactingUser,
		html.EscapeString(reactionToText),
	)
	notificationMessage, err := bot.Send(recipient, notification, telegram.Silent, telegram.ModeHTML)
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
	log.Printf("emoji: %q", textEmoji)
	defer bot.Delete(m)
	reactions := &emojiReactions{}
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
	}
	if m.Sender != nil && reactions.To.UserID != nil {
		bot.notifyOfReaction(strings.Join(textEmoji, ""), m.Sender.Username, reactions.To.Text, &telegram.User{ID: *reactions.To.UserID})
	}
}

func (bot *emojiReactionBot) addReactionsMessageOrAddReactionOrIgnore(m *telegram.Message) {
	switch {
	case m.Sender != nil && m.Sender.ID == bot.Me.ID:
		log.Printf("ignoring %v", m.ID)
		// ignore
	case m.IsReply() && m.ReplyTo.Sender.ID == bot.Me.ID:
		log.Printf("adding reaction or ignoring %v", m.ID)
		bot.addReactionOrIgnore(m)
	case m.IsReply() && isEmojiOnly(m):
		log.Printf("adding reaction post: %v", m.ID)
		defer bot.Delete(m)
		textEmoji, _ := partitionEmoji(m.Text)
		reactions := &emojiReactions{}
		reactions.Add(textEmoji)
		bot.addReactionsMessageTo(m, reactions)
		bot.notifyOfReaction(
			m.Text,
			m.Sender.Username,
			m.ReplyTo.Text,
			m.ReplyTo.Sender,
		)
	}
}
