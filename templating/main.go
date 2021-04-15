package templating

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	m "n9k-modeling/modeling"
	"os"
	"strconv"
)

type VariablesDB struct {
	ServiceName      string `json:"ServiceName"`
	ServiceVariables []struct {
		VariableName  string      `json:"VariableName"`
		VariableValue interface{} `json:"VariableValue"`
	} `json:"ServiceVariables"`
	AddOptions []string `json:"AddOptions"`
}

func LoadTemplateData(fileName string) VariablesDB {
	var VariablesDB VariablesDB
	VariablesDBFile, err := os.Open(fileName)
	if err != nil {
		fmt.Println(err)
	}
	defer VariablesDBFile.Close()

	VariablesDBFileBytes, _ := ioutil.ReadAll(VariablesDBFile)

	err = json.Unmarshal(VariablesDBFileBytes, &VariablesDB)
	if err != nil {
		fmt.Println(err)
	}

	return VariablesDB
}

func LoadTemplateDataMap(VariablesDB VariablesDB) map[string]interface{} {
	m := make(map[string]interface{})
	m["ServiceName"] = VariablesDB.ServiceName
	for _, ServiceVariable := range VariablesDB.ServiceVariables {
		m[ServiceVariable.VariableName] = ServiceVariable.VariableValue
	}
	return m
}

type fn func(map[string]interface{}, map[string]interface{}, AddOptionsDB, string)
type TemplateComponentsDB map[string]TemplateComponentsDBEntry
type TemplateComponentsDBEntry map[string]fn

func LoadTemplateComponentsMap() TemplateComponentsDB {
	TemplateComponentsDB := make(TemplateComponentsDB)

	TemplateComponentsDBEntry := make(TemplateComponentsDBEntry)
	TemplateComponentsDBEntry["L2VNI"] = MakeL2VNITemplate
	TemplateComponentsDBEntry["AGW"] = MakeAGWTemplate
	TemplateComponentsDBEntry["PIM"] = MakePIMTemplate
	TemplateComponentsDBEntry["IR"] = MakeIRTemplate
	TemplateComponentsDBEntry["MS-IR"] = MakeMSIRTemplate

	TemplateComponentsDB["VNI"] = TemplateComponentsDBEntry

	return TemplateComponentsDB
}

func MakeL2VNITemplate(M map[string]interface{}, VariablesMap map[string]interface{}, AddOptionsDB AddOptionsDB, DeviceName string) {
	M["vnid"], _ = strconv.ParseInt(VariablesMap["VNID"].(string), 10, 64)
	M["l2BD.accEncap"] = "vxlan-" + VariablesMap["VNID"].(string)
	M["l2BD.id"], _ = strconv.ParseInt(VariablesMap["VNID"].(string)[3:], 10, 64)
	M["l2BD.name"] = VariablesMap["Segment"].(string) + VariablesMap["ZoneID"].(string) + "Z_" + VariablesMap["Subnet"].(string) + "/" + VariablesMap["Mask"].(string)
	M["rtctrlRttEntry.rtt.export"] = "route-target:as2-nn4:" + strconv.FormatInt(int64(AddOptionsDB[DeviceName]["bgpInst.asn"].(float64)), 10) + ":" + VariablesMap["VNID"].(string)
	M["rtctrlRttEntry.rtt.import"] = "route-target:as2-nn4:" + strconv.FormatInt(int64(AddOptionsDB[DeviceName]["bgpInst.asn"].(float64)), 10) + ":" + VariablesMap["VNID"].(string)
	M["bgpInst.asn"] = AddOptionsDB[DeviceName]["bgpInst.asn"]
}

