package modeling

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	mo "github.com/achelovekov/n9k-modeling/mongo"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
)

type MetaData struct {
	Config                cu.Config
	Enrich                cu.Enrich
	Filter                cu.Filter
	ChunksProcessingPaths cu.ChunksProcessingPaths
	ConversionMap         cu.ConversionMap
}

type NXAPILoginBody struct {
	AaaUser AaaUser `json:"aaaUser"`
}

type AaaUser struct {
	Attributes Attributes `json:"attributes"`
}

type Attributes struct {
	Name string `json:"name"`
	Pwd  string `json:"pwd"`
}

type NXAPILoginResponse struct {
	Imdata []struct {
		AaaLogin struct {
			Attributes struct {
				Token string `json:"token"`
			}
		}
	}
}

func NXAPICall(hmd cu.HostMetaData, DMEPath string) (map[string]interface{}, error) {
	src := make(map[string]interface{})

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: transport}

	NXAPILoginBody := &NXAPILoginBody{
		AaaUser: AaaUser{
			Attributes: Attributes{
				Name: hmd.HostData.Username,
				Pwd:  hmd.HostData.Password,
			},
		},
	}

	requestBody, err := json.MarshalIndent(NXAPILoginBody, "", "  ")
	if err != nil {
		log.Println(err)
	}

	url := hmd.HostData.URL + "/api/mo/aaaLogin.json"

	res, err := client.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		log.Println(err)
		return src, errors.New("Can't reach device API")
	}

	if res.StatusCode != 200 {
		log.Println("Unauthorized acces or something goes wrong while receiving access cookie")
		return src, fmt.Errorf("Can't get access cookie from device: %v", hmd.HostName)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
	}
	res.Body.Close()

	var NXAPILoginResponse NXAPILoginResponse

	err = json.Unmarshal([]byte(body), &NXAPILoginResponse)

	token := "APIC-cookie=" + NXAPILoginResponse.Imdata[0].AaaLogin.Attributes.Token

	url = hmd.HostData.URL + "/api/mo/" + DMEPath + ".json?rsp-subtree=full&rsp-prop-include=config-only"

	req, err := http.NewRequest("GET", url, io.Reader(nil))
	req.Header.Set("Cookie", token)

	resp, err := client.Do(req)
	data, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		fmt.Printf("error = %s \n", err)
	}

	err = json.Unmarshal(data, &src)
	if err != nil {
		panic(err)
	}

	return src, nil

}

type ServiceDefinition struct {
	DMEProcessing        []cu.ChunkDefinition `json:"DMEProcessing"`
	ServiceName          string               `json:"ServiceName"`
	ServiceConstructPath ServiceConstructPath `json:"ServiceConstructPath"`
	ServiceComponents    ServiceComponents    `json:"ServiceComponents"`
}

type ServiceConstructPath []ServiceConstructPathEntry
type ServiceConstructPathEntry struct {
	ChunkName   string             `json:"ChunkName"`
	KeySName    string             `json:"KeySName"`
	KeySType    string             `json:"KeySType"`
	KeyDName    string             `json:"KeyDName"`
	KeyDType    string             `json:"KeyDType"`
	KeyLink     string             `json:"KeyLink"`
	MatchType   string             `json:"MatchType"`
	KeyList     []string           `json:"KeyList"`
	CombineBy   CombineBy          `json:"CombineBy"`
	SplitSearch []SplitSearchEntry `json:"SplitSearch"`
}

type CombineBy struct {
	OptionName string   `json:"OptionName"`
	OptionKeys []string `json:"OptionKeys"`
}

type SplitSearchEntry struct {
	SearchFrom string `json:"SearchFrom"`
	SearchFor  string `json:"SearchFor"`
}

type ServiceComponents []ServiceComponent
type ServiceComponent struct {
	ComponentName string         `json:"ComponentName"`
	ComponentKeys []ComponentKey `json:"ComponentKeys"`
}
type ComponentKey struct {
	Name      string `json:"Name"`
	Value     string `json:"Value"`
	MatchType string `json:"MatchType"`
}

