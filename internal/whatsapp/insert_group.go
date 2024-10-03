package whatsapp

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"whatsapp-manager/internal/database"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func (w *DivulgacaoWorker) monitorInsert(juid string) {
	for {
		if !w.estaAtivo() {
			return
		}
		// Verifica se o campo "inserir" está true
		if w.db.IsInsertEnabled(juid) {
			// Se estiver true, chama a função callback
			w.insertNewGroups()
		}
		// Aguarda 5 minutos antes de verificar novamente
		time.Sleep(5 * time.Minute)
	}
}

func (w *DivulgacaoWorker) acceptGroup(url string) string {
	re := regexp.MustCompile(`https://chat.whatsapp.com/([\S]+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func (w *DivulgacaoWorker) joininvitelink(link string) (types.JID, error) {

	if w.acceptGroup(link) == "" {
		//	logWa.Errorf("Link Invalido: %s, deve começar com https://chat.whatsapp.com/[####]", link)
		return types.EmptyJID, whatsmeow.ErrIQNotFound
	}

	groupID, err := w.Cli.JoinGroupWithLink(link)
	return groupID, err
}

func (w *DivulgacaoWorker) insertNewGroups() {

	groups, _ := w.findAllGroups()
	totalGrupos := len(groups)
	if totalGrupos > 300 {
		w.db.UpdateConfig(w.Cli.Store.ID.User, "Acima de 300", totalGrupos)
		return
	}
	total := 0
	for {
		if !w.estaAtivo() {
			return
		}
		// Comece uma transação para selecionar e travar os próximos 10 registros
		tx, err := w.db.Conn.BeginTx(context.Background(), nil)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			break
		}
		var group database.Group
		if err = tx.QueryRowContext(context.Background(),
			`
		SELECT uuid, link, name, classify, date, created
			FROM groups
		where deleted is false and jid is null and approved is true
		order by date desc FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&group.UUID, &group.Link, &group.Name, &group.Classify, &group.Date, &group.Created); err != nil {
			tx.Rollback()
			if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("no rows in result set")) {
				break
			} else {
				fmt.Printf("Erro: %v\n", err)
				break

			}
		}
		// Processar os registros aqui
		defer tx.Rollback()

		gr, err := w.joininvitelink(group.Link)
		if err == nil {
			println("NOVO ", w.Cli.Store.ID.String(), " N", total, ": ", group.Name)
			total++
			_, err = tx.ExecContext(context.Background(), "UPDATE groups SET jid=$1 WHERE uuid=$2", gr.String(), group.UUID)
			if err != nil {
				log.Printf("erro ao atualizar grupo erro: %v", err)
			}
			w.db.UpdateConfig(w.Cli.Store.ID.User, "", totalGrupos+total)
			err = tx.Commit()
			if err == nil {
				time.Sleep(time.Duration(MapExponential(totalGrupos)+rand.Intn(5)) * time.Second)
			}
		} else if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("already-exists")) {
			err = nil
			tx.Rollback()
			time.Sleep(time.Duration(MapExponential(totalGrupos)+rand.Intn(5)) * time.Second)
		} else if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("not-authorized")) {
			err = nil
			tx.Rollback()
			time.Sleep(time.Duration(MapExponential(totalGrupos)+rand.Intn(5)) * time.Second)
		} else if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("websocket not connected")) {
			err = nil
			tx.Rollback()
			break
		} else if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("rate-overlimit")) {
			w.db.UpdateConfig(w.Cli.Store.ID.User, "rate-overlimit", totalGrupos)
			err = nil
			tx.Rollback()
			break
		} else {
			_, err = tx.ExecContext(context.Background(), "UPDATE groups SET deleted = true, error = $1 WHERE uuid = $2", err.Error(), group.UUID)
			if err != nil {
				println("ERRO ", w.Cli.Store.ID.String(), ": ", err.Error())
			}
			err = tx.Commit()
			if err == nil {
				time.Sleep(time.Duration(60 * time.Second))
			} else {
				time.Sleep(time.Duration(24 * 1 * time.Hour))
				total = 0
			}
		}
		if totalGrupos+total >= 300 {
			w.db.UpdateConfig(w.Cli.Store.ID.User, "Acima de 300", totalGrupos)
			return
		}
	}
}

// MapExponential maps an integer from range [0, 300] to [30, 100] with exponential growth and returns an integer
func MapExponential(x int) int {
	if x < 0 || x > 300 {
		panic("Input x must be in the range [0, 300]")
	}
	k := 5.0
	result := 60 + 100*(1-math.Exp(-k*float64(x)/300))
	return int(math.Round(result)) // Convert the result to an integer by rounding
}

func (w *DivulgacaoWorker) GetActiveGroups() {

	groups, _ := w.findAllGroups()
	println("Atualizando Grupos por telefone...removendo duplicados..")
	for _, group := range groups {
		println(group, group.Name)
		w.db.InsertGroupFone(w.Cli, group, "", len(group.Participants))
	}
}
