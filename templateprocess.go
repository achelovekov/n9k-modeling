package main

import (
	m "n9k-modeling/modeling"
	t "n9k-modeling/templating"
)

func main() {
	RawVariables := t.LoadRawVariables("VNI.vars")
	VariablesMap := t.ProcessRawVariables(RawVariables)
	TemplateMappingDB := t.ConstructTemplateMapping()

	var TemplatedData m.ProcessedData

	ProcessedData := t.LoadProcessedData("ProcessedData.json")
	TemplatedData.ServiceName = ProcessedData.ServiceName
	TemplatedData.ServiceLayoutDB = ProcessedData.ServiceLayoutDB
	TemplatedData.ServiceDataDB = make([]m.ServiceDataDBEntry, 0)
	OptionList := []string{"bgpInst.asn"}
	AddOptionsDB := t.LoadAddInfo(ProcessedData, OptionList)

	for _, Device := range ProcessedData.ServiceLayoutDB {
		var ServiceDataDBEntry m.ServiceDataDBEntry
		ServiceDataDBEntry.DeviceName = Device.DeviceName
		ServiceDataDBEntry.DeviceData = make(map[string]interface{})
		for _, Service := range Device.ServiceLayout {
			if Service.Value == true {
				TemplateMappingDB[TemplatedData.ServiceName][Service.Name](ServiceDataDBEntry.DeviceData, VariablesMap, AddOptionsDB, ServiceDataDBEntry.DeviceName)
			}
		}
		TemplatedData.ServiceDataDB = append(TemplatedData.ServiceDataDB, ServiceDataDBEntry)
	}

	m.PrettyPrint(TemplatedData)
}
