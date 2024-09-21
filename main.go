package main

import (
	"log"
	"net/http"
	"whatsapp-manager/internal/database"
	"whatsapp-manager/internal/rest"
)

func main() {
	// Conectar aos bancos de dados
	whatsAppDB, err := database.ConnectToWhatsAppDB()
	if err != nil {
		log.Fatal("Erro ao conectar ao banco do WhatsApp:", err)
	}
	defer whatsAppDB.Close()

	statusDB, err := database.ConnectToStatusDB()
	if err != nil {
		log.Fatal("Erro ao conectar ao banco de status:", err)
	}
	defer statusDB.Close()

	// Configuração da API REST
	http.HandleFunc("/status", rest.StatusHandler)

	// Inicializar o servidor
	log.Println("Servidor iniciado na porta 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
