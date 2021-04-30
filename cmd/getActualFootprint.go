package main

import (
	"context"
	"flag"
	"log"
	"time"

	m "github.com/achelovekov/n9k-modeling/modeling"
	mo "github.com/achelovekov/n9k-modeling/mongo"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	srcValListFile := flag.String("keys", "00000", "keys list to construct")
	serviceDefinitionFile := flag.String("service", "00000", "service definition")
	/* 	outputFile := flag.String("out", "00000", "output file for result storage and template processing")
	 */inventoryFile := flag.String("i", "00000", "inventory file to proceess")
	flag.Parse()

	config, filter, enrich := cu.Initialize("config.json")
	srcValList := m.LoadSrcValList(*srcValListFile)
	inventory := cu.LoadInventory(*inventoryFile)
	serviceDefinition := m.LoadServiceDefinition(*serviceDefinitionFile)
	chunksProcessingPaths := m.LoadChunksProcessingPaths(serviceDefinition.DMEProcessing)
	conversionMap := cu.CreateConversionMap()
	MetaData := &m.MetaData{Config: config, Filter: filter, Enrich: enrich, ChunksProcessingPaths: chunksProcessingPaths, ConversionMap: conversionMap}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URL))

	if err != nil {
		log.Println(err)
	}

	rawDataCollection := client.Database(config.DBName).Collection(config.CollectionName)
	processedDataCollection := client.Database(serviceDefinition.ServiceName).Collection("processedData")

	var deviceChunksDB m.DeviceChunksDB

	for _, v := range inventory {
		src := mo.FindOne(ctx, rawDataCollection, "DeviceName", v.Host.Hostname)["DeviceDMEData"]
		deviceChunksDB = append(deviceChunksDB, m.Processing(MetaData, v, src))
	}

	deviceFootprintDB := m.ConstructDeviceFootprintDB(deviceChunksDB, srcValList, serviceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	serviceFootprintDB := m.ConstructServiceFootprintDB(serviceDefinition.ServiceComponents, deviceFootprintDB)

	var processedData m.ProcessedData
	processedData.ServiceName = serviceDefinition.ServiceName
	processedData.Keys = srcValList
	processedData.ServiceComponents = m.GetServiceComponentsList(serviceDefinition)
	processedData.DeviceFootprintDB = deviceFootprintDB
	processedData.ServiceFootprintDB = serviceFootprintDB

	processedDataCollection.Drop(ctx)
	mo.InsertOne(ctx, processedDataCollection, processedData)
}
