package main

import (
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

func main() {
	token := os.Getenv("YBOTTOKEN")
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	if err != nil {
		log.Panic(err)
	}
	log.Printf("Starting read")
	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"Send me asciidoc file (.adoc). I don't understand: "+update.Message.Text)
		msg.ReplyToMessageID = update.Message.MessageID

		if update.Message.Document == nil {
			bot.Send(msg)
			continue
		}

		f, err := bot.GetFile(tgbotapi.FileConfig{FileID: update.Message.Document.FileID})
		if err != nil {
			log.Fatal(err)
			msg.Text = "Failed to proceed uploaded file"
			bot.Send(msg)
			continue
		}

		response, err := http.Get(f.Link(token))
		if err != nil {

			log.Fatal(err)
			msg.Text = "Failed to get uploaded file"
			bot.Send(msg)
			continue

		} else {

			defer response.Body.Close()

			lfolder := path.Join("/tmp", path.Dir(f.FilePath))
			lfile := path.Join("/tmp", f.FilePath)
			ext := path.Ext(lfile)
			pdffile := strings.TrimSuffix(lfile, ext) + ".pdf"

			if ext != ".adoc" {
				msg.Text = "Document is not an adoc"
				bot.Send(msg)
				continue
			}

			os.MkdirAll(lfolder, os.ModePerm)

			file, err := os.Create(lfile)
			if err != nil {
				log.Fatal(err)
				msg.Text = "Unable to create new file"
				bot.Send(msg)
				continue
			}
			// Use io.Copy to just dump the response body to the file. This supports huge files
			_, err = io.Copy(file, response.Body)
			if err != nil {
				log.Fatal(err)
				msg.Text = "Unable to buffer uploaded file"
				bot.Send(msg)
				continue
			}
			file.Close()

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			msg.ReplyToMessageID = update.Message.MessageID

			go func() {
				cmd := exec.Command("Ad", lfile)
				cmd.Dir = lfolder
				out, err := cmd.Output()
				if err != nil {
					log.Fatal(err)
					msg.Text = fmt.Sprintf("Fail %v", err)
				} else {
					msg.Text = fmt.Sprintf("Success %v", out)
				}
				bot.Send(msg)
				bot.Send(tgbotapi.NewDocumentUpload(msg.ChatID, pdffile))

			}()
		}

	}

}
