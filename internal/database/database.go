package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type Database struct {
	Conn *sql.DB
}

type Group struct {
	UUID         uuid.UUID  `json:"uuid"`
	Date         time.Time  `json:"date"`
	Deleted      bool       `json:"deleted"`
	Description  string     `json:"description"`
	Link         string     `json:"link"`
	Name         string     `json:"name"`
	JID          string     `json:"jid"`
	Code         string     `json:"code"`
	LastTopic    string     `json:"last_topic"`
	LastName     string     `json:"last_name"`
	Created      *time.Time `json:"created"`
	LastJID      string     `json:"last_jid"`
	AddMode      string     `json:"add_mode"`
	ApprovalMode string     `json:"approval_mode"`
	IsAdmin      bool       `json:"is_admin"`
	IsSuper      bool       `json:"is_super"`
	IsLocked     bool       `json:"is_locked"`
	Invalid      bool       `json:"invalid"`
	Owner        string     `json:"owner"`
	Classify     *string    `json:"classify"`
	Photo        *string    `json:"photo"`
}

type Message struct {
	UUID    uuid.UUID
	JUID    string
	Chat    string
	Name    string
	Date    time.Time
	Message string
}

func NewDatabase() (*Database, error) {

	db, err := sql.Open(os.Getenv("DIALECT"), os.Getenv("ADDRESS"))
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return &Database{db}, nil
}

func (d *Database) GetGroup(id string) *string {
	var err error
	cmdGroupJUID, err := d.getCmdGroupJUID(id)
	fmt.Printf("cmdGroupJUID: %v\n", cmdGroupJUID)
	if err != nil {
		log.Println("Forneça o convite do grupo onde serão enviado os comandos:")
		return nil
	} else {
		return &cmdGroupJUID
	}
}

func (d *Database) getCmdGroupJUID(juid string) (string, error) {
	var cmdGroupJUID string
	err := d.Conn.QueryRow("SELECT cmd_group_juid FROM config WHERE juid = $1", juid).Scan(&cmdGroupJUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("no row found")
		}
		return "", err
	}
	return cmdGroupJUID, nil
}

func (d *Database) JuidExists(cli *whatsmeow.Client, juid types.JID) (bool, error) {
	var exists bool
	err := d.Conn.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM groups_on WHERE juid = $1 AND date > current_timestamp - interval '30 minutes');
	`, juid.String()).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		duplicado, err := d.AnotherSend(juid, cli.Store.ID.User)
		if err == nil && duplicado {
			cli.LeaveGroup(juid)
		}
	}
	return exists, nil
}

func (d *Database) ValidGroupKind(juid types.JID, category []string, ddd *[]string) bool {
	var exists bool

	if ddd != nil && len(*ddd) > 0 {
		sql := "SELECT EXISTS (SELECT 1 FROM groups WHERE jid = $1 AND category = ANY($2) AND ddd = ANY($3) )"
		err := d.Conn.QueryRow(sql,
			juid.String(), pq.Array(category), pq.Array(*ddd)).Scan(&exists)
		if err != nil {
			log.Printf("Falha ao veriricar grupo com ddd: %v %s", err, *ddd)
			return false
		}
	} else {
		sql := "SELECT EXISTS (SELECT 1 FROM groups WHERE jid = $1 AND category = ANY($2) )"
		err := d.Conn.QueryRow(sql,
			juid.String(), pq.Array(category)).Scan(&exists)
		if err != nil {
			return true
		}
	}
	return exists
}

func (d *Database) ValidGroupForbidden(juid types.JID) bool {
	var exists bool
	sql := "SELECT EXISTS (SELECT 1 FROM groups WHERE jid = $1 and (forbidden is true or category IN ('PUTARIA')))"
	err := d.Conn.QueryRow(sql,
		juid.String()).Scan(&exists)
	if err != nil {
		return true
	}
	return !exists
}

func (d *Database) AnotherSend(juid types.JID, phone string) (bool, error) {
	var exists bool
	err := d.Conn.QueryRow(`
			SELECT EXISTS (SELECT 1 FROM groups_on WHERE juid = $1 AND date >= current_timestamp - interval '1 hours' AND sender <> $2)
	`, juid.String(), phone).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (d *Database) IsPhoneExists(juid types.JID) bool {
	var exists bool
	err := d.Conn.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM config WHERE juid = $1);
	`, juid.User).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func (d *Database) IsInsertEnabled(juid string) bool {
	var inserir bool
	err := d.Conn.QueryRow(`
		SELECT COALESCE(inserir, false) FROM config WHERE juid = $1;
	`, juid).Scan(&inserir)
	if err != nil {
		log.Printf("falha ao verificar campo inserir: %v", err)
		return false
	}
	return inserir
}

