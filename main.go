package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	telegram "github.com/sgreben/telegram-emoji-reactions-bot/internal/telebot.v2"
)

var config struct {
	Token              string
	Timeout            time.Duration
	ButtonRowLength    int
	ButtonRowMinLength int
	Verbose            bool
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
	config.ButtonRowMinLength = 2

	flag.DurationVar(&config.Timeout, "timeout", config.Timeout, "")
	flag.StringVar(&config.Token, "token", config.Token, "")
	flag.IntVar(&config.ButtonRowLength, "button-row-length", config.ButtonRowLength, "")
	flag.BoolVar(&config.Verbose, "verbose", config.Verbose, "")
	flag.BoolVar(&config.Verbose, "v", config.Verbose, "(alas for -verbose)")
	flag.IntVar(&config.ButtonRowMinLength, "button-row-min-length", config.ButtonRowMinLength, "")
	flag.Parse()

	if !config.Verbose {
		jsonOut = json.NewEncoder(ioutil.Discard)
	}
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
	bot := &emojiReactionBot{
		Bot:               botAPI,
		ReactionPostCache: make(map[int]int),
		MessageCache:      make(map[int]*telegram.Message),
	}
	bot.init()
	bot.Start()
}
