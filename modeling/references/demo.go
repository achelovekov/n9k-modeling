package main

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
	"strconv"
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
	Config  cu.Config
	Enrich  cu.Enrich
	Filter  cu.Filter
	KeysMap cu.KeysMap
	DB      DB
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

func PrettyPrint(DataDB DataDB) {
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

func Processing(md *MetaData, src map[string]interface{}) {
	var DBEntry DBEntry
	DBEntry.DeviceName = "site-1-ac-2"
	DBEntry.DMEChunkMap = make(map[string]DMEChunk)

	for k, v := range md.KeysMap {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, v := range v {
			buf = worker(src, v, cu.Cadence, md.Filter, md.Enrich)
			/* 			buf = append(buf, result...) */
			DMEChunk = append(DMEChunk, buf...)
		}
		DBEntry.DMEChunkMap[k] = DMEChunk
	}

	md.DB = append(md.DB, DBEntry)
}

func Modeling(DataDB DataDB, DB DB, vlanid int64) {
	for _, DBEntry := range DB {
		DeviceData := make(DeviceData)
		for _, item := range DBEntry.DMEChunkMap["bd"] {
			if item["l2BD.id"] == vlanid {
				DeviceData["l2BD.id"] = vlanid
				DeviceData["l2BD.accEncap"] = item["l2BD.accEncap"]
				DeviceData["l2BD.name"] = item["l2BD.name"]
			}
		}
		for index, item := range DBEntry.DMEChunkMap["evpn"] {
			if item["rtctrlBDEvi.encap"] == DeviceData["l2BD.accEncap"] {
				DeviceData[strconv.Itoa(index%2)+"_"+"rtctrlRttP.type"] = item["rtctrlRttP.type"]
				DeviceData[strconv.Itoa(index%2)+"_"+"rtctrlRttEntry.rtt"] = item["rtctrlRttEntry.rtt"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["svi"] {
			if item["sviIf.vlanId"] == DeviceData["l2BD.id"] {
				DeviceData["sviIf.id"] = item["sviIf.id"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["ipv4"] {
			if item["ipv4If.id"] == DeviceData["sviIf.id"] {
				DeviceData["ipv4Addr.addr"] = item["ipv4Addr.addr"]
				DeviceData["ipv4Addr.tag"] = item["ipv4Addr.tag"]
				DeviceData["ipv4Dom.name"] = item["ipv4Dom.name"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["hmm"] {
			if item["hmmFwdIf.id"] == DeviceData["sviIf.id"] {
				DeviceData["hmmFwdIf.mode"] = item["hmmFwdIf.mode"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["nvo"] {
			if strings.Contains(DeviceData["l2BD.accEncap"].(string), strconv.FormatInt(item["nvoNw.vni"].(int64), 10)) {
				DeviceData["nvoNw.vni"] = item["nvoNw.vni"]
				DeviceData["nvoNw.multisiteIngRepl"] = item["nvoNw.multisiteIngRepl"]
				DeviceData["nvoNw.mcastGroup"] = item["nvoNw.mcastGroup"]
				if _, ok := item["nvoIngRepl.rn"]; ok {
					DeviceData["nvoIngRepl.rn"] = item["nvoIngRepl.rn"]
					DeviceData["nvoIngRepl.proto"] = item["nvoIngRepl.proto"]
				}
			}
		}

		DataDB[DBEntry.DeviceName] = DeviceData
	}
}

func GetService(DataDB DataDB, vlanid int64, ServicesDefinitions ServicesDefinitions) {
	for k, v := range DataDB {
		fmt.Println("service model for", k)
		if CheckKeysExistance(ServicesDefinitions.L2VNI, v) {
			fmt.Printf("L2VNI + ")
		}
		if CheckKeysExistance(ServicesDefinitions.AGW, v) {
			fmt.Printf("AGW + ")
		} else {
			fmt.Printf("NO-AGW + ")
		}
		if CheckKeysExistance(ServicesDefinitions.IR, v) {
			fmt.Printf("IR\n")
		} else {
			fmt.Printf("PIM\n")
		}
	}
}

func CheckKeysExistance(keysList []string, DeviceData map[string]interface{}) bool {
	boolVal := bool(true)

	for _, key := range keysList {
		if _, ok := DeviceData[key]; ok {
			boolVal = boolVal && true
		} else {
			boolVal = boolVal && false
		}
	}
	return boolVal
}

type ServicesDefinitions struct {
	L2VNI []string
	AGW   []string
	IR    []string
}

func main() {

	Config, Filter, Enrich := cu.Initialize("config.json")
	KeysMap := cu.LoadKeysMap(Config.KeysDefinitionFile)

	var DB DB
	MetaData := &MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, DB: DB}

	src := make(map[string]interface{})

	srcFile, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println(err)
	}
	defer srcFile.Close()

	srcFileBytes, err := ioutil.ReadAll(srcFile)
	if err != nil {
		fmt.Println(err)
	}
	err = json.Unmarshal(srcFileBytes, &src)

	if err != nil {
		fmt.Println(err)
	}

	Processing(MetaData, src)
	PrettyPrintDB(MetaData.DB)
}
