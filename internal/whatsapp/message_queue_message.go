package whatsapp

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func (q *MessageQueue) sendAllMessagesLink(ignore string, msg *waE2E.Message, kind *[]string, ddd *[]string) {
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)

	newmsg := &waE2E.Message{ExtendedTextMessage: msg.ExtendedTextMessage}
	db := q.worker.db
	cli := q.worker.Cli
	groups, err := q.worker.findAllGroups()
	if err != nil {
		fmt.Println("FALHA AO BUSCAR GRUPO 2: ", q.worker.Cli.Store.ID.User, err.Error())
	} else {
		for _, group := range groups {

			if group.JID.String() == ignore ||
				group.JID.String() == "120363149950387591@g.us" ||
				group.JID.String() == "120363343818835998@g.us" ||
				group.JID.String() == "120363330490936340@g.us" {
				continue
			}

			valid := kind == nil && db.ValidGroupForbidden(group.JID) || kind != nil && db.ValidGroupKind(group.JID, *kind, ddd)
			if valid {
				cctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
				resp := make(chan whatsmeow.SendResponse)
				var r whatsmeow.SendResponse

				go func() {
					r, _ = cli.SendMessage(cctx, group.JID, newmsg)
					resp <- r
				}()
				select {
				case <-resp:
					fmt.Println("LINK ENVIADO ", q.worker.Cli.Store.ID.User, " GRUPO:", group.Name)

				case <-cctx.Done():
					fmt.Println(cctx.Err())
				}
			}
		}
	}
}

func (q *MessageQueue) sendAllMessages(ignore string, data []byte, msg string, kind *[]string, ddd *[]string, midia string) {
	w := q.worker
	db := w.db
	// envia imagem para servidor
	uploaded, err := w.uploadImage(data)
	if err != nil {
		log.Printf("Failed to upload file: %v", err)
		return
	}
	groups, err := w.findAllGroups()
	total := len(groups)
	atual := 0
	if err != nil {
		fmt.Println("FALHA AO BUSCAR GRUPOS: ", q.worker.Cli.Store.ID.User, err.Error())
		db.UpdateConfig(w.Cli.Store.ID.User, err.Error(), 0)
	} else {
		w.sending = true
		startTime := time.Now()
		db.UpdateConfig(w.Cli.Store.ID.User, "ENVIO", len(groups))
		for _, group := range groups {
			atual++

			elapsedTime := time.Since(startTime)
			remainingTime := q.intervalo - elapsedTime - time.Duration(1)*time.Minute

			if !w.estaAtivo() {
				break
			}

			if group.JID.String() == ignore ||
				group.JID.String() == "120363149950387591@g.us" ||
				group.JID.String() == "120363343818835998@g.us" ||
				group.JID.String() == "120363330490936340@g.us" {
				continue
			}
			exist, _ := db.JuidExists(w.Cli, group.JID)
			if !exist {
				go q.ControleParcitipantes(group)
				q.sendMessage(kind, group, uploaded, data, msg, ddd, atual, total, startTime, midia)
			}
			if remainingTime <= 0 {
				break
			}
		}
		w.sending = false
	}

}

func (q *MessageQueue) ControleParcitipantes(group *types.GroupInfo) {
	q.mu.Lock() // Bloqueia o mapa para garantir segurança em ambiente concorrente
	if _, exists := q.alreadyCalledGroup[group.JID.String()]; !exists {
		q.alreadyCalledGroup[group.JID.String()] = true
		q.mu.Unlock() // Desbloqueia após a escrita no mapa

		if group.Participants != nil && len(group.Participants) > 0 { // Garante que há participantes
			go func(groupJID string, participantsCount int) {
				err := q.worker.db.UpdateParticipants(groupJID, participantsCount)
				if err != nil {
					log.Printf("Erro ao atualizar participantes para o grupo %s: %v", groupJID, err)
				}
			}(group.JID.String(), len(group.Participants))
		}
	} else {
		q.mu.Unlock() // Desbloqueia se o grupo já foi processado
	}
}

func (q *MessageQueue) sendMessage(kind *[]string, group *types.GroupInfo, uploaded whatsmeow.UploadResponse, data []byte, msg string, ddd *[]string, atual int, total int, startSend time.Time, midia string) {
	w := q.worker
	db := w.db

	valid := kind == nil || db.ValidGroupKind(group.JID, *kind, ddd)

	if valid {
		startTime := time.Now()
		modifiedMessage, groupCode := addGroupIDToURLs(msg)

		onSuccess := func() {
			elapsedTime := time.Since(startTime)
			fmt.Println(q.worker.Cli.Store.ID.User, midia, "ENVIADA:", group.Name)
			go db.CreateGroup(group.JID, group.Name, groupCode, w.Cli.Store.ID.User, msg, nil, elapsedTime.Seconds(), len(group.Participants), atual, total, startSend)
			q.waitNext(elapsedTime)
		}

		onError := func(err error) {
			elapsedTime := time.Since(startTime)
			if w.estaAtivo() {
				fmt.Println(q.worker.Cli.Store.ID.User, midia, "ERRO:", group.Name)
				go db.CreateGroup(group.JID, group.Name, groupCode, w.Cli.Store.ID.User, msg, err, elapsedTime.Seconds(), len(group.Participants), atual, total, startSend)
				q.waitNext(elapsedTime)
			}

		}
		if midia == "video" {
			w.sendVideo(group.JID, uploaded, data, modifiedMessage, onSuccess, onError)
		} else {
			w.sendImage(group.JID, uploaded, data, modifiedMessage, onSuccess, onError)
		}

		if !w.estaAtivo() {
			return
		}

		go db.VerifyToLeaveGroup(w.Cli, group)
	}
}

func (*MessageQueue) waitNext(elapsedTime time.Duration) {
	if elapsedTime < 7*time.Second {
		remainingTime := 7*time.Second - elapsedTime
		time.Sleep(remainingTime + time.Duration(rand.Intn(2))*time.Second)
	}
}

func (q *MessageQueue) AddParticipantes(group *types.GroupInfo) {
	for _, user := range group.Participants {
		q.worker.db.InsertParticipant(group.JID.String(), group.Name, user.JID.String(), user.JID.User, user.DisplayName)

	}
}

func generateGroupCode() string {
	rand.Seed(time.Now().UnixNano())
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	code := make([]rune, 6)
	for i := range code {
		code[i] = letters[rand.Intn(len(letters))]
	}
	return string(code)
}

// Função para adicionar o parâmetro ?id_grupo às URLs do domínio https://2ly.link
func addGroupIDToURLs(message string) (string, *string) {
	// Regex para identificar URLs do domínio https://2ly.link
	urlRegex := regexp.MustCompile(`https://2ly\.link/[A-Za-z0-9]+`)
	matches := urlRegex.FindAllString(message, -1)

	// Se não encontrar nenhuma URL, retorna a mensagem original e nil
	if len(matches) == 0 {
		return message, nil
	}

	groupCode := generateGroupCode()

	// Substitui as URLs no texto adicionando o parâmetro ?id_grupo=[CODIGO]
	modifiedMessage := urlRegex.ReplaceAllStringFunc(message, func(url string) string {
		// Verifica se a URL já tem parâmetros
		if strings.Contains(url, "?") {
			return url + "&id_grupo=" + groupCode
		}
		return url + "?id_grupo=" + groupCode
	})

	return modifiedMessage, &groupCode
}
