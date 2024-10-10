package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
	"whatsapp-manager/internal/config"
	"whatsapp-manager/internal/database"
	"whatsapp-manager/internal/whatsapp"

	"github.com/joho/godotenv"
)

func init() {

	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {

	// Captura o panic e registra no arquivo de log
	defer func() {
		if r := recover(); r != nil {
			logErrorToFile(r)
		}
	}()

	// Criar um contexto que será cancelado quando o programa receber um sinal de término (Ctrl+C)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Canal para capturar sinais do sistema (SIGINT e SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	statusDB, err := database.NewDatabase()
	if err != nil {
		log.Fatal("Erro ao conectar ao banco de status:", err)
	}
	defer statusDB.CloneConnection()

	grupoComando := config.GetEnv("COMANDOS", "")

	deviceComando := config.GetEnv("DEVICE_COMMANDO", "")

	// Inicializar gerenciador de WhatsApp
	manager := whatsapp.NewWhatsAppManager(*statusDB)

	store, err := manager.InitializeStore()
	if err != nil {
		log.Fatalf("Erro ao inicializar Banco: %v", err)
	}
	defer store.Close()

	manager.StartComando(grupoComando, deviceComando)
	if err != nil {
		log.Fatalf("Erro ao Iniclicar comandos: %v", err)
	}

	// Iniciar o gerenciamento de dispositivos
	err = manager.StartAllDevices()
	if err != nil {
		log.Fatalf("Erro ao conectar divulgadores: %v", err)
	}

	go func() {
		// Aguardar um sinal de interrupção
		<-sigChan
		log.Println("Recebido sinal de encerramento, finalizando...")
		cancel() // Cancelar o contexto para interromper operações em andamento
	}()

	// Manter o servidor rodando, verificando se o contexto foi cancelado
	<-ctx.Done()
	log.Println("Encerrando a aplicação com segurança...")

	// Adicionar qualquer outra limpeza necessária (fechar conexões, encerrar goroutines, etc.)
	time.Sleep(1 * time.Second) // Simular uma tarefa de limpeza
	log.Println("Aplicação finalizada com sucesso.")
}

// Função para capturar e registrar o erro no arquivo de log
func logErrorToFile(r interface{}) {
	fmt.Printf("Recuperado de um panic: %v\n", r)
	fmt.Printf("Stack Trace:\n%s\n", debug.Stack())
}
