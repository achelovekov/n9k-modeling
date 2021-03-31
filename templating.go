package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
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

type L2VNITemplated struct {
	L2BDaccEncap string `json:"l2BD.accEncap"`
	L2BDid       int64  `json:"l2BD.id"`
	L2BDname     string `json:"l2BD.name"`
}

func MakeL2VNITemplate(VariablesMap map[string]interface{}) L2VNITemplated {
	var L2VNITemplated L2VNITemplated

	L2VNITemplated.L2BDaccEncap = "vxlan-" + VariablesMap["VNID"].(string)
	L2VNITemplated.L2BDid, _ = strconv.ParseInt(VariablesMap["VNID"].(string)[3:], 10, 64)
	L2VNITemplated.L2BDname = VariablesMap["Scope"].(string) + VariablesMap["ZoneID"].(string) + strconv.FormatInt(L2VNITemplated.L2BDid, 10)

	return L2VNITemplated
}

func PrettyPrint(src interface{}) {
	JSONData, err := json.MarshalIndent(&src, "", "  ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("Pretty processed output: %s\n", string(JSONData))
}

func main() {
	RawVariables := LoadRawVariables("VNI.vars")
	VariablesMap := ProcessRawVariables(RawVariables)
	L2VNITemplated := MakeL2VNITemplate(VariablesMap)
	PrettyPrint(L2VNITemplated)
}
