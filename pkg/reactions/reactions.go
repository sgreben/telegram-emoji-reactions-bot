package reactions

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var spaceString = string([]byte{0xc2, 0xad, 0xc2, 0xad})

type Single struct {
	Emoji string `json:"E"`
	Count int64  `json:"C"`
}

func (e *Single) ParseButtonData(data string) error {
	i := strings.IndexRune(data, '|')
	dataBytes := []byte(data[i+1:])
	err := json.Unmarshal(dataBytes, e)
	if err != nil {
		return fmt.Errorf("parse data %s: %v", dataBytes, err)
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
	UserID int
	ID     int
	ChatID int64
}

// Recipient is telebot.Recipient
func (e *To) Recipient() string {
	return strconv.Itoa(e.UserID)
}

func (e *To) Parse(data []byte) error {
	return json.Unmarshal(data, e)
}

// UnmarshalJSON is json.UnmarshalJSON
func (e *To) UnmarshalJSON(data []byte) error {
	var source struct {
		UserID string `json:"H"`
		ID     string `json:"I"`
		ChatID string `json:"C"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		return err
	}
	userID64, _ := strconv.ParseInt(source.UserID, 16, 32)
	e.UserID = int(userID64)
	chatID, _ := strconv.ParseInt(source.ChatID, 16, 64)
	e.ChatID = chatID
	id64, _ := strconv.ParseInt(source.ID, 16, 32)
	e.ID = int(id64)
	return nil
}

// MarshalJSON is json.MarshalJSON
func (e To) MarshalJSON() ([]byte, error) {
	var target struct {
		UserID string `json:"H"`
		ID     string `json:"I"`
		ChatID string `json:"C"`
	}
	target.UserID = fmt.Sprintf("%x", e.UserID)
	target.ID = fmt.Sprintf("%x", e.ID)
	target.ChatID = fmt.Sprintf("%x", e.ChatID)
	return json.Marshal(target)
}

const maxMessageLength = 4096

func (e *To) encode() string {
	dataBytes, _ := json.Marshal(e)
	return url.QueryEscape(string(dataBytes))
}

type Previous struct {
	Count map[string]map[string]int
}

func (e *Previous) Get(userID int, emoji string) int {
	userIDHex := fmt.Sprintf("%x", userID)
	return e.Count[userIDHex][emoji]
}

func (e *Previous) Remove(userID int, emoji string) {
	userIDHex := fmt.Sprintf("%x", userID)
	if n, ok := e.Count[userIDHex][emoji]; ok {
		if n <= 1 {
			delete(e.Count[userIDHex], userIDHex)
		}
		e.Count[userIDHex][emoji] = n - 1
	}
}

func (e *Previous) Add(userID int, emoji string) {
	userIDHex := fmt.Sprintf("%x", userID)
	if e.Count == nil {
		e.Count = map[string]map[string]int{
			userIDHex: {
				emoji: 1,
			},
		}
		return
	}
	if e.Count[userIDHex] == nil {
		e.Count[userIDHex] = map[string]int{
			emoji: 1,
		}
		return
	}
	e.Count[userIDHex][emoji]++
}

func (e *Previous) encode() string {
	dataBytes, _ := json.Marshal(e.Count)
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(dataBytes)
	w.Close()
	return url.QueryEscape(buf.String())
}

func (e *Previous) Parse(data []byte) error {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	dataBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(dataBytes, &e.Count)
}

type Set struct {
	Slice    []Single
	To       *To
	Previous *Previous
	Config   struct {
		ButtonRowLength    int
		ButtonRowMinLength int
	} `json:"-"`
}

func (e *Set) ParseMessage(m *telegram.Message) error {
	e.To = &To{}
	e.Previous = &Previous{}
	if err := e.parseButtons(m.ReplyMarkup.InlineKeyboard); err != nil {
		return fmt.Errorf("parse message: %v", err)
	}

	if err := e.parseLinks(m.Entities); err != nil {
		return fmt.Errorf("parse message: %v", err)
	}

	return nil
}

func (e *Set) parseLinks(entities []telegram.MessageEntity) error {
	for _, entity := range entities {
		if entity.Type != telegram.EntityTextLink {
			continue
		}
		entityURL, err := url.Parse(entity.URL)
		if err != nil {
			return err
		}
		to := entityURL.Query().Get("data") // TODO: DEPRECATE
		if to == "" {
			to = entityURL.Query().Get("t")
		}
		if err := e.To.Parse([]byte(to)); err != nil {
			return err
		}
		previous := entityURL.Query().Get("p")
		if err := e.Previous.Parse([]byte(previous)); err != nil {
			return err
		}
	}
	return nil
}
func (e *Set) parseButtons(rows [][]telegram.InlineButton) error {
	var errors []error
	e.Slice = e.Slice[:0]
	for _, buttons := range rows {
		for _, b := range buttons {
			var r Single
			if err := r.ParseButtonData(b.Data); err != nil {
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

func (e *Set) buttons(id string, f func(*telegram.Callback)) (out [][]telegram.InlineButton) {
	var row []telegram.InlineButton
	for i, r := range e.Slice {
		row = append(row, r.Button(id, f))
		remaining := len(e.Slice) - i
		if len(row) >= e.Config.ButtonRowLength && remaining >= e.Config.ButtonRowMinLength {
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
	to := e.To.encode()
	previous := e.Previous.encode()
	encode := func() string {
		return fmt.Sprintf(`<a href="http://example.com?t=%s&p=%s">%s</a>`, to, previous, spaceString)
	}
	text := encode()
	for len(text) > maxMessageLength && len(e.Previous.Count) > 0 {
		for k := range e.Previous.Count {
			delete(e.Previous.Count, k)
			break
		}
		text = encode()
	}
	return text
}

func (e *Set) ReplyMarkup(id string, f func(*telegram.Callback)) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{
		InlineKeyboard: e.buttons(id, f),
	}
}

func (e *Set) AddOrRemove(userID int, emoji []string) (added, removed int) {
	var toAdd []string
	var toRemove []string
	for _, s := range emoji {
		if e.Previous.Get(userID, s) > 0 {
			toRemove = append(toRemove, s)
			continue
		}
		e.Previous.Add(userID, s)
		toAdd = append(toAdd, s)
	}
	for _, s := range toRemove {
		e.Previous.Remove(userID, s)
		e.remove(s)
	}
	e.add(toAdd)
	return len(toAdd), len(toRemove)
}

func (e *Set) add(emoji []string) {
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

func (e *Set) remove(emoji string) {
	var remaining []Single
	for i, r := range e.Slice {
		if emoji == r.Emoji {
			e.Slice[i].Count--
			if e.Slice[i].Count == 0 {
				continue
			}
		}
		remaining = append(remaining, e.Slice[i])
	}
	e.Slice = remaining
}
