package whatsapp

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	proto "google.golang.org/protobuf/proto"
)

type WhatsAppManager struct {
	divulgadores   map[string]*DivulgacaoWorker
	mu             sync.Mutex
	storeContainer *sqlstore.Container
	db             database.Database
	grupoComando   string
	deviceComando  string
}

func NewWhatsAppManager(db database.Database) *WhatsAppManager {
	return &WhatsAppManager{
		divulgadores: make(map[string]*DivulgacaoWorker),
		db:           db,
	}
}

func (m *WhatsAppManager) StartComando(grupoComando string, deviceComando string) error {
	m.grupoComando = grupoComando
	m.deviceComando = deviceComando
	device, err := m.storeContainer.GetDevice(parseJID(deviceComando))
	if err != nil {
		return err
	} else if device == nil {
		device = m.storeContainer.NewDevice()
	}
	c := NewComandoWorker(m, device, grupoComando, m.db)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recuperado de um panic: %v\n", r)
				fmt.Printf("Stack Trace:\n%s\n", debug.Stack())
				LogErrorToFile(r)
			}
		}()

		c.Start()
	}()
	return nil
}

func (m *WhatsAppManager) startWorker(device *store.Device, qrCodeChan chan []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	worker := NewDivulgacaoWorker(m, device, m.db)

	go func() {

		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recuperado de um panic: %v\n", r)
				fmt.Printf("Stack Trace:\n%s\n", debug.Stack())
				LogErrorToFile(r)
			}
		}()
		worker.Start(qrCodeChan)
	}()
}

func (m *WhatsAppManager) StartAllDevices() error {

	fmt.Println("Inicializando devices")

	devices, err := m.getAllDevices()
	if err != nil {
		return err
	}
	fmt.Println("Devices Ativos: ", len(devices))

	// Mapa para rastrear usuários de dispositivos já inicializados
	inicializados := make(map[string]bool)

	for _, device := range devices {
		go func(device *store.Device) {
			// Verifica se o ID do device é nulo ou se o usuário já existe
			if device.ID == nil || device.ID.User == "" {
				fmt.Println("Device com ID nulo ou User vazio, pulando.")
				return
			}

			// Verifica se já existe um dispositivo com o mesmo User inicializado
			if inicializados[device.ID.User] {
				fmt.Printf("Dispositivo com User %s já inicializado, pulando.\n", device.ID.User)
				return
			}
			// Verifica se o ID do dispositivo é o do comenando e não inicializar
			if !strings.Contains(device.ID.String(), m.deviceComando) {
				fmt.Printf("Iniciando worker para o device: %s\n", device.ID.String())
				m.startWorker(device, nil)

			}
		}(device)
	}
	return nil
}

func (m *WhatsAppManager) AddnewDevice() ([]byte, error) {
	device := m.storeContainer.NewDevice()
	qrCodeChan := make(chan []byte)
	m.startWorker(device, qrCodeChan)

	// Esperar o QR code ser enviado pelo canal
	select {
	case qrCode := <-qrCodeChan:
		if len(qrCode) > 0 {
			// Retornar o QR code se estiver preenchido
			return qrCode, nil
		} else {
			// Caso o QR code esteja vazio
			return nil, fmt.Errorf("QR Code vazio recebido")
		}
	case <-time.After(5 * time.Second): // Timeout opcional de 10 segundos
		// Timeout para evitar que o código fique preso indefinidamente
		return nil, fmt.Errorf("timeout: Qr code não foi recebido a tempo")
	}
}

func (m *WhatsAppManager) getAllDevices() ([]*store.Device, error) {

	devices, err := m.storeContainer.GetAllDevices()
	if err != nil {
		//logWa.Errorf("Erro ao obter dispositivos: %v", err)
		return nil, err
	}
	return devices, nil
}

func (m *WhatsAppManager) InitializeStore() (*sqlstore.Container, error) {
	store.DeviceProps.Os = proto.String("Google Chrome")
	store.DeviceProps.RequireFullSync = proto.Bool(false)
	var err error
	m.storeContainer, err = sqlstore.New(os.Getenv("DIALECT_W"), os.Getenv("ADDRESS_W"), nil)

	if err != nil {
		return nil, fmt.Errorf("erro ao conectar banco API")
	}

	return m.storeContainer, nil
}

func (m *WhatsAppManager) ListarDivulgadoresInativos() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Obter dispositivos ativos do banco de dados
	activeDevices, err := m.db.GetActiveDevicesInfo()
	if err != nil {
		return err.Error()
	}

	// Obter todos os dispositivos e criar um novo mapa com device.ID.User como chave
	allDevices, err := m.getAllDevices()
	if err != nil {
		return err.Error()
	}

	// Mapa para armazenar os dispositivos baseados em device.ID.User
	dispositivosMapa := make(map[string]*store.Device)
	for _, device := range allDevices {
		if device.ID != nil && device.ID.User != "" {
			dispositivosMapa[device.ID.User] = device
		}
	}

	var inativosString string
	total := 0
	// Percorrer os dispositivos ativos e verificar contra o novo mapa de dispositivos
	for _, activeDevice := range activeDevices {
		device, exists := dispositivosMapa[activeDevice.JUID]

		// Se o dispositivo não existir no mapa, ele está inativo
		if !exists {
			total++
			inativosString += fmt.Sprintf("```%s - %s```\n", activeDevice.Local, activeDevice.JUID)
			continue
		}

		// Se o dispositivo existir, verificar se está conectado e inicializado
		if device != nil && !device.Initialized {
			total++
			inativosString += fmt.Sprintf("Divulgador Inativo: %s, Último Update: %s, Total de Grupos: %d\n",
				activeDevice.JUID, activeDevice.LastUpdate.Format("2006-01-02 15:04:05"), activeDevice.TotalGrupos)
		}
	}

	// Caso nenhum divulgador inativo seja encontrado
	if inativosString != "" {
		return fmt.Sprintf("TOTAL INATIVOS: %d\n```-------------------```\n%s```-------------------```", total, inativosString)
	}
	return "Nenhum divulgador inativo encontrado."
}

func (m *WhatsAppManager) ListarDivulgadoresAtivos() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var resultado string
	total := 0
	for nome, divulgador := range m.divulgadores {
		if divulgador.Connected {

			// Verificar se o dispositivo está inicializado e listar o ID
			if divulgador.device != nil && divulgador.device.ID != nil {
				resultado += fmt.Sprintf("```%s```\n", nome)
				total++
			}
		}
	}

	return fmt.Sprintf("TOTAL ATIVOS: %d \n%s", total, resultado)
}

func parseJID(arg string) types.JID {
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer)
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			//logWa.Errorf("Invalid JID %s: %v", arg, err)
			return recipient
		} else if recipient.User == "" {
			//logWa.Errorf("Invalid JID %s: no server specified", arg)
			return recipient
		}
		return recipient
	}
}

// Função para capturar e registrar o erro no arquivo de log
func LogErrorToFile(r interface{}) {
	// Abrir ou criar o arquivo de log (somente erros serão registrados aqui)
	file, err := os.OpenFile("/home/ec2-user/panic-error.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Println("Erro ao abrir arquivo de log:", err)
		return
	}
	defer file.Close()

	// Cria um logger que escreve no arquivo
	logger := log.New(file, "PANIC: ", log.LstdFlags)

	// Registra a mensagem do panic e o stack trace
	logger.Printf("Panic occurred: %v\n", r)
	logger.Printf("Stack Trace:\n%s\n", debug.Stack())
}
