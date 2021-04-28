package main

import (
	"context"
	"flag"
	"log"
	"time"

	m "github.com/achelovekov/n9k-modeling/modeling"

	cu "github.com/achelovekov/collectorutils"
	mo "github.com/achelovekov/n9k-modeling/mongo"
	t "github.com/achelovekov/n9k-modeling/templating"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	varsFile := flag.String("varsFile", "00000", "file contains all the required variables definition to construct the service template")
	/* 	InputFile := flag.String("in", "00000", "file contains modeled actual data from devices")*/
	OutputFile := flag.String("out", "00000", "file to write the result in")
	flag.Parse()
	config, _, _ := cu.Initialize("config.json")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URL))

	if err != nil {
		log.Println(err)
	}

	processedDataCollection := client.Database(config.ServiceName).Collection("processedData")
	processedData := mo.FindOne(ctx, processedDataCollection, "ServiceName", config.ServiceName)

	serviceVariablesDB := t.LoadServiceVariablesDB(*varsFile)
	serviceVariablesDBProcessed := t.LoadServiceVariablesDBProcessed(serviceVariablesDB)
	indirectVariablesDB := t.LoadIndirectVariablesDB(processedData, serviceVariablesDB.IndirectVariables)

	generalTemplateConstructor := t.LoadGeneralTemplateConstructor()

	templatedDeviceFootprintDB := t.TemplateConstruct(config.ServiceName, processedData["ServiceFootprintDB"], serviceVariablesDBProcessed, indirectVariablesDB, generalTemplateConstructor)
	originalDeviceFootprintDB := t.Transform(processedData["DeviceFootprintDB"])

	deviceDiffDB := t.ConstrustDeficeDiffDB(templatedDeviceFootprintDB, originalDeviceFootprintDB)

	MarshalledDeviceDiffDB := m.MarshalToJSON(deviceDiffDB)
	m.WriteDataToFile(*OutputFile, MarshalledDeviceDiffDB)

}
