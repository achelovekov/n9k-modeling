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

type DB []DBEntry
type DBEntry struct {
	DeviceName string
	DMEChunks  DMEChunks
}
type DMEChunks []DMEChunk
type DMEChunk struct {
	Key   string
	Value []map[string]interface{}
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

	/* cu.PrettyPrint(src) */
	cu.FlattenMap(src, path, pathIndex, pathPassed, mode, header, &buf, filter, enrich)

	return buf
}

func Processing(md *MetaData, hmd cu.HostMetaData, src map[string]interface{}) {
	var DBEntry DBEntry
	DBEntry.DeviceName = hmd.Host.Hostname
	DBEntry.DMEChunks = make([]DMEChunk, 0)

	for k, v := range md.KeysMap {
		var DMEChunk DMEChunk
		DMEChunk.Key = k
		DMEChunk.Value = make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, v := range v {
			result := worker(src, v, cu.Cadence, md.Filter, md.Enrich)
			buf = append(buf, result...)
			DMEChunk.Value = append(DMEChunk.Value, buf...)
		}
		DBEntry.DMEChunks = append(DBEntry.DMEChunks, DMEChunk)
	}

	md.DB = append(md.DB, DBEntry)

	fmt.Println(md.DB)
}

func main() {

	Config, Filter, Enrich := cu.Initialize("config.json")
	KeysMap := cu.LoadKeysMap(Config.KeysDefinitionFile)
	Inventory := cu.LoadInventory("inventory.json")

	var DB DB

	for _, v := range Inventory {
		MetaData := &MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, DB: DB}
		src := NXAPICall(v, "sys")
		Processing(MetaData, v, src)
	}
}
