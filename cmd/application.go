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
	var (
		serviceList []bson.M
		inventory   []bson.M
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	cursor, err := serviceListCollection.Find(ctx, bson.D{})
	if err = cursor.All(context.TODO(), &serviceList); err != nil {
		log.Println(err)
	}

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	cursor, err = inventoryCollection.Find(ctx, bson.D{})
	if err = cursor.All(context.TODO(), &inventory); err != nil {
		log.Println(err)
	}

	result := struct {
		ServiceList []bson.M
		Inventory   []bson.M
	}{
		ServiceList: serviceList,
		Inventory:   inventory,
	}

	tpl.ExecuteTemplate(w, "index.gohtml", result)
}

func (md *MetaData) GetRawData(w http.ResponseWriter, r *http.Request) {
	var (
		inventory cu.Inventory
		wg        sync.WaitGroup
	)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	cursor, err := inventoryCollection.Find(ctx, bson.M{})
	for cursor.Next(ctx) {
		hostMetaData := cu.HostMetaData{}
		err := cursor.Decode(&hostMetaData)
		if err != nil {
			log.Println(err)
		}

		inventory = append(inventory, hostMetaData)
	}

	rawDataCollection := client.Database(md.Config.DBName).Collection(md.Config.CollectionName)

	for _, inventoryEntry := range inventory {
		wg.Add(1)
		go m.GetRawData(ctx, rawDataCollection, inventoryEntry, "sys", &wg)
	}

	wg.Wait()
	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

}

func (md *MetaData) LoadInventory(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "LoadInventory.gohtml", nil)
}

func (md *MetaData) LoadSOT(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "LoadSOT.gohtml", nil)
}

func (md *MetaData) LoadServiceNames(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "LoadServiceNames.gohtml", nil)
}

func (md *MetaData) GetActualFootprint(w http.ResponseWriter, r *http.Request) {
	var results []bson.M
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	cursor, err := serviceListCollection.Find(ctx, bson.D{})
	if err = cursor.All(context.TODO(), &results); err != nil {
		log.Println(err)
	}

	tpl.ExecuteTemplate(w, "GetActualFootprint.gohtml", results)
}

func (md *MetaData) DoGetActualFootprint(w http.ResponseWriter, r *http.Request) {
	var (
		keysList       m.KeysList
		inventory      cu.Inventory
		deviceChunksDB m.DeviceChunksDB
		processedData  m.ProcessedData
	)

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

	json.Unmarshal([]byte(r.FormValue("keys")), &keysList)
	inventoryCollection := client.Database("Auxilary").Collection("Inventory")

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
	processedDataCollection := client.Database(serviceDefinition.ServiceName).Collection("ProcessedData")

	for _, inventoryEntry := range inventory {
		devicesRawDMECollectionEntry, err := mo.FindOne(ctx, devicesRawDMECollection, "DeviceName", inventoryEntry.HostName)
		if err != nil {
			log.Println("Can't find RawDME model data for: ", inventoryEntry.HostName)
		}
		deviceChunksDB = append(deviceChunksDB, m.Processing(MetaData, inventoryEntry, devicesRawDMECollectionEntry["DeviceDMEData"]))
	}

	deviceFootprintDB := m.ConstructDeviceFootprintDB(deviceChunksDB, keysList, serviceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	serviceFootprintDB := m.ConstructServiceFootprintDB(serviceDefinition.ServiceComponents, deviceFootprintDB)
	serviceTypeDB := m.ConstructServiceTypeDB(serviceDefinition.ServiceName, serviceFootprintDB, inventory, serviceDefinition.LocalServiceDefinitions)

	processedData.ServiceName = serviceDefinition.ServiceName
	processedData.Keys = keysList
	processedData.ServiceComponents = m.GetServiceComponentsList(serviceDefinition)
	processedData.DeviceFootprintDB = deviceFootprintDB
	processedData.ServiceFootprintDB = serviceFootprintDB
	processedData.ServiceTypeDB = serviceTypeDB

	processedDataCollection.Drop(ctx)
	mo.InsertOne(ctx, processedDataCollection, processedData)

	fmt.Println(serviceTypeDB)

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

}

func (md *MetaData) GetActualDeviceFootprint(w http.ResponseWriter, r *http.Request) {
	var processedData m.ProcessedData

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
	bsonProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	for _, deviceFootprintDBEntry := range processedData.DeviceFootprintDB {
		if deviceFootprintDBEntry.DeviceName == r.FormValue("hostName") {
			tpl.ExecuteTemplate(w, "PrintActualDeviceFootprint.gohtml", deviceFootprintDBEntry)
		}
	}
}

func (md *MetaData) GetActualServiceFootprint(w http.ResponseWriter, r *http.Request) {
	var processedData m.ProcessedData
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
	bsonProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	type Result struct {
		ServiceFootprintDBEntry m.ServiceFootprintDBEntry
		ServiceComponents       []string
	}

	for _, serviceFootprintDBEntry := range processedData.ServiceFootprintDB {
		if serviceFootprintDBEntry.DeviceName == r.FormValue("hostName") {
			result := Result{ServiceFootprintDBEntry: serviceFootprintDBEntry, ServiceComponents: processedData.ServiceComponents}
			tpl.ExecuteTemplate(w, "PrintActualServiceFootprint.gohtml", result)
		}
	}
}

func (md *MetaData) GetActualServiceFootprintForAll(w http.ResponseWriter, r *http.Request) {
	var processedData m.ProcessedData
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
	bsonProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	type ResultEntry struct {
		ServiceFootprintDBEntry m.ServiceFootprintDBEntry
		ServiceComponents       []string
	}

	result := make([]ResultEntry, 0)

	for _, serviceFootprintDBEntry := range processedData.ServiceFootprintDB {
		serviceTypeDBEntry := m.GetServiceTypeDBEntryByDeviceName(processedData.ServiceTypeDB, serviceFootprintDBEntry.DeviceName)
		if !m.CheckNotExistServices(serviceTypeDBEntry) {
			resultEntry := ResultEntry{ServiceFootprintDBEntry: serviceFootprintDBEntry, ServiceComponents: processedData.ServiceComponents}
			result = append(result, resultEntry)
		}
	}

	tpl.ExecuteTemplate(w, "PrintActualServiceFootprintForAll.gohtml", result)

}

func (md *MetaData) GetActualServiceTypeForAll(w http.ResponseWriter, r *http.Request) {
	var processedData m.ProcessedData
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
	bsonProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	serviceTypeDB := processedData.ServiceTypeDB

	fmt.Println(serviceTypeDB)

	tpl.ExecuteTemplate(w, "PrintActualServiceTypeForAll.gohtml", serviceTypeDB)
}

func (md *MetaData) PushInventoryToMongo(w http.ResponseWriter, r *http.Request) {
	var inventory cu.Inventory
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("inventory"))
	err = json.Unmarshal(jsonData, &inventory)
	if err != nil {
		log.Println(err)
	}

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

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)
}

