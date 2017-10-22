package main

import (
	"bytes"
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

var ssl = flag.Int("ssl", 0, "0:no ssl, 1:SelfSign, 2:CA-cert")
var webhook = flag.Bool("webhook", false, "webhook mode")
var debug = flag.Bool("debug", false, "debug")
var noisy = flag.Bool("noisy", false, "noisy")
var token = flag.String("token", os.Getenv("YBOTTOKEN"), "token")
var pubip = flag.String("pubip", "", "public ip, get with 'curl -s https://ipinfo.io/ip'")
var port = flag.Int("port", 8443, "webhook server port")
var cert = flag.String("cert", "cert.pem", "cert for webhook https server")
var key = flag.String("key", "key.pem", "priv key for webhook https server")
// path for required bin/util
//var pathAd = flag.String("Ad", func() string { p, _ := exec.LookPath("Ad"); return p }(), "path to Ad")
var path7z = flag.String("7z", func() string { p, _ := exec.LookPath("7z"); return p }(), "path to 7z")
var pathAdoc = flag.String("adoc", func() string { p, _ := exec.LookPath("asciidoctor"); return p }(), "path to asciidoctor")
var pathGs = flag.String("gs", func() string { p, _ := exec.LookPath("gs"); return p }(), "path to gs")

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
		msg.Text = fmt.Sprintf("Welcome %s (%s %s).\n"+
			"You may send me asciidoc (.adoc) file.\n"+
			"Or you can pack whole *.adoc and its included images + sub-adoc into a single compressed file.",
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
		log.Print(err)
		if Noisy {
			msg.Text = "Failed to proceed uploaded file"
		}
		bot.Send(msg)
		return
	}

	ext := path.Ext(update.Message.Document.FileName)
	tmp := path.Join(os.TempDir(), "ybot."+update.Message.Chat.UserName)
	packed := false
	dpacked := false

	switch strings.ToLower(ext) {

	case ".zip", ".rar", ".7z":
		packed = true
		msg.Text = "Fine, let me try this compressed file"

	case ".tgz", ".tbz2", ".txz":
		packed = true
		dpacked = true
		msg.Text = "Fine, let me try this compressed file."

	case ".gz", ".bz2", ".xz":
		ftar := strings.TrimSuffix(update.Message.Document.FileName, ext)
		exttar := path.Ext(ftar)
		if exttar == ".tar" {
			dpacked = true
			msg.Text = "Fine, let me try this compressed file.."
			ext = exttar + ext
		} else {
			msg.Text = "Not a compressed document that I knew"
			bot.Send(msg)
			return
		}

	case ".adoc":
		msg.Text = "Looks good, let me work on this file"

	default:
		msg.Text = "Document is not an adoc. I might just keep it for next processing"
		bot.Send(msg)
		return
	}

	if packed {
		tmp, err = ioutil.TempDir("", "ybot-")
		if err != nil {
			log.Print(err)
			log.Print(os.RemoveAll(tmp))
			if Noisy {
				msg.Text = "Unable to create temp"
			}
			bot.Send(msg)
			return
		}
	}
	bot.Send(msg)

	workfolder := path.Join(tmp, path.Dir(f.FilePath))
	//basefile := path.Join(workfolder, strings.TrimSuffix(update.Message.Document.FileName, ext))
	workfile := path.Join(workfolder, update.Message.Document.FileName)
	pdffile := path.Join(workfolder, strings.TrimSuffix(update.Message.Document.FileName, ext)+".pdf")

	// get WorkFile from TG
	response, err := http.Get(f.Link(*token))
	if err != nil {
		log.Print(err)
		if packed {
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
		log.Print(err)
		if packed {
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
		log.Print(err)
		if packed {
			log.Print(os.RemoveAll(tmp))
		}
		if Noisy {
			msg.Text = "Unable to buffer uploaded file"
		}
		bot.Send(msg)
		return
	}

	go func() {
		///////////////////////////////////// extraction
		switch {
		case dpacked:
			var out bytes.Buffer
			xcmd1 := exec.Command(*path7z, "x", "-so", workfile)
			xcmd1.Dir = workfolder
			xcmd2 := exec.Command(*path7z, "x", "-si", "-ttar", "-y")
			xcmd2.Dir = workfolder
			xcmd2.Stdin, _ = xcmd1.StdoutPipe()
			xcmd2.Stdout = &out
			_ = xcmd2.Start()
			_ = xcmd1.Run()
			err := xcmd2.Wait()
			if err != nil {
				log.Print(err)
				if packed {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v\n%v", string(out.Bytes()), err)
				}
				bot.Send(msg)
				return
			}
			workfile = path.Join(workfolder, "*.adoc")
			pdffile = path.Join(workfolder, "*.pdf")
		case packed:
			cmd := exec.Command(*path7z, "x", workfile)
			cmd.Dir = workfolder
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Print(err)
				if packed {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v\n%v", string(out), err)
				}
				bot.Send(msg)
				return
			}
			workfile = path.Join(workfolder, "*.adoc")
			pdffile = path.Join(workfolder, "*.pdf")
		}

		///////////////////////////////////// conversion
		files, err := filepath.Glob(workfile)
		if err != nil {
			log.Print(err)
			if packed {
				log.Print(os.RemoveAll(tmp))
			}
			if Noisy {
				msg.Text = fmt.Sprintf("Failed %v.", err)
			}
			bot.Send(msg)
			return
		}

		for _, f := range files {
			log.Printf("Processing %v", f)
			cmd := exec.Command(*pathAdoc, "-a", "allow-uri-read", "-r", "asciidoctor-diagram", "-r", "asciidoctor-pdf", "-b", "pdf", f)
			cmd.Dir = workfolder
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Print(err)
				if packed {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v\n%v", string(out), err)
				}
				bot.Send(msg)
				return
			}
		}

		///////////////////////////////////// optimizing
		files, err = filepath.Glob(pdffile)
		if err != nil {
			log.Print(err)
			if packed {
				log.Print(os.RemoveAll(tmp))
			}
			if Noisy {
				msg.Text = fmt.Sprintf("Failed %v.", err)
			}
			bot.Send(msg)
			return
		}

		for _, f := range files {
			log.Printf("Optimizing %v", f)
			gcmd := exec.Command(*pathGs, "-q",
				"-dNOPAUSE", "-dBATCH", "-dSAFER", "-dNOOUTERSAVE",
				"-sDEVICE=pdfwrite", "-dCompatibilityLevel=1.4",
				"-dPDFSETTINGS=/prepress", "-dCannotEmbedFontPolicy=/Warning",
				"-dDownsampleColorImages=true", "-dColorImageResolution=300",
				"-dDownsampleGrayImages=true", "-dGrayImageResolution=300",
				"-dDownsampleMonoImages=true", "-dMonoImageResolution=300",
				"-sOutputFile="+f+".pdf", f)
			gcmd.Dir = workfolder
			gout, gerr := gcmd.CombinedOutput()
			if err != nil {
				log.Print(gerr)
				if packed {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v\n%v", string(gout), gerr)
				}
				bot.Send(msg)
				return
			}
			err := os.Rename(f+".pdf", f)
			if err != nil {
				log.Print(err)
				if packed {
					log.Print(os.RemoveAll(tmp))
				}
				if Noisy {
					msg.Text = fmt.Sprintf("Failed %v", err)
				}
				bot.Send(msg)
				return
			}
		}

		///////////////////////////////////// send
		files, err = filepath.Glob(pdffile)
		if err != nil {
			log.Print(err)
			if packed {
				log.Print(os.RemoveAll(tmp))
			}
			if Noisy {
				msg.Text = fmt.Sprintf("Failed %v..", err)
			}
			bot.Send(msg)
			return
		}

		for _, f := range files {
			bot.Send(tgbotapi.NewDocumentUpload(msg.ChatID, f))
		}
		msg.Text = "Success.."
		bot.Send(msg)
		if packed {
			log.Print(os.RemoveAll(tmp))
		}

	}()

}

