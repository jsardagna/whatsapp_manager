package whatsapp

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	proto "google.golang.org/protobuf/proto"
)

type DivulgacaoWorker struct {
	*BaseWhatsAppWorker
	cmdGroupJUID string
	nextmessage  bool
	kindmessage  string
	Connected    bool
	queueN       *MessageQueue
	queueAll     *MessageQueue
	mu           sync.Mutex
	Interval     time.Duration
}

func NewDivulgacaoWorker(m *WhatsAppManager, device *store.Device, db database.Database) *DivulgacaoWorker {
	baseWorker := NewBaseWhatsAppWorker(m, device, db)
	return &DivulgacaoWorker{BaseWhatsAppWorker: baseWorker}
}

func (w *DivulgacaoWorker) Start(qrCodeChan chan []byte) {
	onComplete := func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recuperado de um panic: %v\n", r)
				fmt.Printf("Stack Trace:\n%s\n", debug.Stack())
				LogErrorToFile(r)
			}
		}()
		w.workerDivulgacao()
	}
	w.Connect(qrCodeChan, onComplete)
}

func (w *DivulgacaoWorker) workerDivulgacao() error {
	cel := w.Cli.Store.ID.User
	w.Cli.SetStatusMessage("pv liberado")
	println("CELULAR:", w.Cli.Store.ID.User)
	cmd := w.db.GetGroup(w.Cli.Store.ID.User)
	if cmd != nil {
		if *cmd == "https://chat.whatsapp.com/JzeDefo3oBYGFw0zQUOCfW" {
			w.cmdGroupJUID = "120363343818835998@g.us"
		} else if *cmd == "https://chat.whatsapp.com/EeMGDADPOYIFlMbq3noAc8" {
			w.cmdGroupJUID = "120363149950387591@g.us"
		} else if *cmd == "https://chat.whatsapp.com/EOxBEqcfpRq8fZ0KnYGwHp" {
			w.cmdGroupJUID = "120363330490936340@g.us"
		} else {
			group, err := w.Cli.GetGroupInfoFromLink(*cmd)
			if err != nil { //caso de erro
				println("ERRO", cel, err.Error())
				gr, err2 := w.Cli.JoinGroupWithLink(*cmd)
				if err2 != nil {
					println("FALHA CELULAR NÃ0 ACHOU GRUPO", cel, err2.Error())
				} else {
					w.cmdGroupJUID = gr.String()
				}
			} else {
				w.cmdGroupJUID = group.JID.String()
			}
		}
	} else {
		DIVULGACAO1 := "https://chat.whatsapp.com/EeMGDADPOYIFlMbq3noAc8"
		DIVULGACAO2 := "https://chat.whatsapp.com/JzeDefo3oBYGFw0zQUOCfW"
		DIVULGACAO3 := "https://chat.whatsapp.com/EOxBEqcfpRq8fZ0KnYGwHp"
		group, err := w.Cli.JoinGroupWithLink(DIVULGACAO3)
		if err == nil {
			w.db.InsertConfig(w.Cli.Store.ID.User, DIVULGACAO3)
			w.cmdGroupJUID = group.String()
			w.Cli.JoinGroupWithLink(DIVULGACAO1) //DIVULGAÇÃO3
			w.Cli.JoinGroupWithLink(DIVULGACAO2) //DIVULGAÇÃO
		}
	}
	if w.Cli.IsConnected() {
		go w.inicializaFila()
		go w.monitorInsert(w.Cli.Store.ID.User)
		w.Cli.RemoveEventHandlers()
		w.Cli.AddEventHandler(w.handleWhatsAppEvents)
		w.safeAddMap()
		w.Connected = true
		println("GRUPO", w.Cli.Store.ID.User, w.cmdGroupJUID)
	}
	return nil
}

func (w *DivulgacaoWorker) safeAddMap() {
	w.Manager.MuDivulgadores.Lock()
	defer w.Manager.MuDivulgadores.Unlock()
	w.Manager.divulgadores[w.device.ID.User] = w
}

func (w *DivulgacaoWorker) inicializaFila() {
	w.Interval = 60 * time.Minute
	w.queueN = w.NewMessageQueue(w.Interval)
	w.queueAll = w.NewMessageQueue(w.Interval)
	go w.processStack(w.queueN)
	go w.processStack(w.queueAll)
}

func (w *DivulgacaoWorker) findAllGroups() ([]*types.GroupInfo, error) {
	return w.Cli.GetJoinedGroups()
}