func (md *MetaData) PushSOTToMongo(w http.ResponseWriter, r *http.Request) {
	var SOTDB m.SOTDB
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("SOT-DB"))
	err = json.Unmarshal(jsonData, &SOTDB)
	if err != nil {
		log.Println(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	SOTDBCollection := client.Database("SOT").Collection("SOTDB")

	mo.UpdateOne(ctx, SOTDBCollection, "Service", SOTDB.Name, "DB", SOTDB.DB)

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

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
	var serviceList m.ServiceList
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("serviceNames"))
	json.Unmarshal(jsonData, &serviceList)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	for _, serviceListEntry := range serviceList {
		mongoServiceListEntry, _ := mo.FindOne(ctx, serviceListCollection, "ServiceName", serviceListEntry.ServiceName)
		if len(mongoServiceListEntry) == 0 {
			mo.InsertOne(ctx, serviceListCollection, serviceListEntry)
		}
	}

}

func (md *MetaData) GetCompianceData(w http.ResponseWriter, r *http.Request) {
	var serviceList []bson.M
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	serviceListCollection := client.Database("Auxilary").Collection("ServiceList")
	cursor, err := serviceListCollection.Find(ctx, bson.D{})
	if err = cursor.All(context.TODO(), &serviceList); err != nil {
		log.Println(err)
	}

	tpl.ExecuteTemplate(w, "LoadVarsForGetCompianceData.gohtml", serviceList)
}

/* func (md *MetaData) GetTemplatedFootprint(w http.ResponseWriter, r *http.Request) {
	var (
		serviceVariablesDB t.ServiceVariablesDB
		processedData      m.ProcessedData
	)

	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("vars"))
	json.Unmarshal(jsonData, &serviceVariablesDB)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(md.Config.URL))
	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(r.FormValue("serviceName")).Collection("ProcessedData")
	diffDataCollection := client.Database(r.FormValue("serviceName")).Collection("DiffData")

	mongoProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(mongoProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	serviceVariablesDBProcessed := t.LoadServiceVariablesDBProcessed(serviceVariablesDB)
	indirectVariablesDB := t.LoadIndirectVariablesDB(processedData, serviceVariablesDB.IndirectVariables)
	generalTemplateConstructor := t.LoadGeneralTemplateConstructor()
	templatedDeviceFootprintDB := t.TemplateConstruct(r.FormValue("serviceName"), processedData.ServiceFootprintDB, serviceVariablesDBProcessed, indirectVariablesDB, generalTemplateConstructor)
	deviceDiffDB := t.ConstrustDeficeDiffDB(templatedDeviceFootprintDB, processedData.DeviceFootprintDB)

	diffDataCollection.Drop(ctx)
	for _, deviceDiffDBEntry := range deviceDiffDB {
		_, err := mo.InsertOne(ctx, diffDataCollection, deviceDiffDBEntry)
		if err != nil {
			log.Println(err)
		}
	}

	http.Redirect(w, r, "http://127.0.0.1:8080/index", 301)

} */

