package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"text/template"
	"time"

	cu "github.com/achelovekov/collectorutils"
	m "github.com/achelovekov/n9k-modeling/modeling"
	mo "github.com/achelovekov/n9k-modeling/mongo"
	t "github.com/achelovekov/n9k-modeling/templating"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (md *MetaData) Index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	cursor, err := serviceListCollection.Find(ctx, bson.D{})

	var serviceList []bson.M

	if err = cursor.All(context.TODO(), &serviceList); err != nil {
		log.Println(err)
	}

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	cursor, err = inventoryCollection.Find(ctx, bson.D{})

	var inventory []bson.M

	if err = cursor.All(context.TODO(), &inventory); err != nil {
		log.Println(err)
	}

	type Result struct {
		ServiceList []bson.M
		Inventory   []bson.M
	}

	var result = Result{ServiceList: serviceList, Inventory: inventory}

	tpl.ExecuteTemplate(w, "index.gohtml", result)
}

func (md *MetaData) GetRawData(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	fmt.Println("go here")

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	var inventory cu.Inventory
	cursor, err := inventoryCollection.Find(ctx, bson.M{})

	for cursor.Next(ctx) {
		hostMetaData := cu.HostMetaData{}

		err := cursor.Decode(&hostMetaData)

		if err != nil {
			log.Println(err)
		}

		inventory = append(inventory, hostMetaData)
	}

	fmt.Println("Inventory:", inventory)

	rawDataCollection := client.Database(md.Config.DBName).Collection(md.Config.CollectionName)

	var wg sync.WaitGroup

	fmt.Println(inventory)

	for _, v := range inventory {
		wg.Add(1)
		go m.GetRawData(ctx, rawDataCollection, v, "sys", &wg)
	}

	wg.Wait()
	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

}

func (md *MetaData) LoadInventory(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "LoadInventory.gohtml", nil)
}

func (md *MetaData) LoadServiceNames(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "LoadServiceNames.gohtml", nil)
}

func (md *MetaData) GetActualFootprint(w http.ResponseWriter, r *http.Request) {

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")

	cursor, err := serviceListCollection.Find(ctx, bson.D{})

	var results []bson.M

	if err = cursor.All(context.TODO(), &results); err != nil {
		log.Println(err)
	}

	tpl.ExecuteTemplate(w, "GetActualFootprint.gohtml", results)

}

func (md *MetaData) DoGetActualFootprint(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	var srcValList m.SrcValList
	json.Unmarshal([]byte(r.FormValue("keys")), &srcValList)

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	var inventory cu.Inventory
	cursor, err := inventoryCollection.Find(ctx, bson.M{})

	for cursor.Next(ctx) {
		hostMetaData := cu.HostMetaData{}

		err := cursor.Decode(&hostMetaData)

		if err != nil {
			log.Println(err)
		}

		inventory = append(inventory, hostMetaData)
	}

	serviceDefinitionFile := "../serviceDefinitions/" + r.FormValue("serviceName") + "/" + r.FormValue("serviceName") + ".service"

	serviceDefinition := m.LoadServiceDefinition(serviceDefinitionFile)

	chunksProcessingPaths := m.LoadChunksProcessingPaths(serviceDefinition.DMEProcessing)

	conversionMap := cu.CreateConversionMap()
	MetaData := &m.MetaData{Config: md.Config, Filter: md.Filter, Enrich: md.Enrich, ChunksProcessingPaths: chunksProcessingPaths, ConversionMap: conversionMap}

	devicesRawDMECollection := client.Database(md.Config.DBName).Collection(md.Config.CollectionName)

	var deviceChunksDB m.DeviceChunksDB

	for _, v := range inventory {
		src := mo.FindOne(ctx, devicesRawDMECollection, "DeviceName", v.HostName)["DeviceDMEData"]
		deviceChunksDB = append(deviceChunksDB, m.Processing(MetaData, v, src))
	}

	JSONData := m.MarshalToJSON(deviceChunksDB)
	m.WriteDataToFile("BGPJSONData.out", JSONData)

	deviceFootprintDB := m.ConstructDeviceFootprintDB(deviceChunksDB, srcValList, serviceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	serviceFootprintDB := m.ConstructServiceFootprintDB(serviceDefinition.ServiceComponents, deviceFootprintDB)

	var processedData m.ProcessedData
	processedData.ServiceName = serviceDefinition.ServiceName
	processedData.Keys = srcValList
	processedData.ServiceComponents = m.GetServiceComponentsList(serviceDefinition)
	processedData.DeviceFootprintDB = deviceFootprintDB
	processedData.ServiceFootprintDB = serviceFootprintDB

	processedDataCollection := client.Database(serviceDefinition.ServiceName).Collection("ProcessedData")

	processedDataCollection.Drop(ctx)
	mo.InsertOne(ctx, processedDataCollection, processedData)

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

}

func (md *MetaData) GetActualDeviceFootprint(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(r.FormValue("serviceName")).Collection("ProcessedData")
	bsonProcessedData := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))

	var processedData m.ProcessedData

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	for _, deviceFootprintDBEntry := range processedData.DeviceFootprintDB {
		if deviceFootprintDBEntry.DeviceName == r.FormValue("hostName") {
			tpl.ExecuteTemplate(w, "PrintActualDeviceFootprint.gohtml", deviceFootprintDBEntry)
		}
	}

}

