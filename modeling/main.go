package modeling

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	cu "github.com/achelovekov/collectorutils"
)

type MetaData struct {
	Config        cu.Config
	Enrich        cu.Enrich
	Filter        cu.Filter
	KeysMap       cu.KeysMap
	DB            DB
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

type DB []DBEntry
type DBEntry struct {
	DeviceName  string
	DMEChunkMap DMEChunkMap
}
type DMEChunkMap map[string]DMEChunk
type DMEChunk []map[string]interface{}

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
		fmt.Println(err)
	}
	defer ServiceDefinitionFile.Close()

	ServiceDefinitionFileBytes, _ := ioutil.ReadAll(ServiceDefinitionFile)

	err = json.Unmarshal(ServiceDefinitionFileBytes, &ServiceDefinition)
	if err != nil {
		fmt.Println(err)
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
				fmt.Println(err)
			}
			defer pathFile.Close()

			pathFileBytes, _ := ioutil.ReadAll(pathFile)
			var Path cu.Path
			err = json.Unmarshal(pathFileBytes, &Path)
			if err != nil {
				fmt.Println(err)
			}
			Paths = append(Paths, Path)
		}

		KeysMap[v.Key] = Paths
	}

	return KeysMap
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

func Processing(md *MetaData, hmd cu.HostMetaData, src map[string]interface{}) {
	var DBEntry DBEntry
	DBEntry.DeviceName = hmd.Host.Hostname
	DBEntry.DMEChunkMap = make(map[string]DMEChunk)

	for MapKey, Paths := range md.KeysMap {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, Path := range Paths {
			buf = worker(src, Path, cu.Cadence, md.Filter, md.Enrich)
			DMEChunk = append(DMEChunk, buf...)
		}
		DBEntry.DMEChunkMap[MapKey] = DMEChunk
	}

	md.DB = append(md.DB, DBEntry)
}

func DeviceDataFill(DMEChunk DMEChunk, KeySName string, KeyDName string, KeyList []string, DeviceData DeviceData, Options []Option, matchType string) {
	if matchType == "full" {
		if len(Options) == 0 {
			for _, item := range DMEChunk {
				if DeviceData[KeySName] == item[KeyDName] || (KeySName == "any" && KeyDName == "any") {
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
					if (DeviceData[KeySName] == item[KeyDName] && item[Option.OptionKey] == Option.OptionValue) || (KeySName == "any" && KeyDName == "any") {
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
				if strings.Contains(item[KeyDName].(string), DeviceData[KeySName].(string)) || (KeySName == "any" && KeyDName == "any") {
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
					if (strings.Contains(item[KeyDName].(string), DeviceData[KeySName].(string)) && item[Option.OptionKey] == Option.OptionValue) || (KeySName == "any" && KeyDName == "any") {
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
		return ConversionMap[P](srcVal)
	} else {
		return srcVal
	}
}

type ServiceDataDB []ServiceDataDBEntry
type ServiceDataDBEntry struct {
	DeviceName string     `json:"DeviceName"`
	DeviceData DeviceData `json:"DeviceData"`
}
type DeviceData map[string]interface{}

func ConstructServiceDataDB(ServiceDataDB *ServiceDataDB, DB DB, srcVal interface{}, ServiceConstructPath ServiceConstructPath, ConversionMap cu.ConversionMap) {
	for _, DBEntry := range DB {
		var ServiceDataDBEntry ServiceDataDBEntry
		DeviceData := make(DeviceData)
		for _, v := range ServiceConstructPath {
			if v.KeyLink == "direct" {
				DeviceData[v.KeySName] = srcVal
				DeviceData[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, DeviceData[v.KeySName], ConversionMap)
				DeviceDataFill(DBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, DeviceData, v.Options, v.MatchType)
			}
			if v.KeyLink == "indirect" {
				if _, ok := DeviceData[v.KeySName]; ok {
					DeviceData[v.KeySName] = TypeConversion(v.KeySType, v.KeyDType, DeviceData[v.KeySName], ConversionMap)
					DeviceDataFill(DBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, DeviceData, v.Options, v.MatchType)
				}
			}
			if v.KeyLink == "no-link" {
				DeviceDataFill(DBEntry.DMEChunkMap[v.ChunkName], v.KeySName, v.KeyDName, v.KeyList, DeviceData, v.Options, v.MatchType)
			}
		}
		ServiceDataDBEntry.DeviceName = DBEntry.DeviceName
		ServiceDataDBEntry.DeviceData = DeviceData
		*ServiceDataDB = append(*ServiceDataDB, ServiceDataDBEntry)
	}
}

func PrettyPrint(src interface{}) {
	JSONData, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("Pretty processed output: %s\n", string(JSONData))

	file, err := os.Create("ProcessedData.json")
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer file.Close()
	_, err = file.Write(JSONData)
	if err != nil {
		log.Fatalf(err.Error())
	}
}

type ServiceLayoutDB []ServiceLayoutDBEntry
type ServiceLayoutDBEntry struct {
	DeviceName    string        `json:"DeviceName"`
	ServiceLayout ServiceLayout `json:"ServiceLayout"`
}
type ServiceLayout []ComponentBitMap
type ComponentBitMap struct {
	Name  string `json:"Name"`
	Value bool   `json:"Value"`
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

func ConstructServiceLayout(ServiceComponents ServiceComponents, ServiceDataDB ServiceDataDB, ServiceLayoutDB *ServiceLayoutDB) {
	for _, ServiceDataDBEntry := range ServiceDataDB {
		var ServiceLayoutDBEntry ServiceLayoutDBEntry
		for _, ServiceComponent := range ServiceComponents {
			var ComponentBitMap ComponentBitMap
			if CheckComponentKeys(ServiceComponent.ComponentKeys, ServiceDataDBEntry.DeviceData) {
				ComponentBitMap.Value = true
			} else {
				ComponentBitMap.Value = false
			}
			ComponentBitMap.Name = ServiceComponent.ComponentName
			ServiceLayoutDBEntry.ServiceLayout = append(ServiceLayoutDBEntry.ServiceLayout, ComponentBitMap)
			ServiceLayoutDBEntry.DeviceName = ServiceDataDBEntry.DeviceName
		}
		*ServiceLayoutDB = append(*ServiceLayoutDB, ServiceLayoutDBEntry)
	}
}

type ProcessedData struct {
	ServiceName     string          `json:"ServiceName"`
	ServiceDataDB   ServiceDataDB   `json:"ServiceDataDB"`
	ServiceLayoutDB ServiceLayoutDB `json:"ServiceLayoutDB"`
}
