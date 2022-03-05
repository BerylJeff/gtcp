package logger

import (
	"io"
	"log"
	"os"
)

func init(){
	f, err := os.OpenFile("./log/log.txt", os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		log.Fatalf("create file log.txt failed: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
}