func (md *MetaData) GetActualServiceFootprint(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(r.FormValue("serviceName")).Collection("ProcessedData")
	bsonProcessedData := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))

	var processedData m.ProcessedData

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	type Result struct {
		ServiceFootprintDBEntry m.ServiceFootprintDBEntry
		ServiceComponents       []string
	}

	for _, serviceFootprintDBEntry := range processedData.ServiceFootprintDB {
		if serviceFootprintDBEntry.DeviceName == r.FormValue("hostName") {
			var result = Result{ServiceFootprintDBEntry: serviceFootprintDBEntry, ServiceComponents: processedData.ServiceComponents}
			tpl.ExecuteTemplate(w, "PrintActualServiceFootprint.gohtml", result)
		}
	}

}

func (md *MetaData) PushInventoryToMongo(w http.ResponseWriter, r *http.Request) {
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
		mo.UpdateOne(ctx, inventoryCollection, "HostName", inventoryEntry.HostName, "HostData", inventoryEntry.HostData)
	}

	fmt.Fprintln(w, "Successfully loaded")
}

func (md *MetaData) DropInventory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	inventoryCollection.Drop(ctx)
	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)
}

func (md *MetaData) PushServiceNamesToMongo(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("serviceNames"))

	var serviceList m.ServiceList

	json.Unmarshal(jsonData, &serviceList)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")

	for _, serviceListEntry := range serviceList {
		mongoServiceListEntry := mo.FindOne(ctx, serviceListCollection, "ServiceName", serviceListEntry.ServiceName)
		if len(mongoServiceListEntry) == 0 {
			mo.InsertOne(ctx, serviceListCollection, serviceListEntry)
		}
	}

}

func (md *MetaData) GetCompianceData(w http.ResponseWriter, r *http.Request) {

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	cursor, err := serviceListCollection.Find(ctx, bson.D{})

	var serviceList []bson.M

	if err = cursor.All(context.TODO(), &serviceList); err != nil {
		log.Println(err)
	}

	tpl.ExecuteTemplate(w, "LoadVarsForGetCompianceData.gohtml", serviceList)
}

