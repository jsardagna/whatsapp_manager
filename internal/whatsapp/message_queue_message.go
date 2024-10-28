package whatsapp

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"sort"
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

	// Função para inverter a slice
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Participants) > len(groups[j].Participants) && len(groups[i].Participants) < 600 ||
			len(groups[i].Participants) < len(groups[j].Participants) && len(groups[i].Participants) > 600
	})

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

	// Função para inverter a slice
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Participants) > len(groups[j].Participants) && len(groups[i].Participants) < 600 ||
			len(groups[i].Participants) < len(groups[j].Participants) && len(groups[i].Participants) > 600
	})

	total := len(groups)
	atual := 0
	if err != nil {
		fmt.Println("FALHA AO BUSCAR GRUPOS: ", q.worker.Cli.Store.ID.User, err.Error())
		db.UpdateConfig(w.Cli.Store.ID.User, err.Error(), 0)
	} else {

		startTime := time.Now()
		db.UpdateConfig(w.Cli.Store.ID.User, "ENVIO", total)
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
			startTimeGroup := time.Now()
			if !db.JuidExists(w.Cli, group.JID) && db.VerifyToLeaveGroup(w.Cli, group) && w.estaAtivo() {
				q.removeLidParticipants(group)
				go q.ControleParcitipantes(group)
				q.sendMessage(kind, group, uploaded, data, msg, ddd, atual, total, startTime, midia, startTimeGroup)

			}
			if remainingTime <= 0 {
				break
			}
		}
	}

}
func (q *MessageQueue) removeLidParticipants(group *types.GroupInfo) {
	for j := len(group.Participants) - 1; j >= 0; j-- {
		if strings.HasSuffix(group.Participants[j].JID.String(), "@lid") {
			group.Participants = append(group.Participants[:j], group.Participants[j+1:]...)
		}
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

func (q *MessageQueue) sendMessage(kind *[]string, group *types.GroupInfo, uploaded whatsmeow.UploadResponse, data []byte, msg string, ddd *[]string, atual int, total int, startSend time.Time, midia string, startTime time.Time) {
	w := q.worker
	db := w.db

	valid := kind == nil || db.ValidGroupKind(group.JID, *kind, ddd)

	if valid {
		modifiedMessage, groupCode := addGroupIDToURLs(msg)

		onSuccess := func() {
			elapsedTime := time.Since(startTime)
			fmt.Println(q.worker.Cli.Store.ID.User, midia, "ENVIADA:", group.Name)
			go db.CreateGroup(group.JID, group.Name, groupCode, w.Cli.Store.ID.User, msg, nil, elapsedTime.Seconds(), len(group.Participants), atual, total, startSend)
			q.waitNext(elapsedTime, total)
		}

		onError := func(err error) {
			elapsedTime := time.Since(startTime)
			if w.estaAtivo() {
				fmt.Println(q.worker.Cli.Store.ID.User, midia, "ERRO:", group.Name, err.Error())
				str := err.Error()
				go db.CreateGroup(group.JID, group.Name, groupCode, w.Cli.Store.ID.User, msg, err, elapsedTime.Seconds(), len(group.Participants), atual, total, startSend)
				q.waitNext(elapsedTime, total)
				if str == "server returned error 420" || str == "server returned error 401" {
					w.Cli.LeaveGroup(group.JID)
				}
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

	}
}

func (q *MessageQueue) waitNext(elapsedTime time.Duration, totalGrupos int) {
	// Define a duração total para processar todos os grupos, com base no intervalo definido no worker
	totalDuration := q.worker.Interval - 20*time.Minute

	// Calcula o tempo mínimo entre o processamento de cada grupo
	minTime := totalDuration / time.Duration(totalGrupos)

	// Verifica se o tempo decorrido é menor que o tempo mínimo esperado por grupo
	if elapsedTime < minTime {
		remainingTime := minTime - elapsedTime
		// Pausa o processo pelo tempo restante mais um atraso aleatório de 0 a 2 segundos
		time.Sleep(remainingTime + time.Duration(rand.Intn(3))*time.Second)
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
