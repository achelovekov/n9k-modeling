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
	"strconv"

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

	cu.FlattenMap(src, path, pathIndex, pathPassed, mode, header, &buf, filter, enrich)

	return buf
}

type DB []DBEntry
type DBEntry struct {
	DeviceName  string
	DMEChunkMap DMEChunkMap
}
type DMEChunkMap map[string]DMEChunk
type DMEChunk []map[string]interface{}

func Processing(md *MetaData, hmd cu.HostMetaData, src map[string]interface{}) {
	var DBEntry DBEntry
	DBEntry.DeviceName = hmd.Host.Hostname
	DBEntry.DMEChunkMap = make(map[string]DMEChunk)

	for k, v := range md.KeysMap {
		DMEChunk := make([]map[string]interface{}, 0)

		buf := make([]map[string]interface{}, 0)
		for _, v := range v {
			result := worker(src, v, cu.Cadence, md.Filter, md.Enrich)
			buf = append(buf, result...)
			DMEChunk = append(DMEChunk, buf...)
		}
		DBEntry.DMEChunkMap[k] = DMEChunk
	}

	md.DB = append(md.DB, DBEntry)
}

func modeling(DB DB, vlanid int64) {
	for _, DBEntry := range DB {
		resultMap := make(map[string]interface{})
		fmt.Println("get values for:", DBEntry.DeviceName)
		for _, item := range DBEntry.DMEChunkMap["bd"] {
			if item["l2BD.id"] == vlanid {
				resultMap["l2BD.id"] = vlanid
				resultMap["l2BD.accEncap"] = item["l2BD.accEncap"]
			}
		}
		for index, item := range DBEntry.DMEChunkMap["evpn"] {
			if item["rtctrlBDEvi.encap"] == resultMap["l2BD.accEncap"] {
				resultMap[strconv.Itoa(index)+"_"+"rtctrlRttP.type"] = item["rtctrlRttP.type"]
				resultMap[strconv.Itoa(index)+"_"+"rtctrlRttEntry.rtt"] = item["rtctrlRttEntry.rtt"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["svi"] {
			if item["sviIf.vlanId"] == resultMap["l2BD.id"] {
				resultMap["sviIf.id"] = item["sviIf.id"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["ipv4"] {
			if item["ipv4If.id"] == resultMap["sviIf.id"] {
				resultMap["ipv4Addr.addr"] = item["ipv4Addr.addr"]
				resultMap["ipv4Addr.tag"] = item["ipv4Addr.tag"]
				resultMap["ipv4Dom.name"] = item["ipv4Dom.name"]
			}
		}

		for _, item := range DBEntry.DMEChunkMap["hmm"] {
			if item["hmmFwdIf.id"] == resultMap["sviIf.id"] {
				resultMap["hmmFwdIf.mode"] = item["hmmFwdIf.mode"]
			}
		}

		cu.PrettyPrint(resultMap)
	}
}

func main() {

	Config, Filter, Enrich := cu.Initialize("config.json")
	KeysMap := cu.LoadKeysMap(Config.KeysDefinitionFile)
	Inventory := cu.LoadInventory("inventory.json")

	var DB DB
	MetaData := &MetaData{Config: Config, Filter: Filter, Enrich: Enrich, KeysMap: KeysMap, DB: DB}

	for _, v := range Inventory {
		src := NXAPICall(v, "sys")
		Processing(MetaData, v, src)
		modeling(MetaData.DB, 2008)
	}
}