func (md *MetaData) GetTemplatedFootprint(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("vars"))

	var serviceVariablesDB t.ServiceVariablesDB

	json.Unmarshal(jsonData, &serviceVariablesDB)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(r.FormValue("serviceName")).Collection("ProcessedData")
	diffDataCollection := client.Database(r.FormValue("serviceName")).Collection("DiffData")

	mongoProcessedData := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))

	var processedData m.ProcessedData

	bsonProcessedDataBytes, _ := bson.Marshal(mongoProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	serviceVariablesDBProcessed := t.LoadServiceVariablesDBProcessed(serviceVariablesDB)
	indirectVariablesDB := t.LoadIndirectVariablesDB(processedData, serviceVariablesDB.IndirectVariables)

	generalTemplateConstructor := t.LoadGeneralTemplateConstructor()

	templatedDeviceFootprintDB := t.TemplateConstruct(r.FormValue("serviceName"), processedData.ServiceFootprintDB, serviceVariablesDBProcessed, indirectVariablesDB, generalTemplateConstructor)

	deviceDiffDB := t.ConstrustDeficeDiffDB(templatedDeviceFootprintDB, processedData.DeviceFootprintDB)

	var devices []string

	diffDataCollection.Drop(ctx)
	for _, deviceDiffDBEntry := range deviceDiffDB {
		_, err := mo.InsertOne(ctx, diffDataCollection, deviceDiffDBEntry)
		if err != nil {
			log.Println(err)
		}

		devices = append(devices, deviceDiffDBEntry.DeviceName)

	}

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

}

func (md *MetaData) GetHostnameForCompianceReport(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	diffDataCollection := client.Database(r.FormValue("serviceName")).Collection("DiffData")
	cursor, err := diffDataCollection.Find(ctx, bson.D{})

	var deviceDiffDB t.DeviceDiffDB

	for cursor.Next(ctx) {
		deviceDiffDBEntry := t.DeviceDiffDBEntry{}

		err := cursor.Decode(&deviceDiffDBEntry)

		if err != nil {
			log.Println(err)
		}

		deviceDiffDB = append(deviceDiffDB, deviceDiffDBEntry)
	}

	result := t.CheckForChanges(deviceDiffDB)

	http.SetCookie(w, &http.Cookie{
		Name:  "serviceName",
		Value: r.FormValue("serviceName"),
	})

	tpl.ExecuteTemplate(w, "GetHostnameForCompianceReport.gohtml", result)
}

func (md *MetaData) GetComplianceReport(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))

	if err != nil {
		log.Println(err)
	}

	serviceName, err := r.Cookie("serviceName")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	diffDataCollection := client.Database(serviceName.Value).Collection("DiffData")

	diffDataEntry := mo.FindOne(ctx, diffDataCollection, "DeviceName", r.FormValue("deviceName"))

	tpl.ExecuteTemplate(w, "GetCompianceReport.gohtml", diffDataEntry)

}

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

type MetaData struct {
	Config                cu.Config
	Enrich                cu.Enrich
	Filter                cu.Filter
	ChunksProcessingPaths cu.ChunksProcessingPaths
	ConversionMap         cu.ConversionMap
}

func main() {

	config, filter, enrich := cu.Initialize("config.json")

	var metaData MetaData

	metaData.Config = config
	metaData.Enrich = enrich
	metaData.Filter = filter

	http.Handle("/", http.FileServer(http.Dir("css/")))

	http.HandleFunc("/index", metaData.Index)
	http.HandleFunc("/getRawData", metaData.GetRawData)

	http.HandleFunc("/loadInventory", metaData.LoadInventory)
	http.HandleFunc("/dropInventory", metaData.DropInventory)
	http.HandleFunc("/pushInventoryToMongo", metaData.PushInventoryToMongo)
	http.HandleFunc("/loadServiceNames", metaData.LoadServiceNames)
	http.HandleFunc("/pushServiceNamesToMongo", metaData.PushServiceNamesToMongo)

	http.HandleFunc("/getActualFootprint", metaData.GetActualFootprint)
	http.HandleFunc("/doGetActualFootprint", metaData.DoGetActualFootprint)

	http.HandleFunc("/getActualDeviceFootprint", metaData.GetActualDeviceFootprint)
	http.HandleFunc("/getActualServiceFootprint", metaData.GetActualServiceFootprint)

	http.HandleFunc("/getCompianceData", metaData.GetCompianceData)
	http.HandleFunc("/getTemplatedFootprint", metaData.GetTemplatedFootprint)

	http.HandleFunc("/getHostnameForCompianceReport", metaData.GetHostnameForCompianceReport)
	http.HandleFunc("/getComplianceReport", metaData.GetComplianceReport)
	http.ListenAndServe(":8080", nil)
}
