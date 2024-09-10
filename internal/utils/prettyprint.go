package utils

import (
	"encoding/json"
	"log"
)

// PrettyPrint for debug only
func PrettyPrint(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		log.Println(string(b))
	}
}
