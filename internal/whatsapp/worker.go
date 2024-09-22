package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"whatsapp-manager/internal/database"

	qrterminal "github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	proto "google.golang.org/protobuf/proto"
)

var isWaitingForPair atomic.Bool

type WhatsAppWorker struct {
	device       *store.Device
	cli          *whatsmeow.Client
	cmdGroupJUID string
	db           database.Database
	nextmessage  bool
	kindmessage  string
}

func NewWhatsAppWorker(device *store.Device, cmdGroupJUID string, db database.Database) *WhatsAppWorker {
	return &WhatsAppWorker{device: device, cmdGroupJUID: cmdGroupJUID, db: db}
}

func (w *WhatsAppWorker) Start() error {
	var pairRejectChan = make(chan bool, 1)
	w.cli = whatsmeow.NewClient(w.device, waLog.Stdout("Cliente", logLevel, true))

	w.cli.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		isWaitingForPair.Store(true)
		defer isWaitingForPair.Store(false)
		logWa.Infof("Pairing %s (platform: %q, business name: %q). Type r within 3 seconds to reject pair", jid, platform, businessName)
		select {
		case reject := <-pairRejectChan:
			if reject {
				logWa.Infof("Rejecting pair")
				return false
			}
		case <-time.After(3 * time.Second):
		}
		logWa.Infof("Accepting pair")
		return true
	}

	// Verificar se precisa de QR Code
	ch, err := w.cli.GetQRChannel(context.Background())
	if err != nil && !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
		return err
	} else {
		go func() {
			for evt := range ch {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				}
			}
		}()
	}

	err = w.cli.Connect()
	if err != nil {
		return err
	}

	if w.cli.Store != nil && w.cli.Store.ID != nil {
		go w.inicializaFila()
		w.cli.AddEventHandler(w.handleWhatsAppEvents)
		groups, _ := w.findAllGroups()
		println("Conta", w.cli.Store.ID.String())
		println("Total de grupos", len(groups))
		go w.monitorInsert(w.cli.Store.ID.User)
	}

	log.Printf("Dispositivo %s conectado com sucesso", w.device.ID)
	return nil
}

func (w *WhatsAppWorker) findAllGroups() ([]*types.GroupInfo, error) {
	return w.cli.GetJoinedGroups()
}

