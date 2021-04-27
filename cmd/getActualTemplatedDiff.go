package main

import (
	"flag"
	m "n9k-modeling/modeling"
	t "n9k-modeling/templating"
)

func main() {
	TmplFile := flag.String("tmpl", "00000", "templated data")
	OrigFile := flag.String("act", "00000", "actual data")
	OutputFile := flag.String("out", "0000", "output file")
	flag.Parse()

	var DeviceDiffDB t.DeviceDiffDB

	Tmpl := t.LoadProcessedData(*TmplFile)
	Orig := t.LoadProcessedData(*OrigFile)

	t.ConstrustDeficeDiffDB(Tmpl.DeviceFootprintDB, Orig.DeviceFootprintDB, &DeviceDiffDB)

	MarshalledTemplatedData := m.MarshalToJSON(DeviceDiffDB)
	m.WriteDataToFile(*OutputFile, MarshalledTemplatedData)
}
