package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"text/template"
	"time"

	cu "github.com/achelovekov/collectorutils"
	mo "github.com/achelovekov/n9k-modeling/mongo"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func loadInventory(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "loadInventory.gohtml", nil)
}

func (md *MetaData) pushToMongo(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("inventory"))

	var inventory cu.Inventory

	json.Unmarshal(jsonData, &inventory)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")

	for _, inventoryEntry := range inventory {
		mo.UpdateOne(ctx, inventoryCollection, "HostName", inventoryEntry.Hostname, "HostData", inventoryEntry.HostData)
	}

}

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

type MetaData struct {
	Config cu.Config
}

func main() {

	config, _, _ := cu.Initialize("config.json")

	var metaData MetaData

	metaData.Config = config

	http.HandleFunc("/loadInventory", loadInventory)
	http.HandleFunc("/pushToMongo", metaData.pushToMongo)
	http.ListenAndServe(":8080", nil)
}
