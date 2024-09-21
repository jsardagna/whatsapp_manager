package rest

import (
	"net/http"
)

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	// Lógica para retornar status dos dispositivos
	w.Write([]byte("Status OK"))
}
