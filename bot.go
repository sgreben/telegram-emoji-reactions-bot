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

type emojiReactionBot struct{ *telegram.Bot }

func (bot *emojiReactionBot) init() {
	bot.Handle(telegram.OnText, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnPhoto, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnAudio, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnDocument, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnSticker, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVideo, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVoice, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVideoNote, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnContact, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnLocation, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnVenue, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnPinned, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
	bot.Handle(telegram.OnChannelPost, printAndHandleMessage(bot.addReactionPostOrAddReactionOrIgnore))
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

	option := reactions.ReplyMarkup(fmt.Sprint(m.Message.ID), bot.handleCallback)
	if _, err := bot.Edit(m.Message, m.Message.Text, option, telegram.ModeHTML); err != nil {
		log.Printf("callback %v: edit: %v", m.ID, err)
	}

	bot.notifyOfReaction(
		reaction.Emoji,
		m.Sender.Username,
		reactions.To.Text,
		&telegram.User{ID: *reactions.To.UserID},
	)
}

func (bot *emojiReactionBot) addReactionPost(m *telegram.Message, reactions *emojiReactions) {
	text := m.Text
	senderID := m.Sender.ID
	reactions.To = emojiReactionsTo{
		UserID: &senderID,
		Text:   text,
	}
	option := reactions.ReplyMarkup(fmt.Sprint(m.ID), bot.handleCallback)
	if _, err := bot.Reply(m, spaceString, telegram.Silent, option); err != nil {
		log.Printf("add reaction post: %v", err)
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
	if _, err := bot.Send(recipient, notification, telegram.Silent, telegram.ModeHTML); err != nil {
		log.Printf("notify: %v", err)
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
	reactionsPost := m.ReplyTo
	reactions := &emojiReactions{}
	if err := reactions.ParseMessage(reactionsPost); err != nil {
		log.Printf("%v: %v", m.ID, err)
	}
	reactions.Add(textEmoji)
	option := reactions.ReplyMarkup(fmt.Sprint(m.ID), bot.handleCallback)
	if _, err := bot.Edit(reactionsPost, spaceString, option); err != nil {
		log.Printf("edit: %v", err)
	}
	bot.notifyOfReaction(strings.Join(textEmoji, ""), m.Sender.Username, reactions.To.Text, &telegram.User{ID: *reactions.To.UserID})
	if err := bot.Delete(m); err != nil {
		log.Printf("delete: %v", err)
	}
}

func (bot *emojiReactionBot) addReactionPostOrAddReactionOrIgnore(m *telegram.Message) {
	switch {
	case m.Sender != nil && m.Sender.ID == bot.Me.ID:
		log.Printf("ignoring %v", m.ID)
		// ignore
	case m.IsReply() && m.ReplyTo.Sender.ID == bot.Me.ID:
		log.Printf("adding reaction or ignoring %v", m.ID)
		bot.addReactionOrIgnore(m)
	case m.IsReply() && isEmojiOnly(m):
		log.Printf("adding reaction post: %v", m.ID)
		textEmoji, _ := partitionEmoji(m.Text)
		reactions := &emojiReactions{}
		reactions.Add(textEmoji)
		bot.addReactionPost(m.ReplyTo, reactions)
		bot.notifyOfReaction(
			m.Text,
			m.Sender.Username,
			m.ReplyTo.Text,
			m.ReplyTo.Sender,
		)
		bot.Delete(m)
	}
}
