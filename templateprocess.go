package main

import (
	"flag"
	m "n9k-modeling/modeling"
	t "n9k-modeling/templating"
)

func main() {
	varsFile := flag.String("varsFile", "00000", "file contains all the required variables definition to construct the service template")
	InputFile := flag.String("in", "00000", "file contains modeled actual data from devices")
	OutputFile := flag.String("out", "00000", "file to write the result in")
	flag.Parse()

	TemplateData := t.LoadTemplateData(*varsFile)
	TemplateDataMap := t.LoadTemplateDataMap(TemplateData)
	TemplateComponentsMap := t.LoadTemplateComponentsMap()

	var TemplatedData m.ProcessedData

	ProcessedData := t.LoadProcessedData(*InputFile)
	TemplatedData.ServiceName = ProcessedData.ServiceName
	TemplatedData.ServiceLayoutDB = ProcessedData.ServiceLayoutDB
	TemplatedData.ServiceDataDB = make([]m.ServiceDataDBEntry, 0)
	AddOptions := t.LoadAddOptions(ProcessedData, TemplateData.AddOptions)

	t.TemplateConstruct(ProcessedData, &TemplatedData, AddOptions, TemplateDataMap, TemplateComponentsMap)
	MarshalledTemplatedData := m.MarshalToJSON(TemplatedData)
	m.WriteDataToFile(*OutputFile, MarshalledTemplatedData)
}
