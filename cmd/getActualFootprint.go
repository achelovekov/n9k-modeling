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
	outputFile := flag.String("out", "00000", "output file for result storage and template processing")
	inventoryFile := flag.String("i", "00000", "inventory file to proceess")
	flag.Parse()

	config, filter, enrich := cu.Initialize("config.json")
	srcValList := m.LoadSrcValList(*srcValListFile)
	inventory := cu.LoadInventory(*inventoryFile)
	serviceDefinition := m.LoadServiceDefinition(*serviceDefinitionFile)
	keysMap := m.LoadKeysMap(serviceDefinition.DMEProcessing)
	conversionMap := cu.CreateConversionMap()
	MetaData := &m.MetaData{Config: config, Filter: filter, Enrich: enrich, KeysMap: keysMap, ConversionMap: conversionMap}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URL))

	if err != nil {
		log.Println(err)
	}

	collection := client.Database(config.DBName).Collection(config.CollectionName)

	var deviceChunksDB m.DeviceChunksDB

	for _, v := range inventory {
		src := mo.FindOne(ctx, collection, "DeviceName", v.Host.Hostname)["DeviceDMEData"]
		deviceChunksDB = append(deviceChunksDB, m.Processing(MetaData, v, src))
	}

	/* 	ServiceFootprintDB := make(m.ServiceFootprintDB, 0) */

	deviceFootprintDB := m.ConstructDeviceFootprintDB(deviceChunksDB, srcValList, serviceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	serviceFootprintDB := m.ConstructServiceFootprintDB(serviceDefinition.ServiceComponents, deviceFootprintDB)

	marshalledProcessedData := m.MarshalToJSON(serviceFootprintDB)

	m.WriteDataToFile(*outputFile, marshalledProcessedData)
	/* 	m.ConstructDeviceFootprintDB(&DeviceFootprintDB, DeviceChunksDB, *srcVal, serviceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	   	m.ConstructServiceFootprintDB(serviceDefinition.ServiceComponents, DeviceFootprintDB, &ServiceFootprintDB)

	   	var ProcessedData m.ProcessedData
	   	ProcessedData.DeviceFootprintDB = DeviceFootprintDB
	   	ProcessedData.ServiceFootprintDB = ServiceFootprintDB
	   	ProcessedData.ServiceName = serviceDefinition.ServiceName

	   	MarshalledProcessedData := m.MarshalToJSON(ProcessedData)

	   	m.WriteDataToFile(*outputFile, MarshalledProcessedData) */
}
