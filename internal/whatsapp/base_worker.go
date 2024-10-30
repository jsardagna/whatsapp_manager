package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"
	"whatsapp-manager/internal/database"

	qrterminal "github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	proto "google.golang.org/protobuf/proto"
)

type BaseWhatsAppWorker struct {
	device  *store.Device
	Cli     *whatsmeow.Client
	db      database.Database
	Manager *WhatsAppManager
}

func NewBaseWhatsAppWorker(m *WhatsAppManager, device *store.Device, db database.Database) *BaseWhatsAppWorker {
	return &BaseWhatsAppWorker{Manager: m, device: device, db: db}
}

func (w *BaseWhatsAppWorker) Connect(qrCodeChan chan []byte, onComplete func()) error {
	var isWaitingForPair atomic.Bool
	var pairRejectChan = make(chan bool, 1)
	w.Cli = whatsmeow.NewClient(w.device, nil)
	w.Cli.EmitAppStateEventsOnFullSync = true
	w.Cli.AutomaticMessageRerequestFromPhone = false
	w.Cli.ErrorOnSubscribePresenceWithoutToken = false
	//w.Cli.PreRetryCallback = func(receipt *events.Receipt, id types.MessageID, retryCount int, msg *waE2E.Message) bool {
	//	return false
	//}
	w.Cli.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		isWaitingForPair.Store(true)
		defer isWaitingForPair.Store(false)
		//logWa.Infof("Pairing %s (platform: %q, business name: %q). Type r within 3 seconds to reject pair", jid, platform, businessName)
		select {
		case reject := <-pairRejectChan:
			if reject {
				//		logWa.Infof("Rejecting pair")
				return false
			}
		case <-time.After(3 * time.Second):
		}
		//logWa.Infof("Accepting pair")
		return true
	}

	// Verificar se precisa de QR Code
	if w.Cli.Store.ID == nil {
		ch, err := w.Cli.GetQRChannel(context.Background())
		if err != nil && !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			return err
		} else {
			go func() {
				for evt := range ch {
					if evt.Event == "code" {
						if qrCodeChan != nil {
							qrCodeChan <- w.GerarQRCodeEmBytes(evt.Code)
						} else {
							qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						}

					} else {
						// Aqui chama o método passado por parâmetro quando o QR code não é necessário
						if onComplete != nil {
							onComplete()
						}
					}
				}
			}()
		}
		w.Cli.Connect()
	} else {
		fmt.Println("Conenctando: ", w.Cli.Store.ID.User)
		err := w.Cli.Connect()
		if err != nil {
			fmt.Println("Falha ao conectar Device: ", w.device.ID.User)
			return err
		} else if w.device != nil && w.device.ID != nil {
			if onComplete != nil {
				onComplete()
			}
		}
	}
	return nil
}

func (w *BaseWhatsAppWorker) uploadImage(data []byte) (whatsmeow.UploadResponse, error) {
	return w.Cli.Upload(context.Background(), data, whatsmeow.MediaImage)
}

func (w *BaseWhatsAppWorker) GerarQRCodeEmBytes(conteudo string) []byte {
	// Gerar o QR code em formato PNG e retornar como []byte
	qrCode, err := qrcode.Encode(conteudo, qrcode.Medium, 256)
	if err != nil {
		return nil
	}

	return qrCode
}

func (b *BaseWhatsAppWorker) sendMessage(ctx context.Context, recipient types.JID, uploaded whatsmeow.UploadResponse, data []byte, caption string) (resp whatsmeow.SendResponse, err error) {

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

	return b.Cli.SendMessage(ctx, recipient, msg)
}
