package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func main() {

	router := mux.NewRouter()
	router.HandleFunc("/provision", provision)

	log.Fatal(http.ListenAndServe(":8080", router))
}

func provision(w http.ResponseWriter, r *http.Request) {

	json, err := r.GetBody()
	if err != nil {
		fmt.Println("cannot read body")
	}

	
}
