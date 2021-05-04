package templating

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	m "github.com/achelovekov/n9k-modeling/modeling"
	"go.mongodb.org/mongo-driver/bson"
)

type ServiceVariablesDB struct {
	Name              string                  `json:"Name"`
	Variables         []ServiceVariablesEntry `json:"Variables"`
	IndirectVariables []string                `json:"IndirectVariables"`
}

type ServiceVariablesEntry struct {
	Key  string         `json:"Key"`
	Data []VariableData `json:"Data"`
}

type VariableData struct {
	Name  string      `json:"Name"`
	Value interface{} `json:"Value"`
}

func LoadServiceVariablesDB(fileName string) ServiceVariablesDB {
	var serviceVariablesDB ServiceVariablesDB
	serviceVariablesDBFile, err := os.Open(fileName)
	if err != nil {
		fmt.Println(err)
	}
	defer serviceVariablesDBFile.Close()

	VariablesDBFileBytes, _ := ioutil.ReadAll(serviceVariablesDBFile)

	err = json.Unmarshal(VariablesDBFileBytes, &serviceVariablesDB)
	if err != nil {
		fmt.Println(err)
	}

	return serviceVariablesDB
}

type ServiceVariablesDBProcessed map[string]map[string]interface{}

func LoadServiceVariablesDBProcessed(serviceVariablesDB ServiceVariablesDB) ServiceVariablesDBProcessed {
	serviceVariablesDBProcessed := make(ServiceVariablesDBProcessed)

	for _, serviceVariablesEntry := range serviceVariablesDB.Variables {

		data := make(map[string]interface{})

		for _, dataEntry := range serviceVariablesEntry.Data {
			data[dataEntry.Name] = dataEntry.Value
		}
		serviceVariablesDBProcessed[serviceVariablesEntry.Key] = data
	}

	return serviceVariablesDBProcessed
}

type IndirectVariablesDB map[string]IndirectVariablesDBData
type IndirectVariablesDBData map[string]IndirectVariablesDBDataForKey
type IndirectVariablesDBDataForKey map[string]interface{}

func LoadIndirectVariablesDB(processedData m.ProcessedData, variablesList []string) IndirectVariablesDB {
	indirectVariablesDB := make(IndirectVariablesDB)

	for _, deviceFootprintDBEntry := range processedData.DeviceFootprintDB {
		indirectVariablesDBData := make(IndirectVariablesDBData)
		indirectVariablesDB[deviceFootprintDBEntry.DeviceName] = indirectVariablesDBData

		for _, deviceDataEntry := range deviceFootprintDBEntry.DeviceData {
			indirectVariablesDBDataForKey := make(IndirectVariablesDBDataForKey)
			indirectVariablesDBData[deviceDataEntry.Key] = indirectVariablesDBDataForKey
			for _, variable := range variablesList {
				if v, ok := deviceDataEntry.Data[variable]; ok {
					indirectVariablesDBDataForKey[variable] = v
				}
			}
		}

	}
	return indirectVariablesDB
}

func TemplateConstruct(serviceName string, serviceFootprintDB m.ServiceFootprintDB, serviceVariablesDBProcessed ServiceVariablesDBProcessed, indirectVariablesDB IndirectVariablesDB, generalTemplateConstructor GeneralTemplateConstructor) m.DeviceFootprintDB {

	var deviceFootprintDB m.DeviceFootprintDB

	for _, serviceFootprintDBEntry := range serviceFootprintDB {

		var deviceFootprintDBEntry m.DeviceFootprintDBEntry
		deviceFootprintDBEntry.DeviceName = serviceFootprintDBEntry.DeviceName

		for _, serviceLayoutEntry := range serviceFootprintDBEntry.ServiceLayouts {

			var deviceDataEntry m.DeviceDataEntry
			deviceDataEntry.Key = serviceLayoutEntry.Key
			m := make(map[string]interface{})
			deviceDataEntry.Data = m
			generalTemplateConstructor[serviceName]["Default"](m, serviceLayoutEntry.Key, serviceVariablesDBProcessed, indirectVariablesDB, serviceFootprintDBEntry.DeviceName)
			for _, dataEntry := range serviceLayoutEntry.Data {
				if dataEntry.Value {
					generalTemplateConstructor[serviceName][dataEntry.Name](m, serviceLayoutEntry.Key, serviceVariablesDBProcessed, indirectVariablesDB, serviceFootprintDBEntry.DeviceName)
				}
			}

			deviceFootprintDBEntry.DeviceData = append(deviceFootprintDBEntry.DeviceData, deviceDataEntry)

		}

		deviceFootprintDB = append(deviceFootprintDB, deviceFootprintDBEntry)
	}

	return deviceFootprintDB
}