func (d *Database) CreateGroup(juid types.JID, name string, code *string, sender string, msg string, err1 error) error {
	erStr := ""
	if err1 != nil {
		erStr = err1.Error()
	}
	_, err := d.Conn.Exec(`
		INSERT INTO groups_on (juid, name, code, sender, msg, error) VALUES ($1, $2, $3, $4, $5, $6)
	`, juid.String(), name, code, sender, msg, erStr)

	/*if erStr == "context deadline exceeded" || erStr == "failed to get device list: unknown user server 'lid'" {
		log.Printf("removendo grupo %s", name)
		err2 := cli.LeaveGroup(juid)
		log.Printf("removendo grupo ERRR: %W", err2)
		fmt.Println(cli.GetGroupInfo(juid))
	}*/
	return err
}

func (d *Database) UpdateConfig(juid string, lastError string, totalGrupos int) error {
	_, err := d.Conn.Exec(
		"UPDATE config SET last_update = $1, last_error = $2, total_grupos = $3 WHERE juid = $4",
		time.Now(), lastError, totalGrupos, juid,
	)
	if err != nil {
		log.Printf("falha ao atualizar configuração: %v", err)
		return err
	}
	return nil
}

func (d *Database) InsertLink(text, jid string) error {
	_, err := d.Conn.Exec("INSERT INTO public.links (uuid, date, text, jid) VALUES ($1, $2, $3, $4)",
		uuid.New(), time.Now(), text, jid)
	if err != nil {
		log.Printf("falha ao inserir link: %v", err)
		return err
	}
	return nil
}

func (d *Database) InsertConfig(juid string, cmdGroupJUID string) error {
	_, err := d.Conn.Exec("INSERT INTO config (uuid, juid, cmd_group_juid) VALUES ($1, $2, $3)", uuid.New(), juid, cmdGroupJUID)
	if err != nil {
		log.Printf("falha ao veriricar grupo: %v", err)
		return err
	}
	return nil
}

func (d *Database) VerifyAndInsertTelegram(link string, newGroup Group) bool {

	// Verifica se o link já existe na tabela groups
	var exists bool
	err := d.Conn.QueryRow("SELECT EXISTS (SELECT 1 FROM telegram_groups WHERE link=$1 LIMIT 1)", link).Scan(&exists)
	if err != nil {
		log.Printf("falha ao veriricar grupo: %v", err)
		return false
	}

	// Insere os dados na tabela se o link não existir
	if !exists {
		_, err = d.Conn.Exec(`
			INSERT INTO telegram_groups (uuid, chat_id, date, is_deleted, description, link, title, 
									     is_fake, is_public, is_scam, is_verified, member_count, extra ) 
								VALUES ($1, $2, $3, $4, $5, $6, $7,$8,$9,$10,$11,$12,$13)
			`,
			uuid.New(), 0, time.Now(), false, newGroup.Description, link, newGroup.Name,
			false, false, false, false, 0, "")
		if err != nil {
			log.Printf("falha ao inserir grupo: %v", err)
			return false
		} else {
			fmt.Println("Dados inseridos com sucesso!")
			return true
		}

	} else {
		fmt.Println("O link já existe na tabela!")
		return false
	}
}

