package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/mux"
)

func StartWebServer(port int, cluster *Cluster) {
	app := App{cluster: cluster}

	r := mux.NewRouter()
	r.HandleFunc("/store/{key}", app.Put).Methods(http.MethodPut)
	r.HandleFunc("/store/{key}", app.Get).Methods(http.MethodGet)
	r.HandleFunc("/local/stored/{key}", app.PutStored).Methods(http.MethodPut)
	r.HandleFunc("/local/proposed/{key}", app.PutProposed).Methods(http.MethodPut)
	r.HandleFunc("/local", app.GetLocal).Methods(http.MethodGet)
	r.HandleFunc("/peers", app.Peers).Methods(http.MethodGet)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}

	fmt.Println("HTTP Listening on:", listener.Addr().(*net.TCPAddr).Port)
	log.Fatal(http.Serve(listener, r))
}

type App struct {
	server  *http.Server
	cluster *Cluster
}

type Response struct {
	Errors PeerErrors        `json:"errors,omitempty"`
	Error  error             `json:"error,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

func (a *App) Put(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	var response Response

	value, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading body failed: %-v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.Errors, response.Error = a.cluster.replicationService.Store(key, string(value))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (a *App) Get(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	var response Response

	response.Values, response.Errors = a.cluster.replicationService.GetGlobal(key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) PutStored(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	value, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading body failed: %-v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.cluster.replicationService.StoreLocal(key, string(value))

	w.WriteHeader(http.StatusOK)
}

type StorageResponse struct {
	Stored   map[string]string `json:"stored"`
	Proposed map[string]string `json:"proposed"`
}

func (a *App) GetLocal(w http.ResponseWriter, r *http.Request) {
	a.cluster.replicationService.storeLock.Lock()
	defer a.cluster.replicationService.storeLock.Unlock()

	response := StorageResponse{
		Stored:   a.cluster.replicationService.stored,
		Proposed: a.cluster.replicationService.proposed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) PutProposed(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	value, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading body failed: %-v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.cluster.replicationService.storeLock.Lock()
	defer a.cluster.replicationService.storeLock.Unlock()

	a.cluster.replicationService.proposed[key] = string(value)

	w.WriteHeader(http.StatusOK)
}

func (a *App) Peers(w http.ResponseWriter, r *http.Request) {
	peers, _ := a.cluster.Peers(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(peers)
}
