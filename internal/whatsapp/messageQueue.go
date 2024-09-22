package whatsapp

import (
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

var queueN *MessageQueue
var queueAll *MessageQueue

// MessageQueue representa a fila de mensagens
type MessageQueue struct {
	worker    *WhatsAppWorker
	stack     []messageRequest
	intervalo time.Duration
}

// messageRequest representa uma solicitação para enviar mensagens
type messageRequest struct {
	db      database.Database
	ignore  string
	data    *[]byte
	text    *string
	message *waE2E.Message
	tipo    string
	kind    *[]string
	ddd     *[]string
}

// NewMessageQueue inicializa uma nova fila de mensagens
func (w *WhatsAppWorker) NewMessageQueue(intervalo time.Duration) *MessageQueue {
	return &MessageQueue{
		stack:     make([]messageRequest, 0),
		intervalo: intervalo,
		worker:    w,
	}
}

// Enqueue adiciona uma nova solicitação à pilha
func (q *MessageQueue) EnqueueImage(db database.Database, ignore string, data []byte, msg string, kind *[]string, ddd *[]string) {
	q.stack = append(q.stack, messageRequest{db, ignore, &data, &msg, nil, "image", kind, ddd})
}

// Enqueue adiciona uma nova solicitação à pilha
func (q *MessageQueue) EnqueueVideo(db database.Database, ignore string, data []byte, msg string, kind *[]string, ddd *[]string) {
	q.stack = append(q.stack, messageRequest{db, ignore, &data, &msg, nil, "video", kind, ddd})
}

// Enqueue adiciona uma nova solicitação à pilha
func (q *MessageQueue) EnqueueLink(db database.Database, ignore string, msg *waE2E.Message, kind *[]string, ddd *[]string) {
	q.stack = append(q.stack, messageRequest{db, ignore, nil, nil, msg, "link", kind, ddd})
}

// processStack processa a pilha de mensagens
func (w *WhatsAppWorker) processStack(queue *MessageQueue) {
	for {
		if len(queue.stack) > 0 {
			request := queue.stack[0]
			queue.stack = queue.stack[1:]
			// Chame a função sendAllMessages com os dados da solicitação
			if request.tipo == "image" {
				go queue.sendAllMessages(request.ignore, *request.data, *request.text, request.kind, request.ddd)
			} else if request.tipo == "video" {
				go queue.sendAllMessagesVideo(request.ignore, *request.data, *request.text, request.kind, request.ddd)
			} else if request.tipo == "link" {
				go queue.sendAllMessagesLink(request.ignore, request.message, request.kind, request.ddd)
			}
			time.Sleep(time.Duration(queue.intervalo * time.Hour))
		} else {
			time.Sleep(time.Duration(10 * time.Second))
		}
	}
}

func (w *WhatsAppWorker) inicializaFila() {
	queueN = w.NewMessageQueue(1)
	queueAll = w.NewMessageQueue(1)
	go w.processStack(queueN)
	go w.processStack(queueAll)

}