func (w *WhatsAppWorker) handleWhatsAppEvents(rawEvt interface{}) {
	db := w.db
	switch evt := rawEvt.(type) {
	case *events.Message:
		metaParts := []string{fmt.Sprintf("pushname: %s", evt.Info.PushName), fmt.Sprintf("timestamp: %s", evt.Info.Timestamp)}
		if evt.Info.Type != "" {
			metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
		}
		if evt.Info.Category != "" {
			metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "view once")
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "ephemeral")
		}
		if evt.IsViewOnceV2 {
			metaParts = append(metaParts, "ephemeral (v2)")
		}
		if evt.IsDocumentWithCaption {
			metaParts = append(metaParts, "document with caption")
		}
		if evt.IsEdit {
			metaParts = append(metaParts, "edit")
		}

		if evt.Message.Conversation != nil {
			cmd := *evt.Message.Conversation
			if cmd == "ENVIAR" {
				fmt.Printf("evt.Info.Chat.String(): %v\n", evt.Info.Chat.String())
			}
		}

		if !evt.Info.IsFromMe {

			if evt.Info.IsGroup && w.cmdGroupJUID == evt.Info.Chat.String() && !w.db.IsPhoneExists(evt.Info.Sender) {
				logWa.Infof("Comando %s from %s (%s): %+v", evt.Info.ID, evt.Info.SourceString(), strings.Join(metaParts, ", "), evt.Message)
				if w.nextmessage && evt.Message.ImageMessage != nil {
					img := evt.Message.GetImageMessage()
					data, err := w.cli.Download(img)
					if err != nil {
						logWa.Errorf("Failed to download image: %v", err)
						return
					}
					var caption = ""
					if img.Caption != nil {
						caption = *img.Caption
					}
					if strings.Contains(w.kindmessage, "ENVIAR-") {
						kind, ddd := extractPartsAndNumbers(w.kindmessage)
						queueN.EnqueueImage(w.db, w.cmdGroupJUID, data, caption, kind, ddd)
					} else {
						queueAll.EnqueueImage(w.db, w.cmdGroupJUID, data, caption, nil, nil)
					}
					w.nextmessage = false
				} else if w.nextmessage && evt.Message.VideoMessage != nil {
					video := evt.Message.GetVideoMessage()
					data, err := w.cli.Download(video)
					if err != nil {
						logWa.Errorf("Failed to download video: %v", err)
						return
					}
					var caption = ""
					if video.Caption != nil {
						caption = *video.Caption
					}

					if strings.Contains(w.kindmessage, "ENVIAR-") {
						kind, ddd := extractPartsAndNumbers(w.kindmessage)
						queueN.EnqueueVideo(db, w.cmdGroupJUID, data, caption, kind, ddd)
					} else {
						queueAll.EnqueueVideo(db, w.cmdGroupJUID, data, caption, nil, nil)
					}

					w.nextmessage = false
				} else if w.nextmessage && evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil &&
					(strings.HasPrefix(*evt.Message.ExtendedTextMessage.MatchedText, "https://www.instagram.com/") ||
						strings.HasPrefix(*evt.Message.ExtendedTextMessage.MatchedText, "https://x.com/") ||
						strings.HasPrefix(*evt.Message.ExtendedTextMessage.MatchedText, "https://desejocasual.com/") ||
						strings.HasPrefix(*evt.Message.ExtendedTextMessage.MatchedText, "https://twitter.com/") ||
						strings.HasPrefix(*evt.Message.ExtendedTextMessage.MatchedText, "https://youtu.be/")) {

					if strings.Contains(w.kindmessage, "ENVIAR-") {
						kind, ddd := extractPartsAndNumbers(w.kindmessage)
						queueN.EnqueueLink(db, w.cmdGroupJUID, evt.Message, kind, ddd)
					} else {
						queueAll.EnqueueLink(db, w.cmdGroupJUID, evt.Message, nil, nil)
					}

					w.nextmessage = false
				}
				if evt.Message.ExtendedTextMessage != nil || evt.Message.Conversation != nil {
					cmd := ""
					if evt.Message.ExtendedTextMessage != nil {
						cmd = *evt.Message.ExtendedTextMessage.Text
					} else {
						cmd = *evt.Message.Conversation
					}
					if strings.Contains(cmd, "ENVIAR") {
						w.nextmessage = true
						w.kindmessage = cmd
						total := 0
						if strings.Contains(cmd, "ENVIAR-") {
							total = len(queueN.stack)
						} else {
							total = len(queueAll.stack)
						}
						w.enviarTexto(cmd, total, evt)
					}
				}
			} else {
				if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, "https://chat.whatsapp.com/") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					log.Println(msg)
					log.Println("Grupo Encontrado, inserindo...")
					go w.verifyAndInsertGroup(msg, evt)
				} else if evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, "https://chat.whatsapp.com/") {
					msg := *evt.Message.Conversation
					log.Println(msg)
					log.Println("Grupo Encontrado, inserindo...")
					go w.verifyAndInsertGroup(msg, evt)
				} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, "https://t.me/") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					log.Println(msg)
					log.Println("Grupo Telegram")
					go w.verifyAndInsertGroupTelegram(msg, evt)
				} else if !evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, "https://t.me/") {
					msg := *evt.Message.Conversation
					log.Println(msg)
					log.Println("Grupo Telegram")
					go w.verifyAndInsertGroupTelegram(msg, evt)
				} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, ".com/share/") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					go db.InsertLink(msg, evt.Info.Chat.String())
				} else if !evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, ".com/share/") {
					msg := *evt.Message.Conversation
					go db.InsertLink(msg, evt.Info.Chat.String())
				} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, "?id=") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					go db.InsertLink(msg, evt.Info.Chat.String())
				} else if evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, "?id=") {
					msg := *evt.Message.Conversation
					go db.InsertLink(msg, evt.Info.Chat.String())
				} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(strings.ToLower(*evt.Message.ExtendedTextMessage.MatchedText), "lançamento") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					go db.InsertLink(msg, evt.Info.Chat.String())
				} else if evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(strings.ToLower(*evt.Message.Conversation), "lançamento") {
					msg := *evt.Message.Conversation
					go db.InsertLink(msg, evt.Info.Chat.String())
				}
			}
		}
	}
}

