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

var comandos string
var divulgador string

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

	divulgador = config.GetEnv("DIVULGACACAO", divulgador)

	grupoComando := config.GetEnv("COMANDOS", divulgador)

	deviceComando := config.GetEnv("DEVICE_COMMANDO", divulgador)

	// Inicializar gerenciador de WhatsApp
	manager := whatsapp.NewWhatsAppManager(divulgador, *statusDB)

	err = manager.InitializeStore()
	if err != nil {
		log.Fatalf("Erro ao inicializar Banco: %v", err)
	}

	manager.StartComando(grupoComando, deviceComando)
	if err != nil {
		log.Fatalf("Erro ao Iniclicar comandos: %v", err)
	}

	// Iniciar o gerenciamento de dispositivos
	err = manager.StartAllDevices()
	if err != nil {
		log.Fatalf("Erro ao conectar divulgadores: %v", err)
	}

	// Inicializar o servidor
	// Configuração da API REST
	http.HandleFunc("/status", rest.StatusHandler)

	//log.Println("Servidor iniciado na porta 8080")
	//log.Fatal(http.ListenAndServe(":8080", nil))

	// Manter o servidor rodando
	select {}
}