func (md *MetaData) GetHostnameForCompianceReport(w http.ResponseWriter, r *http.Request) {
	var deviceDiffDB t.DeviceDiffDB
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
	diffDataEntry, err := mo.FindOne(ctx, diffDataCollection, "DeviceName", r.FormValue("deviceName"))
	if err != nil {
		log.Println("Can't find diffDataEntry for: ", r.FormValue("deviceName"))
	}

	tpl.ExecuteTemplate(w, "GetCompianceReport.gohtml", diffDataEntry)

}

func (md *MetaData) GetGlobalServiceTypeReport(w http.ResponseWriter, r *http.Request) {
	var processedData m.ProcessedData
	var sOTDB m.SOTDB

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
	inventoryCollection := client.Database("Auxilary").Collection("Inventory")
	SOTDBCollection := client.Database("SOT").Collection("SOTDB")

	bsonProcessedData, err := mo.FindOne(ctx, processedDataCollection, "ServiceName", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ := bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &processedData)

	cursor, err := inventoryCollection.Find(ctx, bson.D{})

	var inventory cu.Inventory

	for cursor.Next(ctx) {
		hostMetaData := cu.HostMetaData{}
		err := cursor.Decode(&hostMetaData)
		if err != nil {
			log.Println(err)
		}

		inventory = append(inventory, hostMetaData)
	}

	bsonProcessedData, err = mo.FindOne(ctx, SOTDBCollection, "Service", r.FormValue("serviceName"))
	if err != nil {
		log.Println("Can't find processedData for:", r.FormValue("serviceName"))
	}

	bsonProcessedDataBytes, _ = bson.Marshal(bsonProcessedData)
	bson.Unmarshal(bsonProcessedDataBytes, &sOTDB)

	m.CheckServiceTypeDB(processedData.ServiceTypeDB, sOTDB)

	serviceDefinitionFile := "../serviceDefinitions/" + r.FormValue("serviceName") + "/" + r.FormValue("serviceName") + ".service"
	serviceDefinition := m.LoadServiceDefinition(serviceDefinitionFile)

	serviceVariablesDBProcessed := t.LoadServiceVariablesDBProcessed(sOTDB)

	generalTemplateConstructor := t.LoadGeneralTemplateConstructor()
	sOTTemplatingReference := t.GetSOTTemplatingReference(sOTDB, serviceDefinition.LocalServiceDefinitions, serviceVariablesDBProcessed, generalTemplateConstructor, r.FormValue("serviceName"))

	deviceDiffDB := t.ComplienceReport(processedData, sOTTemplatingReference)

	tpl.ExecuteTemplate(w, "GetCompianceReport.gohtml", deviceDiffDB)

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
	http.HandleFunc("/loadSOT", metaData.LoadSOT)
	http.HandleFunc("/dropInventory", metaData.DropInventory)
	http.HandleFunc("/pushInventoryToMongo", metaData.PushInventoryToMongo)
	http.HandleFunc("/pushSOTToMongo", metaData.PushSOTToMongo)
	http.HandleFunc("/loadServiceNames", metaData.LoadServiceNames)
	http.HandleFunc("/pushServiceNamesToMongo", metaData.PushServiceNamesToMongo)

	http.HandleFunc("/getActualFootprint", metaData.GetActualFootprint)
	http.HandleFunc("/doGetActualFootprint", metaData.DoGetActualFootprint)

	http.HandleFunc("/getActualDeviceFootprint", metaData.GetActualDeviceFootprint)
	http.HandleFunc("/getActualServiceFootprint", metaData.GetActualServiceFootprint)
	http.HandleFunc("/getActualServiceFootprintForAll", metaData.GetActualServiceFootprintForAll)
	http.HandleFunc("/getActualServiceTypeForAll", metaData.GetActualServiceTypeForAll)

	http.HandleFunc("/getCompianceData", metaData.GetCompianceData)
	/* 	http.HandleFunc("/getTemplatedFootprint", metaData.GetTemplatedFootprint) */

	http.HandleFunc("/getHostnameForCompianceReport", metaData.GetHostnameForCompianceReport)
	http.HandleFunc("/getComplianceReport", metaData.GetComplianceReport)

	http.HandleFunc("/getGlobalServiceTypeReport", metaData.GetGlobalServiceTypeReport)

	http.ListenAndServe(":8080", nil)
}
