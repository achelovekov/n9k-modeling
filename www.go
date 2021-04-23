package main

import (
	"context"
	"log"
	m "n9k-modeling/modeling"
	mo "n9k-modeling/mongo"
	"net/http"
	"text/template"
	"time"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func a(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "index.gohtml", nil)
}

func b(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}
	key := r.FormValue("key")
	service := r.FormValue("service")
	inventory := r.FormValue("inventory")

	result := ProcessData(key, service, inventory)

	tpl.ExecuteTemplate(w, "action.gohtml", result)
}

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

func ProcessData(key string, service string, inventory string) m.ProcessedData {
	Config, Filter, Enrich := cu.Initialize("config.json")
	Inventory := cu.LoadInventory(inventory)
	ServiceDefinition := m.LoadServiceDefinition(service)
	KeysMap := m.LoadKeysMap(ServiceDefinition.DMEProcessing)
	ConversionMap := cu.CreateConversionMap()
	MetaData := &m.MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, ConversionMap: ConversionMap}

	MongoDBMetaData := mo.MongoDBMetaData{
		DBName:         "RawDME",
		CollectionName: "DevicesRawDME",
		URL:            "mongodb://localhost:27017",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(MongoDBMetaData.URL))

	if err != nil {
		log.Println(err)
	}

	collection := client.Database(MongoDBMetaData.DBName).Collection(MongoDBMetaData.CollectionName)

	var DeviceChunksDB m.DeviceChunksDB

	for _, v := range Inventory {
		src := mo.FindOne(ctx, collection, "DeviceName", v.Host.Hostname)["DeviceDMEData"]
		DeviceChunksDB = append(DeviceChunksDB, m.Processing(MetaData, v, src))
	}
	DeviceFootprintDB := make(m.DeviceFootprintDB, 0)
	ServiceFootprintDB := make(m.ServiceFootprintDB, 0)

	m.ConstructDeviceFootprintDB(&DeviceFootprintDB, DeviceChunksDB, key, ServiceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	m.ConstructServiceFootprintDB(ServiceDefinition.ServiceComponents, DeviceFootprintDB, &ServiceFootprintDB)

	var ProcessedData m.ProcessedData
	ProcessedData.DeviceFootprintDB = DeviceFootprintDB
	ProcessedData.ServiceFootprintDB = ServiceFootprintDB
	ProcessedData.ServiceName = ServiceDefinition.ServiceName

	return ProcessedData
}

func main() {
	http.HandleFunc("/", a)
	http.HandleFunc("/action", b)

	http.ListenAndServe(":8080", nil)
}
