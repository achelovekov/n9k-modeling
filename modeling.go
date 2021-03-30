package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	cu "github.com/achelovekov/collectorutils"
)

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

type MetaData struct {
	Config        cu.Config
	Enrich        cu.Enrich
	Filter        cu.Filter
	KeysMap       cu.KeysMap
	DB            DB
	ConversionMap cu.ConversionMap
}

func NXAPICall(hmd cu.HostMetaData, DMEPath string) map[string]interface{} {
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
		log.Fatal(err)
	}

	url := hmd.Host.URL + "/api/mo/aaaLogin.json"

	res, err := client.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
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

	src := make(map[string]interface{})

	err = json.Unmarshal(data, &src)
	if err != nil {
		panic(err)
	}

	return src

}

func worker(src map[string]interface{}, path cu.Path, mode int, filter cu.Filter, enrich cu.Enrich) []map[string]interface{} {
	var pathIndex int
	header := make(map[string]interface{})
	buf := make([]map[string]interface{}, 0)
	pathPassed := make([]string, 0)
	keysLeftFromPrevLayer := bool(false)

	cu.FlattenMap(src, path, pathIndex, pathPassed, mode, header, &buf, filter, enrich, keysLeftFromPrevLayer)

	return buf
}

type DB []DBEntry
type DBEntry struct {
	DeviceName  string
	DMEChunkMap DMEChunkMap
}
type DMEChunkMap map[string]DMEChunk
type DMEChunk []map[string]interface{}

type DataDB map[string]DeviceData
type DeviceData map[string]interface{}

type ProcessPath []struct {
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

func LoadProcessPath(fineName string) ProcessPath {
	var ProcessPath ProcessPath
	ProcessPathFile, err := os.Open(fineName)
	if err != nil {
		fmt.Println(err)
	}
	defer ProcessPathFile.Close()

	ProcessPathFileBytes, _ := ioutil.ReadAll(ProcessPathFile)

	err = json.Unmarshal(ProcessPathFileBytes, &ProcessPath)
	if err != nil {
		fmt.Println(err)
	}

	return ProcessPath
}

func PrettyPrintDataDB(DataDB DataDB) {
	for k, v := range DataDB {
		fmt.Println("Device:", k)

		JSONData, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			log.Fatalf(err.Error())
		}
		fmt.Printf("Pretty processed output %s\n", string(JSONData))
	}
}

func PrettyPrintDB(DB DB) {
	for _, DBEntry := range DB {
		fmt.Println("Device:", DBEntry.DeviceName)

		for k, DMEChunk := range DBEntry.DMEChunkMap {
			fmt.Println("	Key:", k)
			for _, v := range DMEChunk {
				cu.PrettyPrint(v)
			}
		}
	}
}

func Processing(md *MetaData, hmd cu.HostMetaData, src map[string]interface{}) {
	var DBEntry DBEntry
	DBEntry.DeviceName = hmd.Host.Hostname
	DBEntry.DMEChunkMap = make(map[string]DMEChunk)

	for k, v := range md.KeysMap {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, v := range v {
			buf = worker(src, v, cu.Cadence, md.Filter, md.Enrich)
			DMEChunk = append(DMEChunk, buf...)
		}
		DBEntry.DMEChunkMap[k] = DMEChunk
	}

	md.DB = append(md.DB, DBEntry)
}

