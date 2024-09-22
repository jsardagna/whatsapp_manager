package main

import (
	"log"
	"net/http"
	"whatsapp-manager/internal/database"
	"whatsapp-manager/internal/rest"
	"whatsapp-manager/internal/whatsapp"
)

var cmdGroupJUID *string

func main() {

	statusDB, err := database.NewDatabase()
	if err != nil {
		log.Fatal("Erro ao conectar ao banco de status:", err)
	}
	defer statusDB.CloneConnection()

	cmdGroupJUID = statusDB.GetGroup("ID CONTROLE")

	// Configuração da API REST
	http.HandleFunc("/status", rest.StatusHandler)

	// Inicializar o servidor
	log.Println("Servidor iniciado na porta 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))

	// Inicializar gerenciador de WhatsApp
	manager := whatsapp.NewWhatsAppManager()

	// Iniciar o gerenciamento de dispositivos
	err = manager.StartManagingDevices(*cmdGroupJUID, *statusDB)
	if err != nil {
		log.Fatalf("Erro ao gerenciar dispositivos: %v", err)
	}

	// Manter o servidor rodando
	select {}
}
