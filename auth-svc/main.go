/*
Piccola frontend con user-password form e login button
Quando il login è fatto, viene generato un access e refresh token che vengono codificati firmati dalla chiave privata del auth-svc
Viene salvata la sessione (il refresh token) nel KV-Store e viene annunciata la creazione della sessione sul message broker.

Quando un servizio viene colpito (oppure un gateway iniziale) e non trova token, allora bisogna generare un token randomico, associare quell'utente a quel token, e re-indirizzare l'utente verso il login.
Eseguito il login, avviene un-reindirizzamento al gateway originale, che a questo punto controllerà il token randomico, vedrà a che percorso originalmente voleva puntare
*/

package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {

	port := 8080
	mux := http.NewServeMux()

	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/login called")
	})

	log.Printf("Auth Service running on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}
