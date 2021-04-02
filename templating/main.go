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
}

func LoadRawVariables(fileName string) VariablesDB {
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

func ProcessRawVariables(VariablesDB VariablesDB) map[string]interface{} {
	m := make(map[string]interface{})
	m["ServiceName"] = VariablesDB.ServiceName
	for _, ServiceVariable := range VariablesDB.ServiceVariables {
		m[ServiceVariable.VariableName] = ServiceVariable.VariableValue
	}
	return m
}

type L2VNITemplate struct {
	L2BDaccEncap string `json:"l2BD.accEncap"`
	L2BDid       int64  `json:"l2BD.id"`
	L2BDname     string `json:"l2BD.name"`
	VNID         int64  `json:"vnid"`
}

type TemplateProcessingDefinition []struct {
	Component    string `json:"Component"`
	TemplateFunc fn     `json:"TemplateFunc"`
}

type fn func(map[string]interface{}, map[string]interface{}, AddOptionsDB, string)
type TemplateMappingDB map[string]TemplateMappingDBEntry
type TemplateMappingDBEntry map[string]fn

func ConstructTemplateMapping() TemplateMappingDB {
	TemplateMappingDB := make(TemplateMappingDB)

	TemplateProcessingMap := make(TemplateMappingDBEntry)
	TemplateProcessingMap["L2VNI"] = MakeL2VNITemplate
	TemplateProcessingMap["AGW"] = MakeAGWTemplate
	TemplateProcessingMap["PIM"] = MakePIMTemplate
	TemplateProcessingMap["IR"] = MakeIRTemplate
	TemplateProcessingMap["MS-IR"] = MakeMSIRTemplate

	TemplateMappingDB["VNI"] = TemplateProcessingMap

	return TemplateMappingDB
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

func LoadAddInfo(ProcessedData m.ProcessedData, OptionList []string) AddOptionsDB {
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
