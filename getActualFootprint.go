package main

import (
	"context"
	"flag"
	"log"
	"time"

	m "n9k-modeling/modeling"
	mo "n9k-modeling/mongo"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	srcVal := flag.String("key", "00000", "vnid to construct the model")
	ServiceDefinitionFile := flag.String("service", "00000", "service definition")
	OutputFile := flag.String("out", "00000", "output file for result storage and template processing")
	InventoryFile := flag.String("i", "00000", "inventory file to proceess")
	flag.Parse()

	Config, Filter, Enrich := cu.Initialize("config.json")
	Inventory := cu.LoadInventory(*InventoryFile)
	ServiceDefinition := m.LoadServiceDefinition(*ServiceDefinitionFile)
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

	m.ConstructDeviceFootprintDB(&DeviceFootprintDB, DeviceChunksDB, *srcVal, ServiceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	m.ConstructServiceFootprintDB(ServiceDefinition.ServiceComponents, DeviceFootprintDB, &ServiceFootprintDB)

	var ProcessedData m.ProcessedData
	ProcessedData.DeviceFootprintDB = DeviceFootprintDB
	ProcessedData.ServiceFootprintDB = ServiceFootprintDB
	ProcessedData.ServiceName = ServiceDefinition.ServiceName

	MarshalledProcessedData := m.MarshalToJSON(ProcessedData)

	m.WriteDataToFile(*OutputFile, MarshalledProcessedData)
}