func (d *Database) VerifyAndInsert(newGroup Group) (Group, bool) {

	// Verifica se o link já existe na tabela groups
	var exists bool
	err := d.Conn.QueryRow("SELECT EXISTS (SELECT 1 FROM groups WHERE link=$1 LIMIT 1)", newGroup.Link).Scan(&exists)
	if err != nil {
		log.Printf("falha ao veriricar grupo: %v", err)
		return newGroup, false
	}

	// Insere os dados na tabela se o link não existir
	if !exists {
		newGroup.Date = time.Now()
		newGroup.Deleted = false
		newGroup.UUID = uuid.New()

		_, err = d.Conn.Exec("INSERT INTO groups (uuid, date, deleted, description, link, name,code) VALUES ($1, $2, $3, $4, $5, $6,$7)", newGroup.UUID, newGroup.Date, newGroup.Deleted, newGroup.Description, newGroup.Link, newGroup.Name, newGroup.Code)
		if err != nil {
			log.Printf("falha ao inserir grupo: %v", err)
			return newGroup, false
		} else {
			fmt.Println("Dados inseridos com sucesso!")
			return newGroup, true
		}

	} else {
		fmt.Println("O link já existe na tabela!")
		return newGroup, false
	}
}

func (d *Database) InsertGroupFone(cli *whatsmeow.Client, group *types.GroupInfo, link string, total int) {

	// Verifica se o link já existe na tabela groups
	var exists bool
	var leave bool
	phone := cli.Store.ID.User
	err := d.Conn.QueryRow("SELECT true, leave FROM groups_phone WHERE jid=$1 LIMIT 1", group.JID).Scan(&exists, &leave)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), strings.ToLower("no rows in result set")) {
		exists = false
	} else if err != nil {
		log.Printf("falha ao veriricar grupo: %v", err)
		return
	}

	// Insere os dados na tabela se o link não existir
	if !exists {
		_, err = d.Conn.Exec(`
		INSERT INTO groups_phone ( uuid, date, phone, jid, name, description, link, created, paticipants) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, uuid.New(), time.Now(), phone, group.JID, group.Name, group.Topic, link, group.GroupCreated, total)
		if err != nil {
			log.Printf("falha ao inserir grupo: %v", err)
			return
		} else {
			fmt.Println("Dados inseridos com sucesso!")
			return
		}
	} else {
		_, err = d.Conn.Exec(` update groups_phone set paticipants = $1  WHERE jid=$2 `, total, group.JID)
		if err != nil {
			log.Printf("grupo atualizado: %v", err)
		} else {
			fmt.Println("Dados atualizado com sucesso!")
		}
		if leave || total < 2 {
			cli.LeaveGroup(group.JID)
			time.Sleep(time.Duration(1 * time.Minute))
		}
	}
}

func (d *Database) InsertMessage(message Message) error {
	_, err := d.Conn.Exec("INSERT INTO messages (uuid, juid, chat, name, message) VALUES ($1, $2, $3, $4, $5)",
		message.UUID, message.JUID, message.Chat, message.Name, message.Message)
	if err != nil {
		return err
	}

	return nil
}

type DeviceInfo struct {
	JUID        string
	LastUpdate  time.Time
	TotalGrupos int
}

func (d *Database) RemoveDevice(juid string) error {
	// Verificar se o JUID está vazio
	if juid == "" {
		return errors.New("JUID não pode estar vazio")
	}

	// Preparar o comando SQL para desativar o dispositivo
	query := `UPDATE config SET active = false WHERE juid = $1`

	// Executar o comando de update no banco de dados
	result, err := d.Conn.Exec(query, juid)
	if err != nil {
		return fmt.Errorf("erro ao desativar o dispositivo com JUID %s: %w", juid, err)
	}

	// Verificar se algum registro foi atualizado
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("erro ao verificar linhas afetadas: %w", err)
	}
	if rowsAffected == 0 {
		return errors.New("nenhum dispositivo foi encontrado com o JUID fornecido")
	}

	return nil
}

func (d *Database) GetActiveDevicesInfo() ([]DeviceInfo, error) {
	rows, err := d.Conn.Query(`
		SELECT juid, last_update, total_grupos
		FROM config
		WHERE active = true
		ORDER BY last_update DESC
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devicesInfo []DeviceInfo
	for rows.Next() {
		var deviceInfo DeviceInfo
		if err := rows.Scan(&deviceInfo.JUID, &deviceInfo.LastUpdate, &deviceInfo.TotalGrupos); err != nil {
			return nil, err
		}
		devicesInfo = append(devicesInfo, deviceInfo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return devicesInfo, nil
}

func (d *Database) GetNewLinks() ([]Group, error) {
	rows, err := d.Conn.Query(`
	SELECT uuid, link, name, classify, last_topic, date, created
		FROM groups
	where deleted is false and jid is null 
	and approved is true
	order by date desc
 `)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var group Group
		if err := rows.Scan(&group.UUID, &group.Link, &group.Name, &group.Classify, &group.LastTopic, &group.Date, &group.Created); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

func (d *Database) UpdateJID(group Group, jid string) {
	log.Println(jid, group.UUID)
	tx, err := d.Conn.Exec("UPDATE groups SET jid=$1 WHERE uuid=$2", jid, group.UUID)
	if err != nil {
		log.Printf("falha ao atulizar link %v", err)
	}
	log.Println(tx.RowsAffected())

}

func (d *Database) CloneConnection() {
	defer d.Conn.Close()
}

func (d *Database) UpdateGroup(g Group, resp *types.GroupInfo) error {
	query := `
		UPDATE groups
		SET
		    last_name = $1,
			created = $2,
			approval_mode = $3,
			is_locked = $4,
			add_mode = $5,
			last_topic = $6,
			is_admin = $7,
			is_super = $8,
			last_jid = $9,
			owner = $10
		WHERE
			uuid = $11
	`

	var isAdmin = false
	var isSuper = false
	if resp.Participants != nil && len(resp.Participants) > 0 {
		isAdmin = resp.Participants[0].IsAdmin
		isSuper = resp.Participants[0].IsSuperAdmin
	}
	_, err := d.Conn.Exec(query, resp.GroupName.Name, resp.GroupCreated, resp.DefaultMembershipApprovalMode, resp.IsLocked, resp.MemberAddMode, resp.GroupTopic.Topic, isAdmin, isSuper, resp.JID.String(), resp.OwnerJID.User, g.UUID)
	if err != nil {
		log.Printf("Failed to update group: %v", err)
		return err
	}

	log.Printf("group update: %v", g.UUID)

	return nil
}

func (d *Database) InvalidGroup(g Group) error {
	query := `
		UPDATE groups
		SET
		    invalid = true
		WHERE
			uuid = $1
	`
	_, err := d.Conn.Exec(query, g.UUID)
	if err != nil {
		log.Printf("Failed to update group: %v", err)
		return err
	}

	return nil
}

func (d *Database) InvalidGroupLink(link string, er string) error {
	query := `
		UPDATE groups
		SET
		    deleted = true,
			error = $1
		WHERE
			link = $2
	`
	_, err := d.Conn.Exec(query, er, link)
	if err != nil {
		log.Printf("Failed to update group: %v", err)
		return err
	}

	return nil
}

func (d *Database) ParticipantExists(groupJID, userJID string) (bool, error) {
	// SQL statement to check if a participant exists
	sqlStatement := `
		SELECT COUNT(*) FROM public.participants
		WHERE group_jid = $1 AND user_jid = $2;
	`

	var count int
	err := d.Conn.QueryRow(sqlStatement, groupJID, userJID).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (d *Database) GetGroupsFromPublic(category []string, ddd string) ([]string, error) {
	rows, err := d.Conn.Query(`
	SELECT jid
	FROM groups
	where approved is true
		and deleted is false
		and invalid is null
		AND category = ANY($1)
		AND ddd  = ($2)
		and jid is not null`, category, ddd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

func (d *Database) InsertParticipant(groupJID, groupName, userJID, status, picture string, device []string, phones []string) error {

	// Check if the participant already exists
	exists, err := d.ParticipantExists(groupJID, userJID)
	if err != nil {
		return err
	}

	if exists {
		fmt.Println("Participant already exists with the same group_jid and user_jid.")
		return nil
	}

	// SQL statement with placeholders
	sqlStatement := `
		INSERT INTO public.participants (group_jid, group_name, user_jid, status, picture, device, phones)
		VALUES ($1, $2, $3, $4, $5, $6,$7);
	`
	// Executing the SQL statement

	_, err2 := d.Conn.Exec(sqlStatement, groupJID, groupName, userJID, status, picture, pq.StringArray(device), pq.StringArray(phones))
	if err2 != nil {
		fmt.Printf("err2: %v\n", err2)
		return err2
	}
	return nil
}
