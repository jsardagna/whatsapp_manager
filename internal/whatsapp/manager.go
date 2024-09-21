package whatsapp

import (
	"sync"

	"go.mau.fi/whatsmeow"
)

type WhatsAppManager struct {
	workers map[string]*WhatsAppWorker
	mu      sync.Mutex
}

func NewWhatsAppManager() *WhatsAppManager {
	return &WhatsAppManager{
		workers: make(map[string]*WhatsAppWorker),
	}
}

func (m *WhatsAppManager) StartWorker(deviceID string, client *whatsmeow.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	worker := NewWhatsAppWorker(deviceID, client)
	m.workers[deviceID] = worker
	go worker.Start()
}