func (w *DivulgacaoWorker) handleWhatsAppEvents(rawEvt interface{}) {
	db := w.db
	switch evt := rawEvt.(type) {
	case *events.Message:
		if !evt.Info.IsFromMe {
			if evt.Info.IsGroup && w.cmdGroupJUID == evt.Info.Chat.String() && !db.IsPhoneExists(evt.Info.Sender) {
				if w.nextmessage && evt.Message.ImageMessage != nil {
					img := evt.Message.GetImageMessage()
					data, err := w.Cli.Download(img)
					if err != nil {
						return
					}
					var caption = ""
					if img.Caption != nil {
						caption = *img.Caption
					}
					if strings.Contains(w.kindmessage, "ENVIAR-") {
						kind, ddd := extractPartsAndNumbers(w.kindmessage)
						w.queueN.EnqueueImage(w.db, w.cmdGroupJUID, data, caption, kind, ddd)
					} else {
						w.queueAll.EnqueueImage(w.db, w.cmdGroupJUID, data, caption, nil, nil)
					}
					w.nextmessage = false
				} else if w.nextmessage && evt.Message.VideoMessage != nil {
					video := evt.Message.GetVideoMessage()
					data, err := w.Cli.Download(video)
					if err != nil {
						return
					}
					var caption = ""
					if video.Caption != nil {
						caption = *video.Caption
					}

					if strings.Contains(w.kindmessage, "ENVIAR-") {
						kind, ddd := extractPartsAndNumbers(w.kindmessage)
						w.queueN.EnqueueVideo(db, w.cmdGroupJUID, data, caption, kind, ddd)
					} else {
						w.queueAll.EnqueueVideo(db, w.cmdGroupJUID, data, caption, nil, nil)
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
						w.queueN.EnqueueLink(db, w.cmdGroupJUID, evt.Message, kind, ddd)
					} else {
						w.queueAll.EnqueueLink(db, w.cmdGroupJUID, evt.Message, nil, nil)
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
							total = len(w.queueN.stack)
						} else {
							total = len(w.queueAll.stack)
						}
						w.enviarTexto(cmd, total, evt)
						go db.UpdateConfig(w.Cli.Store.ID.User, "PREPARA-ENVIO", 0)
					}
				}
			} else {
				if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, "https://chat.whatsapp.com/") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					go w.verifyAndInsertGroup(msg, evt)
				} else if evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, "https://chat.whatsapp.com/") {
					msg := *evt.Message.Conversation
					go w.verifyAndInsertGroup(msg, evt)
				} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.MatchedText != nil && strings.Contains(*evt.Message.ExtendedTextMessage.MatchedText, "https://t.me/") {
					msg := *evt.Message.ExtendedTextMessage.MatchedText
					go w.verifyAndInsertGroupTelegram(msg, evt)
				} else if !evt.Info.IsGroup && evt.Message.Conversation != nil && strings.Contains(*evt.Message.Conversation, "https://t.me/") {
					msg := *evt.Message.Conversation
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

func (w *DivulgacaoWorker) enviarTexto(cmd string, total int, evt *events.Message) {
	msg := &waE2E.Message{Conversation: proto.String(fmt.Sprintf("Aguardando MSG, Fila: %s na espera: %d", cmd, total))}
	println("enviando texto..", *msg.Conversation)
	w.internMessage(evt.Info.Chat, msg, func() {}, func(error) {})
}

func (w *DivulgacaoWorker) verifyAndInsertGroupTelegram(msg string, evt *events.Message) {
	codes := findWhatsAppCodes("https://t.me/", msg)

	for _, code := range codes {
		msg = "https://t.me/" + code
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

func (w *DivulgacaoWorker) verifyAndInsertGroup(msg string, evt *events.Message) {
	codes := findWhatsAppCodes("https://chat.whatsapp.com/", msg)

	for _, code := range codes {
		msg = "https://chat.whatsapp.com/" + code
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
			resp, err := w.Cli.GetGroupInfoFromLink(msg)
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

func (w *DivulgacaoWorker) queryInveteLink(g database.Group) bool {

	resp, err := w.Cli.GetGroupInfoFromLink(g.Link)

	if err != nil {
		w.db.InvalidGroup(g)
	} else {
		w.db.UpdateGroup(g, resp)
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

func (w *DivulgacaoWorker) estaAtivo() bool {
	if w.Cli == nil || w.Cli.Store == nil || w.Cli.Store.ID == nil {
		w.Connected = false
		return false
	}
	return true
}

// Função final com Mutex
func (w *DivulgacaoWorker) internMessage(recipient types.JID, msg *waE2E.Message, onSuccess func(), onError func(error)) bool {

	if !w.estaAtivo() {
		return false
	}
	// Garantindo acesso exclusivo usando Mutex
	w.mu.Lock()
	defer w.mu.Unlock()

	cctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := make(chan whatsmeow.SendResponse)
	errChan := make(chan error)

	// Goroutine para envio de mensagem

	go func() {

		defer func() {
			if r := recover(); r != nil {
				controlePanic(w, r)
			}
		}()
		r, err := w.Cli.SendMessage(cctx, recipient, msg, whatsmeow.SendRequestExtra{Timeout: 10 * time.Second})
		if err != nil {
			errChan <- err // Envia erro no canal de erro
		} else {
			resp <- r // Envia sucesso no canal de resposta
		}
	}()
	select {
	case <-cctx.Done():
		// Se o contexto expirar
		onError(cctx.Err())
		return false
	case err := <-errChan:
		// Se houver erro durante o envio
		onError(err)
		return false
	case <-resp:
		// Operação de sucesso
		onSuccess()
		return true
	}
}

func controlePanic(w *DivulgacaoWorker, r interface{}) {
	if w.Cli.Store != nil && w.Cli.Store.ID != nil {
		println(w.Cli.Store.ID, "Erro ao enviar MSG", r)
	} else {
		println(w.Cli.Store.ID, "Celular Perdido", r)
	}
	fmt.Printf("Stack Trace:\n%s\n", debug.Stack())
	LogErrorToFile(r)
}

func (w *DivulgacaoWorker) sendVideo(recipient types.JID, uploaded whatsmeow.UploadResponse, data []byte, caption string, onSuccess func(), onError func(error)) bool {

	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String(http.DetectContentType(data)),
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(data))),
	}}

	return w.internMessage(recipient, msg, onSuccess, onError)
}

func (w *DivulgacaoWorker) sendImage(recipient types.JID, uploaded whatsmeow.UploadResponse, data []byte, caption string, onSuccess func(), onError func(error)) bool {

	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String(http.DetectContentType(data)),
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(data))),
	}}

	return w.internMessage(recipient, msg, onSuccess, onError)
}
