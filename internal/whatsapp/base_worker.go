package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"whatsapp-manager/internal/database"

	qrterminal "github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type BaseWhatsAppWorker struct {
	device *store.Device
	Cli    *whatsmeow.Client
	db     database.Database
}

func NewBaseWhatsAppWorker(device *store.Device, db database.Database) *BaseWhatsAppWorker {
	return &BaseWhatsAppWorker{device: device, db: db}
}

func (w *BaseWhatsAppWorker) Connect() error {
	var isWaitingForPair atomic.Bool
	var pairRejectChan = make(chan bool, 1)
	w.Cli = whatsmeow.NewClient(w.device, waLog.Stdout("Cliente", logLevel, true))

	w.Cli.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
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
	ch, err := w.Cli.GetQRChannel(context.Background())
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

	err = w.Cli.Connect()
	if err != nil {
		fmt.Println("Falha ao conectar Device: ", w.device.ID.User)
		return err
	} else {
		fmt.Println("Device Connectado: ", w.device.ID.User)
	}
	return nil
}
