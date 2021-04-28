package main

import (
	"context"
	"log"
	"net/http"
	"text/template"
	"time"

	cu "github.com/achelovekov/collectorutils"
	mo "github.com/achelovekov/n9k-modeling/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MetaData struct {
	ProcessedData bson.M
}

func (md *MetaData) index(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "index.gohtml", md.ProcessedData)
}

func (md *MetaData) deviceFootprint(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	for _, DeviceFootprintDBEntry := range md.ProcessedData["DeviceFootprintDB"].(bson.A) {
		if DeviceFootprintDBEntry.(bson.M)["DeviceName"].(string) == r.FormValue("device") {
			tpl.ExecuteTemplate(w, "deviceFootprint.gohtml", DeviceFootprintDBEntry.(bson.M))
		}
	}
}

func (md *MetaData) serviceFootprint(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	for _, serviceFootprintDBEntry := range md.ProcessedData["ServiceFootprintDB"].(bson.A) {
		if serviceFootprintDBEntry.(bson.M)["DeviceName"].(string) == r.FormValue("device") {
			serviceFootprintDBEntry.(bson.M)["ServiceComponents"] = md.ProcessedData["ServiceComponents"]
			tpl.ExecuteTemplate(w, "serviceFootprint.gohtml", serviceFootprintDBEntry.(bson.M))
		}
	}
}

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

func main() {

	config, _, _ := cu.Initialize("config.json")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URL))

	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(config.ServiceName).Collection("processedData")

	result := mo.FindOne(ctx, processedDataCollection, "ServiceName", config.ServiceName)

	metaData := MetaData{ProcessedData: result}

	http.HandleFunc("/index", metaData.index)
	http.HandleFunc("/deviceFootprint", metaData.deviceFootprint)
	http.HandleFunc("/serviceFootprint", metaData.serviceFootprint)
	http.Handle("/", http.FileServer(http.Dir("css/")))
	http.ListenAndServe(":8080", nil)
}
