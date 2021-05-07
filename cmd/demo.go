package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

type ServiceConstructPathEntry struct {
	ChunkName string    `json:"ChunkName"`
	KeySName  string    `json:"KeySName"`
	KeySType  string    `json:"KeySType"`
	KeyDName  string    `json:"KeyDName"`
	KeyDType  string    `json:"KeyDType"`
	KeyLink   string    `json:"KeyLink"`
	MatchType string    `json:"MatchType"`
	KeyList   []string  `json:"KeyList"`
	CombineBy CombineBy `json:"CombineBy"`
}

type CombineBy struct {
	OptionName string   `json:"OptionName"`
	OptionKeys []string `json:"OptionKeys"`
}

func main() {

	var serviceConstructPathEntry ServiceConstructPathEntry
	serviceConstructPathEntryFile, err := os.Open("demo.json")
	if err != nil {
		log.Println(err)
	}
	defer serviceConstructPathEntryFile.Close()

	serviceConstructPathEntryFileBytes, _ := ioutil.ReadAll(serviceConstructPathEntryFile)

	err = json.Unmarshal(serviceConstructPathEntryFileBytes, &serviceConstructPathEntry)
	if err != nil {
		log.Println(err)
	}

	if len(serviceConstructPathEntry.CombineBy.OptionName) == 0 {
		fmt.Println("no combine")
	} else {
		fmt.Println(serviceConstructPathEntry)
	}
}