func MakeAGWTemplate(M map[string]interface{}, VariablesMap map[string]interface{}, AddOptionsDB AddOptionsDB, DeviceName string) {
	M["hmmFwdIf.mode"] = "anycastGW"
	M["ipv4Addr.addr"] = VariablesMap["IPAddress"].(string) + "/" + VariablesMap["Mask"].(string)
	M["ipv4Addr.tag"], _ = strconv.ParseInt("39"+VariablesMap["ZoneID"].(string), 10, 64)
	M["ipv4Dom.name"] = VariablesMap["ZoneName"].(string)
	M["sviIf.id"] = "vlan" + VariablesMap["VNID"].(string)[3:]
}

func MakeIRTemplate(M map[string]interface{}, VariablesMap map[string]interface{}, AddOptionsDB AddOptionsDB, DeviceName string) {
	M["nvoNw.mcastGroup"] = "0.0.0.0"
	M["nvoNw.multisiteIngRepl"] = "disable"
	M["nvoNw.vni"], _ = strconv.ParseInt(VariablesMap["VNID"].(string), 10, 64)
	M["nvoIngRepl.proto"] = "bgp"
	M["nvoIngRepl.rn"] = "IngRepl"
}

func MakePIMTemplate(M map[string]interface{}, VariablesMap map[string]interface{}, AddOptionsDB AddOptionsDB, DeviceName string) {
	M["nvoNw.mcastGroup"] = "239.1.0." + VariablesMap["ZoneID"].(string)
	M["nvoNw.multisiteIngRepl"] = "disable"
	M["nvoNw.vni"], _ = strconv.ParseInt(VariablesMap["VNID"].(string), 10, 64)
}

func MakeMSIRTemplate(M map[string]interface{}, VariablesMap map[string]interface{}, AddOptionsDB AddOptionsDB, DeviceName string) {
	M["nvoNw.multisiteIngRepl"] = "enable"
}

type AddOptionsDB map[string]AddOptionsDBEntry
type AddOptionsDBEntry map[string]interface{}

func LoadAddOptions(ProcessedData m.ProcessedData, OptionList []string) AddOptionsDB {
	AddOptionsDB := make(AddOptionsDB)
	for _, Device := range ProcessedData.ServiceDataDB {
		AddOptionsDBEntry := make(AddOptionsDBEntry)
		for _, Option := range OptionList {
			if v, ok := Device.DeviceData[Option]; ok {
				AddOptionsDBEntry[Option] = v
			}
		}
		AddOptionsDB[Device.DeviceName] = AddOptionsDBEntry
	}
	fmt.Println(AddOptionsDB)
	return AddOptionsDB
}

func PrettyPrint(src interface{}) {
	JSONData, err := json.MarshalIndent(&src, "", "  ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("Pretty processed output: %s\n", string(JSONData))
}

func LoadProcessedData(fineName string) m.ProcessedData {
	var ProcessedData m.ProcessedData
	ProcessedDataFile, err := os.Open(fineName)
	if err != nil {
		fmt.Println(err)
	}
	defer ProcessedDataFile.Close()

	ProcessedDataFileBytes, _ := ioutil.ReadAll(ProcessedDataFile)

	err = json.Unmarshal(ProcessedDataFileBytes, &ProcessedData)
	if err != nil {
		fmt.Println(err)
	}

	return ProcessedData
}

func TemplateConstruct(ProcessedData m.ProcessedData, TemplatedData *m.ProcessedData, AddOptions AddOptionsDB, TemplateDataMap map[string]interface{}, TemplateComponentsMap TemplateComponentsDB) {
	for _, Device := range ProcessedData.ServiceLayoutDB {
		var ServiceDataDBEntry m.ServiceDataDBEntry
		ServiceDataDBEntry.DeviceName = Device.DeviceName
		ServiceDataDBEntry.DeviceData = make(map[string]interface{})
		for _, Component := range Device.ServiceLayout {
			if Component.Value == true {
				TemplateComponentsMap[TemplatedData.ServiceName][Component.Name](ServiceDataDBEntry.DeviceData, TemplateDataMap, AddOptions, ServiceDataDBEntry.DeviceName)
			}
		}
		TemplatedData.ServiceDataDB = append(TemplatedData.ServiceDataDB, ServiceDataDBEntry)
	}
}
