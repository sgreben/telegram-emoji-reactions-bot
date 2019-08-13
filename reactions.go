package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/transform"

	"golang.org/x/text/runes"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var noSpaceOrPunct = transform.Chain(
	runes.Remove(runes.In(unicode.Space)),
	runes.Remove(runes.In(unicode.Punct)),
)

type emojiReaction struct {
	Emoji string `json:"E"`
	Count int64  `json:"C"`
}

func (e *emojiReaction) ParseButton(b *telegram.InlineButton) error {
	return e.ParseButtonData(b.Data)
}

func (e *emojiReaction) ParseButtonData(data string) error {
	i := strings.IndexRune(data, '|')
	return e.Parse([]byte(data[i+1:]))
}

func (e *emojiReaction) Parse(data []byte) error {
	err := json.Unmarshal(data, e)
	if err != nil {
		return fmt.Errorf("parse data %s: %v", data, err)
	}
	return nil
}

func (e *emojiReaction) Button(id string, f func(*telegram.Callback)) telegram.InlineButton {
	label := e.Emoji
	if e.Count > 1 {
		label = fmt.Sprintf("%d %s", e.Count, e.Emoji)
	}
	dataBytes, _ := json.Marshal(e)
	data := string(dataBytes)
	idBytes := md5.Sum([]byte(id + e.Emoji))
	return telegram.InlineButton{
		Unique: fmt.Sprintf("%x", idBytes[:8]),
		Data:   string(data),
		Text:   label,
		Action: f,
	}
}

type emojiReactionsTo struct {
	UserID    *int   `json:"U,omitempty"`
	UserIDHex string `json:"H,omitempty"`
	Text      string `json:"T,omitempty"`
}

func (e *emojiReactionsTo) Parse(data []byte) {
	json.Unmarshal(data, e)
	if e.UserID != nil {
		return
	}
	userID64, _ := strconv.ParseInt(e.UserIDHex, 16, 32)
	userID := int(userID64)
	e.UserID = &userID
}

func (e *emojiReactionsTo) Encode() string {
	eCopy := *e
	eCopy.UserIDHex = fmt.Sprintf("%x", e.UserID)
	eCopy.UserID = nil
	dataBytes, _ := json.Marshal(eCopy)
	textLength := len(e.Text)
	for len(dataBytes) > 63 || textLength == 0 {
		textLength--
		eCopy.Text = e.Text[:textLength] + "…"
		dataBytes, _ = json.Marshal(eCopy)
	}
	return string(dataBytes)
}

func (e *emojiReactionsTo) Button() telegram.InlineButton {
	return telegram.InlineButton{
		Text: spaceString,
		Data: e.Encode(),
	}
}

type emojiReactions struct {
	Slice []emojiReaction
	To    emojiReactionsTo
}

func (e *emojiReactions) ParseMessage(m *telegram.Message) error {
	var buttons []telegram.InlineButton
	if len(m.ReplyMarkup.InlineKeyboard) > 0 {
		buttons = m.ReplyMarkup.InlineKeyboard[0]
	}
	if err := e.Parse(buttons); err != nil {
		return fmt.Errorf("parse message: %v", err)
	}
	return nil
}

func (e *emojiReactions) Parse(buttons []telegram.InlineButton) error {
	var errors []error
	e.Slice = nil
	if len(buttons) == 0 {
		return nil
	}
	for _, b := range buttons[:len(buttons)-1] {
		var r emojiReaction
		if err := r.ParseButton(&b); err != nil {
			errors = append(errors, err)
			continue
		}
		e.Slice = append(e.Slice, r)
	}
	if len(buttons) > 0 {
		e.To.Parse([]byte(buttons[len(buttons)-1].Data))
	}
	if len(errors) > 0 {
		return fmt.Errorf("parse buttons: %v", errors)
	}
	return nil
}

const buttonRowLength = 5

func (e *emojiReactions) Buttons(id string, f func(*telegram.Callback)) (out [][]telegram.InlineButton) {
	var row []telegram.InlineButton
	for i, r := range e.Slice {
		row = append(row, r.Button(id, f))
		if len(row) == buttonRowLength && (len(e.Slice)-i) > 1 {
			out = append(out, row)
			row = nil
		}
	}
	row = append(row, e.To.Button())
	out = append(out, row)
	return
}

func (e *emojiReactions) ReplyMarkup(id string, f func(*telegram.Callback)) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{
		InlineKeyboard: e.Buttons(id, f),
	}
}

func (e *emojiReactions) Add(emoji []string) {
	var added []emojiReaction
adding:
	for _, add := range emoji {
		for i, r := range e.Slice {
			if add == r.Emoji {
				e.Slice[i].Count++
				continue adding
			}
		}
		added = append(added, emojiReaction{
			Emoji: add,
			Count: 1,
		})
	}
	e.Slice = append(e.Slice, added...)
}
