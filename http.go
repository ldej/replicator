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
	r.HandleFunc("/local/stored/{key}", app.GetStored).Methods(http.MethodGet)
	r.HandleFunc("/local/proposed/{key}", app.PutProposed).Methods(http.MethodPut)
	r.HandleFunc("/local/proposed/{key}", app.GetProposed).Methods(http.MethodGet)
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
	Errors PeerErrors `json:"errors,omitempty"`
	Error  string     `json:"error,omitempty"`
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
	response.Errors, err = a.cluster.replicationService.Store(key, value)
	if err != nil {
		response.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (a *App) Get(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	value, err := a.cluster.replicationService.GetGlobal(key)
	if err != nil {
		log.Printf("Error while getting value for key %q: %-v", key, err)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(value)
}

func (a *App) PutStored(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	value, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading body failed: %-v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.cluster.replicationService.StoreLocal(key, value)

	w.WriteHeader(http.StatusOK)
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

	a.cluster.replicationService.proposed[key] = value

	w.WriteHeader(http.StatusOK)
}

func (a *App) GetStored(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	value, err := a.cluster.replicationService.GetLocal(key)
	if err == ErrNotFound {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Write(value)
	w.WriteHeader(http.StatusOK)
}

func (a *App) GetProposed(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	a.cluster.replicationService.storeLock.Lock()
	defer a.cluster.replicationService.storeLock.Unlock()

	value, found := a.cluster.replicationService.proposed[key]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Write(value)
	w.WriteHeader(http.StatusOK)
}

func (a *App) Peers(w http.ResponseWriter, r *http.Request) {
	peers, _ := a.cluster.Peers(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(peers)
}
