package database

import (
	"database/sql"
	"log"
)

func StoreWhatsAppMessage(db *sql.DB, message string) error {
	_, err := db.Exec("INSERT INTO messages (message) VALUES ($1)", message)
	if err != nil {
		log.Println("Erro ao armazenar mensagem no banco do WhatsApp:", err)
		return err
	}
	return nil
}
