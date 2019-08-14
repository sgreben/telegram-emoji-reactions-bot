package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
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

func (e *emojiReactionsTo) ParseURL(link string) (err error) {
	linkURL, err := url.Parse(link)
	if err != nil {
		return err
	}
	data := linkURL.Query().Get("data")
	return e.Parse([]byte(data))
}

func (e *emojiReactionsTo) Parse(data []byte) (err error) {
	err = json.Unmarshal(data, e)
	if err != nil {
		return
	}
	userID64, err := strconv.ParseInt(e.UserIDHex, 16, 32)
	userID := int(userID64)
	e.UserID = &userID
	return
}

const maxMetadataLength = 4096 - 128

func (e *emojiReactionsTo) Encode() string {
	shortened := *e
	if shortened.UserID == nil {
		zero := 0
		shortened.UserID = &zero
	}
	shortened.UserIDHex = fmt.Sprintf("%x", *shortened.UserID)
	shortened.UserID = nil
	dataBytes, _ := json.Marshal(shortened)
	textLength := len(e.Text)
	for len(dataBytes) >= maxMetadataLength && textLength > 0 {
		textLength--
		shortened.Text = e.Text[:textLength] + "…"
		dataBytes, _ = json.Marshal(shortened)
	}
	return string(dataBytes)
}

func (e *emojiReactionsTo) MessageText() string {
	data := url.QueryEscape(e.Encode())
	return fmt.Sprintf(`<a href="http://example.com?data=%s">`+spaceString+`</a>`, data)
}

type emojiReactions struct {
	Slice []emojiReaction
	To    emojiReactionsTo
}

func (e *emojiReactions) ParseMessage(m *telegram.Message) error {
	if err := e.ParseButtons(m.ReplyMarkup.InlineKeyboard); err != nil {
		return fmt.Errorf("parse message: %v", err)
	}
	for _, entity := range m.Entities {
		if entity.Type == telegram.EntityTextLink {
			if err := e.To.ParseURL(entity.URL); err != nil {
				return fmt.Errorf("parse link %q: %v", entity.URL, err)
			}
		}
	}
	return nil
}

func (e *emojiReactions) ParseButtons(rows [][]telegram.InlineButton) error {
	var errors []error
	e.Slice = nil
	for _, buttons := range rows {
		for _, b := range buttons {
			var r emojiReaction
			if err := r.ParseButton(&b); err != nil {
				errors = append(errors, err)
				continue
			}
			e.Slice = append(e.Slice, r)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("parse buttons: %v", errors)
	}
	return nil
}

func (e *emojiReactions) Buttons(id string, f func(*telegram.Callback)) (out [][]telegram.InlineButton) {
	var row []telegram.InlineButton
	for i, r := range e.Slice {
		row = append(row, r.Button(id, f))
		if len(row) == config.ButtonRowLength && (len(e.Slice)-i) >= config.ButtonRowMinLength {
			out = append(out, row)
			row = nil
		}
	}
	if len(row) > 0 {
		out = append(out, row)
	}
	return
}

func (e *emojiReactions) MessageText() string {
	return e.To.MessageText()
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
