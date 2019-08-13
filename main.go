package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var config struct {
	Token           string
	Timeout         time.Duration
	ButtonRowLength int
}

var name = "emoji-reactions-bot"
var version = "dev"
var jsonOut = json.NewEncoder(os.Stdout)

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("[%s %s] ", filepath.Base(name), version))
	config.Timeout = 2 * time.Second
	config.ButtonRowLength = 5
	flag.DurationVar(&config.Timeout, "timeout", config.Timeout, "")
	flag.StringVar(&config.Token, "token", config.Token, "")
	flag.IntVar(&config.ButtonRowLength, "button-row-length", config.ButtonRowLength, "")
	flag.Parse()
}

func main() {
	botAPI, err := telegram.NewBot(telegram.Settings{
		Token:  config.Token,
		Poller: &telegram.LongPoller{Timeout: config.Timeout},
	})
	if err != nil {
		log.Fatal(err)
		return
	}
	bot := &emojiReactionBot{botAPI}
	bot.init()
	bot.Start()
}