func (w *WhatsAppWorker) enviarTexto(cmd string, total int, evt *events.Message) {
	msg := &waE2E.Message{Conversation: proto.String(fmt.Sprintf("Aguardando MSG, Fila: %s na espera: %d", cmd, total))}
	w.cli.SendMessage(context.Background(), evt.Info.Chat, msg)
}

func (w *WhatsAppWorker) uploadImage(data []byte) (whatsmeow.UploadResponse, error) {
	return w.cli.Upload(context.Background(), data, whatsmeow.MediaImage)
}

func (w *WhatsAppWorker) verifyAndInsertGroupTelegram(msg string, evt *events.Message) {
	codes := findWhatsAppCodes("https://t.me/", msg)

	for _, code := range codes {
		msg = "https://t.me/" + code
		fmt.Println(msg)
		logWa.Infof("Achou grupo %s", msg)
		g := database.Group{
			Link: msg,
			Code: code,
		}
		if evt.Message != nil && evt.Message.ExtendedTextMessage != nil {
			if evt.Message.ExtendedTextMessage.Title != nil {
				g.Name = *evt.Message.ExtendedTextMessage.Title
			}
			if evt.Message.ExtendedTextMessage.Description != nil {
				g.Description = *evt.Message.ExtendedTextMessage.Description
			}
		}
		w.db.VerifyAndInsertTelegram(msg, g)
	}
}

func (w *WhatsAppWorker) verifyAndInsertGroup(msg string, evt *events.Message) {
	codes := findWhatsAppCodes("https://chat.whatsapp.com/", msg)

	for _, code := range codes {
		msg = "https://chat.whatsapp.com/" + code
		fmt.Println(msg)
		logWa.Infof("Achou grupo %s", msg)
		g := database.Group{
			Link: msg,
			Code: code,
		}
		if evt.Message != nil && evt.Message.ExtendedTextMessage != nil {
			if evt.Message.ExtendedTextMessage.Title != nil {
				g.Name = *evt.Message.ExtendedTextMessage.Title
			}
			if evt.Message.ExtendedTextMessage.Description != nil {
				g.Description = *evt.Message.ExtendedTextMessage.Description
			}
		} else {
			resp, err := w.cli.GetGroupInfoFromLink(msg)
			if err == nil {
				g.Name = resp.Name
				g.Description = resp.Topic
			}
		}
		g, result := w.db.VerifyAndInsert(g)
		if result {
			w.queryInveteLink(g)
		}
	}
}

func findWhatsAppCodes(pattern, texto string) []string {
	// Defina a expressão regular para encontrar URLs do WhatsApp com grupos de captura
	padrao := regexp.MustCompile(pattern + `([\S]+)`)

	// Encontre todas as correspondências no texto
	correspondencias := padrao.FindAllStringSubmatch(texto, -1)

	var codigos []string

	for _, match := range correspondencias {
		if len(match) > 1 {
			// O grupo de captura está na posição 1 (match[1])
			codigos = append(codigos, match[1])
		}
	}
	return codigos
}

func (w *WhatsAppWorker) queryInveteLink(g database.Group) bool {

	resp, err := w.cli.GetGroupInfoFromLink(g.Link)

	if err != nil {
		w.db.InvalidGroup(g)
		logWa.Errorf("Failed to resolve group invite link: %v", err)
	} else {
		w.db.UpdateGroup(g, resp)
		logWa.Infof("Group info: %+v", resp)
	}
	return false
}

func extractPartsAndNumbers(s string) (*[]string, *[]string) {
	// Regex para capturar o padrão "ENVIAR-" seguido por uma ou mais partes e números após "-"
	re := regexp.MustCompile(`^ENVIAR-((?:[^,-]+,)*[^,-]+)(?:-(\d{2}(?:,\d{2})*))?$`)
	matches := re.FindStringSubmatch(s)

	if len(matches) > 1 {
		// Captura todas as partes antes dos números, separadas por vírgulas
		parts := strings.Split(matches[1], ",")

		// Captura os números após o segundo "-"
		var numbers []string
		if len(matches) > 2 && matches[2] != "" {
			numbers = regexp.MustCompile(`\d{2}`).FindAllString(matches[2], -1)
		}
		return &parts, &numbers
	}
	// Retorna nil se não encontrar o padrão desejado
	return nil, nil
}