func LoadServiceDefinition(fineName string) ServiceDefinition {
	var ServiceDefinition ServiceDefinition
	ServiceDefinitionFile, err := os.Open(fineName)
	if err != nil {
		log.Println(err)
	}
	defer ServiceDefinitionFile.Close()

	ServiceDefinitionFileBytes, _ := ioutil.ReadAll(ServiceDefinitionFile)

	err = json.Unmarshal(ServiceDefinitionFileBytes, &ServiceDefinition)
	if err != nil {
		log.Println(err)
	}

	return ServiceDefinition
}

func LoadChunksProcessingPaths(chunksDefinition []cu.ChunkDefinition) cu.ChunksProcessingPaths {

	ChunksProcessingPaths := make(cu.ChunksProcessingPaths)

	for _, chunksDefinitionEntry := range chunksDefinition {
		var Paths cu.Paths

		for _, v := range chunksDefinitionEntry.Paths {
			pathFile, err := os.Open(v.Path)
			if err != nil {
				log.Println(err)
			}
			defer pathFile.Close()

			pathFileBytes, _ := ioutil.ReadAll(pathFile)
			var Path cu.Path
			err = json.Unmarshal(pathFileBytes, &Path)
			fmt.Println(Path)
			if err != nil {
				log.Println(err)
			}
			Paths = append(Paths, Path)
		}

		ChunksProcessingPaths[chunksDefinitionEntry.ChunkName] = Paths
	}

	return ChunksProcessingPaths
}

type SrcValList []string

func LoadSrcValList(fileName string) SrcValList {
	var srcValList SrcValList
	srcValListFile, err := os.Open(fileName)
	if err != nil {
		log.Println(err)
	}
	defer srcValListFile.Close()

	ServiceDefinitionFileBytes, _ := ioutil.ReadAll(srcValListFile)

	err = json.Unmarshal(ServiceDefinitionFileBytes, &srcValList)
	if err != nil {
		log.Println(err)
	}

	return srcValList
}

func worker(src interface{}, path cu.Path, mode int, filter cu.Filter, enrich cu.Enrich) []map[string]interface{} {
	var pathIndex int
	header := make(map[string]interface{})
	buf := make([]map[string]interface{}, 0)
	pathPassed := make([]string, 0)
	keysLeftFromPrevLayer := bool(false)

	cu.FlattenMapMongo(src, path, pathIndex, pathPassed, mode, header, &buf, filter, enrich, keysLeftFromPrevLayer)

	return buf
}

func GetRawData(ctx context.Context, collection *mongo.Collection, hmd cu.HostMetaData, DMEPath string, wg *sync.WaitGroup) {

	src, err := NXAPICall(hmd, DMEPath)
	if err != nil {
		log.Println("Can't get data from device:", hmd.HostName)
		wg.Done()
	}
	fmt.Println("got data from:", hmd.HostName)

	mo.UpdateOne(ctx, collection, "DeviceName", hmd.HostName, "DeviceDMEData", src)
	wg.Done()

}

func Processing(md *MetaData, hmd cu.HostMetaData, src interface{}) DeviceChunksDBEntry {

	var deviceChunksDBEntry DeviceChunksDBEntry
	deviceChunksDBEntry.DeviceName = hmd.HostName
	deviceChunksDBEntry.DMEChunkMap = make(map[string]DMEChunk)

	for chunk, paths := range md.ChunksProcessingPaths {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, path := range paths {
			buf = worker(src, path, cu.Cadence, md.Filter, md.Enrich)
			DMEChunk = append(DMEChunk, buf...)
		}
		deviceChunksDBEntry.DMEChunkMap[chunk] = DMEChunk
	}
	return deviceChunksDBEntry
}

func TypeConversion(srcType string, dstType string, srcVal interface{}, ConversionMap cu.ConversionMap) interface{} {
	if srcType != dstType {
		P := cu.Pair{SrcType: srcType, DstType: dstType}
		return ConversionMap[P](srcVal)
	} else {
		return srcVal
	}
}

type DeviceChunksDB []DeviceChunksDBEntry
type DeviceChunksDBEntry struct {
	DeviceName  string
	DMEChunkMap DMEChunkMap
}
type DMEChunkMap map[string]DMEChunk
type DMEChunk []map[string]interface{}