func Transform(mongoDeviceFootprintDB interface{}) m.DeviceFootprintDB {
	var deviceFootprintDB m.DeviceFootprintDB

	for _, mongoDeviceFootprintDBEntry := range mongoDeviceFootprintDB.(bson.A) {

		var deviceFootprintDBEntry m.DeviceFootprintDBEntry
		deviceFootprintDBEntry.DeviceName = mongoDeviceFootprintDBEntry.(bson.M)["DeviceName"].(string)

		for _, mongoDeviceDataEntry := range mongoDeviceFootprintDBEntry.(bson.M)["DeviceData"].(bson.A) {

			var deviceDataEntry m.DeviceDataEntry
			deviceDataEntry.Key = mongoDeviceDataEntry.(bson.M)["Key"].(string)
			m := make(map[string]interface{})
			deviceDataEntry.Data = m

			for k, v := range mongoDeviceDataEntry.(bson.M)["Data"].(bson.M) {
				m[k] = v
			}
			deviceFootprintDBEntry.DeviceData = append(deviceFootprintDBEntry.DeviceData, deviceDataEntry)
		}

		deviceFootprintDB = append(deviceFootprintDB, deviceFootprintDBEntry)

	}

	return deviceFootprintDB
}

type fn func(map[string]interface{}, string, ServiceVariablesDBProcessed, IndirectVariablesDB, string)
type GeneralTemplateConstructor map[string]ServiceConstructor
type ServiceConstructor map[string]fn

func LoadGeneralTemplateConstructor() GeneralTemplateConstructor {
	GeneralTemplateConstructor := make(GeneralTemplateConstructor)

	VNIServiceConstructor := make(ServiceConstructor)
	VNIServiceConstructor["L2VNI"] = VNIMakeL2VNITemplate
	VNIServiceConstructor["AGW"] = VNIMakeAGWTemplate
	VNIServiceConstructor["PIM"] = VNIMakePIMTemplate
	VNIServiceConstructor["IR"] = VNIMakeIRTemplate
	VNIServiceConstructor["MS-IR"] = VNIMakeMSIRTemplate
	VNIServiceConstructor["ARP-Suppress"] = VNIMakeARPSuppressTemplate
	VNIServiceConstructor["Default"] = VNIMakeDefaultTemplate

	GeneralTemplateConstructor["VNI"] = VNIServiceConstructor

	OSPFServiceConstructor := make(ServiceConstructor)
	OSPFServiceConstructor["Interface"] = OSPFMakeInterfaceTemplate
	OSPFServiceConstructor["LSA-control"] = OSPFMakeLSAControlTemplate
	OSPFServiceConstructor["BFD"] = OSPFMakeBFDTemplate
	OSPFServiceConstructor["Default"] = OSPFMakeBFDTemplate

	GeneralTemplateConstructor["OSPF"] = OSPFServiceConstructor

	return GeneralTemplateConstructor
}

func VNIMakeL2VNITemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["vnid"], _ = strconv.ParseInt(serviceVariablesDB[key]["VNID"].(string), 10, 64)
	M["l2BD.accEncap"] = "vxlan-" + serviceVariablesDB[key]["VNID"].(string)
	M["l2BD.id"], _ = strconv.ParseInt(serviceVariablesDB[key]["VNID"].(string)[3:], 10, 64)
	M["l2BD.name"] = serviceVariablesDB[key]["Segment"].(string) + serviceVariablesDB[key]["ZoneID"].(string) + "Z_" + serviceVariablesDB[key]["Subnet"].(string) + "/" + serviceVariablesDB[key]["Mask"].(string)
	M["rtctrlRttEntry.rtt.export"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["VNID"].(string)
	M["rtctrlRttEntry.rtt.import"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["VNID"].(string)
	M["bgpInst.asn"] = IndirectVariablesDB[deviceName][key]["bgpInst.asn"]
	M["nvoNw.suppressARP"] = "off"
}

func VNIMakeAGWTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["hmmFwdIf.mode"] = "anycastGW"
	M["ipv4Addr.addr"] = serviceVariablesDB[key]["IPAddress"].(string) + "/" + serviceVariablesDB[key]["Mask"].(string)
	M["ipv4Addr.tag"], _ = strconv.ParseInt("39"+serviceVariablesDB[key]["ZoneID"].(string), 10, 64)
	M["ipv4Dom.name"] = serviceVariablesDB[key]["ZoneName"].(string)
	M["sviIf.id"] = "vlan" + serviceVariablesDB[key]["VNID"].(string)[3:]
}

func VNIMakeIRTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["nvoNw.mcastGroup"] = "0.0.0.0"
	M["nvoNw.multisiteIngRepl"] = "disable"
	M["nvoNw.vni"], _ = strconv.ParseInt(serviceVariablesDB[key]["VNID"].(string), 10, 64)
	M["nvoIngRepl.proto"] = "bgp"
	M["nvoIngRepl.rn"] = "IngRepl"
}

func VNIMakePIMTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["nvoNw.mcastGroup"] = "225.1.0." + serviceVariablesDB[key]["ZoneIDForMcast"].(string)
	M["nvoNw.multisiteIngRepl"] = "disable"
	M["nvoNw.vni"], _ = strconv.ParseInt(serviceVariablesDB[key]["VNID"].(string), 10, 64)
}

func VNIMakeMSIRTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["nvoNw.multisiteIngRepl"] = "enable"
}

func VNIMakeARPSuppressTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["nvoNw.suppressARP"] = "enabled"
}

func VNIMakeDefaultTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["bgpInst.asn"] = IndirectVariablesDB[deviceName][key]["bgpInst.asn"]
	M["vnid"], _ = strconv.ParseInt(serviceVariablesDB[key]["VNID"].(string), 10, 64)
}

func OSPFMakeInterfaceTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["ospfIf.adminSt"] = "enabled"
	M["ospfIf.area"] = serviceVariablesDB[key]["Area"].(string)
	M["ospfIf.helloIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["HelloIntvl"].(string), 10, 64)
	M["ospfIf.nwT"] = serviceVariablesDB[key]["nwT"].(string)
	M["ospfIf.rexmitIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["RexmitIntvl"].(string), 10, 64)
}

func OSPFMakeLSAControlTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["ospfLsaCtrl.arrivalIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["ArrivalIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.gpPacingIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["GpPacingIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.holdIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["HoldIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.maxIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["MaxIntvl"].(string), 10, 64)
}

func OSPFMakeBFDTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["ospfIf.bfdCtrl"] = "enabled"
	M["ospfDom.ctrl"] = "bfd"
}

func OSPFMakeDefaultTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed, IndirectVariablesDB IndirectVariablesDB, deviceName string) {
	M["ifid"] = serviceVariablesDB[key]["Name"].(string)
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

func FindCommonKeys(src map[string]interface{}, dst map[string]interface{}) []string {
	result := make([]string, 0)
	for k, _ := range src {
		if _, ok := dst[k]; ok {
			result = append(result, k)
		}
	}
	return result
}

func FindDistinctKeys(src map[string]interface{}, dst map[string]interface{}) []string {
	result := make([]string, 0)
	for k, _ := range src {
		if _, ok := dst[k]; !ok {
			result = append(result, k)
		}
	}
	return result
}

type DeviceDiffDB []DeviceDiffDBEntry
type DeviceDiffDBEntry struct {
	DeviceName string          `bson:"DeviceName"`
	DiffData   []DiffDataEntry `bson:"DiffData"`
}
type DiffDataEntry struct {
	Key      string                 `bson:"Key"`
	ToChange map[string]interface{} `bson:"ToChange"`
	ToAdd    map[string]interface{} `bson:"ToAdd"`
	ToDelete map[string]interface{} `bson:"ToDelete"`
}

func ConstrustDeficeDiffDB(t m.DeviceFootprintDB, o m.DeviceFootprintDB) DeviceDiffDB {

	var deviceDiffDB DeviceDiffDB

	for deviceDataDBEntryIndex, deviceDataDBEntry := range t {
		var DeviceDiffDBEntry DeviceDiffDBEntry
		DeviceDiffDBEntry.DeviceName = deviceDataDBEntry.DeviceName

		for deviceDataEntryIndex, deviceDataEntry := range deviceDataDBEntry.DeviceData {
			var diffDataEntry DiffDataEntry
			diffDataEntry.Key = deviceDataEntry.Key

			ToChange := make(map[string]interface{})
			ToAdd := make(map[string]interface{})
			ToDelete := make(map[string]interface{})

			diffDataEntry.ToChange = ToChange
			diffDataEntry.ToAdd = ToAdd
			diffDataEntry.ToDelete = ToDelete

			tmplDeviceData := deviceDataEntry.Data
			origDeviceData := o[deviceDataDBEntryIndex].DeviceData[deviceDataEntryIndex].Data

			commonKeys := FindCommonKeys(tmplDeviceData, origDeviceData)
			distinctKeysSrcOnly := FindDistinctKeys(tmplDeviceData, origDeviceData)
			distinctKeysDstOnly := FindDistinctKeys(tmplDeviceData, origDeviceData)

			for _, v := range commonKeys {
				if tmplDeviceData[v] != origDeviceData[v] {
					ToChange[v] = tmplDeviceData[v]
				}
			}

			for _, v := range distinctKeysSrcOnly {
				ToAdd[v] = tmplDeviceData[v]
			}

			for _, v := range distinctKeysDstOnly {
				ToDelete[v] = origDeviceData[v]
			}

			DeviceDiffDBEntry.DiffData = append(DeviceDiffDBEntry.DiffData, diffDataEntry)
		}

		deviceDiffDB = append(deviceDiffDB, DeviceDiffDBEntry)

	}

	return deviceDiffDB
}

func CheckForChanges(deviceDiffDB DeviceDiffDB) []string {

	var result []string

	for _, deviceDiffDBEntry := range deviceDiffDB {

		var marker bool

		for _, diffDataEntry := range deviceDiffDBEntry.DiffData {
			if len(diffDataEntry.ToAdd) == 0 && len(diffDataEntry.ToChange) == 0 && len(diffDataEntry.ToDelete) == 0 {
				continue
			} else {
				marker = marker || true
			}
		}

		if marker {
			result = append(result, deviceDiffDBEntry.DeviceName)
		}
	}
	return result
}
