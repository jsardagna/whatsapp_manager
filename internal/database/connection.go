package database

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq" // Driver para PostgreSQL
)

func ConnectToWhatsAppDB() (*sql.DB, error) {
	db, err := sql.Open("postgres", "user=whatsapp password=secret dbname=whatsapp sslmode=disable")
	if err != nil {
		log.Println("Erro ao conectar ao banco WhatsApp:", err)
		return nil, err
	}
	return db, nil
}

func ConnectToStatusDB() (*sql.DB, error) {
	db, err := sql.Open("postgres", "user=status password=secret dbname=status sslmode=disable")
	if err != nil {
		log.Println("Erro ao conectar ao banco Status:", err)
		return nil, err
	}
	return db, nil
}