func DeviceDataFill(DMEChunk DMEChunk, srcVal interface{}, KeyDName string, KeyList []string, DeviceData DeviceData, Options []Option, matchType string) {
	if matchType == "full" {
		if len(Options) == 0 {
			for _, item := range DMEChunk {
				if srcVal == item[KeyDName] {
					for _, v := range KeyList {
						if _, ok := item[v]; ok {
							DeviceData[v] = item[v]
						}
					}
				}
			}
		} else {
			for _, Option := range Options {
				for _, item := range DMEChunk {
					if srcVal == item[KeyDName] && item[Option.OptionKey] == Option.OptionValue {
						for _, v := range KeyList {
							if _, ok := item[v]; ok {
								DeviceData[v+"."+Option.OptionValue] = item[v]
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
				if strings.Contains(item[KeyDName].(string), srcVal.(string)) {
					for _, v := range KeyList {
						if _, ok := item[v]; ok {
							DeviceData[v] = item[v]
						}
					}
				}
			}
		} else {
			for _, Option := range Options {
				for _, item := range DMEChunk {
					if strings.Contains(item[KeyDName].(string), srcVal.(string)) && item[Option.OptionKey] == Option.OptionValue {
						for _, v := range KeyList {
							if _, ok := item[v]; ok {
								DeviceData[v+"."+Option.OptionValue] = item[v]
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
		fmt.Printf("%v %T", srcVal, srcVal)
		return ConversionMap[P](srcVal)
	} else {
		return srcVal
	}
}

func PathModeling(DataDB DataDB, DB DB, srcVal interface{}, ProcessPath ProcessPath, ConversionMap cu.ConversionMap) {
	for _, DBEntry := range DB {
		DeviceData := make(DeviceData)
		for _, v := range ProcessPath {
			if v.KeyLink == "direct" {
				DeviceData[v.KeySName] = srcVal
				DeviceData[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, DeviceData[v.KeySName], ConversionMap)
				DeviceDataFill(DBEntry.DMEChunkMap[v.ChunkName], DeviceData[v.KeySName], v.KeyDName, v.KeyList, DeviceData, v.Options, v.MatchType)
				fmt.Println("DeviceData after direct:", DeviceData)
			}

			if v.KeyLink == "indirect" {
				if _, ok := DeviceData[v.KeySName]; ok {
					DeviceData[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, DeviceData[v.KeySName], ConversionMap)
					DeviceDataFill(DBEntry.DMEChunkMap[v.ChunkName], DeviceData[v.KeySName], v.KeyDName, v.KeyList, DeviceData, v.Options, v.MatchType)
					fmt.Println("DeviceData after indirect:", DeviceData)
				}
			}
		}
		DataDB[DBEntry.DeviceName] = DeviceData
	}
}

type ServicesDefinition []ServiceDefinition
type ServiceDefinition struct {
	ServiceName string       `json:"ServiceName"`
	ServiceKeys []ServiceKey `json:"ServiceKyes"`
}
type ServiceKey struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}
type DeviceServiceDB map[string]DeviceServiceData
type DeviceServiceData map[string]bool

func LoadServicesDefinition(fineName string) ServicesDefinition {
	var ServicesDefinition ServicesDefinition
	ServicesDefinitionFile, err := os.Open(fineName)
	if err != nil {
		fmt.Println(err)
	}
	defer ServicesDefinitionFile.Close()

	ServicesDefinitionFileBytes, _ := ioutil.ReadAll(ServicesDefinitionFile)

	err = json.Unmarshal(ServicesDefinitionFileBytes, &ServicesDefinition)
	if err != nil {
		fmt.Println(err)
	}

	return ServicesDefinition
}

func CheckServiceKeys(ServiceKeys []ServiceKey, DeviceData map[string]interface{}) bool {
	var flag bool = true
	for _, ServiceKey := range ServiceKeys {
		if v, ok := DeviceData[ServiceKey.Name]; ok {
			if v == ServiceKey.Value || ServiceKey.Value == "anyValue" {
				flag = flag && true
			}
		} else {
			flag = flag && false
		}
	}
	return flag
}

func GetService(ServicesDefinition ServicesDefinition, DataDB DataDB, DeviceServiceDB DeviceServiceDB) {
	for k, v := range DataDB {
		DeviceServiceData := make(DeviceServiceData)
		fmt.Println("Get service for device:", k)
		for _, Service := range ServicesDefinition {
			if CheckServiceKeys(Service.ServiceKeys, v) {
				DeviceServiceData[Service.ServiceName] = true
			} else {
				DeviceServiceData[Service.ServiceName] = false
			}
		}
		DeviceServiceDB[k] = DeviceServiceData
	}
}

func main() {

	Config, Filter, Enrich := cu.Initialize("config.json")
	KeysMap := cu.LoadKeysMap(Config.KeysDefinitionFile)
	Inventory := cu.LoadInventory("inventory.json")

	srcVal := flag.String("key", "00000", "vnid to construct the model")
	flag.Parse()

	var DB DB
	DataDB := make(DataDB)
	ConversionMap := cu.CreateConversionMap()
	MetaData := &MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, DB: DB, ConversionMap: ConversionMap}

	for _, v := range Inventory {
		src := NXAPICall(v, "sys")
		Processing(MetaData, v, src)
	}

	PrettyPrintDB(MetaData.DB)

	var ProcessPath ProcessPath
	ProcessPath = LoadProcessPath("PathProcessingModel.json")

	PathModeling(DataDB, MetaData.DB, *srcVal, ProcessPath, MetaData.ConversionMap)
	PrettyPrintDataDB(DataDB)

	ServicesDefinition := LoadServicesDefinition("ServicesDefinition.json")
	DeviceServiceDB := make(DeviceServiceDB)
	GetService(ServicesDefinition, DataDB, DeviceServiceDB)

	fmt.Println(DeviceServiceDB)
}
