package data

import (
	"log"
	"os"
)

func DataPush(bucket string, entry string) {

	db, err := os.OpenFile(bucket, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	_entry := entry + "\n"
	_, err = db.Write([]byte(_entry))

	if err != nil {
		log.Fatal(err)
	}
}
