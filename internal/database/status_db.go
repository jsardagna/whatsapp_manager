package database

import (
	"database/sql"
	"log"
)

func UpdateStatus(db *sql.DB, deviceID string, status string) error {
	_, err := db.Exec("UPDATE status SET status = $1 WHERE device_id = $2", status, deviceID)
	if err != nil {
		log.Println("Erro ao atualizar status:", err)
		return err
	}
	return nil
}
