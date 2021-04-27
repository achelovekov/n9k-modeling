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
	"strings"
	"sync"

	mo "github.com/achelovekov/n9k-modeling/mongo"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
)

type MetaData struct {
	Config        cu.Config
	Enrich        cu.Enrich
	Filter        cu.Filter
	KeysMap       cu.KeysMap
	ConversionMap cu.ConversionMap
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
				Name: hmd.Host.Username,
				Pwd:  hmd.Host.Password,
			},
		},
	}

	requestBody, err := json.MarshalIndent(NXAPILoginBody, "", "  ")
	if err != nil {
		log.Println(err)
	}

	url := hmd.Host.URL + "/api/mo/aaaLogin.json"

	res, err := client.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		log.Println(err)
		return src, errors.New("Can't reach device API")
	}

	if res.StatusCode != 200 {
		log.Println("Unauthorized acces or something goes wrong while receiving access cookie")
		return src, fmt.Errorf("Can't get access cookie from device: %v", hmd.Host.Hostname)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
	}
	res.Body.Close()

	var NXAPILoginResponse NXAPILoginResponse

	err = json.Unmarshal([]byte(body), &NXAPILoginResponse)

	token := "APIC-cookie=" + NXAPILoginResponse.Imdata[0].AaaLogin.Attributes.Token

	url = hmd.Host.URL + "/api/mo/" + DMEPath + ".json?rsp-subtree=full&rsp-prop-include=config-only"

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
	DMEProcessing        []cu.KeyDefinition   `json:"DMEProcessing"`
	ServiceName          string               `json:"ServiceName"`
	ServiceConstructPath ServiceConstructPath `json:"ServiceConstructPath"`
	ServiceComponents    ServiceComponents    `json:"ServiceComponents"`
}

type ServiceConstructPath []struct {
	ChunkName string   `json:"ChunkName"`
	KeySName  string   `json:"KeySName"`
	KeySType  string   `json:"KeySType"`
	KeyDName  string   `json:"KeyDName"`
	KeyDType  string   `json:"KeyDType"`
	KeyLink   string   `json:"KeyLink"`
	MatchType string   `json:"MatchType"`
	KeyList   []string `json:"KeyList"`
	Options   []Option `json:"Options"`
}
type Option struct {
	OptionKey   string `json:"optionKey"`
	OptionValue string `json:"optionValue"`
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

func LoadKeysMap(KeysDefinition []cu.KeyDefinition) cu.KeysMap {

	KeysMap := make(cu.KeysMap)

	for _, v := range KeysDefinition {
		var Paths cu.Paths

		for _, v := range v.Paths {
			pathFile, err := os.Open(v.Path)
			if err != nil {
				log.Println(err)
			}
			defer pathFile.Close()

			pathFileBytes, _ := ioutil.ReadAll(pathFile)
			var Path cu.Path
			err = json.Unmarshal(pathFileBytes, &Path)
			if err != nil {
				log.Println(err)
			}
			Paths = append(Paths, Path)
		}

		KeysMap[v.Key] = Paths
	}

	return KeysMap
}

type SrcValList []interface{}

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
		log.Println("Can't get data from device:", hmd.Host.Hostname)
		wg.Done()
	}
	fmt.Println("got data from:", hmd.Host.Hostname)

	mo.UpdateOne(ctx, collection, "DeviceName", hmd.Host.Hostname, "DeviceDMEData", src)
	wg.Done()

}

func Processing(md *MetaData, hmd cu.HostMetaData, src interface{}) DeviceChunksDBEntry {

	var DeviceChunksDBEntryVal DeviceChunksDBEntry
	DeviceChunksDBEntryVal.DeviceName = hmd.Host.Hostname
	DeviceChunksDBEntryVal.DMEChunkMap = make(map[string]DMEChunk)

	for MapKey, Paths := range md.KeysMap {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, Path := range Paths {
			buf = worker(src, Path, cu.Cadence, md.Filter, md.Enrich)
			DMEChunk = append(DMEChunk, buf...)
		}
		DeviceChunksDBEntryVal.DMEChunkMap[MapKey] = DMEChunk
	}
	return DeviceChunksDBEntryVal
}

