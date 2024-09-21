package whatsapp

import (
	"log"

	"go.mau.fi/whatsmeow"
)

func ConnectWhatsApp(deviceID string, client *whatsmeow.Client) error {
	err := client.Connect()
	if err != nil {
		log.Printf("Erro ao conectar o dispositivo %s: %v", deviceID, err)
		return err
	}
	log.Printf("Dispositivo %s conectado com sucesso!", deviceID)
	return nil
}
