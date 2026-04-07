package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
)

func init() {
	log.Println("Initializing application...")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	}
	tmpl = template.Must(template.ParseFiles("templates/index.html"))
	log.Println("Templates parsed successfully")
}

func main() {
	log.Println("Starting ipinfo service...")

	hadCachedDB, err := bootstrapDatabase()
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		os.Exit(1)
	}

	if hadCachedDB {
		go func() {
			log.Println("Running background database update...")
			updateDB()
			startUpdateTicker()
		}()
	} else {
		go startUpdateTicker()
	}

	http.HandleFunc("/", handleRequest)
	log.Printf("Server listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