func DeviceDataFill(DMEChunk DMEChunk, KeySName string, KeyDName string, KeyList []string, data Data, Options []Option, matchType string) {
	if matchType == "full" {
		if len(Options) == 0 {
			for _, item := range DMEChunk {
				if data[KeySName] == item[KeyDName] || (KeySName == "any" && KeyDName == "any") {
					for _, v := range KeyList {
						if _, ok := item[v]; ok {
							data[v] = item[v]
						}
					}
				}
			}
		} else {
			for _, Option := range Options {
				for _, item := range DMEChunk {
					if (data[KeySName] == item[KeyDName] && item[Option.OptionKey] == Option.OptionValue) || (KeySName == "any" && KeyDName == "any") {
						for _, v := range KeyList {
							if _, ok := item[v]; ok {
								data[v+"."+Option.OptionValue] = item[v]
							}
						}
					}
				}
			}
		}
	}
	if matchType == "partial" {
		if len(Options) == 0 {
			for _, item := range DMEChunk {
				if strings.Contains(item[KeyDName].(string), data[KeySName].(string)) || (KeySName == "any" && KeyDName == "any") {
					for _, v := range KeyList {
						if _, ok := item[v]; ok {
							data[v] = item[v]
						}
					}
				}
			}
		} else {
			for _, Option := range Options {
				for _, item := range DMEChunk {
					if (strings.Contains(item[KeyDName].(string), data[KeySName].(string)) && item[Option.OptionKey] == Option.OptionValue) || (KeySName == "any" && KeyDName == "any") {
						for _, v := range KeyList {
							if _, ok := item[v]; ok {
								data[v+"."+Option.OptionValue] = item[v]
							}
						}
					}
				}
			}
		}
	}
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
	DeviceName string     `json:"DeviceName"`
	DeviceData DeviceData `json:"DeviceData"`
}
type DeviceData []DeviceDataEntry
type DeviceDataEntry struct {
	Key  interface{} `json:"Key"`
	Data Data        `json:"Data"`
}
type Data map[string]interface{}

func ConstructDeviceDataEntry(srcVal interface{}, deviceChunksDBEntry DeviceChunksDBEntry, ServiceConstructPath ServiceConstructPath, ConversionMap cu.ConversionMap) DeviceDataEntry {
	var deviceDataEntry DeviceDataEntry
	data := make(Data)

	deviceDataEntry.Key = srcVal
	deviceDataEntry.Data = data

	for _, v := range ServiceConstructPath {
		if v.KeyLink == "direct" {
			data[v.KeySName] = srcVal
			data[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, data[v.KeySName], ConversionMap)
			DeviceDataFill(deviceChunksDBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, data, v.Options, v.MatchType)
		}
		if v.KeyLink == "indirect" {
			if _, ok := data[v.KeySName]; ok {
				data[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, data[v.KeySName], ConversionMap)
				DeviceDataFill(deviceChunksDBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, data, v.Options, v.MatchType)
			}
		}
		if v.KeyLink == "no-link" {
			DeviceDataFill(deviceChunksDBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, data, v.Options, v.MatchType)
		}
	}

	return deviceDataEntry
}

func ConstructDeviceFootprintDB(DeviceChunksDB DeviceChunksDB, srcValList []interface{}, ServiceConstructPath ServiceConstructPath, ConversionMap cu.ConversionMap) DeviceFootprintDB {

	deviceFootprintDB := make(DeviceFootprintDB, 0)

	for _, deviceChunksDBEntry := range DeviceChunksDB {
		var deviceFootprintDBEntry DeviceFootprintDBEntry

		deviceFootprintDBEntry.DeviceName = deviceChunksDBEntry.DeviceName

		for _, srcVal := range srcValList {
			deviceDataEntry := ConstructDeviceDataEntry(srcVal, deviceChunksDBEntry, ServiceConstructPath, ConversionMap)
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
	DeviceName     string          `json:"DeviceName"`
	ServiceLayouts []ServiceLayout `json:"ServiceLayouts"`
}

type ServiceLayout struct {
	Key  interface{}             `json:"Key"`
	Data ServiceComponentBitMaps `json:"Data"`
}

type ServiceComponentBitMaps []ServiceComponentBitMap
type ServiceComponentBitMap struct {
	Name  string `json:"Name"`
	Value bool   `json:"Value"`
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

type ProcessedData struct {
	ServiceName        string             `json:"ServiceName"`
	DeviceFootprintDB  DeviceFootprintDB  `json:"DeviceFootprintDB"`
	ServiceFootprintDB ServiceFootprintDB `json:"ServiceFootprintDB"`
}
