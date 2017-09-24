package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api"
)

var webhook = flag.Bool("webhook", false, "webhook mode")
var debug = flag.Bool("debug", false, "debug")
var token = flag.String("token", "", "token")
var pubip = flag.String("pubip", "", "public ip, get with 'curl -s https://ipinfo.io/ip'")
var port = flag.Int("port", 8443, "webhook server port")
var cert = flag.String("cert", "cert.pem", "cert for webhook https server")
var key = flag.String("key", "key.pem", "priv key for webhook https server")

func handleUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update) {

	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID,
		"Send me asciidoc file (.adoc). I don't understand: "+update.Message.Text)
	msg.ReplyToMessageID = update.Message.MessageID

	switch update.Message.Text {
	case "/start":
		msg.Text = fmt.Sprintf("Welcome %s (%s %s). You may send me asciidoc (.adoc) file",
			update.Message.From.UserName, update.Message.From.FirstName, update.Message.From.LastName)
		bot.Send(msg)
		return
	default:
		if update.Message.Document == nil {
			bot.Send(msg)
			return
		}
	}

	f, err := bot.GetFile(tgbotapi.FileConfig{FileID: update.Message.Document.FileID})
	//log.Printf("DocFile: %s", update.Message.Document.FileName)
	if err != nil {
		log.Fatal(err)
		msg.Text = "Failed to proceed uploaded file"
		bot.Send(msg)
		return
	}

	lfolder := path.Join("/tmp", path.Dir(f.FilePath))
	//lfile := path.Join("/tmp", f.FilePath)
	ext := path.Ext(update.Message.Document.FileName)
	pdffile := path.Join(lfolder, strings.TrimSuffix(update.Message.Document.FileName, ext)+".pdf")
	adocfile := path.Join(lfolder, update.Message.Document.FileName)

	if ext != ".adoc" {
		msg.Text = "Document is not an adoc"
		bot.Send(msg)
		return
	}

	msg.Text = "Looks good, let me work on this file"
	bot.Send(msg)

	response, err := http.Get(f.Link(*token))
	if err != nil {

		log.Fatal(err)
		msg.Text = "Failed to get uploaded file"
		bot.Send(msg)
		return

	}
	defer response.Body.Close()

	os.MkdirAll(lfolder, os.ModePerm)

	file, err := os.Create(adocfile)
	if err != nil {
		log.Fatal(err)
		msg.Text = "Unable to create new file"
		bot.Send(msg)
		return
	}
	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(file, response.Body)
	file.Close()
	if err != nil {
		log.Fatal(err)
		msg.Text = "Unable to buffer uploaded file"
		bot.Send(msg)
		return
	}

	go func() {
		cmd := exec.Command("Ad", adocfile)
		cmd.Dir = lfolder
		out, err := cmd.Output()
		if err != nil {
			log.Fatal(err)
			msg.Text = fmt.Sprintf("Fail %v\n%v", out, err)
		} else if len(out) > 0 {
			msg.Text = fmt.Sprintf("Success %v", out)
		} else {
			msg.Text = fmt.Sprintf("Success")
		}
		bot.Send(msg)
		bot.Send(tgbotapi.NewDocumentUpload(msg.ChatID, pdffile))

	}()

}

func main() {

	flag.Parse()

	if *token == "" {
		*token = os.Getenv("YBOTTOKEN")
	}
	bot, err := tgbotapi.NewBotAPI(*token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = *debug

	log.Printf("Authorized on account %s", bot.Self.UserName)

	if *webhook {

		url := fmt.Sprintf("https://%s:%d/%s", *pubip, *port, bot.Token)
		//log.Print(url)
		_, err = bot.SetWebhook(tgbotapi.NewWebhookWithCert(url, *cert))
		if err != nil {
			log.Fatal(err)
		}

		updates := bot.ListenForWebhook("/" + bot.Token)
		go http.ListenAndServeTLS(fmt.Sprintf("0.0.0.0:%d", *port), *cert, *key, nil)

		log.Printf("Starting Collect Update from WebHook")
		for update := range updates {
			handleUpdate(bot, update)
		}

	} else {

		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60

		updates, err := bot.GetUpdatesChan(u)

		if err != nil {
			log.Panic(err)
		}

		log.Printf("Starting GetUpdate")
		for update := range updates {
			handleUpdate(bot, update)
		}
	}

}
