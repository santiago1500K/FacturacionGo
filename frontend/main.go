// Servidor estático para el frontend (HTML + Bootstrap + JavaScript).
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.Handle("/", http.FileServer(http.Dir("public")))
	log.Printf("Frontend disponible en http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
