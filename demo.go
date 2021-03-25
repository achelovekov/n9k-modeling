package main

import (
	"fmt"
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

func main() {
	NXAPILoginBodyr := &NXAPILoginBody{
		AaaUser: AaaUser{
			Attributes: Attributes{
				Name: "aaa",
				Pwd:  "bbb",
			},
		},
	}
	fmt.Println(NXAPILoginBodyr)

}