func main() {

	flag.Parse()

	//if *token == "" {
	//	*token = os.Getenv("YBOTTOKEN")
	//}
	bot, err := tgbotapi.NewBotAPI(*token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = *debug

	log.Printf("Authorized on account %s", bot.Self.UserName)

	if *webhook {

		var updates tgbotapi.UpdatesChannel
		switch *ssl {
		case 1:
			url := fmt.Sprintf("https://%s:%d/%s", *pubip, *port, bot.Token)
			log.Print("Webhook URL: " + url)
			_, err = bot.SetWebhook(tgbotapi.NewWebhookWithCert(url, *cert))
			if err != nil {
				log.Fatal(err)
			}

			updates = bot.ListenForWebhook("/" + bot.Token)
			go http.ListenAndServeTLS(fmt.Sprintf("0.0.0.0:%d", *port), *cert, *key, nil)

		case 2:
			url := fmt.Sprintf("https://%s:%d/%s", *pubip, *port, bot.Token)
			log.Print("Webhook URL: " + url)
			_, err = bot.SetWebhook(tgbotapi.NewWebhook(url))
			if err != nil {
				log.Fatal(err)
			}

			updates = bot.ListenForWebhook("/" + bot.Token)
			go http.ListenAndServeTLS(fmt.Sprintf("0.0.0.0:%d", *port), *cert, *key, nil)

		default:
			url := fmt.Sprintf("http://%s:%d/%s", *pubip, *port, bot.Token)
			log.Print("Webhook URL: " + url)
			_, err = bot.SetWebhook(tgbotapi.NewWebhook(url))
			if err != nil {
				log.Fatal(err)
			}

			updates = bot.ListenForWebhook("/" + bot.Token)
			go http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *port), nil)
		}

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
