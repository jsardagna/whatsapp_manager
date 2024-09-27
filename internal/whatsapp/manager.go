package whatsapp

import (
	"fmt"
	"os"
	"sync"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	proto "google.golang.org/protobuf/proto"
)

type WhatsAppManager struct {
	divulgadores map[string]*DivulgacaoWorker
	mu           sync.Mutex
}

var logWa waLog.Logger
var logLevel = "ERROR"

func NewWhatsAppManager() *WhatsAppManager {
	return &WhatsAppManager{
		divulgadores: make(map[string]*DivulgacaoWorker),
	}

}

func (m *WhatsAppManager) StartWorker(device *store.Device, cmdGroupJUID string, db database.Database) {
	m.mu.Lock()
	defer m.mu.Unlock()

	worker := NewDivulgacaoWorker(device, cmdGroupJUID, db)
	m.divulgadores[device.ID.User] = worker
	go worker.Start()
}

func (m *WhatsAppManager) StartManagingDevices(cmdGroupJUID string, db database.Database) error {
	fmt.Println("Inicializando devices")
	logWa = waLog.Stdout("Main", logLevel, false)

	devices, err := m.getAllDevices()
	fmt.Println("Devices Ativos: ", len(devices))

	if err != nil {
		return err
	}
	for _, device := range devices {
		go func(device *store.Device) {
			m.StartWorker(device, cmdGroupJUID, db)
		}(device)
	}
	return nil
}

func (m *WhatsAppManager) getAllDevices() ([]*store.Device, error) {

	storeContainer, err := m.initializeStore()
	if err != nil {
		return nil, err
	}

	devices, err := storeContainer.GetAllDevices()
	if err != nil {
		logWa.Errorf("Erro ao obter dispositivos: %v", err)
		return nil, err
	}
	return devices, nil
}

func (*WhatsAppManager) initializeStore() (*sqlstore.Container, error) {
	logLevel := "ERROR"
	store.DeviceProps.Os = proto.String("Google Chrome")
	store.DeviceProps.RequireFullSync = proto.Bool(false)
	dbLog := waLog.Stdout("Database", logLevel, false)
	storeContainer, err := sqlstore.New(os.Getenv("DIALECT_W"), os.Getenv("ADDRESS_W"), dbLog)
	if err != nil {
		logWa.Errorf("Erro ao conectar ao banco de dados: %v", err)
		return nil, err
	}
	return storeContainer, nil
}
