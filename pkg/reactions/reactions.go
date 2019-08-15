package reactions

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var spaceString = "­­"

type Single struct {
	Emoji string `json:"E"`
	Count int64  `json:"C"`
}

func (e *Single) ParseButton(b *telegram.InlineButton) error {
	return e.ParseButtonData(b.Data)
}

func (e *Single) ParseButtonData(data string) error {
	i := strings.IndexRune(data, '|')
	return e.Parse([]byte(data[i+1:]))
}

func (e *Single) Parse(data []byte) error {
	err := json.Unmarshal(data, e)
	if err != nil {
		return fmt.Errorf("parse data %s: %v", data, err)
	}
	return nil
}

func (e *Single) Button(id string, f func(*telegram.Callback)) telegram.InlineButton {
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

type To struct {
	UserID    int    `json:"-,omitempty"`
	UserIDHex string `json:"H,omitempty"`
	Text      string `json:"-,omitempty"`
	ID        int    `json:"-,omitempty"`
	IDHex     string `json:"I,omitempty"`
	ChatID    int64  `json:"-,omitempty"`
	ChatIDHex string `json:"C,omitempty"`
}

func (e *To) Recipient() string {
	return strconv.Itoa(e.UserID)
}

func (e *To) ParseURL(link string) (err error) {
	linkURL, err := url.Parse(link)
	if err != nil {
		return err
	}
	data := linkURL.Query().Get("data")
	return e.Parse([]byte(data))
}

func (e *To) Parse(data []byte) error {
	var errors []error
	if err := json.Unmarshal(data, e); err != nil {
		errors = append(errors, err)
	}
	if err := e.parseUserID(); err != nil {
		errors = append(errors, err)
	}
	if err := e.parseChatID(); err != nil {
		errors = append(errors, err)
	}
	if err := e.parseID(); err != nil {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		return fmt.Errorf("%v", errors)
	}
	return nil
}

func (e *To) parseUserID() error {
	if e.UserIDHex == "" {
		return nil
	}
	userID64, err := strconv.ParseInt(e.UserIDHex, 16, 32)
	e.UserID = int(userID64)
	return err
}

func (e *To) parseChatID() error {
	if e.ChatIDHex == "" {
		return nil
	}
	chatID, err := strconv.ParseInt(e.ChatIDHex, 16, 64)
	e.ChatID = chatID
	return err
}

func (e *To) parseID() error {
	if e.IDHex == "" {
		return nil
	}
	id64, err := strconv.ParseInt(e.IDHex, 16, 32)
	e.ID = int(id64)
	return err
}

const maxMetadataLength = 4096 - 128

func (e *To) Encode() string {
	shortened := *e
	shortened.UserIDHex = fmt.Sprintf("%x", shortened.UserID)
	shortened.UserID = 0
	shortened.IDHex = fmt.Sprintf("%x", shortened.ID)
	shortened.ID = 0
	shortened.ChatIDHex = fmt.Sprintf("%x", shortened.ChatID)
	shortened.ChatID = 0
	dataBytes, _ := json.Marshal(shortened)
	textLength := len(e.Text)
	for len(dataBytes) >= maxMetadataLength && textLength > 0 {
		textLength--
		shortened.Text = e.Text[:textLength] + "…"
		dataBytes, _ = json.Marshal(shortened)
	}
	return string(dataBytes)
}

func (e *To) MessageText() string {
	data := url.QueryEscape(e.Encode())
	return fmt.Sprintf(`<a href="http://example.com?data=%s">`+spaceString+`</a>`, data)
}

type Set struct {
	Slice  []Single
	To     To
	Config struct {
		ButtonRowLength    int
		ButtonRowMinLength int
	} `json:"-"`
}

func (e *Set) ParseMessage(m *telegram.Message) error {
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

func (e *Set) ParseButtons(rows [][]telegram.InlineButton) error {
	var errors []error
	e.Slice = nil
	for _, buttons := range rows {
		for _, b := range buttons {
			var r Single
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

func (e *Set) Buttons(id string, f func(*telegram.Callback)) (out [][]telegram.InlineButton) {
	var row []telegram.InlineButton
	for i, r := range e.Slice {
		row = append(row, r.Button(id, f))
		if len(row) == e.Config.ButtonRowLength && (len(e.Slice)-i) >= e.Config.ButtonRowMinLength {
			out = append(out, row)
			row = nil
		}
	}
	if len(row) > 0 {
		out = append(out, row)
	}
	return
}

func (e *Set) MessageText() string {
	return e.To.MessageText()
}

func (e *Set) ReplyMarkup(id string, f func(*telegram.Callback)) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{
		InlineKeyboard: e.Buttons(id, f),
	}
}

func (e *Set) Add(emoji []string) {
	var added []Single
adding:
	for _, add := range emoji {
		for i, r := range e.Slice {
			if add == r.Emoji {
				e.Slice[i].Count++
				continue adding
			}
		}
		added = append(added, Single{
			Emoji: add,
			Count: 1,
		})
	}
	e.Slice = append(e.Slice, added...)
}
