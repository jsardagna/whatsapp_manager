package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	proto "google.golang.org/protobuf/proto"
)

type ComandoWorker struct {
	*BaseWhatsAppWorker
	cmdGroupJUID string
	m            *WhatsAppManager
}

func NewComandoWorker(m *WhatsAppManager, device *store.Device, cmdGroupJUID string, db database.Database) *ComandoWorker {
	baseWorker := NewBaseWhatsAppWorker(device, db)
	return &ComandoWorker{BaseWhatsAppWorker: baseWorker, cmdGroupJUID: cmdGroupJUID, m: m}
}

func (w *ComandoWorker) Start() error {
	err := w.Connect(nil)
	if err != nil {
		return err
	}
	return w.inicializaCommando()
}

func (w *ComandoWorker) inicializaCommando() error {
	if w.Cli.Store != nil && w.Cli.Store.ID != nil {
		w.Cli.AddEventHandler(w.handleWhatsAppEvents)
	}

	log.Printf("Dispositivo %s conectado com sucesso", w.device.ID)
	return nil
}

func (w *ComandoWorker) handleWhatsAppEvents(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		if !evt.Info.IsFromMe {
			if evt.Info.IsGroup && w.cmdGroupJUID == evt.Info.Chat.String() && !w.db.IsPhoneExists(evt.Info.Sender) {
				logWa.Infof("Comando %s from %s: %+v", evt.Info.ID, evt.Info.SourceString(), evt.Message)
				if evt.Message.Conversation != nil {
					cmd := ""
					if evt.Message.ExtendedTextMessage != nil {
						cmd = *evt.Message.ExtendedTextMessage.Text
					} else {
						cmd = *evt.Message.Conversation
					}
					if strings.Contains(cmd, "LIST") {
						w.enviarTexto(w.m.ListarDivulgadoresAtivos(), evt)
					}
					if strings.Contains(cmd, "OFF") {
						w.enviarTexto(w.m.ListarDivulgadoresInativos(), evt)
					}
					if strings.Contains(cmd, "ADD") {
						qrcod, err := w.m.AddnewDevice()
						if err != nil {
							w.enviarTexto(err.Error(), evt)
						} else {
							w.sendImage(evt.Info.Chat, qrcod)
						}
					}
					if strings.Contains(cmd, "R-") {
						juid, err := ExtrairNumero(cmd)
						if err == nil {
							w.m.db.RemoveDevice(juid)
						}
					}
				}
			}
		}
	}
}

func (w *ComandoWorker) sendImage(recipient types.JID, data []byte) {
	uploaded, err := w.uploadImage(data)
	if err != nil {
		log.Printf("Failed to upload file: %v", err)
		return
	}
	cctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	resp := make(chan whatsmeow.SendResponse)
	go func() {
		r, _ := w.sendMessage(cctx, recipient, uploaded, data, "")
		resp <- r
	}()
	select {
	case <-cctx.Done():
		fmt.Println(cctx.Err())
	case <-resp:
	}
}

func (w *ComandoWorker) enviarTexto(message string, evt *events.Message) {
	msg := &waE2E.Message{Conversation: proto.String(message)}
	w.Cli.SendMessage(context.Background(), evt.Info.Chat, msg)
}

func ExtrairNumero(input string) (string, error) {
	// Expressão regular para capturar o número no formato REMOVE-NNNNNNNNNNNN (12 ou 13 dígitos)
	re := regexp.MustCompile(`R-(\d{12,13})`)

	// Encontrar o número correspondente
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", errors.New("não foi possível encontrar um número no formato REMOVE-NNNNNNNNNNNN ou REMOVE-NNNNNNNNNNN")
}
