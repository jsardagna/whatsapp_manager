package whatsapp

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	proto "google.golang.org/protobuf/proto"
)

type WhatsAppManager struct {
	divulgadores   map[string]*DivulgacaoWorker
	mu             sync.Mutex
	storeContainer *sqlstore.Container
	cmdGroupJUID   string
	db             database.Database
	grupoComando   string
	deviceComando  string
}

var logWa waLog.Logger
var logLevel = "ERROR"

func NewWhatsAppManager(cmdGroupJUID string, db database.Database) *WhatsAppManager {
	return &WhatsAppManager{
		divulgadores: make(map[string]*DivulgacaoWorker),
		db:           db,
		cmdGroupJUID: cmdGroupJUID,
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
	go c.Start()
	return nil
}

func (m *WhatsAppManager) startWorker(device *store.Device, qrCodeChan chan []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	worker := NewDivulgacaoWorker(m, device, m.cmdGroupJUID, m.db)

	go func() {
		/*
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Recuperado de um panic: %v\n", r)
				}
			}()
		*/

		worker.Start(qrCodeChan)
	}()
}

func (m *WhatsAppManager) StartAllDevices() error {

	fmt.Println("Inicializando devices")
	logWa = waLog.Stdout("Main", logLevel, true)

	devices, err := m.getAllDevices()
	if err != nil {
		return err
	}
	fmt.Println("Devices Ativos: ", len(devices))
	for _, device := range devices {
		go func(device *store.Device) {
			if !strings.Contains(device.ID.String(), m.deviceComando) {
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
		logWa.Errorf("Erro ao obter dispositivos: %v", err)
		return nil, err
	}
	return devices, nil
}

func (m *WhatsAppManager) InitializeStore() (*sqlstore.Container, error) {
	logLevel := "INFO"
	store.DeviceProps.Os = proto.String("Google Chrome")
	store.DeviceProps.RequireFullSync = proto.Bool(false)
	dbLog := waLog.Stdout("Database", logLevel, true)

	var err error
	m.storeContainer, err = sqlstore.New(os.Getenv("DIALECT_W"), os.Getenv("ADDRESS_W"), dbLog)
	if err != nil {
		logWa.Errorf("Erro ao conectar ao banco de dados: %v", err)
		return m.storeContainer, err
	}

	return m.storeContainer, nil
}

func (m *WhatsAppManager) ListarDivulgadoresInativos() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Obter dispositivos ativos do banco de dados
	activeDevices, err := m.db.GetActiveDevicesInfo()
	if err != nil {
		return ""
	}

	var inativosString string

	// Percorrer os dispositivos ativos e verificar contra os divulgadores
	for _, activeDevice := range activeDevices {
		divulgador, exists := m.divulgadores[activeDevice.JUID]

		// Se o divulgador não existir, ele está inativo
		if !exists {
			inativosString += fmt.Sprintf("%s \n",
				activeDevice.JUID)
			continue
		}

		// Se o divulgador existir, verificar se está conectado e inicializado
		if !divulgador.Connected || (divulgador.device != nil && !divulgador.device.Initialized) {
			inativosString += fmt.Sprintf("Divulgador Inativo: %s, Último Update: %s, Total de Grupos: %d\n",
				activeDevice.JUID, activeDevice.LastUpdate.Format("2006-01-02 15:04:05"), activeDevice.TotalGrupos)
		}
	}

	if inativosString == "" {
		inativosString = "Nenhum divulgador inativo encontrado."
	}

	return inativosString
}

func (m *WhatsAppManager) ListarDivulgadoresAtivos() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var resultado string

	for nome, divulgador := range m.divulgadores {
		if divulgador.Connected {
			// Verificar se o dispositivo está inicializado e listar o ID
			if divulgador.device != nil && divulgador.device.ID != nil {
				resultado += fmt.Sprintf("Divulgador: %s - Inicializado: %v\n", nome, divulgador.device.Initialized)
			}
		}
	}

	return resultado
}

func parseJID(arg string) types.JID {
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer)
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			logWa.Errorf("Invalid JID %s: %v", arg, err)
			return recipient
		} else if recipient.User == "" {
			logWa.Errorf("Invalid JID %s: no server specified", arg)
			return recipient
		}
		return recipient
	}
}