type DeviceFootprintDB []DeviceFootprintDBEntry
type DeviceFootprintDBEntry struct {
	DeviceName string     `bson:"DeviceName"`
	DeviceData DeviceData `bson:"DeviceData"`
}
type DeviceData []DeviceDataEntry
type DeviceDataEntry struct {
	Key  string `bson:"Key"`
	Data Data   `bson:"Data"`
}
type Data map[string]interface{}

type CombineByDB []CombineByDBEntry
type CombineByDBEntry struct {
	OptionName   string
	OptionValues []interface{}
}

func ConstructDeviceDataEntry(srcVal string, deviceChunksDBEntry DeviceChunksDBEntry, ServiceConstructPath ServiceConstructPath, ConversionMap cu.ConversionMap) DeviceDataEntry {
	var deviceDataEntry DeviceDataEntry
	var combineByDB CombineByDB
	data := make(Data)

	deviceDataEntry.Key = srcVal
	deviceDataEntry.Data = data

	for _, entry := range ServiceConstructPath {
		if entry.KeyLink == "direct" {
			data[entry.KeySName] = srcVal
			data[entry.KeySName] = TypeConversion(entry.KeySType, entry.KeyDType, data[entry.KeySName], ConversionMap)
		}
		if len(entry.CombineBy.OptionName) > 0 {
			combineByDBEntry := ConstructCombineByDBEntry(deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], entry, data)
			combineByDB = append(combineByDB, combineByDBEntry)
			fmt.Println("go 1")
			//fmt.Println(data)
			fmt.Println(combineByDB)
			DeviceDataFillCombine(deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], entry, data, combineByDBEntry)
			DeviceDataFillNoCombine(deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], entry, data)
			cu.PrettyPrint(data)
		}
		if len(entry.CombineBy.OptionName) == 0 && len(entry.SplitSearch) > 0 {
			//DeviceDataFillNoCombine(deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], entry, data)
			fmt.Println("go 2")
			searchStruct := ConstructSplitSearchL1(entry, combineByDB)
			fmt.Println(searchStruct)
			CheckDMEChunkForKeys(entry, searchStruct, deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], data)
			cu.PrettyPrint(data)
		}
		/* 		if entry.SplitSearch != (SplitSearch{}) {
			//DeviceDataFillSplitSearch(deviceChunksDBEntry.DMEChunkMap[entry.ChunkName], entry, data, combineByDB)
			fmt.Println("go 3")
			//fmt.Println(data)
		} */
	}

	return deviceDataEntry
}

func CheckDMEChunkForKeys(serviceConstructPathEntry ServiceConstructPathEntry, searchStruct []SearchStructEntry, dMEChunk DMEChunk, data Data) {
	for _, dMEChunkEntry := range dMEChunk {
		for _, searchStructEntry := range searchStruct {
			if CheckForKeys(dMEChunkEntry, searchStructEntry.KeysList) {
				fmt.Println("go here")
				for _, key := range serviceConstructPathEntry.KeyList {
					if v, ok := dMEChunkEntry[key]; ok {
						data[key+searchStructEntry.Prefix] = v
					}
				}
			}
		}
	}
}

func CheckForKeys(dMEChunkEntry map[string]interface{}, keyList map[string]interface{}) bool {
	var result bool
	for key, val := range keyList {
		fmt.Println(key, val)
		if v, ok := dMEChunkEntry[key]; ok {
			if v == val {
				fmt.Println("yes")
				result = true
			}
		}
	}
	fmt.Println(result)
	return result
}

type SearchStructEntry struct {
	KeysList map[string]interface{}
	Prefix   string
}

func ConstructSplitSearchL1(entry ServiceConstructPathEntry, combineByDB CombineByDB) []SearchStructEntry {
	combineByDBEntry := FindCombineByDBEntry(entry.SplitSearch[0].SearchFrom, combineByDB)
	var searchStruct []SearchStructEntry

	for _, v := range combineByDBEntry.OptionValues {
		m := make(map[string]interface{})
		m[entry.SplitSearch[0].SearchFor] = v
		prefix := "." + v.(string)
		var searchStructEntry SearchStructEntry
		searchStructEntry.KeysList = m
		searchStructEntry.Prefix = prefix
		searchStruct = append(searchStruct, searchStructEntry)
	}

	return searchStruct
}

