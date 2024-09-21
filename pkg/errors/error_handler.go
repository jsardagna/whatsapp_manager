package errors

import (
	"log"
)

func HandleError(err error, message string) {
	if err != nil {
		log.Printf("%s: %v", message, err)
	}
}
