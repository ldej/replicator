package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"

	_ "github.com/ldej/replicator/docs"

	"github.com/gorilla/mux"
	"github.com/swaggo/http-swagger"
)

// @title Replicator
// @version 1.0
// @description This is a libp2p replicator
// @termsOfService http://swagger.io/terms/

// @contact.name Laurence de Jong
// @contact.url https://ldej.nl
// @contact.email info@ldej.nl

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @BasePath /
// @query.collection.format multi

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
	r.HandleFunc("/clusters", app.Clusters).Methods(http.MethodGet)
	r.PathPrefix("/swagger").Handler(httpSwagger.WrapHandler)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}

	port = listener.Addr().(*net.TCPAddr).Port

	fmt.Println("HTTP Listening on:", port)
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

// Put example
// @Summary Add a key and value
// @Accept  plain
// @Produce json
// @Param   key      path   string     true  "Key"
// @Param   value    body   string     true  "Value"
// @Success 200 {string} json	"Empty"
// @Failure 500 {object} Response "Something went wrong"
// @Router /store/{key} [put]
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Get example
// @Summary Get the value for a key
// @Produce json
// @Param   key      path   string     true  "Key"
// @Success 200 {string} plain	"Bytes"
// @Failure 500 {object} string "Something went wrong"
// @Router /store/{key} [get]
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

// PutStored example
// @Summary Add a key and value to the local peer's "stored" map
// @Accept  plain
// @Produce json
// @Param   key      path   string     true  "Key"
// @Param   value    body   string     true  "Value"
// @Success 200 {string} json	"Empty"
// @Router /local/stored/{key} [put]
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

// PutProposed example
// @Summary Add a key and value to the local peer's proposed map
// @Accept  plain
// @Produce json
// @Param   key      path   string     true  "Key"
// @Param   value    body   string     true  "Value"
// @Success 200 {string} json	"Empty"
// @Router /local/proposed/{key} [put]
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

// GetProposed example
// @Summary Get the value for a key from the local peer's "stored" map
// @Produce json
// @Param   key      path   string     true  "Key"
// @Success 200 {string} plain	"Bytes"
// @Failure 500 {object} string "Something went wrong"
// @Router /local/stored/{key} [get]
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

// GetProposed example
// @Summary Get the value for a key from the local peer's "proposed" map
// @Produce json
// @Param   key      path   string     true  "Key"
// @Success 200 {string} plain	"Bytes"
// @Failure 500 {object} string "Something went wrong"
// @Router /local/proposed/{key} [get]
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

// Peers example
// @Summary Get the peer information from all peers
// @Produce json
// @Success 200 {object} string	"Peers"
// @Router /peers [get]
func (a *App) Peers(w http.ResponseWriter, r *http.Request) {
	peers, _ := a.cluster.Peers(r.Context())
	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(peers)
}

// Clusters example
// @Summary Get the peer information from all clusters
// @Produce json
// @Success 200 {object} string	"Clusters"
// @Router /clusters [get]
func (a *App) Clusters(w http.ResponseWriter, r *http.Request) {
	clusters := a.cluster.Clusters()

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(clusters)
}