/* func DeviceDataFillSplitSearch(dMEChunk DMEChunk, serviceConstructPathEntry ServiceConstructPathEntry, data Data, combineByDB CombineByDB) {
	combineByDBEntry := FindCombineByDBEntry(serviceConstructPathEntry.SplitSearch.SearchFrom, combineByDB)

	for _, optionValue := range combineByDBEntry.OptionValues {
		for _, dMEChunkEntry := range dMEChunk {
			if data[serviceConstructPathEntry.KeySName] == dMEChunkEntry[serviceConstructPathEntry.KeyDName] && dMEChunkEntry[serviceConstructPathEntry.SplitSearch.SearchFor] == optionValue {
				for _, key := range serviceConstructPathEntry.KeyList {
					if v, ok := dMEChunkEntry[key]; ok {
						data[key+"."+dMEChunkEntry[serviceConstructPathEntry.SplitSearch.SearchFor].(string)] = v
					}
				}
			}
		}
	}
} */

func FindCombineByDBEntry(searchFor string, combineByDB CombineByDB) CombineByDBEntry {
	var result CombineByDBEntry
	for _, combineByDBEntry := range combineByDB {
		if combineByDBEntry.OptionName == searchFor {
			result = combineByDBEntry
		}
	}

	return result
}

func DeviceDataFillCombine(dMEChunk DMEChunk, serviceConstructPathEntry ServiceConstructPathEntry, data Data, combineByDBEntry CombineByDBEntry) {
	for _, dMEChunkEntry := range dMEChunk {
		for _, optionValue := range combineByDBEntry.OptionValues {
			if data[serviceConstructPathEntry.KeySName] == dMEChunkEntry[serviceConstructPathEntry.KeyDName] && dMEChunkEntry[combineByDBEntry.OptionName] == optionValue {
				for _, key := range serviceConstructPathEntry.CombineBy.OptionKeys {
					data[key+"."+dMEChunkEntry[combineByDBEntry.OptionName].(string)] = dMEChunkEntry[key]
				}
			}
		}
	}
}

func DeviceDataFillNoCombine(dMEChunk DMEChunk, serviceConstructPathEntry ServiceConstructPathEntry, data Data) {
	for _, dMEChunkEntry := range dMEChunk {
		if data[serviceConstructPathEntry.KeySName] == dMEChunkEntry[serviceConstructPathEntry.KeyDName] {
			for _, key := range serviceConstructPathEntry.KeyList {
				data[key] = dMEChunkEntry[key]
			}
		}
	}
}

func ConstructCombineByDBEntry(dMEChunk DMEChunk, serviceConstructPathEntry ServiceConstructPathEntry, data Data) CombineByDBEntry {

	var combineByDBEntry CombineByDBEntry
	combineByDBEntry.OptionName = serviceConstructPathEntry.CombineBy.OptionName

	for _, dMEChunkEntry := range dMEChunk {
		if data[serviceConstructPathEntry.KeySName] == dMEChunkEntry[serviceConstructPathEntry.KeyDName] {
			combineByDBEntry.OptionValues = append(combineByDBEntry.OptionValues, dMEChunkEntry[serviceConstructPathEntry.CombineBy.OptionName])
		}
	}
	return combineByDBEntry
}

func ConstructDeviceFootprintDB(DeviceChunksDB DeviceChunksDB, srcValList []string, serviceConstructPath ServiceConstructPath, ConversionMap cu.ConversionMap) DeviceFootprintDB {

	deviceFootprintDB := make(DeviceFootprintDB, 0)

	for _, deviceChunksDBEntry := range DeviceChunksDB {
		var deviceFootprintDBEntry DeviceFootprintDBEntry

		deviceFootprintDBEntry.DeviceName = deviceChunksDBEntry.DeviceName

		for _, srcVal := range srcValList {
			deviceDataEntry := ConstructDeviceDataEntry(srcVal, deviceChunksDBEntry, serviceConstructPath, ConversionMap)
			deviceFootprintDBEntry.DeviceData = append(deviceFootprintDBEntry.DeviceData, deviceDataEntry)
		}
		deviceFootprintDB = append(deviceFootprintDB, deviceFootprintDBEntry)
	}

	return deviceFootprintDB
}

