package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api"
)

var webhook = flag.Bool("webhook", false, "webhook mode")
var debug = flag.Bool("debug", false, "debug")
var noisy = flag.Bool("noisy", false, "noisy")
var token = flag.String("token", "", "token")
var pubip = flag.String("pubip", "", "public ip, get with 'curl -s https://ipinfo.io/ip'")
var port = flag.Int("port", 8443, "webhook server port")
var cert = flag.String("cert", "cert.pem", "cert for webhook https server")
var key = flag.String("key", "key.pem", "priv key for webhook https server")

func handleUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update) {

	if update.Message == nil {
		return
	}
	Noisy := *noisy
	if update.Message.From.UserName == "sehari24jam" {
		Noisy = true
	}

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Failed")
	msg.ReplyToMessageID = update.Message.MessageID

	switch update.Message.Text {
	case "/start":
		msg.Text = fmt.Sprintf("Welcome %s (%s %s).\nYou may send me asciidoc (.adoc) file",
			update.Message.From.UserName, update.Message.From.FirstName, update.Message.From.LastName)
		bot.Send(msg)
		return
	default:
		if update.Message.Document == nil {
			msg.Text = "Send me asciidoc file (.adoc). I don't understand: " + update.Message.Text
			bot.Send(msg)
			return
		}
	}

	f, err := bot.GetFile(tgbotapi.FileConfig{FileID: update.Message.Document.FileID})
	//log.Printf("DocFile: %s", update.Message.Document.FileName)
	if err != nil {
		log.Fatal(err)
		if Noisy {
			msg.Text = "Failed to proceed uploaded file"
		}
		bot.Send(msg)
		return
	}

	ext := path.Ext(update.Message.Document.FileName)
	tmp := path.Join(os.TempDir(), "ybot."+update.Message.Chat.UserName)
	zipped := false
	switch strings.ToLower(ext) {

	case ".zip":
		zipped = true
		tmp, err = ioutil.TempDir("", "ybot-")
		if err != nil {
			log.Fatal(err)
			log.Print(os.RemoveAll(tmp))
			if Noisy {
				msg.Text = "Unable to create temp"
			}
			bot.Send(msg)
			return
		}
		msg.Text = "Looks good, let me work on this zip file"
		bot.Send(msg)

	case ".adoc":
		msg.Text = "Looks good, let me work on this file"
		bot.Send(msg)

	default:
		msg.Text = "Document is not an adoc"
		bot.Send(msg)
	}
	workfolder := path.Join(tmp, path.Dir(f.FilePath))
	//lfile := path.Join("/tmp", f.FilePath)
	pdffile := path.Join(workfolder, strings.TrimSuffix(update.Message.Document.FileName, ext)+".pdf")
	workfile := path.Join(workfolder, update.Message.Document.FileName)

	// get WorkFile from TG
	response, err := http.Get(f.Link(*token))
	if err != nil {
		log.Fatal(err)
		if zipped {
			log.Print(os.RemoveAll(tmp))
		}
		if Noisy {
			msg.Text = "Failed to get uploaded file"
		}
		bot.Send(msg)
		return
	}
	defer response.Body.Close()

	// create sub folder as necessary
	os.MkdirAll(workfolder, os.ModePerm)

	// save WorkFile
	file, err := os.Create(workfile)
	if err != nil {
		log.Fatal(err)
		if zipped {
			log.Print(os.RemoveAll(tmp))
		}
		if Noisy {
			msg.Text = "Unable to create new file"
		}
		bot.Send(msg)
		return
	}
	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(file, response.Body)
	file.Close()
	if err != nil {
		log.Fatal(err)
		if zipped {
			log.Print(os.RemoveAll(tmp))
		}
		if Noisy {
			msg.Text = "Unable to buffer uploaded file"
		}
		bot.Send(msg)
		return
	}

	go func() {
		if zipped {
			cmd := exec.Command("7z", "x", workfile)
			cmd.Dir = workfolder
			out, err := cmd.Output()
			if err != nil {
				log.Fatal(err)
				if zipped {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v\n%v", out, err)
				}
				bot.Send(msg)
				return
			}
			workfile = "*.adoc"
		}
		cmd := exec.Command("Ad", workfile)
		cmd.Dir = workfolder
		out, err := cmd.Output()
		if err != nil {
			log.Fatal(err)
			if zipped {
				log.Print(os.RemoveAll(tmp))
			}
			if Noisy {
				msg.Text = fmt.Sprintf("Failed %v\n%v", out, err)
			}
			bot.Send(msg)
			return
		}

		if zipped {
			files, err := filepath.Glob(path.Join(workfolder, "*.pdf"))
			if err != nil {
				log.Fatal(err)
				if zipped {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v", err)
				}
				bot.Send(msg)
				return
			}

			for _, f := range files {
				bot.Send(tgbotapi.NewDocumentUpload(msg.ChatID, f))
			}
			if Noisy {
				msg.Text = fmt.Sprintf("Success %v", out)
			} else {
				msg.Text = "Success"
			}
			bot.Send(msg)
			log.Print(os.RemoveAll(tmp))
		} else {
			if Noisy {
				msg.Text = fmt.Sprintf("Success %v", out)
			} else {
				msg.Text = "Success"
			}
			bot.Send(msg)
			bot.Send(tgbotapi.NewDocumentUpload(msg.ChatID, pdffile))
		}

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
			//log.Printf("%+v\n", update)
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
