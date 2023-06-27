package logger

import (
	"log"
	"os"
)

func init(){
	path := "./log/"
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		log.Fatalf("create file log.txt failed: %v", err)
		return
	}
	f, err := os.OpenFile(path + "log.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("create file log.txt failed: %v", err)
		return
	}
	//log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
}