func MarshalToJSON(src interface{}) []byte {
	JSONData, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	return JSONData
}

func WriteDataToFile(fileName string, JSONData []byte) {
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer file.Close()
	_, err = file.Write(JSONData)
	if err != nil {
		log.Fatalf(err.Error())
	}
}

type ServiceFootprintDB []ServiceFootprintDBEntry
type ServiceFootprintDBEntry struct {
	DeviceName     string          `bson:"DeviceName"`
	ServiceLayouts []ServiceLayout `bson:"ServiceLayouts"`
}

type ServiceLayout struct {
	Key  string                  `bson:"Key"`
	Data ServiceComponentBitMaps `bson:"Data"`
}

type ServiceComponentBitMaps []ServiceComponentBitMap
type ServiceComponentBitMap struct {
	Name  string `bson:"Name"`
	Value bool   `bson:"Value"`
}

func ConstructServiceFootprintDB(ServiceComponents ServiceComponents, DeviceFootprintDB DeviceFootprintDB) ServiceFootprintDB {

	serviceFootprintDB := make(ServiceFootprintDB, 0)

	for _, deviceFootprintDBEntry := range DeviceFootprintDB {
		var serviceFootprintDBEntry ServiceFootprintDBEntry
		serviceFootprintDBEntry.DeviceName = deviceFootprintDBEntry.DeviceName

		for _, deviceDataEntry := range deviceFootprintDBEntry.DeviceData {

			var serviceLayout ServiceLayout
			serviceLayout.Key = deviceDataEntry.Key

			for _, serviceComponent := range ServiceComponents {

				var serviceComponentBitMap ServiceComponentBitMap
				serviceComponentBitMap.Name = serviceComponent.ComponentName

				if CheckComponentKeys(serviceComponent.ComponentKeys, deviceDataEntry.Data) {
					serviceComponentBitMap.Value = true
				} else {
					serviceComponentBitMap.Value = false
				}

				serviceLayout.Data = append(serviceLayout.Data, serviceComponentBitMap)
			}
			serviceFootprintDBEntry.ServiceLayouts = append(serviceFootprintDBEntry.ServiceLayouts, serviceLayout)
		}
		serviceFootprintDB = append(serviceFootprintDB, serviceFootprintDBEntry)
	}
	return serviceFootprintDB
}

func CheckComponentKeys(ComponentKeys []ComponentKey, DeviceData map[string]interface{}) bool {
	var flag bool = true
	for _, ComponentKey := range ComponentKeys {
		if v, ok := DeviceData[ComponentKey.Name]; ok {
			if ComponentKey.MatchType == "equal" {
				if v == ComponentKey.Value || ComponentKey.Value == "anyValue" {
					flag = flag && true
				} else {
					flag = flag && false
				}
			}
			if ComponentKey.MatchType == "not-equal" {
				if v != ComponentKey.Value {
					flag = flag && true
				} else {
					flag = flag && false
				}
			}
		} else {
			flag = flag && false
		}
	}
	return flag
}

func GetServiceComponentsList(serviceDefinition ServiceDefinition) []string {
	var result []string

	for _, serviceComponent := range serviceDefinition.ServiceComponents {
		result = append(result, serviceComponent.ComponentName)
	}

	return result
}

type ProcessedData struct {
	ServiceName        string             `bson:"ServiceName"`
	Keys               []string           `bson:"Keys"`
	ServiceComponents  []string           `bson:"ServiceComponents"`
	DeviceFootprintDB  DeviceFootprintDB  `bson:"DeviceFootprintDB"`
	ServiceFootprintDB ServiceFootprintDB `bson:"ServiceFootprintDB"`
}

type ServiceList []ServiceListEntry
type ServiceListEntry struct {
	ServiceName string `bson:"ServiceName"`
}
