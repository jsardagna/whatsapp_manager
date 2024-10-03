package whatsapp

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	proto "google.golang.org/protobuf/proto"

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
		println("FALHA AO BUSCAR GRUPOS:", q.worker.device.ID.User)
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
					time.Sleep(time.Duration(15+rand.Intn(5)) * time.Second)
				case <-cctx.Done():
					fmt.Println(cctx.Err())
				}
			}
		}
	}
}

func (q *MessageQueue) sendAllMessagesVideo(ignore string, data []byte, msg string, kind *[]string, ddd *[]string) {
	// envia imagem para servidor
	cli := q.worker.Cli
	db := q.worker.db
	uploaded, err := cli.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		log.Printf("Failed to upload file: %v", err)
		return
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	groups, err := q.worker.findAllGroups()
	if err != nil {
		log.Printf("Failed to get group list: %v", err)
	} else {
		for _, group := range groups {

			if group.JID.String() == ignore ||
				group.JID.String() == "120363149950387591@g.us" ||
				group.JID.String() == "120363343818835998@g.us" ||
				group.JID.String() == "120363330490936340@g.us" {
				continue
			}

			exists, err := db.JuidExists(cli, group.JID)
			if err != nil {
				log.Printf("Failed to QUERY to DB: %v", err)
			}
			if !exists {
				valid := kind == nil && db.ValidGroupForbidden(group.JID) || kind != nil && db.ValidGroupKind(group.JID, *kind, ddd)
				if valid {
					//clientLog.Infof("%+v", group)
					q.SendVideo(group, uploaded, data, msg, ddd)
				}
			}

		}
	}
}

func (q *MessageQueue) SendVideo(group *types.GroupInfo, uploaded whatsmeow.UploadResponse, data []byte, msg string, ddd *[]string) {
	cctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
	resp := make(chan whatsmeow.SendResponse)
	go func() {
		r, err2 := q.sendMessageVideo(cctx, group.JID, uploaded, data, msg)
		q.worker.db.CreateGroup(group.JID, group.Name, nil, q.worker.Cli.Store.ID.User, msg, err2)
		resp <- r
	}()
	select {
	case <-cctx.Done():
		fmt.Println(cctx.Err())
		q.worker.db.CreateGroup(group.JID, group.Name, nil, q.worker.Cli.Store.ID.User, msg, cctx.Err())
	case <-resp:
		fmt.Println("VIDEO ENVIADO ", q.worker.Cli.Store.ID.User, " GRUPO:", group.Name)
		time.Sleep(time.Duration(15+rand.Intn(5)) * time.Second)
	}
}

func (q *MessageQueue) sendAllMessages(ignore string, data []byte, msg string, kind *[]string, ddd *[]string) {
	w := q.worker
	db := w.db
	// envia imagem para servidor
	uploaded, err := w.uploadImage(data)
	if err != nil {
		log.Printf("Failed to upload file: %v", err)
		return
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	groups, err := w.findAllGroups()

	if err != nil {
		fmt.Println("FALHA AO BUSCAR GRUPOS: ", q.worker.Cli.Store.ID.User)
	} else {
		db.UpdateConfig(w.Cli.Store.ID.User, "ENVIO", len(groups))
		for _, group := range groups {

			if group.JID.String() == ignore ||
				group.JID.String() == "120363149950387591@g.us" ||
				group.JID.String() == "120363343818835998@g.us" ||
				group.JID.String() == "120363330490936340@g.us" {
				continue
			}

			exists, err := db.JuidExists(w.Cli, group.JID)
			if err != nil {
				log.Printf("Failed to QUERY to DB: %v", err)
			}
			if !exists {
				//clientLog.Infof("%+v", group)
				q.sendMessages(kind, group, uploaded, data, msg, ddd)
			}

		}
	}
}

func (q *MessageQueue) sendMessages(kind *[]string, group *types.GroupInfo, uploaded whatsmeow.UploadResponse, data []byte, msg string, ddd *[]string) {
	w := q.worker
	db := w.db
	cli := w.Cli
	valid := kind == nil || db.ValidGroupKind(group.JID, *kind, ddd)

	if valid {

		cctx, _ := context.WithTimeout(context.Background(), 15*time.Second)
		resp := make(chan whatsmeow.SendResponse)
		go func() {
			r, err2 := w.sendMessage(cctx, group.JID, uploaded, data, msg)
			go db.CreateGroup(group.JID, group.Name, nil, cli.Store.ID.User, msg, err2)
			resp <- r
		}()
		select {
		case <-cctx.Done():
			fmt.Println(q.worker.Cli.Store.ID.User, cctx.Err())
			go db.CreateGroup(group.JID, group.Name, nil, cli.Store.ID.User, msg, cctx.Err())
		case <-resp:
			fmt.Println(q.worker.Cli.Store.ID.User, "IMAGEM ENVIADA:", group.Name)
			time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second)
		}
	}
}

func (q *MessageQueue) sendMessageVideo(ctx context.Context, recipient types.JID, uploaded whatsmeow.UploadResponse, data []byte, caption string) (resp whatsmeow.SendResponse, err error) {

	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String(http.DetectContentType(data)),
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(data))),
	}}
	resp, err = q.worker.Cli.SendMessage(ctx, recipient, msg)
	if err != nil {
		//logWa.Errorf("Error sending image message: %v", err)
	} else {
		//logWa.Infof("Image message sent (server timestamp: %s)", resp.Timestamp)
	}
	return resp, err
}
