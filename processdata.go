package main

import (
	"flag"
	"sync"

	m "n9k-modeling/modeling"

	cu "github.com/achelovekov/collectorutils"
)

func main() {

	srcVal := flag.String("key", "00000", "vnid to construct the model")
	ServiceDefinitionFile := flag.String("service", "00000", "service definition")
	OutputFile := flag.String("out", "00000", "output file for result storage and template processing")
	flag.Parse()

	Config, Filter, Enrich := cu.Initialize("config.json")
	Inventory := cu.LoadInventory("inventory.json")
	ServiceDefinition := m.LoadServiceDefinition(*ServiceDefinitionFile)
	KeysMap := m.LoadKeysMap(ServiceDefinition.DMEProcessing)
	ConversionMap := cu.CreateConversionMap()
	MetaData := &m.MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, ConversionMap: ConversionMap}

	ch := make(chan m.RawDataDBEntry, len(Inventory))
	var wg sync.WaitGroup

	for _, v := range Inventory {
		wg.Add(1)
		go m.GetRawData(MetaData, v, "sys", ch, &wg)
	}

	wg.Wait()
	close(ch)

	var RawDataDB m.RawDataDB

	for elem := range ch {
		RawDataDB = append(RawDataDB, elem)
	}

	ServiceDataDB := make(m.ServiceDataDB, 0)
	ServiceLayoutDB := make(m.ServiceLayoutDB, 0)

	m.ConstructServiceDataDB(&ServiceDataDB, RawDataDB, *srcVal, ServiceDefinition.ServiceConstructPath, MetaData.ConversionMap)
	m.ConstructServiceLayout(ServiceDefinition.ServiceComponents, ServiceDataDB, &ServiceLayoutDB)

	var ProcessedData m.ProcessedData
	ProcessedData.ServiceDataDB = ServiceDataDB
	ProcessedData.ServiceLayoutDB = ServiceLayoutDB
	ProcessedData.ServiceName = ServiceDefinition.ServiceName

	MarshalledProcessedData := m.MarshalToJSON(ProcessedData)

	m.WriteDataToFile(*OutputFile, MarshalledProcessedData)
}
