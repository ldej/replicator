package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func StartWebServer(port int, cluster *Cluster) {
	app := App{cluster: cluster}

	r := mux.NewRouter()
	r.HandleFunc("/{key}", app.Put).Methods(http.MethodPost)
	r.HandleFunc("/{key}", app.Get).Methods(http.MethodGet)
	r.HandleFunc("/{key}/local", app.PutLocal).Methods(http.MethodPost)
	r.HandleFunc("/{key}/proposed", app.PutProposed).Methods(http.MethodPost)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}
	log.Fatal(server.ListenAndServe())
}

type App struct {
	server  *http.Server
	cluster *Cluster
}

type StoreRequest struct {
	Value string `json:"value"`
}

type Response struct {
	Errors PeerErrors `json:"errors,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

func (a *App) Put(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	var storeRequest StoreRequest
	var response Response

	_ = json.NewDecoder(r.Body).Decode(&storeRequest)
	response.Errors = a.cluster.replicationService.Store(key, storeRequest.Value)
	json.NewEncoder(w).Encode(response)
}

func (a *App) Get(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	var response Response

	response.Values, response.Errors = a.cluster.replicationService.GetFromPeers(key)
	json.NewEncoder(w).Encode(response)
}

func (a *App) PutLocal(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	var storeRequest StoreRequest
	_ = json.NewDecoder(r.Body).Decode(&storeRequest)

	a.cluster.replicationService.StoreLocal(key, storeRequest.Value)

	w.WriteHeader(http.StatusOK)
}

func (a *App) PutProposed(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	var storeRequest StoreRequest
	_ = json.NewDecoder(r.Body).Decode(&storeRequest)

	a.cluster.replicationService.proposed.Store(key, storeRequest.Value)

	w.WriteHeader(http.StatusOK)
}
