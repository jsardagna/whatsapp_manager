package whatsapp

import (
	"log"

	"go.mau.fi/whatsmeow"
)

type WhatsAppWorker struct {
	deviceID string
	client   *whatsmeow.Client
}

func NewWhatsAppWorker(deviceID string, client *whatsmeow.Client) *WhatsAppWorker {
	return &WhatsAppWorker{deviceID: deviceID, client: client}
}

func (w *WhatsAppWorker) Start() {
	err := w.client.Connect()
	if err != nil {
		log.Printf("Erro ao conectar dispositivo %s: %v", w.deviceID, err)
		return
	}
	log.Printf("Dispositivo %s conectado com sucesso", w.deviceID)
}
