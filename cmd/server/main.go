package main

import (
	"log"
	"net/http"
	"whatsapp-manager/internal/config"
	"whatsapp-manager/internal/database"
	"whatsapp-manager/internal/rest"
	"whatsapp-manager/internal/whatsapp"

	"github.com/joho/godotenv"
)

var cmdGroupJUID string

func init() {

	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {

	statusDB, err := database.NewDatabase()
	if err != nil {
		log.Fatal("Erro ao conectar ao banco de status:", err)
	}
	defer statusDB.CloneConnection()

	cmdGroupJUID = config.GetEnv("DIVULGACACAO", cmdGroupJUID)

	// Configuração da API REST
	http.HandleFunc("/status", rest.StatusHandler)

	// Inicializar gerenciador de WhatsApp
	manager := whatsapp.NewWhatsAppManager()

	// Iniciar o gerenciamento de dispositivos
	err = manager.StartManagingDevices(cmdGroupJUID, *statusDB)
	if err != nil {
		log.Fatalf("Erro ao gerenciar dispositivos: %v", err)
	}

	// Inicializar o servidor
	//log.Println("Servidor iniciado na porta 8080")
	//log.Fatal(http.ListenAndServe(":8080", nil))

	// Manter o servidor rodando
	select {}
}
